package graph

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// conceptWithKey builds a concept carrying an arbitrary extra frontmatter key,
// used to exercise the authored stable-id identity path.
func conceptWithKey(id, typ, title, key, val string) *okf.Concept {
	c := concept(id, typ, title)
	if key != "" {
		c.Frontmatter.Set(key, val)
	}
	return c
}

func TestDescribeNodeLabelsAndCounts(t *testing.T) {
	set := Describe(sampleBundle(), "2026-08-15", "")
	if len(set.Graphs) != 1 {
		t.Fatalf("graphs = %d, want exactly 1 (single local projection)", len(set.Graphs))
	}
	g := set.Graphs[0]

	if g.Name != "b" { // sampleBundle Root is "/b"
		t.Errorf("name = %q, want %q (bundle dir basename)", g.Name, "b")
	}
	if g.Source.Kind != "okf-bundle" || g.Source.Root != "/b" {
		t.Errorf("source = %+v, want {okf-bundle /b}", g.Source)
	}
	if g.Counts.Nodes != 2 || g.Counts.Edges != 1 {
		t.Errorf("counts = %+v, want {nodes:2 edges:1}", g.Counts)
	}

	// Node labels = distinct types present, sorted: Guide, Note.
	if len(g.NodeLabels) != 2 {
		t.Fatalf("node_labels = %d, want 2", len(g.NodeLabels))
	}
	if g.NodeLabels[0].Label != "Guide" || g.NodeLabels[1].Label != "Note" {
		t.Errorf("node labels not sorted: %q, %q", g.NodeLabels[0].Label, g.NodeLabels[1].Label)
	}
	for _, nl := range g.NodeLabels {
		if nl.Count != 1 {
			t.Errorf("label %q count = %d, want 1", nl.Label, nl.Count)
		}
	}

	// Single LINKS edge label.
	if len(g.EdgeLabels) != 1 {
		t.Fatalf("edge_labels = %d, want 1", len(g.EdgeLabels))
	}
	if g.EdgeLabels[0].Label != "LINKS" || g.EdgeLabels[0].Count != 1 {
		t.Errorf("edge label = %+v, want {LINKS 1}", g.EdgeLabels[0])
	}
}

// TestDescribePropertyDeclarationsMatchModel is the schema-fidelity gate: the
// advertised property declarations are exactly the json field names of the
// Node/Edge model the `graph` export emits — parity by construction.
func TestDescribePropertyDeclarationsMatchModel(t *testing.T) {
	wantNode := []string{"id", "title", "type", "tier", "stale"}
	wantEdge := []string{"from", "to", "text"}

	if got := nodeProperties(); !reflect.DeepEqual(got, wantNode) {
		t.Errorf("node properties = %v, want %v", got, wantNode)
	}
	if got := edgeProperties(); !reflect.DeepEqual(got, wantEdge) {
		t.Errorf("edge properties = %v, want %v", got, wantEdge)
	}

	g := Describe(sampleBundle(), "2026-08-15", "").Graphs[0]
	for _, nl := range g.NodeLabels {
		if !reflect.DeepEqual(nl.Properties, wantNode) {
			t.Errorf("node label %q properties = %v, want %v", nl.Label, nl.Properties, wantNode)
		}
	}
	if !reflect.DeepEqual(g.EdgeLabels[0].Properties, wantEdge) {
		t.Errorf("edge properties = %v, want %v", g.EdgeLabels[0].Properties, wantEdge)
	}
}

func TestDescribeIsDeterministic(t *testing.T) {
	a, _ := json.Marshal(Describe(sampleBundle(), "2026-08-15", ""))
	b, _ := json.Marshal(Describe(sampleBundle(), "2026-08-15", ""))
	if string(a) != string(b) {
		t.Errorf("Describe not deterministic:\n%s\n%s", a, b)
	}
}

// TestNodeKeyForIdentity covers all three identity cases (design §C.3 #6): no
// id_key → path; id_key present → authored value; id_key absent on a concept →
// per-concept path fallback. binder never mints an id.
func TestNodeKeyForIdentity(t *testing.T) {
	// Case 1: no id_key → path identity (Concept.ID).
	c := conceptWithKey("metrics/revenue", "Metric", "Revenue", "concept-id", "urn:acme:revenue")
	if k, s := NodeKeyFor(c, ""); k != "metrics/revenue" || s != "path" {
		t.Errorf("no id_key: got (%q,%q), want (metrics/revenue,path)", k, s)
	}

	// Case 2: id_key present in frontmatter → authored value.
	if k, s := NodeKeyFor(c, "concept-id"); k != "urn:acme:revenue" || s != "frontmatter" {
		t.Errorf("id_key present: got (%q,%q), want (urn:acme:revenue,frontmatter)", k, s)
	}

	// Case 3: id_key set but absent on this concept → per-concept path fallback.
	noKey := concept("metrics/margin", "Metric", "Margin")
	if k, s := NodeKeyFor(noKey, "concept-id"); k != "metrics/margin" || s != "path" {
		t.Errorf("id_key absent: got (%q,%q), want (metrics/margin,path)", k, s)
	}

	// Empty-string frontmatter value is not a mint: falls back to path.
	empty := conceptWithKey("x", "Note", "X", "concept-id", "")
	if k, s := NodeKeyFor(empty, "concept-id"); k != "x" || s != "path" {
		t.Errorf("empty id value: got (%q,%q), want (x,path)", k, s)
	}
}

// TestDescribeNodeKeyStrategy: the graph-level strategy is "frontmatter" only
// when id_key is set and resolves on at least one concept; the requested key is
// echoed back regardless.
func TestDescribeNodeKeyStrategy(t *testing.T) {
	withKey := conceptWithKey("a", "Note", "A", "concept-id", "urn:a")
	without := concept("b", "Note", "B")
	b := &okf.Bundle{Root: "/mixed", Concepts: []*okf.Concept{withKey, without}}

	// No id_key → path, empty key.
	if nk := Describe(b, "2026-08-15", "").Graphs[0].NodeKey; nk.Strategy != "path" || nk.Key != "" {
		t.Errorf("no id_key: node_key = %+v, want {path }", nk)
	}
	// id_key resolving on at least one concept → frontmatter, echoed key.
	if nk := Describe(b, "2026-08-15", "concept-id").Graphs[0].NodeKey; nk.Strategy != "frontmatter" || nk.Key != "concept-id" {
		t.Errorf("id_key resolves: node_key = %+v, want {frontmatter concept-id}", nk)
	}
	// id_key set but resolving on NO concept → path strategy, key still echoed.
	if nk := Describe(b, "2026-08-15", "absent-key").Graphs[0].NodeKey; nk.Strategy != "path" || nk.Key != "absent-key" {
		t.Errorf("id_key absent everywhere: node_key = %+v, want {path absent-key}", nk)
	}
}
