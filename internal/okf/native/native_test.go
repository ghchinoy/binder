package native

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

const goldenBundles = "../../../testdata/okf-bundles"

// TestRoundTripByteFaithful is the core §3.2 guarantee: parsing a real OKF
// concept and serializing it back with no edits reproduces the source bytes
// exactly — including nested-mapping key order and scalar quoting/folding style
// that a decode→re-encode round-trip could not preserve. It runs over every
// concept in the vendored golden bundles.
func TestRoundTripByteFaithful(t *testing.T) {
	c := New()
	var checked int
	err := filepath.Walk(goldenBundles, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		rel, _ := filepath.Rel(goldenBundles, p)
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if c.IsReservedFile(rel) {
			return nil // reserved files need not carry parseable frontmatter
		}
		if !strings.HasPrefix(string(raw), "---\n") {
			return nil // non-concept fixture files (e.g. ATTRIBUTION.md)
		}
		con, err := c.ParseConcept(rel, raw)
		if err != nil {
			t.Errorf("%s: parse: %v", rel, err)
			return nil
		}
		out, err := c.Serialize(con)
		if err != nil {
			t.Errorf("%s: serialize: %v", rel, err)
			return nil
		}
		if string(out) != string(raw) {
			t.Errorf("%s: round-trip not byte-faithful\n--- want ---\n%q\n--- got ---\n%q", rel, raw, out)
			return nil
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked == 0 {
		t.Fatal("no golden concepts checked; fixtures missing?")
	}
	t.Logf("byte-faithful round-trip verified on %d golden concepts", checked)
}

func TestParseConceptPreservesOrderAndTrust(t *testing.T) {
	raw := []byte("---\n" +
		"type: BigQuery Table\n" +
		"title: Orders\n" +
		"description: One row per order.\n" +
		"tags:\n" +
		"- sales\n" +
		"- orders\n" +
		"generated:\n" +
		"  by: binder/0.1.0\n" +
		"  at: '2026-01-01T00:00:00Z'\n" +
		"sources:\n" +
		"- id: s1\n" +
		"  title: Source One\n" +
		"custom_key: kept\n" +
		"---\n\n# Orders\n\nbody\n")

	c := New()
	con, err := c.ParseConcept("tables/orders.md", raw)
	if err != nil {
		t.Fatal(err)
	}
	if con.ID != "tables/orders" {
		t.Fatalf("ID = %q", con.ID)
	}
	if con.Type != "BigQuery Table" {
		t.Fatalf("Type = %q", con.Type)
	}
	wantOrder := []string{"type", "title", "description", "tags", "generated", "sources", "custom_key"}
	if got := con.Frontmatter.Keys(); !equalStrings(got, wantOrder) {
		t.Fatalf("top-level order = %v, want %v", got, wantOrder)
	}
	if con.Trust.Generated == nil || con.Trust.Generated.By != "binder/0.1.0" {
		t.Fatalf("generated not projected: %+v", con.Trust.Generated)
	}
	if con.Trust.Generated.At != "2026-01-01T00:00:00Z" {
		t.Fatalf("timestamp not kept as string: %q", con.Trust.Generated.At)
	}
	if len(con.Trust.Sources) != 1 || con.Trust.Sources[0].ID != "s1" {
		t.Fatalf("sources not projected: %+v", con.Trust.Sources)
	}
	if v, _ := con.Frontmatter.Get("custom_key"); v != "kept" {
		t.Fatalf("unknown key dropped: %v", v)
	}
}

func TestParseConceptErrors(t *testing.T) {
	c := New()
	cases := map[string][]byte{
		"no frontmatter": []byte("# Just markdown\n"),
		"unterminated":   []byte("---\ntype: Note\nno closing fence\n"),
		"invalid yaml":   []byte("---\ntype: [unterminated\n---\n"),
		"not a mapping":  []byte("---\n- just\n- a list\n---\n"),
	}
	for name, raw := range cases {
		if _, err := c.ParseConcept("x.md", raw); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestSerializeSynthesisedFrontmatter(t *testing.T) {
	// A concept the converter built from plain markdown: no OriginalFrontmatter,
	// so Serialize canonically encodes, top-level order preserved.
	fm := okf.NewOrderedMap()
	fm.Set("type", "Note")
	fm.Set("title", "Hello")
	fm.Set("generated", map[string]any{"by": "binder/0.1.0", "at": "2026-01-01T00:00:00Z"})
	con := &okf.Concept{ID: "hello", RelPath: "hello.md", Type: "Note", Frontmatter: fm, Body: "# Hello\n"}

	out, err := New().Serialize(con)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	// Deterministic and re-parseable.
	reparsed, err := New().ParseConcept("hello.md", out)
	if err != nil {
		t.Fatalf("serialized output does not re-parse: %v", err)
	}
	if !equalStrings(reparsed.Frontmatter.Keys(), []string{"type", "title", "generated"}) {
		t.Fatalf("top-level order not preserved on encode: %v", reparsed.Frontmatter.Keys())
	}
	if !strings.HasPrefix(got, "---\ntype: Note\ntitle: Hello\n") {
		t.Fatalf("unexpected canonical encoding:\n%s", got)
	}
	if !strings.Contains(got, "---\n\n# Hello\n") {
		t.Fatalf("body spacing wrong:\n%s", got)
	}
}

func TestSerializeIsIdempotent(t *testing.T) {
	fm := okf.NewOrderedMap()
	fm.Set("type", "Note")
	fm.Set("generated", map[string]any{"by": "binder/0.1.0", "at": "2026-01-01T00:00:00Z"})
	con := &okf.Concept{ID: "x", RelPath: "x.md", Type: "Note", Frontmatter: fm, Body: "body\n"}
	c := New()
	first, _ := c.Serialize(con)
	reparsed, err := c.ParseConcept("x.md", first)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := c.Serialize(reparsed)
	if string(first) != string(second) {
		t.Fatalf("serialize not idempotent:\n%q\nvs\n%q", first, second)
	}
}

func TestConceptIDAndReserved(t *testing.T) {
	c := New()
	if id, ok := c.ConceptIDFromRel("tables/orders.md"); !ok || id != "tables/orders" {
		t.Fatalf("ConceptIDFromRel = %q,%v", id, ok)
	}
	if _, ok := c.ConceptIDFromRel("index.md"); ok {
		t.Fatal("index.md must not be a concept")
	}
	if _, ok := c.ConceptIDFromRel("a/../../etc.md"); ok {
		t.Fatal("path traversal must be rejected")
	}
	if !c.IsReservedFile("sub/log.md") || !c.IsReservedFile("index.md") {
		t.Fatal("index.md/log.md must be reserved")
	}
	if c.IsReservedFile("notes.md") {
		t.Fatal("notes.md is not reserved")
	}
	if rel, err := c.RelFromConceptID("tables/orders"); err != nil || rel != "tables/orders.md" {
		t.Fatalf("RelFromConceptID = %q,%v", rel, err)
	}
}

func TestExtractAndResolveLinks(t *testing.T) {
	c := New()
	body := "See [orders](tables/orders.md) and [ext](https://x) and code:\n\n" +
		"```\n[fake](trap.md)\n```\n\ninline `[also](trap.md)` here.\n"
	links := c.ExtractLinks("intro", body)
	// The two links in code (fenced + inline span) must NOT appear.
	for _, l := range links {
		if strings.Contains(l.RawTarget, "trap.md") {
			t.Fatalf("link inside code was extracted: %+v", links)
		}
	}
	var foundOrders bool
	for _, l := range links {
		if l.RawTarget == "tables/orders.md" {
			foundOrders = true
			if !l.Resolved || l.TargetID != "tables/orders" {
				t.Fatalf("orders link should resolve: %+v", l)
			}
		}
	}
	if !foundOrders {
		t.Fatalf("expected orders link, got %+v", links)
	}

	if id, ok := c.ResolveLink("tables/orders", "../intro.md"); !ok || id != "intro" {
		t.Fatalf("parent-relative resolve = %q,%v", id, ok)
	}
	if _, ok := c.ResolveLink("intro", "https://example.com"); ok {
		t.Fatal("external link must not resolve")
	}
	if _, ok := c.ResolveLink("intro", "pic.png"); ok {
		t.Fatal("non-.md link must not resolve")
	}
}

func equalStrings(a, b []string) bool {
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
