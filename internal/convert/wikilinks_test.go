package convert

import "testing"

func testIndex() *corpusIndex {
	return buildCorpusIndex([]indexEntry{
		{srcRel: "intro.md", outRel: "intro.md", title: "Introduction"},
		{srcRel: "tables/orders.md", outRel: "tables/orders.md", title: "Customer Orders"},
		{srcRel: "guides/setup.md", outRel: "guides/setup.md", title: "Setup Guide"},
		{srcRel: "guides/index.md", outRel: "guides/index-note.md", title: "Guides"},
	})
}

func TestRewriteWikilinks(t *testing.T) {
	ix := testIndex()

	t.Run("by rel-path", func(t *testing.T) {
		out, links := rewriteWikilinks("See [[tables/orders]].", "", ix)
		if out != "See [tables/orders](/tables/orders.md)." {
			t.Fatalf("got %q", out)
		}
		if len(links) != 1 || !links[0].Resolved || links[0].TargetID != "tables/orders" {
			t.Fatalf("link: %+v", links)
		}
	})

	t.Run("by filename", func(t *testing.T) {
		out, links := rewriteWikilinks("[[orders]]", "guides", ix)
		if out != "[orders](/tables/orders.md)" {
			t.Fatalf("got %q", out)
		}
		if !links[0].Resolved {
			t.Fatalf("should resolve by filename: %+v", links[0])
		}
	})

	t.Run("by title-slug", func(t *testing.T) {
		out, links := rewriteWikilinks("[[Customer Orders]]", "", ix)
		if out != "[Customer Orders](/tables/orders.md)" {
			t.Fatalf("got %q", out)
		}
		if !links[0].Resolved {
			t.Fatalf("should resolve by title: %+v", links[0])
		}
	})

	t.Run("alias display", func(t *testing.T) {
		out, _ := rewriteWikilinks("[[tables/orders|the orders table]]", "", ix)
		if out != "[the orders table](/tables/orders.md)" {
			t.Fatalf("got %q", out)
		}
	})

	t.Run("unresolved left in place and reported", func(t *testing.T) {
		out, links := rewriteWikilinks("[[Nonexistent Page]]", "", ix)
		if out != "[[Nonexistent Page]]" {
			t.Fatalf("unresolved must be left untouched, got %q", out)
		}
		if len(links) != 1 || links[0].Resolved {
			t.Fatalf("unresolved should be reported: %+v", links)
		}
	})

	t.Run("renamed reserved target resolves to output name", func(t *testing.T) {
		out, _ := rewriteWikilinks("[[guides/index]]", "", ix)
		if out != "[guides/index](/guides/index-note.md)" {
			t.Fatalf("got %q", out)
		}
	})

	t.Run("wikilink inside code left untouched", func(t *testing.T) {
		body := "`[[tables/orders]]`"
		out, links := rewriteWikilinks(body, "", ix)
		if out != body {
			t.Fatalf("code wikilink must be untouched, got %q", out)
		}
		if len(links) != 0 {
			t.Fatalf("no edge inside code: %+v", links)
		}
	})
}

func TestAmbiguousFilenameUnresolved(t *testing.T) {
	ix := buildCorpusIndex([]indexEntry{
		{srcRel: "a/notes.md", outRel: "a/notes.md", title: "A Notes"},
		{srcRel: "b/notes.md", outRel: "b/notes.md", title: "B Notes"},
	})
	_, resolved, ambiguous := ix.resolve("", "notes")
	if resolved || !ambiguous {
		t.Fatalf("ambiguous filename should not resolve: resolved=%v ambiguous=%v", resolved, ambiguous)
	}
	// but an exact rel-path still resolves.
	out, ok, _ := ix.resolve("", "a/notes")
	if !ok || out != "a/notes.md" {
		t.Fatalf("exact rel-path should resolve: %q %v", out, ok)
	}
}
