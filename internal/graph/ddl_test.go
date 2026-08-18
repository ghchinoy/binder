package graph

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// projConcept builds a concept with an arbitrary set of extra frontmatter keys
// and trust signals, for exercising the projection's identity/tier/stale paths.
func projConcept(id, typ, title string, links []okf.Link, trust okf.TrustSignals, extra map[string]string) *okf.Concept {
	fm := okf.NewOrderedMap()
	if title != "" {
		fm.Set("title", title)
	}
	for k, v := range extra {
		fm.Set(k, v)
	}
	return &okf.Concept{ID: id, RelPath: id + ".md", Type: typ, Frontmatter: fm, Links: links, Trust: trust}
}

// projBundle mirrors testdata/project/corpus in memory: a→b→c, with graph_id on
// every concept, partial_id on a only, human/machine/absent verification, and
// stale_after past/absent/future.
func projBundle() *okf.Bundle {
	a := projConcept("a", "Guide", "Alpha",
		[]okf.Link{{TargetID: "b", RawTarget: "./b.md", Text: "Beta", Resolved: true}},
		okf.TrustSignals{Verified: []okf.Actorstamp{{By: "human:reviewer@acme"}}, StaleAfter: "2020-01-01"},
		map[string]string{"graph_id": "node-alpha", "partial_id": "alpha-only"})
	b := projConcept("b", "Note", "Beta",
		[]okf.Link{{TargetID: "c", RawTarget: "./c.md", Text: "Gamma", Resolved: true}},
		okf.TrustSignals{Verified: []okf.Actorstamp{{By: "agent:bot"}}},
		map[string]string{"graph_id": "node-beta"})
	c := projConcept("c", "Guide", "Gamma", nil,
		okf.TrustSignals{StaleAfter: "2099-01-01"},
		map[string]string{"graph_id": "node-gamma"})
	return &okf.Bundle{Root: "/corpus", Concepts: []*okf.Concept{a, b, c}}
}

const projToday = "2026-08-18"

// TestProjectEdgeParity pins G1 criterion 3: the projected edge set is exactly
// EdgesFromConcepts for the fixture — parity by construction (Project calls it
// verbatim; there is no second edge definition).
func TestProjectEdgeParity(t *testing.T) {
	b := projBundle()
	p := Project(b, ProjectOptions{Today: projToday})
	want := EdgesFromConcepts(b.Concepts)
	if !reflect.DeepEqual(p.Edges, want) {
		t.Fatalf("projected edges = %+v, want EdgesFromConcepts %+v", p.Edges, want)
	}
	if p.Counts.Edges != len(want) {
		t.Errorf("counts.edges = %d, want %d", p.Counts.Edges, len(want))
	}
}

// TestProjectTierStaleSnapshot pins G1 criterion 4: tier/stale columns equal the
// in-memory Build values for the same `today`; stale_after reflects the raw
// input; AsOf equals that `today`.
func TestProjectTierStaleSnapshot(t *testing.T) {
	b := projBundle()
	m := Build(b, projToday)
	p := Project(b, ProjectOptions{Today: projToday, IDKey: "graph_id"})

	if p.AsOf != projToday {
		t.Errorf("AsOf = %q, want %q", p.AsOf, projToday)
	}
	if len(p.Nodes) != len(m.Nodes) {
		t.Fatalf("node count = %d, want %d", len(p.Nodes), len(m.Nodes))
	}
	// Build sorts by ID; Project preserves that order, so index-parity holds.
	for i, n := range m.Nodes {
		pn := p.Nodes[i]
		if pn.Tier != n.Tier {
			t.Errorf("node %q tier = %q, want Build value %q", n.ID, pn.Tier, n.Tier)
		}
		if pn.Stale != n.Stale {
			t.Errorf("node %q stale = %v, want Build value %v", n.ID, pn.Stale, n.Stale)
		}
	}
	// Spot-check the derived snapshot varies as authored, and stale_after is raw.
	byKey := map[string]ProjectedNode{}
	for _, pn := range p.Nodes {
		byKey[pn.Key] = pn
	}
	if got := byKey["node-alpha"]; got.Tier != okf.TierHumanReviewed || !got.Stale || got.StaleAfter != "2020-01-01" {
		t.Errorf("alpha = %+v; want human-reviewed, stale, stale_after 2020-01-01", got)
	}
	if got := byKey["node-beta"]; got.Tier != okf.TierMachineConfirmed || got.Stale || got.StaleAfter != "" {
		t.Errorf("beta = %+v; want machine-confirmed, not stale, no stale_after", got)
	}
	if got := byKey["node-gamma"]; got.Tier != okf.TierUnverified || got.Stale || got.StaleAfter != "2099-01-01" {
		t.Errorf("gamma = %+v; want unverified, not stale, stale_after 2099-01-01", got)
	}
}

