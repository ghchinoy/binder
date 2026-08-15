package convert

import (
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

func TestHasFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"plain markdown", "# Title\n\nbody\n", false},
		{"hr not frontmatter", "---\n\njust text, no closing\n", false},
		{"real frontmatter", "---\ntype: Note\n---\n\nbody\n", true},
		{"crlf frontmatter", "---\r\ntype: Note\r\n---\r\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasFrontmatter([]byte(tc.in)); got != tc.want {
				t.Fatalf("hasFrontmatter(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestEnsureTypePrecedence(t *testing.T) {
	typeMap := map[string]string{"guides": "Guide", "guides/deep": "DeepGuide"}

	t.Run("existing wins", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		fm.Set("type", "BigQuery Table")
		got := ensureType(fm, "guides/x.md", typeMap, "Note")
		if got != "BigQuery Table" {
			t.Fatalf("existing type must win, got %q", got)
		}
	})
	t.Run("type-map when no existing", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		got := ensureType(fm, "guides/x.md", typeMap, "Note")
		if got != "Guide" {
			t.Fatalf("type-map should apply, got %q", got)
		}
	})
	t.Run("most specific dir wins", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		got := ensureType(fm, "guides/deep/x.md", typeMap, "Note")
		if got != "DeepGuide" {
			t.Fatalf("most specific dir should win, got %q", got)
		}
	})
	t.Run("default when nothing matches", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		got := ensureType(fm, "root.md", typeMap, "Note")
		if got != "Note" {
			t.Fatalf("default should apply, got %q", got)
		}
	})
	t.Run("empty existing type falls through", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		fm.Set("type", "  ")
		got := ensureType(fm, "guides/x.md", typeMap, "Note")
		if got != "Guide" {
			t.Fatalf("blank type must fall through to map, got %q", got)
		}
	})
}

func TestEnsureTitle(t *testing.T) {
	t.Run("existing wins", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		fm.Set("title", "Kept")
		ensureTitle(fm, "a/b.md", "# Heading\n")
		if v, _ := fm.Get("title"); v != "Kept" {
			t.Fatalf("existing title must win, got %v", v)
		}
	})
	t.Run("first H1", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		ensureTitle(fm, "a/b.md", "intro\n\n# The Heading\n\nmore\n")
		if v, _ := fm.Get("title"); v != "The Heading" {
			t.Fatalf("want H1 title, got %v", v)
		}
	})
	t.Run("humanized filename", func(t *testing.T) {
		fm := okf.NewOrderedMap()
		ensureTitle(fm, "a/getting-started_now.md", "no heading here\n")
		if v, _ := fm.Get("title"); v != "Getting Started Now" {
			t.Fatalf("want humanized filename, got %v", v)
		}
	})
}

func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"getting-started.md": "Getting Started",
		"my_note.md":         "My Note",
		"README.md":          "README",
	}
	for in, want := range cases {
		if got := humanize(in); got != want {
			t.Fatalf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTypeMap(t *testing.T) {
	got, err := ParseTypeMap("docs=Guide, adr=Decision")
	if err != nil {
		t.Fatal(err)
	}
	if got["docs"] != "Guide" || got["adr"] != "Decision" {
		t.Fatalf("unexpected parse: %v", got)
	}
	if _, err := ParseTypeMap("bad-entry"); err == nil {
		t.Fatal("expected error for malformed entry")
	}
	if m, err := ParseTypeMap(""); err != nil || m != nil {
		t.Fatalf("empty should be (nil,nil), got %v %v", m, err)
	}
}
