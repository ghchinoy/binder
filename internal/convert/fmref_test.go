package convert

import (
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

func TestFrontmatterRefEdges(t *testing.T) {
	ix := testIndex()
	fm := okf.NewOrderedMap()
	fm.Set("related", []any{"tables/orders", "Setup Guide"})
	fm.Set("parent", "[[Introduction]]")

	links := frontmatterRefEdges(fm, "guides/setup.md", []string{"related", "parent"}, ix)
	if len(links) != 3 {
		t.Fatalf("want 3 edges, got %d: %+v", len(links), links)
	}
	// original keys preserved (additive)
	if _, ok := fm.Get("related"); !ok {
		t.Fatal("related key must be preserved")
	}
	if _, ok := fm.Get("parent"); !ok {
		t.Fatal("parent key must be preserved")
	}
	for _, l := range links {
		if !l.Resolved {
			t.Fatalf("edge should resolve: %+v", l)
		}
	}
}

func TestParseFMRefKeys(t *testing.T) {
	got := ParseFMRefKeys(" related, parent ,related, ")
	if len(got) != 2 || got[0] != "related" || got[1] != "parent" {
		t.Fatalf("ParseFMRefKeys = %v", got)
	}
}
