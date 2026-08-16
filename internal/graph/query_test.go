package graph

import (
	"reflect"
	"testing"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// acmeBundle is the stable, conformant fixture shared with the graph/list_graphs
// tests. fixedToday pins staleness so hand-computed expectations hold.
const (
	acmeBundle = "../../testdata/okf-bundles/acme_retail"
	fixedToday = "2026-08-15"
)

// acmeIndex loads the acme_retail bundle and builds the query index over the
// deterministic Build model. Any query in these tests runs against the same
// nine-node / fifteen-edge graph the `graph`/`list_graphs` tests use.
func acmeIndex(t *testing.T) *Index {
	t.Helper()
	b, err := bundle.Load(acmeBundle, native.New())
	if err != nil {
		t.Fatalf("load %s: %v", acmeBundle, err)
	}
	return NewIndex(Build(b, fixedToday))
}

func nodeIDs(ns []Node) []string {
	ids := []string{}
	for _, n := range ns {
		ids = append(ids, n.ID)
	}
	return ids
}

func edgeTriples(es []Edge) [][3]string {
	out := [][3]string{}
	for _, e := range es {
		out = append(out, [3]string{e.From, e.To, e.Text})
	}
	return out
}

func TestLookupByID(t *testing.T) {
	idx := acmeIndex(t)

	got := idx.Lookup("", "tables/orders", "")
	if got.NotFound == nil || *got.NotFound {
		t.Fatalf("not_found = %v, want false", got.NotFound)
	}
	if ids := nodeIDs(got.Nodes); !reflect.DeepEqual(ids, []string{"tables/orders"}) {
		t.Fatalf("nodes = %v, want [tables/orders]", ids)
	}
	if got.Truncated != nil {
		t.Errorf("by-id lookup must not carry truncated, got %v", *got.Truncated)
	}

	// Absent id is a finding, not an error (never-reject).
	miss := idx.Lookup("", "does/not-exist", "")
	if miss.NotFound == nil || !*miss.NotFound {
		t.Fatalf("absent id not_found = %v, want true", miss.NotFound)
	}
	if len(miss.Nodes) != 0 {
		t.Fatalf("absent id nodes = %v, want []", nodeIDs(miss.Nodes))
	}
}

func TestLookupByLabel(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Lookup("", "", "Metric")
	want := []string{"metrics/gross-margin", "metrics/gross-margin-legacy", "metrics/revenue"}
	if ids := nodeIDs(got.Nodes); !reflect.DeepEqual(ids, want) {
		t.Fatalf("Metric nodes = %v, want %v", ids, want)
	}
	if got.Truncated == nil || *got.Truncated {
		t.Fatalf("truncated = %v, want false", got.Truncated)
	}
	if got.NotFound != nil {
		t.Errorf("by-label lookup must not carry not_found, got %v", *got.NotFound)
	}

	// A label with no nodes is an empty result, not an error.
	none := idx.Lookup("", "", "NoSuchLabel")
	if len(none.Nodes) != 0 {
		t.Fatalf("unknown label nodes = %v, want []", nodeIDs(none.Nodes))
	}
}

func TestNeighborsDirections(t *testing.T) {
	idx := acmeIndex(t)
	const gm = "metrics/gross-margin"

	out := idx.Neighbors("", gm, "out", "")
	wantOut := []string{"computations/gross-margin-period", "metrics/gross-margin-legacy", "metrics/revenue"}
	if ids := nodeIDs(out.Nodes); !reflect.DeepEqual(ids, wantOut) {
		t.Fatalf("out neighbors = %v, want %v", ids, wantOut)
	}
	// The two parallel gross-margin -> gross-margin-legacy edges both appear.
	if len(out.Edges) != 4 {
		t.Fatalf("out edges = %d (%v), want 4", len(out.Edges), edgeTriples(out.Edges))
	}
	if out.NotFound {
		t.Error("existing node must not be not_found")
	}

	in := idx.Neighbors("", gm, "in", "")
	wantIn := []string{"metrics/gross-margin-legacy", "policies/margin-standard", "policies/revenue-recognition"}
	if ids := nodeIDs(in.Nodes); !reflect.DeepEqual(ids, wantIn) {
		t.Fatalf("in neighbors = %v, want %v", ids, wantIn)
	}

	both := idx.Neighbors("", gm, "both", "")
	wantBoth := []string{
		"computations/gross-margin-period", "metrics/gross-margin-legacy",
		"metrics/revenue", "policies/margin-standard", "policies/revenue-recognition",
	}
	if ids := nodeIDs(both.Nodes); !reflect.DeepEqual(ids, wantBoth) {
		t.Fatalf("both neighbors = %v, want %v", ids, wantBoth)
	}
}

func TestNeighborsRelFilter(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Neighbors("", "metrics/gross-margin", "out", "Revenue")
	if ids := nodeIDs(got.Nodes); !reflect.DeepEqual(ids, []string{"metrics/revenue"}) {
		t.Fatalf("rel-filtered neighbors = %v, want [metrics/revenue]", ids)
	}
	want := [][3]string{{"metrics/gross-margin", "metrics/revenue", "Revenue"}}
	if tr := edgeTriples(got.Edges); !reflect.DeepEqual(tr, want) {
		t.Fatalf("rel-filtered edges = %v, want %v", tr, want)
	}
}

func TestNeighborsNotFound(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Neighbors("", "nope", "out", "")
	if !got.NotFound {
		t.Fatal("absent start must be not_found")
	}
	if len(got.Nodes) != 0 || len(got.Edges) != 0 {
		t.Fatalf("absent start must have empty nodes/edges, got %v / %v", nodeIDs(got.Nodes), edgeTriples(got.Edges))
	}
}

func TestNeighborhoodDepths(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Neighborhood("", "metrics/gross-margin", 2, "out", "")

	wantNodes := []string{
		"computations/gross-margin-period", "computations/revenue-ytd",
		"metrics/gross-margin", "metrics/gross-margin-legacy", "metrics/revenue",
	}
	if ids := nodeIDs(got.Nodes); !reflect.DeepEqual(ids, wantNodes) {
		t.Fatalf("neighborhood nodes = %v, want %v", ids, wantNodes)
	}

	wantDepths := []Depth{
		{"metrics/gross-margin", 0},
		{"computations/gross-margin-period", 1},
		{"metrics/gross-margin-legacy", 1},
		{"metrics/revenue", 1},
		{"computations/revenue-ytd", 2},
	}
	if !reflect.DeepEqual(got.Depths, wantDepths) {
		t.Fatalf("depths = %v, want %v", got.Depths, wantDepths)
	}

	// Induced subgraph on the reached nodes includes the reverse cycle edge.
	if len(got.Edges) != 7 {
		t.Fatalf("neighborhood edges = %d (%v), want 7", len(got.Edges), edgeTriples(got.Edges))
	}
}

// TestNeighborhoodCycleTerminates: a corpus with a 2-cycle (gross-margin <->
// gross-margin-legacy) terminates even at the max depth, emitting each node once.
func TestNeighborhoodCycleTerminates(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Neighborhood("", "metrics/gross-margin", MaxDepth, "both", "")
	seen := map[string]bool{}
	for _, d := range got.Depths {
		if seen[d.ID] {
			t.Fatalf("node %q emitted more than once (cycle guard failed)", d.ID)
		}
		seen[d.ID] = true
	}
	// gross-margin and its cycle partner are both present, each once.
	if !seen["metrics/gross-margin"] || !seen["metrics/gross-margin-legacy"] {
		t.Fatalf("expected both cycle members present, got %v", got.Depths)
	}
}

func TestPatternToLabel(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Pattern("", "Policy", "Metric", "", nil)
	wantNodes := []string{"policies/margin-standard", "policies/revenue-recognition"}
	if ids := nodeIDs(got.Nodes); !reflect.DeepEqual(ids, wantNodes) {
		t.Fatalf("pattern nodes = %v, want %v", ids, wantNodes)
	}
	wantEdges := [][3]string{
		{"policies/margin-standard", "metrics/gross-margin", "metrics/gross-margin"},
		{"policies/margin-standard", "metrics/gross-margin-legacy", "metrics/gross-margin-legacy"},
		{"policies/revenue-recognition", "metrics/gross-margin", "metrics/gross-margin"},
		{"policies/revenue-recognition", "metrics/revenue", "metrics/revenue"},
	}
	if tr := edgeTriples(got.Edges); !reflect.DeepEqual(tr, wantEdges) {
		t.Fatalf("pattern edges = %v, want %v", tr, wantEdges)
	}
}

func TestPatternWhereType(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Pattern("", "Policy", "", "", &WhereClause{Prop: "type", Eq: "BigQuery Table"})
	if ids := nodeIDs(got.Nodes); !reflect.DeepEqual(ids, []string{"policies/revenue-recognition"}) {
		t.Fatalf("pattern-where nodes = %v, want [policies/revenue-recognition]", ids)
	}
	want := [][3]string{{"policies/revenue-recognition", "tables/orders", "tables/orders"}}
	if tr := edgeTriples(got.Edges); !reflect.DeepEqual(tr, want) {
		t.Fatalf("pattern-where edges = %v, want %v", tr, want)
	}
}

func TestPatternWhereStale(t *testing.T) {
	idx := acmeIndex(t)
	// No node is stale at fixedToday, so a stale=true predicate matches nothing —
	// an empty result, not an error.
	got := idx.Pattern("", "Policy", "", "", &WhereClause{Prop: "stale", Eq: "true"})
	if len(got.Nodes) != 0 || len(got.Edges) != 0 {
		t.Fatalf("stale=true pattern should be empty, got %v / %v", nodeIDs(got.Nodes), edgeTriples(got.Edges))
	}
}

func TestPathShortest(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Path("", "metrics/gross-margin", "computations/revenue-ytd", "out", 3)
	if !got.Exists {
		t.Fatal("path should exist")
	}
	want := []string{"metrics/gross-margin", "computations/gross-margin-period", "computations/revenue-ytd"}
	if !reflect.DeepEqual(got.Path, want) {
		t.Fatalf("path = %v, want %v", got.Path, want)
	}
	if got.Length != 2 {
		t.Fatalf("length = %d, want 2", got.Length)
	}
}

func TestPathUnreachable(t *testing.T) {
	idx := acmeIndex(t)
	// tables/orders has no outgoing edges, so nothing is reachable from it.
	got := idx.Path("", "tables/orders", "metrics/gross-margin", "out", MaxDepth)
	if got.Exists {
		t.Fatal("path should not exist")
	}
	if got.NotFound {
		t.Fatal("both endpoints exist, so not_found must be false")
	}
	if len(got.Path) != 0 {
		t.Fatalf("path = %v, want []", got.Path)
	}
}

func TestPathAbsentEndpoint(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Path("", "metrics/gross-margin", "does/not-exist", "out", MaxDepth)
	if !got.NotFound {
		t.Fatal("absent endpoint must be not_found")
	}
	if got.Exists {
		t.Fatal("absent endpoint must not report exists")
	}
}

func TestPathSelf(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Path("", "tables/orders", "tables/orders", "out", MaxDepth)
	if !got.Exists || got.Length != 0 {
		t.Fatalf("self path exists=%v length=%d, want true/0", got.Exists, got.Length)
	}
	if !reflect.DeepEqual(got.Path, []string{"tables/orders"}) {
		t.Fatalf("self path = %v, want [tables/orders]", got.Path)
	}
}

// TestNodeKeyEcho: a non-empty id_key is echoed verbatim and never honored in v1
// (§14.1). Identity is unchanged: the node id remains the path-derived Concept.ID.
func TestNodeKeyEcho(t *testing.T) {
	idx := acmeIndex(t)
	got := idx.Lookup("concept-id", "tables/orders", "")
	if got.NodeKey.Strategy != "path" {
		t.Errorf("strategy = %q, want path", got.NodeKey.Strategy)
	}
	if got.NodeKey.Key != "concept-id" {
		t.Errorf("key = %q, want concept-id (echoed verbatim)", got.NodeKey.Key)
	}
	if got.NodeKey.Honored {
		t.Error("honored must be false in v1 (id_key never re-keys identity)")
	}
	// Identity is still the path id.
	if nodeIDs(got.Nodes)[0] != "tables/orders" {
		t.Errorf("identity changed under id_key: %v", nodeIDs(got.Nodes))
	}
}

// TestLabelTruncation drives the MaxResults cap over a synthetic model of
// MaxResults+1 same-typed nodes: the result is sorted then truncated to exactly
// MaxResults with truncated:true.
func TestLabelTruncation(t *testing.T) {
	m := &Model{}
	for i := 0; i <= MaxResults; i++ { // MaxResults+1 nodes
		id := idFor(i)
		m.Nodes = append(m.Nodes, Node{ID: id, Title: id, Type: "T"})
	}
	sortNodes(m.Nodes) // Build guarantees sorted order; mirror it here
	idx := NewIndex(m)
	got := idx.Lookup("", "", "T")
	if len(got.Nodes) != MaxResults {
		t.Fatalf("nodes = %d, want %d (capped)", len(got.Nodes), MaxResults)
	}
	if got.Truncated == nil || !*got.Truncated {
		t.Fatalf("truncated = %v, want true", got.Truncated)
	}
	// The retained prefix is the sorted prefix (stable).
	if got.Nodes[0].ID != m.Nodes[0].ID {
		t.Fatalf("retained prefix not stable: first = %q, want %q", got.Nodes[0].ID, m.Nodes[0].ID)
	}
}

// TestNeighborsTruncation drives the cap over a hub with MaxResults+1 out-edges:
// neighbors are capped and edges to dropped nodes are filtered out.
func TestNeighborsTruncation(t *testing.T) {
	m := &Model{}
	m.Nodes = append(m.Nodes, Node{ID: "hub", Type: "H"})
	for i := 0; i <= MaxResults; i++ {
		id := idFor(i)
		m.Nodes = append(m.Nodes, Node{ID: id, Type: "T"})
		m.Edges = append(m.Edges, Edge{From: "hub", To: id})
	}
	sortNodes(m.Nodes)
	sortEdges(m.Edges)
	idx := NewIndex(m)
	got := idx.Neighbors("", "hub", "out", "")
	if len(got.Nodes) != MaxResults {
		t.Fatalf("neighbors = %d, want %d", len(got.Nodes), MaxResults)
	}
	if !got.Truncated {
		t.Fatal("truncated = false, want true")
	}
	// Every retained edge points at a retained node.
	keep := map[string]bool{"hub": true}
	for _, n := range got.Nodes {
		keep[n.ID] = true
	}
	for _, e := range got.Edges {
		if !keep[e.To] {
			t.Fatalf("edge to dropped node survived: %v", e)
		}
	}
	if len(got.Edges) != MaxResults {
		t.Fatalf("edges = %d, want %d", len(got.Edges), MaxResults)
	}
}

// TestLookupByLabelSortsUnsortedInput proves the by-label lookup path sorts
// explicitly rather than trusting an upstream ordering guarantee. NewIndex builds
// byLabel in Model.Nodes order, so a Model whose same-typed nodes are inserted out
// of ID order produces an out-of-order byLabel bucket. The result must still come
// back sorted by ID. This test goes RED if the explicit sortNodes call is removed
// from Index.Lookup's by-label branch (verified by deleting it: nodes returned as
// [n/c, n/a, n/b], failing this assertion), and green with it restored.
func TestLookupByLabelSortsUnsortedInput(t *testing.T) {
	m := &Model{Nodes: []Node{
		{ID: "n/c", Title: "c", Type: "T"},
		{ID: "n/a", Title: "a", Type: "T"},
		{ID: "n/b", Title: "b", Type: "T"},
	}}
	idx := NewIndex(m) // deliberately NOT sorted; byLabel["T"] = [n/c, n/a, n/b]
	got := idx.Lookup("", "", "T")
	want := []string{"n/a", "n/b", "n/c"}
	if ids := nodeIDs(got.Nodes); !reflect.DeepEqual(ids, want) {
		t.Fatalf("by-label lookup on unsorted input = %v, want %v (must sort by ID)", ids, want)
	}
}

// idFor produces a zero-padded id so lexical order matches numeric order.
func idFor(i int) string {
	const width = 5
	b := []byte("00000")
	for p := width - 1; p >= 0; p-- {
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return "n/" + string(b)
}
