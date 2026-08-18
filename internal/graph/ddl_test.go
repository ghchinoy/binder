package graph

import (
	"bytes"
	"encoding/csv"
	"reflect"
	"strings"
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

// --- G2: loader row-data tests. ---

// ddlTableColumns parses the (name, type) pairs of a CREATE TABLE block out of
// the emitted schema.ddl text, in declaration order. It reads the emitted bytes
// (not the internal column slices), so comparing its output to the CSV header and
// the DML column list is a genuine cross-artifact round-trip, not a tautology.
func ddlTableColumns(t *testing.T, ddl, table string) (names, types []string) {
	t.Helper()
	lines := strings.Split(ddl, "\n")
	in := false
	for _, ln := range lines {
		if strings.HasPrefix(ln, "CREATE TABLE "+table+" (") {
			in = true
			continue
		}
		if in {
			if strings.HasPrefix(ln, ")") {
				break
			}
			fields := strings.Fields(ln)
			if len(fields) < 2 {
				continue
			}
			names = append(names, fields[0])
			types = append(types, strings.TrimRight(fields[1], ","))
		}
	}
	return names, types
}

// csvHeaderCols returns the header (first line) of a CSV artifact, split on ",".
func csvHeaderCols(t *testing.T, csv []byte) []string {
	t.Helper()
	s := string(csv)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.Split(s, ",")
}

// insertCols returns the column list of the "INSERT INTO <table> ( ... ) VALUES"
// statement in the emitted load.sql, in order.
func insertCols(t *testing.T, sql, table string) []string {
	t.Helper()
	marker := "INSERT INTO " + table + " ("
	i := strings.Index(sql, marker)
	if i < 0 {
		t.Fatalf("load.sql has no INSERT for %q\n%s", table, sql)
	}
	rest := sql[i+len(marker):]
	j := strings.Index(rest, ")")
	if j < 0 {
		t.Fatalf("load.sql INSERT for %q is malformed\n%s", table, sql)
	}
	cols := strings.Split(rest[:j], ", ")
	return cols
}

func sameCols(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRowsRoundTripWithSchema pins the G2 round-trip criterion: the CSV header
// and the DML column list are byte-consistent with schema.ddl's declared columns
// (names and order), for BOTH tables — asserted by parsing the emitted artifacts,
// not by eyeballing. It also checks the declared column TYPES are the frozen set.
func TestRowsRoundTripWithSchema(t *testing.T) {
	p := Project(projBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"})
	ddl := string(p.DDL())

	nodeNames, nodeTypes := ddlTableColumns(t, ddl, "Nodes")
	if got := csvHeaderCols(t, p.NodesCSV()); !sameCols(got, nodeNames) {
		t.Errorf("nodes.csv header %v != schema.ddl Nodes columns %v", got, nodeNames)
	}
	if got := insertCols(t, string(p.LoadSQL()), "Nodes"); !sameCols(got, nodeNames) {
		t.Errorf("load.sql Nodes columns %v != schema.ddl Nodes columns %v", got, nodeNames)
	}
	if want := []string{"node_key", "title", "type", "tier", "stale", "stale_after"}; !sameCols(nodeNames, want) {
		t.Errorf("Nodes column names = %v, want %v", nodeNames, want)
	}
	if want := []string{"STRING(MAX)", "STRING(MAX)", "STRING(MAX)", "STRING(MAX)", "BOOL", "DATE"}; !sameCols(nodeTypes, want) {
		t.Errorf("Nodes column types = %v, want %v", nodeTypes, want)
	}

	edgeNames, edgeTypes := ddlTableColumns(t, ddl, "Edges")
	if got := csvHeaderCols(t, p.EdgesCSV()); !sameCols(got, edgeNames) {
		t.Errorf("edges.csv header %v != schema.ddl Edges columns %v", got, edgeNames)
	}
	if got := insertCols(t, string(p.LoadSQL()), "Edges"); !sameCols(got, edgeNames) {
		t.Errorf("load.sql Edges columns %v != schema.ddl Edges columns %v", got, edgeNames)
	}
	if want := []string{"from_key", "to_key", "rel"}; !sameCols(edgeNames, want) {
		t.Errorf("Edges column names = %v, want %v", edgeNames, want)
	}
	if want := []string{"STRING(MAX)", "STRING(MAX)", "STRING(MAX)"}; !sameCols(edgeTypes, want) {
		t.Errorf("Edges column types = %v, want %v", edgeTypes, want)
	}

	// C11 control: the round-trip comparison must REDDEN under a real mismatch. A
	// mutated column list must not compare equal, or the assertions above are
	// vacuous.
	mutated := append([]string(nil), nodeNames...)
	mutated[0] = "not_node_key"
	if sameCols(mutated, nodeNames) {
		t.Fatal("C11 control failed: sameCols accepted a mutated column list")
	}
}

// TestRowCountsMatchCounts pins the G2 criterion that row count == counts: the
// number of CSV data rows (excluding the header) equals the projection's counts.
func TestRowCountsMatchCounts(t *testing.T) {
	p := Project(projBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"})
	dataRows := func(csv []byte) int {
		lines := strings.Split(strings.TrimRight(string(csv), "\n"), "\n")
		return len(lines) - 1 // minus the header
	}
	if got := dataRows(p.NodesCSV()); got != p.Counts.Nodes {
		t.Errorf("nodes.csv data rows = %d, want counts.nodes %d", got, p.Counts.Nodes)
	}
	if got := dataRows(p.EdgesCSV()); got != p.Counts.Edges {
		t.Errorf("edges.csv data rows = %d, want counts.edges %d", got, p.Counts.Edges)
	}
}

// TestRowsDeterministic pins G2 determinism at the unit level: repeated emission
// of the same projection is byte-identical for every row artifact.
func TestRowsDeterministic(t *testing.T) {
	mk := func() (n, e, l []byte) {
		p := Project(projBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"})
		return p.NodesCSV(), p.EdgesCSV(), p.LoadSQL()
	}
	n1, e1, l1 := mk()
	n2, e2, l2 := mk()
	if !bytes.Equal(n1, n2) || !bytes.Equal(e1, e2) || !bytes.Equal(l1, l2) {
		t.Fatal("row artifacts not deterministic across repeated projections")
	}
}

// TestRowsNodesSortedByKey pins the deterministic node ordering (by node_key),
// independent of the Build/Concept.ID order the Projection.Nodes slice carries.
func TestRowsNodesSortedByKey(t *testing.T) {
	p := Project(projBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"})
	lines := strings.Split(strings.TrimRight(string(p.NodesCSV()), "\n"), "\n")[1:]
	var keys []string
	for _, ln := range lines {
		keys = append(keys, strings.SplitN(ln, ",", 2)[0])
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] > keys[i] {
			t.Errorf("nodes.csv not sorted by node_key: %v", keys)
			break
		}
	}
}

// TestRowsCSVAndDMLEscaping proves CSV quoting and SQL-literal escaping handle
// values that would otherwise break the format: a comma/quote/newline in a title
// (CSV must quote it) and a single quote/backslash in link text (DML must escape
// it). This is the injection-safety seam for row data.
func TestRowsCSVAndDMLEscaping(t *testing.T) {
	a := projConcept("a", "Guide", "has, \"comma\" and\nnewline",
		[]okf.Link{{TargetID: "b", RawTarget: "./b.md", Text: "O'Brien \\ backslash", Resolved: true}},
		okf.TrustSignals{}, map[string]string{"graph_id": "k-a"})
	b := projConcept("b", "Note", "Beta", nil, okf.TrustSignals{}, map[string]string{"graph_id": "k-b"})
	bundle := &okf.Bundle{Root: "/corpus", Concepts: []*okf.Concept{a, b}}
	p := Project(bundle, ProjectOptions{Today: projToday, IDKey: "graph_id"})

	// CSV: the awkward title must be wrapped in quotes with the inner quote doubled
	// (encoding/csv), and the whole record must round-trip back through a CSV reader.
	nodesCSV := string(p.NodesCSV())
	if !strings.Contains(nodesCSV, "\"has, \"\"comma\"\" and\nnewline\"") {
		t.Errorf("nodes.csv did not quote/escape the awkward title:\n%s", nodesCSV)
	}
	r := csv.NewReader(strings.NewReader(nodesCSV))
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatalf("emitted nodes.csv is not parseable CSV: %v", err)
	}
	if len(recs) != 3 { // header + 2 nodes
		t.Fatalf("nodes.csv parsed to %d records, want 3", len(recs))
	}
	if recs[1][1] != "has, \"comma\" and\nnewline" {
		t.Errorf("round-tripped title = %q, want the original awkward value", recs[1][1])
	}

	// DML: the link text's single quote and backslash must be backslash-escaped so
	// the literal cannot break out.
	loadSQL := string(p.LoadSQL())
	if !strings.Contains(loadSQL, `'O\'Brien \\ backslash'`) {
		t.Errorf("load.sql did not escape the awkward rel value:\n%s", loadSQL)
	}
}

// TestAggregateNodeKeyParity is the dedup regression guard folded in from the G1
// [Consider]: the graph-level node-key aggregation now has a SINGLE definition
// (aggregateNodeKey), and both callers — Describe (list_graphs) and Project
// (binder project) — must produce identical NodeKey values for every id-key case.
func TestAggregateNodeKeyParity(t *testing.T) {
	b := projBundle()
	for _, idKey := range []string{"", "graph_id", "partial_id", "no_such_key"} {
		describe := Describe(b, projToday, idKey).Graphs[0].NodeKey
		project := Project(b, ProjectOptions{Today: projToday, IDKey: idKey}).NodeKey
		if describe != project {
			t.Errorf("idKey=%q: Describe NodeKey %+v != Project NodeKey %+v", idKey, describe, project)
		}
		// And both equal the shared helper directly.
		if want := aggregateNodeKey(b.Concepts, idKey); describe != want || project != want {
			t.Errorf("idKey=%q: helper %+v, describe %+v, project %+v", idKey, want, describe, project)
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
