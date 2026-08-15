package graph

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

func concept(id, typ, title string, links ...okf.Link) *okf.Concept {
	fm := okf.NewOrderedMap()
	if title != "" {
		fm.Set("title", title)
	}
	return &okf.Concept{ID: id, RelPath: id + ".md", Type: typ, Frontmatter: fm, Links: links}
}

func sampleBundle() *okf.Bundle {
	intro := concept("intro", "Note", "Intro",
		okf.Link{TargetID: "guide", RawTarget: "guide", Text: "related", Resolved: true},
		okf.Link{RawTarget: "missing.md", Resolved: false}, // unresolved: not an edge
	)
	guide := concept("guide", "Guide", "Guide")
	guide.Trust = okf.TrustSignals{StaleAfter: "2020-01-01"} // stale
	return &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{guide, intro}}
}

func TestBuildEdgesAreResolvedOnlyAndSorted(t *testing.T) {
	m := Build(sampleBundle(), "2026-08-15")
	if len(m.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(m.Nodes))
	}
	// Nodes sorted by ID: guide, intro.
	if m.Nodes[0].ID != "guide" || m.Nodes[1].ID != "intro" {
		t.Errorf("nodes not sorted by id: %+v", m.Nodes)
	}
	if !m.Nodes[0].Stale {
		t.Error("guide should be marked stale")
	}
	if len(m.Edges) != 1 {
		t.Fatalf("edges = %d, want 1 (unresolved excluded)", len(m.Edges))
	}
	if m.Edges[0].From != "intro" || m.Edges[0].To != "guide" || m.Edges[0].Text != "related" {
		t.Errorf("edge = %+v", m.Edges[0])
	}
}

func TestExportAllFormatsRenderAndParse(t *testing.T) {
	b := sampleBundle()

	dot, err := Export(b, "dot", "2026-08-15")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(dot, []byte("digraph okf {")) || !bytes.Contains(dot, []byte("->")) {
		t.Errorf("dot output unexpected:\n%s", dot)
	}

	js, err := Export(b, "json", "2026-08-15")
	if err != nil {
		t.Fatal(err)
	}
	var model Model
	if err := json.Unmarshal(js, &model); err != nil {
		t.Fatalf("json does not parse: %v", err)
	}
	if len(model.Nodes) != 2 || len(model.Edges) != 1 {
		t.Errorf("json model = %+v", model)
	}

	gml, err := Export(b, "graphml", "2026-08-15")
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(gml, new(interface{})); err != nil {
		t.Fatalf("graphml is not well-formed XML: %v", err)
	}
	if !bytes.Contains(gml, []byte("graphml")) {
		t.Errorf("graphml missing root element:\n%s", gml)
	}

	html, err := Export(b, "html", "2026-08-15")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte("<!doctype html>")) || !bytes.Contains(html, []byte("graph-data")) {
		t.Errorf("html output unexpected:\n%s", html)
	}

	if _, err := Export(b, "svg", "2026-08-15"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestExportIsDeterministic(t *testing.T) {
	b := sampleBundle()
	for _, f := range []string{"dot", "json", "graphml", "html"} {
		a, _ := Export(b, f, "2026-08-15")
		c, _ := Export(b, f, "2026-08-15")
		if !bytes.Equal(a, c) {
			t.Errorf("format %q is not deterministic", f)
		}
	}
}

func TestDOTEscapesQuotes(t *testing.T) {
	c := concept(`weird"id`, "Note", `A "quoted" title`)
	m := Build(&okf.Bundle{Concepts: []*okf.Concept{c}}, "2026-08-15")
	dot := string(m.DOT())
	if !strings.Contains(dot, `\"`) {
		t.Errorf("expected escaped quotes in dot:\n%s", dot)
	}
}
