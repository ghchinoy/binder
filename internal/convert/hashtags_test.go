package convert

import (
	"reflect"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

func TestExtractHashtags(t *testing.T) {
	body := "# Heading is not a tag\n\nThis has #finance and #margin/quarterly tags.\n" +
		"Duplicate #finance ignored. Inside code `#nope` and:\n\n```\n#alsonope\n```\n"
	got := extractHashtags(body)
	want := []string{"finance", "margin/quarterly"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractHashtags = %v, want %v", got, want)
	}
}

func TestMergeTagsAppendsNewPreservingExisting(t *testing.T) {
	fm := okf.NewOrderedMap()
	fm.Set("tags", []any{"sales", "orders"})
	mergeTags(fm, []string{"orders", "finance"}) // "orders" dup, "finance" new
	v, _ := fm.Get("tags")
	got := v.([]any)
	want := []any{"sales", "orders", "finance"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged tags = %v, want %v", got, want)
	}
}

func TestMergeTagsNoNewLeavesUntouched(t *testing.T) {
	fm := okf.NewOrderedMap()
	orig := []any{"sales", "orders"}
	fm.Set("tags", orig)
	mergeTags(fm, []string{"orders"}) // nothing new
	v, _ := fm.Get("tags")
	if !reflect.DeepEqual(v.([]any), orig) {
		t.Fatalf("tags should be untouched when no new tags, got %v", v)
	}
}

func TestMergeTagsFromScalar(t *testing.T) {
	fm := okf.NewOrderedMap()
	fm.Set("tags", "finance")
	mergeTags(fm, []string{"margin"})
	v, _ := fm.Get("tags")
	want := []any{"finance", "margin"}
	if !reflect.DeepEqual(v.([]any), want) {
		t.Fatalf("scalar merge = %v, want %v", v, want)
	}
}
