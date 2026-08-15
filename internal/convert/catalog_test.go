package convert

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf"
)

// catalogFixture returns a small, deterministic concept set spanning several
// directories and types, with a mix of resolved and unresolved links. It backs
// the catalog golden, annotation, idempotency, and edge-parity tests.
func catalogFixture() []*okf.Concept {
	title := func(s string) *okf.OrderedMap {
		fm := okf.NewOrderedMap()
		if s != "" {
			fm.Set("title", s)
		}
		return fm
	}
	return []*okf.Concept{
		{
			ID: "patterns/alpha", RelPath: "patterns/alpha.md", Type: "Pattern",
			Frontmatter: title("Alpha"),
			Links: []okf.Link{
				{TargetID: "guides/setup", Resolved: true},
				{RawTarget: "/missing.md", Resolved: false}, // unresolved: excluded from edges
			},
		},
		{
			ID: "patterns/beta", RelPath: "patterns/beta.md", Type: "Pattern",
			Frontmatter: title("Beta"),
			Links: []okf.Link{
				{TargetID: "patterns/alpha", Resolved: true},
			},
		},
		{
			ID: "guides/setup", RelPath: "guides/setup.md", Type: "Guide",
			Frontmatter: title("Setup"),
		},
		{
			ID: "misc/notes", RelPath: "misc/notes.md", Type: "",
			Frontmatter: title(""), // no title → conceptTitle falls back to ID
		},
	}
}

