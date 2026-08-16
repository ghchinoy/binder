package linkcheck

import "testing"

// TestIsRootEntrypoint: only a corpus-root README.md/index.md (any case, no
// directory component) is auto-recognized as an entrypoint (issue #24).
func TestIsRootEntrypoint(t *testing.T) {
	cases := []struct {
		relPath string
		want    bool
	}{
		{"README.md", true},
		{"readme.md", true},
		{"index.md", true},
		{"INDEX.md", true},
		{"", false},
		{"docs/README.md", false}, // nested, not the corpus root
		{"docs/index.md", false},
		{"guide.md", false},
		{"README.markdown", false},
		{"readme", false},
	}
	for _, c := range cases {
		if got := IsRootEntrypoint(c.relPath); got != c.want {
			t.Errorf("IsRootEntrypoint(%q) = %v, want %v", c.relPath, got, c.want)
		}
	}
}

// TestEntrypointSet: user designations are normalized to concept ids — a trailing
// ".md" (any case) and surrounding space are stripped; empty entries are dropped.
func TestEntrypointSet(t *testing.T) {
	set := EntrypointSet([]string{" docs/intro.md ", "hub", "root.MD", "", "   "})
	want := []string{"docs/intro", "hub", "root"}
	for _, id := range want {
		if !set[id] {
			t.Errorf("EntrypointSet missing normalized id %q; got %v", id, set)
		}
	}
	if len(set) != len(want) {
		t.Errorf("EntrypointSet size = %d, want %d (%v)", len(set), len(want), set)
	}
}