// TestProjectIdentityStrategies pins G1 criterion 2: id-key on all → frontmatter
// / stable / fallback 0; a mixed corpus → frontmatter / not stable / correct
// fallback count; no id-key → path / not stable.
func TestProjectIdentityStrategies(t *testing.T) {
	b := projBundle()
	cases := []struct {
		name         string
		idKey        string
		wantStrategy string
		wantStable   bool
		wantFallback int
	}{
		{"all-frontmatter", "graph_id", "frontmatter", true, 0},
		{"mixed", "partial_id", "frontmatter", false, 2},
		{"none", "", "path", false, 3},
		{"key-nobody-has", "no_such_key", "path", false, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Project(b, ProjectOptions{Today: projToday, IDKey: c.idKey})
			if p.NodeKey.Strategy != c.wantStrategy {
				t.Errorf("strategy = %q, want %q", p.NodeKey.Strategy, c.wantStrategy)
			}
			if p.Identity.ReRootingStable != c.wantStable {
				t.Errorf("re_rooting_stable = %v, want %v", p.Identity.ReRootingStable, c.wantStable)
			}
			if p.Identity.PathFallbackCount != c.wantFallback {
				t.Errorf("path_fallback_count = %d, want %d", p.Identity.PathFallbackCount, c.wantFallback)
			}
		})
	}
}

// isNeverMinted reports whether key is one binder is allowed to emit for c under
// idKey: the authored frontmatter value or the path-derived Concept.ID. Anything
// else (a hash, a UUID, a synthesized id) is a minted key.
func isNeverMinted(key string, c *okf.Concept, idKey string) bool {
	if key == c.ID {
		return true
	}
	if idKey != "" && c.Frontmatter != nil {
		if v, ok := c.Frontmatter.Get(idKey); ok {
			if s, isStr := v.(string); isStr && s == key {
				return true
			}
		}
	}
	return false
}

// TestProjectNeverMints pins G1 criterion 2's never-mint invariant. It asserts
// every emitted node_key is authored-or-path, and — as the C11 control in the
// same test — proves the guard REDDENS against a minted key: a fabricated hash
// value must be rejected by isNeverMinted, or the guard proves nothing.
func TestProjectNeverMints(t *testing.T) {
	b := projBundle()
	for _, idKey := range []string{"", "graph_id", "partial_id"} {
		p := Project(b, ProjectOptions{Today: projToday, IDKey: idKey})
		for _, n := range p.Nodes {
			// Every emitted node_key must be authored-or-path for SOME concept.
			ok := false
			for _, c := range b.Concepts {
				if isNeverMinted(n.Key, c, idKey) {
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("idKey=%q: node_key %q is neither authored nor path for any concept — minted", idKey, n.Key)
			}
		}
	}
	// C11 control: the guard must FAIL for a minted (hashed) key. If this passes,
	// isNeverMinted is vacuous and the assertion above proves nothing.
	c := b.Concepts[0] // concept "a", carries graph_id=node-alpha
	minted := "sha256:deadbeefcafe"
	if isNeverMinted(minted, c, "graph_id") {
		t.Fatalf("C11 control failed: isNeverMinted accepted a minted key %q", minted)
	}
}

// TestDDLDeterministic pins G1 criterion 5 at the unit level: the emitted DDL is
// byte-identical across repeated projections of the same bundle.
func TestDDLDeterministic(t *testing.T) {
	b := projBundle()
	first := Project(b, ProjectOptions{Today: projToday, IDKey: "graph_id"}).DDL()
	second := Project(b, ProjectOptions{Today: projToday, IDKey: "graph_id"}).DDL()
	if !bytes.Equal(first, second) {
		t.Fatalf("DDL not deterministic across runs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	// The DDL is schema-only in G1: it must not vary with the id-key or today,
	// which only affect row data (later) and the report envelope.
	other := Project(b, ProjectOptions{Today: "2000-01-01", IDKey: ""}).DDL()
	if !bytes.Equal(first, other) {
		t.Fatalf("G1 DDL varied with options (should be schema-only):\n%s\n---\n%s", first, other)
	}
}

// TestDDLShape checks the DDL carries the frozen decisions: fixed node/edge
// columns, single LINKS label, target-neutral tables + Spanner property graph.
func TestDDLShape(t *testing.T) {
	ddl := string(Project(projBundle(), ProjectOptions{Today: projToday}).DDL())
	for _, want := range []string{
		"CREATE TABLE Nodes (",
		"node_key    STRING(MAX) NOT NULL,",
		"stale_after DATE,",
		"CREATE TABLE Edges (",
		"rel      STRING(MAX),",
		") PRIMARY KEY (from_key, to_key, rel);",
		"CREATE PROPERTY GRAPH OkfGraph",
		"LABEL LINKS",
	} {
		if !contains(ddl, want) {
			t.Errorf("DDL missing %q\n---\n%s", want, ddl)
		}
	}
	// OQ-7: exactly one edge label, and it is LINKS (no derived labels).
	if n := countSub(ddl, "LABEL "); n != 1 {
		t.Errorf("edge LABEL count = %d, want exactly 1 (single LINKS label)", n)
	}
}

// TestSanitizeIdentifier pins deterministic identifier sanitization (G1
// criterion 1). Clean identifiers are unchanged; unsafe runes become underscore;
// a leading digit is prefixed.
func TestSanitizeIdentifier(t *testing.T) {
	cases := map[string]string{
		"node_key": "node_key",
		"Nodes":    "Nodes",
		"a-b.c d":  "a_b_c_d",
		"1st":      "_1st",
		"":         "_",
		"héllo":    "h_llo",
	}
	for in, want := range cases {
		if got := sanitizeIdentifier(in); got != want {
			t.Errorf("sanitizeIdentifier(%q) = %q, want %q", in, got, want)
		}
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

func countSub(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
