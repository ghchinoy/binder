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

	t.Run("by_rel_path", func(t *testing.T) {
		in := "See [[tables/orders]]."
		want := "See [tables/orders](/tables/orders.md)."
		out, links := rewriteWikilinks(in, "", ix)
		if out != want {
			t.Fatalf("rewriteWikilinks(%q) = %q, want %q", in, out, want)
		}
		if len(links) != 1 || !links[0].Resolved || links[0].TargetID != "tables/orders" {
			t.Fatalf("rewriteWikilinks(%q) links = %+v, want resolved tables/orders", in, links)
		}
	})

	t.Run("by_filename", func(t *testing.T) {
		in := "[[orders]]"
		want := "[orders](/tables/orders.md)"
		out, links := rewriteWikilinks(in, "guides", ix)
		if out != want {
			t.Fatalf("rewriteWikilinks(%q) = %q, want %q", in, out, want)
		}
		if !links[0].Resolved {
			t.Fatalf("rewriteWikilinks(%q) should resolve by filename: %+v", in, links[0])
		}
	})

	t.Run("by_title_slug", func(t *testing.T) {
		in := "[[Customer Orders]]"
		want := "[Customer Orders](/tables/orders.md)"
		out, links := rewriteWikilinks(in, "", ix)
		if out != want {
			t.Fatalf("rewriteWikilinks(%q) = %q, want %q", in, out, want)
		}
		if !links[0].Resolved {
			t.Fatalf("rewriteWikilinks(%q) should resolve by title: %+v", in, links[0])
		}
	})

	t.Run("alias_display", func(t *testing.T) {
		in := "[[tables/orders|the orders table]]"
		want := "[the orders table](/tables/orders.md)"
		out, _ := rewriteWikilinks(in, "", ix)
		if out != want {
			t.Fatalf("rewriteWikilinks(%q) = %q, want %q", in, out, want)
		}
	})

	t.Run("unresolved_left_in_place_and_reported", func(t *testing.T) {
		in := "[[Nonexistent Page]]"
		out, links := rewriteWikilinks(in, "", ix)
		if out != in {
			t.Fatalf("rewriteWikilinks(%q) = %q, want untouched %q", in, out, in)
		}
		if len(links) != 1 || links[0].Resolved {
			t.Fatalf("rewriteWikilinks(%q) links = %+v, want 1 unresolved link", in, links)
		}
	})

	t.Run("renamed_reserved_target_resolves_to_output_name", func(t *testing.T) {
		in := "[[guides/index]]"
		want := "[guides/index](/guides/index-note.md)"
		out, _ := rewriteWikilinks(in, "", ix)
		if out != want {
			t.Fatalf("rewriteWikilinks(%q) = %q, want %q", in, out, want)
		}
	})

	t.Run("wikilink_inside_code_left_untouched", func(t *testing.T) {
		body := "`[[tables/orders]]`"
		out, links := rewriteWikilinks(body, "", ix)
		if out != body {
			t.Fatalf("rewriteWikilinks(%q) = %q, want untouched %q", body, out, body)
		}
		if len(links) != 0 {
			t.Fatalf("rewriteWikilinks(%q) links = %+v, want empty", body, links)
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