func TestRenderCatalogGroupByType(t *testing.T) {
	got := string(renderCatalog(catalogFixture(), IndexOptions{GroupByType: true}))
	want := "\n# Catalog\n" +
		"\n## Guide\n\n" +
		"* [Setup](/guides/setup.md)\n" +
		"\n## Pattern\n\n" +
		"* [Alpha](/patterns/alpha.md)\n" +
		"* [Beta](/patterns/beta.md)\n" +
		"\n## (untyped)\n\n" +
		"* [misc/notes](/misc/notes.md)\n"
	if got != want {
		t.Errorf("catalog mismatch:\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

func TestRenderCatalogWithBacklinks(t *testing.T) {
	got := string(renderCatalog(catalogFixture(), IndexOptions{GroupByType: true, IncludeBacklinks: true}))
	want := "\n# Catalog\n" +
		"\n## Guide\n\n" +
		"* [Setup](/guides/setup.md)\n" +
		"  * backlink: [Alpha](/patterns/alpha.md)\n" +
		"\n## Pattern\n\n" +
		"* [Alpha](/patterns/alpha.md)\n" +
		"  * backlink: [Beta](/patterns/beta.md)\n" +
		"* [Beta](/patterns/beta.md)\n" +
		"\n## (untyped)\n\n" +
		"* [misc/notes](/misc/notes.md)\n"
	if got != want {
		t.Errorf("catalog+backlinks mismatch:\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

func TestRenderCatalogWithGraph(t *testing.T) {
	got := string(renderCatalog(catalogFixture(), IndexOptions{GroupByType: true, IncludeGraph: true}))
	want := "\n# Catalog\n" +
		"\n## Guide\n\n" +
		"* [Setup](/guides/setup.md)\n" +
		"\n## Pattern\n\n" +
		"* [Alpha](/patterns/alpha.md)\n" +
		"  * link: [Setup](/guides/setup.md)\n" +
		"* [Beta](/patterns/beta.md)\n" +
		"  * link: [Alpha](/patterns/alpha.md)\n" +
		"\n## (untyped)\n\n" +
		"* [misc/notes](/misc/notes.md)\n"
	if got != want {
		t.Errorf("catalog+graph mismatch:\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

func TestRenderCatalogAllFlags(t *testing.T) {
	got := string(renderCatalog(catalogFixture(), IndexOptions{GroupByType: true, IncludeBacklinks: true, IncludeGraph: true}))
	want := "\n# Catalog\n" +
		"\n## Guide\n\n" +
		"* [Setup](/guides/setup.md)\n" +
		"  * backlink: [Alpha](/patterns/alpha.md)\n" +
		"\n## Pattern\n\n" +
		"* [Alpha](/patterns/alpha.md)\n" +
		"  * backlink: [Beta](/patterns/beta.md)\n" +
		"  * link: [Setup](/guides/setup.md)\n" +
		"* [Beta](/patterns/beta.md)\n" +
		"  * link: [Alpha](/patterns/alpha.md)\n" +
		"\n## (untyped)\n\n" +
		"* [misc/notes](/misc/notes.md)\n"
	if got != want {
		t.Errorf("catalog+all mismatch:\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

// TestRenderCatalogEdgeCap exercises the maxCatalogEdges truncation branch: an
// entry with more than maxCatalogEdges outbound edges renders exactly
// maxCatalogEdges annotation lines followed by a single "… and N more" overflow
// line carrying the correct remainder.
func TestRenderCatalogEdgeCap(t *testing.T) {
	const extra = 5
	total := maxCatalogEdges + extra

	title := func(s string) *okf.OrderedMap {
		fm := okf.NewOrderedMap()
		fm.Set("title", s)
		return fm
	}
	hub := &okf.Concept{ID: "hub", RelPath: "hub.md", Type: "Pattern", Frontmatter: title("Hub")}
	concepts := []*okf.Concept{hub}
	// hub links to `total` distinct concepts, so every link resolves to an edge.
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("t/%02d", i)
		hub.Links = append(hub.Links, okf.Link{TargetID: id, Resolved: true})
		concepts = append(concepts, &okf.Concept{
			ID: id, RelPath: id + ".md", Type: "Target", Frontmatter: title("T" + id),
		})
	}

	got := string(renderCatalog(concepts, IndexOptions{GroupByType: true, IncludeGraph: true}))

	if n := strings.Count(got, "  * link: "); n != maxCatalogEdges {
		t.Errorf("rendered %d edge annotations, want exactly %d", n, maxCatalogEdges)
	}
	overflow := fmt.Sprintf("  * … and %d more\n", extra)
	if !strings.Contains(got, overflow) {
		t.Errorf("missing overflow line %q in:\n%s", overflow, got)
	}
	if n := strings.Count(got, "… and "); n != 1 {
		t.Errorf("want exactly one overflow line, got %d", n)
	}
}

// TestGenerateIndexesDefaultUnchanged proves the no-flag path is byte-identical
// to a run with the zero IndexOptions, and that no "# Catalog" leaks in.
func TestGenerateIndexesDefaultUnchanged(t *testing.T) {
	concepts := catalogFixture()
	def := GenerateIndexes(concepts, okf.SpecV02, IndexOptions{})
	for rel, data := range def {
		if bytes.Contains(data, []byte("# Catalog")) {
			t.Errorf("default output for %q unexpectedly contains a catalog", rel)
		}
	}
	// Catalog appears ONLY in the root index, and ONLY when GroupByType is set.
	grouped := GenerateIndexes(concepts, okf.SpecV02, IndexOptions{GroupByType: true})
	if !bytes.Contains(grouped["index.md"], []byte("# Catalog")) {
		t.Fatal("root index.md missing catalog with --group-by-type")
	}
	for rel, data := range grouped {
		if rel == "index.md" {
			continue
		}
		if bytes.Contains(data, []byte("# Catalog")) {
			t.Errorf("non-root index %q unexpectedly contains a catalog", rel)
		}
	}
	// The additive catalog must leave the pre-existing root nav bytes intact.
	if !bytes.HasPrefix(grouped["index.md"], def["index.md"]) {
		t.Error("catalog is not purely additive: root nav bytes changed")
	}
}

// TestGenerateIndexesIdempotent proves two runs on identical input yield
// byte-identical output for every generated index (determinism/idempotency).
func TestGenerateIndexesIdempotent(t *testing.T) {
	opts := IndexOptions{GroupByType: true, IncludeBacklinks: true, IncludeGraph: true}
	a := GenerateIndexes(catalogFixture(), okf.SpecV02, opts)
	b := GenerateIndexes(catalogFixture(), okf.SpecV02, opts)
	if len(a) != len(b) {
		t.Fatalf("index count differs: %d vs %d", len(a), len(b))
	}
	for rel, av := range a {
		if !bytes.Equal(av, b[rel]) {
			t.Errorf("index %q not idempotent:\n--- run1 ---\n%s\n--- run2 ---\n%s", rel, av, b[rel])
		}
	}
}

// TestEdgeSetParity is the REQUIRED edge-set parity invariant: the resolved-edge
// set the catalog annotations derive from (graph.EdgesFromConcepts) is exactly
// the edge set `binder graph` builds (graph.Build(...).Edges). Same helper, same
// rule — proven equal on a fixture so they can never drift.
func TestEdgeSetParity(t *testing.T) {
	concepts := catalogFixture()
	catalogEdges := graph.EdgesFromConcepts(concepts)
	graphEdges := graph.Build(&okf.Bundle{Concepts: concepts}, "").Edges
	if !reflect.DeepEqual(catalogEdges, graphEdges) {
		t.Errorf("edge-set parity broken:\ncatalog=%v\ngraph  =%v", catalogEdges, graphEdges)
	}
	// Sanity: only the two resolved links are edges; the unresolved one is dropped.
	if len(catalogEdges) != 2 {
		t.Fatalf("want 2 resolved edges, got %d: %v", len(catalogEdges), catalogEdges)
	}
}
