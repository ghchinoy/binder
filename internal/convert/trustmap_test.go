package convert

import (
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

func newConcept(body string, kv ...string) *okf.Concept {
	fm := okf.NewOrderedMap()
	for i := 0; i+1 < len(kv); i += 2 {
		fm.Set(kv[i], kv[i+1])
	}
	return &okf.Concept{ID: "c", RelPath: "c.md", Type: "Note", Frontmatter: fm, Body: body}
}

func sources(t *testing.T, c *okf.Concept) []any {
	t.Helper()
	v, ok := c.Frontmatter.Get("sources")
	if !ok {
		return nil
	}
	list, _ := v.([]any)
	return list
}

func TestMapTrustNoOpByDefault(t *testing.T) {
	c := newConcept("# Citations\n- https://example.com/a\n", "draft", "true")
	before := c.Frontmatter.Keys()
	mapTrust(c, Options{}) // all mapping options off
	if len(c.Frontmatter.Keys()) != len(before) {
		t.Errorf("mapTrust mutated frontmatter with no options set: %v", c.Frontmatter.Keys())
	}
	if _, ok := c.Frontmatter.Get("sources"); ok {
		t.Error("sources should not be created when MapCitations is off")
	}
	if _, ok := c.Frontmatter.Get("status"); ok {
		t.Error("status should not be created when MapDraft is off")
	}
}

func TestMapDraftSetsStatusOnlyWhenAbsent(t *testing.T) {
	c := newConcept("", "draft", "true")
	mapTrust(c, Options{MapDraft: true})
	if v, _ := c.Frontmatter.Get("status"); v != "draft" {
		t.Errorf("status = %v, want draft", v)
	}

	// Existing status must never be clobbered.
	c2 := newConcept("", "draft", "true", "status", "stable")
	mapTrust(c2, Options{MapDraft: true})
	if v, _ := c2.Frontmatter.Get("status"); v != "stable" {
		t.Errorf("status = %v, want stable (must not clobber)", v)
	}
}

func TestMapSourceKeys(t *testing.T) {
	c := newConcept("", "source", "https://example.com/spec", "author", "human:alice")
	mapTrust(c, Options{SourceKeys: []string{"source", "author"}})
	list := sources(t, c)
	if len(list) != 2 {
		t.Fatalf("sources = %d, want 2: %v", len(list), list)
	}
	e0 := list[0].(map[string]any)
	if e0["resource"] != "https://example.com/spec" {
		t.Errorf("source[0] = %v", e0)
	}
	e1 := list[1].(map[string]any)
	if e1["author"] != "human:alice" {
		t.Errorf("source[1] = %v", e1)
	}
	// Original keys preserved (additive).
	if !c.Frontmatter.Has("source") || !c.Frontmatter.Has("author") {
		t.Error("original source/author keys must be preserved")
	}
}

func TestMapCitationsParsesLinksURLsAndText(t *testing.T) {
	body := "# Notes\n\nintro\n\n## Citations\n" +
		"- [Orders schema](https://example.com/schema)\n" +
		"- https://example.com/bare\n" +
		"- Some book, 2020\n" +
		"- \n"
	c := newConcept(body)
	mapTrust(c, Options{MapCitations: true})
	list := sources(t, c)
	if len(list) != 3 {
		t.Fatalf("sources = %d, want 3: %v", len(list), list)
	}
	if m := list[0].(map[string]any); m["title"] != "Orders schema" || m["resource"] != "https://example.com/schema" {
		t.Errorf("citation[0] = %v", m)
	}
	if m := list[1].(map[string]any); m["resource"] != "https://example.com/bare" {
		t.Errorf("citation[1] = %v", m)
	}
	if m := list[2].(map[string]any); m["title"] != "Some book, 2020" {
		t.Errorf("citation[2] = %v", m)
	}
}

func TestMapTrustDedupesAgainstExistingSources(t *testing.T) {
	c := newConcept("# Citations\n- https://example.com/dup\n")
	c.Frontmatter.Set("sources", []any{
		map[string]any{"resource": "https://example.com/dup"},
	})
	mapTrust(c, Options{MapCitations: true})
	if list := sources(t, c); len(list) != 1 {
		t.Errorf("sources = %d, want 1 (deduped): %v", len(list), list)
	}
}

func TestParseCitationsIgnoresOutsideSection(t *testing.T) {
	body := "# Intro\n- not a citation\n# Citations\n- real one\n# After\n- also not\n"
	got := parseCitations(body)
	if len(got) != 1 || got[0].title != "real one" {
		t.Errorf("parseCitations = %+v, want one 'real one'", got)
	}
}
