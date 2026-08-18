package graph

import (
	"bytes"
	"encoding/csv"
	"reflect"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// provBundle mirrors testdata/project/verified/corpus in memory: a multi-entry
// verified list mixing human/non-human actors (human NOT first, one entry with an
// empty `at`), a single-human node, a single-agent node, and an unverified node,
// with past/future/absent stale_after so tier and staleness vary independently.
func provBundle() *okf.Bundle {
	multi := projConcept("m", "Guide", "Multi", nil,
		okf.TrustSignals{
			Verified: []okf.Actorstamp{
				{By: "agent:etl", At: "2025-03-01T12:00:00Z"},
				{By: "human:alice@corp", At: "2025-04-02T09:30:00Z"},
				{By: "process:nightly-refresh"},
			},
			StaleAfter: "2020-06-15",
		},
		map[string]string{"graph_id": "node-multi"})
	human := projConcept("h", "Note", "Human", nil,
		okf.TrustSignals{
			Verified:   []okf.Actorstamp{{By: "human:bob@corp", At: "2025-05-05T05:05:05Z"}},
			StaleAfter: "2099-12-31",
		},
		map[string]string{"graph_id": "node-human"})
	gen := projConcept("g", "Note", "Generated", nil,
		okf.TrustSignals{Verified: []okf.Actorstamp{{By: "agent:bot", At: "2025-01-02T00:00:00Z"}}},
		map[string]string{"graph_id": "node-gen"})
	none := projConcept("n", "Guide", "None", nil,
		okf.TrustSignals{StaleAfter: "2010-01-01"},
		map[string]string{"graph_id": "node-none"})
	return &okf.Bundle{Root: "/verified", Concepts: []*okf.Concept{multi, human, gen, none}}
}

// TestNodeVerifiedRowsByteFaithful pins G3 (b): the NodeVerified rows are
// byte-faithful to each concept's verified[] — order preserved, by/at verbatim,
// is_human = okf.IsHumanActor. The expected rows are written out explicitly (not
// re-derived from the same code path) so the assertion is independent.
func TestNodeVerifiedRowsByteFaithful(t *testing.T) {
	p := Project(provBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"})
	got := p.NodeVerifiedRows()
	// Node order is Build's sort-by-ID: g, h, m, n.
	want := []NodeVerifiedRow{
		{NodeKey: "node-gen", Seq: 0, By: "agent:bot", At: "2025-01-02T00:00:00Z", IsHuman: false},
		{NodeKey: "node-human", Seq: 0, By: "human:bob@corp", At: "2025-05-05T05:05:05Z", IsHuman: true},
		{NodeKey: "node-multi", Seq: 0, By: "agent:etl", At: "2025-03-01T12:00:00Z", IsHuman: false},
		{NodeKey: "node-multi", Seq: 1, By: "human:alice@corp", At: "2025-04-02T09:30:00Z", IsHuman: true},
		{NodeKey: "node-multi", Seq: 2, By: "process:nightly-refresh", At: "", IsHuman: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NodeVerifiedRows mismatch\n got: %+v\nwant: %+v", got, want)
	}
	// is_human must equal the SHARED predicate okf.IsHumanActor for every row —
	// never a second definition of "human".
	for _, r := range got {
		if r.IsHuman != okf.IsHumanActor(r.By) {
			t.Errorf("row %q seq %d: is_human=%v, IsHumanActor=%v", r.NodeKey, r.Seq, r.IsHuman, okf.IsHumanActor(r.By))
		}
	}
}

// TestNodeVerifiedOrderPreservedControl is the C11 order control: reordering a
// concept's verified[] must change the emitted rows. If it did not, the byte
// golden would not actually be guarding order preservation.
func TestNodeVerifiedOrderPreservedControl(t *testing.T) {
	base := Project(provBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"}).NodeVerifiedCSV()

	// Reorder the multi node's verified[] (swap seq 0 and seq 1) at the source.
	b := provBundle()
	for _, c := range b.Concepts {
		if c.ID == "m" {
			v := c.Trust.Verified
			v[0], v[1] = v[1], v[0]
		}
	}
	reordered := Project(b, ProjectOptions{Today: projToday, IDKey: "graph_id"}).NodeVerifiedCSV()

	if bytes.Equal(base, reordered) {
		t.Fatal("C11 control failed: reordering verified[] did not change node_verified.csv — order is not actually preserved/guarded")
	}
}

// TestNodeVerifiedIsHumanControl is the C11 is_human control: flipping the
// human: prefix off the sole human stamp of a node must change is_human (and thus
// the emitted rows). Proves is_human is not hard-wired.
func TestNodeVerifiedIsHumanControl(t *testing.T) {
	base := Project(provBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"}).NodeVerifiedCSV()

	b := provBundle()
	for _, c := range b.Concepts {
		if c.ID == "h" {
			c.Trust.Verified[0].By = "agent:bob@corp" // strip the human: prefix
		}
	}
	flipped := Project(b, ProjectOptions{Today: projToday, IDKey: "graph_id"}).NodeVerifiedCSV()
	if bytes.Equal(base, flipped) {
		t.Fatal("C11 control failed: dropping the human: prefix did not change node_verified.csv — is_human is not derived from by")
	}
}

// TestDerivationRecompute pins G3 (a)/(d): the derivation RECOMPUTES tier/stale
// from the STORED facts (NodeVerified rows + Nodes.stale_after) and matches
// okf.TrustTier / okf.IsStale exactly, for several chosen dates. RecomputeTier /
// RecomputeStale are the Go statement of derivation.sql's SQL.
func TestDerivationRecompute(t *testing.T) {
	b := provBundle()
	p := Project(b, ProjectOptions{Today: projToday, IDKey: "graph_id"})

	// Group the emitted NodeVerified rows (the stored facts) by node_key.
	rowsByKey := map[string][]NodeVerifiedRow{}
	for _, r := range p.NodeVerifiedRows() {
		rowsByKey[r.NodeKey] = append(rowsByKey[r.NodeKey], r)
	}
	// Map node_key → concept and its stored stale_after (the Nodes column).
	staleAfterByKey := map[string]string{}
	conceptByKey := map[string]*okf.Concept{}
	for _, pn := range p.Nodes {
		staleAfterByKey[pn.Key] = pn.StaleAfter
	}
	for _, c := range b.Concepts {
		key, _ := NodeKeyFor(c, "graph_id")
		conceptByKey[key] = c
	}

	dates := []string{"2010-01-01", "2020-06-15", "2026-08-18", "2099-12-31", "2100-01-01"}
	for _, pn := range p.Nodes {
		c := conceptByKey[pn.Key]
		// (a)/(d): tier recomputed from stored NodeVerified rows == TrustTier.
		if got, want := RecomputeTier(rowsByKey[pn.Key]), okf.TrustTier(c); got != want {
			t.Errorf("node %q: RecomputeTier=%q, TrustTier=%q", pn.Key, got, want)
		}
		for _, d := range dates {
			// (a): stale recomputed from stored stale_after == IsStale for any date.
			if got, want := RecomputeStale(staleAfterByKey[pn.Key], d), okf.IsStale(c, d); got != want {
				t.Errorf("node %q as-of %s: RecomputeStale=%v, IsStale=%v", pn.Key, d, got, want)
			}
		}
	}
}

// TestDerivationRecomputeControl is the C11 control for the recompute: a
// deliberately WRONG recompute (treating any-non-human as human-reviewed) must
// disagree with okf.TrustTier for the machine-confirmed node, proving the
// comparison in TestDerivationRecompute can actually fail.
func TestDerivationRecomputeControl(t *testing.T) {
	b := provBundle()
	p := Project(b, ProjectOptions{Today: projToday, IDKey: "graph_id"})
	rowsByKey := map[string][]NodeVerifiedRow{}
	for _, r := range p.NodeVerifiedRows() {
		rowsByKey[r.NodeKey] = append(rowsByKey[r.NodeKey], r)
	}
	// A wrong tier rule: "any verified row ⇒ human-reviewed".
	wrongTier := func(rows []NodeVerifiedRow) okf.Tier {
		if len(rows) == 0 {
			return okf.TierUnverified
		}
		return okf.TierHumanReviewed
	}
	// node-gen is machine-confirmed (single agent stamp); the wrong rule calls it
	// human-reviewed, so the equality used in the real test MUST fail here.
	conceptByKey := map[string]*okf.Concept{}
	for _, c := range b.Concepts {
		key, _ := NodeKeyFor(c, "graph_id")
		conceptByKey[key] = c
	}
	got := wrongTier(rowsByKey["node-gen"])
	want := okf.TrustTier(conceptByKey["node-gen"])
	if got == want {
		t.Fatalf("control failed: wrong tier rule agreed with TrustTier (%q) — recompute test would be vacuous", want)
	}
}

// TestNodeVerifiedCSVDeterministic pins determinism of the row artifact.
func TestNodeVerifiedCSVDeterministic(t *testing.T) {
	first := Project(provBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"}).NodeVerifiedCSV()
	second := Project(provBundle(), ProjectOptions{Today: "2000-01-01", IDKey: "graph_id"}).NodeVerifiedCSV()
	if !bytes.Equal(first, second) {
		t.Fatalf("node_verified.csv not deterministic / varied with today:\n%s\n---\n%s", first, second)
	}
}

// TestNodeVerifiedCSVParsesBack proves the emitted rows round-trip as CSV (the
// framing is well-formed) and that by/at survive verbatim through a parse.
func TestNodeVerifiedCSVParsesBack(t *testing.T) {
	p := Project(provBundle(), ProjectOptions{Today: projToday, IDKey: "graph_id"})
	data := p.NodeVerifiedCSV()
	recs, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatalf("emitted node_verified.csv is not valid CSV: %v", err)
	}
	rows := p.NodeVerifiedRows()
	if len(recs) != len(rows)+1 {
		t.Fatalf("parsed %d records (incl header), want %d rows + header", len(recs), len(rows))
	}
	// The process:nightly-refresh entry (multi seq 2) has an empty at; confirm it
	// survives as an empty field, not dropped.
	last := recs[len(recs)-1]
	if last[2] != "process:nightly-refresh" || last[3] != "" || last[4] != "false" {
		t.Errorf("last row by/at/is_human = %q/%q/%q, want process:nightly-refresh//false", last[2], last[3], last[4])
	}
}
