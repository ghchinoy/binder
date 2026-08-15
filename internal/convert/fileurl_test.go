package convert

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newFileResolver builds a linkResolver rooted at root (used as both the corpus
// source root and the workspace boundary) with a warning collector.
func newFileResolver(root string, srcToOut map[string]string) (*linkResolver, *[]string) {
	var warnings []string
	r := &linkResolver{
		srcToOut: srcToOut,
		srcRoot:  filepath.Clean(root),
		wsRoot:   filepath.Clean(root),
		warn:     func(f string, a ...any) { warnings = append(warnings, f) },
	}
	return r, &warnings
}

// fileURL builds a file:// URI (empty authority) for an absolute OS path.
func fileURL(absPath string) string {
	return "file://" + filepath.ToSlash(absPath)
}

func TestFileURLInCorpusResolves(t *testing.T) {
	root := t.TempDir()
	srcToOut := map[string]string{"docs/doc.md": "docs/doc.md"}
	r, warns := newFileResolver(root, srcToOut)

	target := fileURL(filepath.Join(root, "docs", "doc.md"))
	body := "See [doc](" + target + ") now.\n"
	out, links := r.rewrite(body, "intro.md")

	if want := "See [doc](/docs/doc.md) now.\n"; out != want {
		t.Fatalf("file:// not rewritten to internal edge:\ngot  %q\nwant %q", out, want)
	}
	if len(links) != 1 || !links[0].Resolved || links[0].TargetID != "docs/doc" {
		t.Fatalf("expected one resolved edge, got %+v", links)
	}
	if len(*warns) != 0 {
		t.Fatalf("in-corpus resolve should not warn, got %v", *warns)
	}
}

func TestFileURLFragmentPreserved(t *testing.T) {
	root := t.TempDir()
	r, _ := newFileResolver(root, map[string]string{"docs/doc.md": "docs/doc.md"})

	target := fileURL(filepath.Join(root, "docs", "doc.md")) + "#section"
	out, links := r.rewrite("[d]("+target+")\n", "intro.md")
	if want := "[d](/docs/doc.md#section)\n"; out != want {
		t.Fatalf("fragment not preserved:\ngot %q want %q", out, want)
	}
	if !links[0].Resolved {
		t.Fatalf("link should resolve: %+v", links[0])
	}
}

func TestFileURLPercentDecoding(t *testing.T) {
	root := t.TempDir()
	// The concept path contains a space; the URI encodes it as %20.
	srcToOut := map[string]string{"docs/my doc.md": "docs/my doc.md"}
	r, _ := newFileResolver(root, srcToOut)

	target := "file://" + filepath.ToSlash(filepath.Join(root, "docs", "my doc.md"))
	target = strings.ReplaceAll(target, " ", "%20")
	out, links := r.rewrite("[d]("+target+")\n", "intro.md")

	if want := "[d](/docs/my doc.md)\n"; out != want {
		t.Fatalf("percent-decoded path not resolved:\ngot %q want %q", out, want)
	}
	if len(links) != 1 || !links[0].Resolved {
		t.Fatalf("percent-encoded target should resolve: %+v", links)
	}
}

func TestFileURLLocalhostAuthority(t *testing.T) {
	root := t.TempDir()
	r, warns := newFileResolver(root, map[string]string{"a.md": "a.md"})

	target := "file://localhost" + filepath.ToSlash(filepath.Join(root, "a.md"))
	out, links := r.rewrite("[a]("+target+")\n", "intro.md")
	if want := "[a](/a.md)\n"; out != want {
		t.Fatalf("localhost authority should resolve:\ngot %q want %q", out, want)
	}
	if len(links) != 1 || !links[0].Resolved {
		t.Fatalf("localhost target should resolve: %+v", links)
	}
	if len(*warns) != 0 {
		t.Fatalf("localhost should not warn, got %v", *warns)
	}
}

func TestFileURLOtherHostStaysExternal(t *testing.T) {
	root := t.TempDir()
	r, warns := newFileResolver(root, map[string]string{"a.md": "a.md"})

	target := "file://otherhost" + filepath.ToSlash(filepath.Join(root, "a.md"))
	body := "[a](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")
	if out != body {
		t.Fatalf("remote-host file:// must be left untouched, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("remote-host file:// must not be an edge: %+v", links)
	}
	if len(*warns) != 1 {
		t.Fatalf("remote-host file:// should emit one advisory, got %v", *warns)
	}
}

func TestFileURLOutsideRootStaysExternal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "corpus")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	r, warns := newFileResolver(root, map[string]string{"a.md": "a.md"})

	// A sibling of the corpus root, outside the boundary.
	outside := fileURL(filepath.Join(filepath.Dir(root), "other", "a.md"))
	body := "[a](" + outside + ")\n"
	out, links := r.rewrite(body, "intro.md")
	if out != body {
		t.Fatalf("outside-root file:// must be untouched, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("outside-root file:// must not be an edge: %+v", links)
	}
	if len(*warns) != 1 {
		t.Fatalf("outside-root file:// should warn once, got %v", *warns)
	}
}

func TestFileURLTraversalCannotEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "corpus")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	r, warns := newFileResolver(root, map[string]string{"a.md": "a.md"})

	// Lexical .. escape: file:///<root>/../secret.md must not resolve.
	target := "file://" + filepath.ToSlash(root) + "/../secret.md"
	body := "[x](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")
	if out != body {
		t.Fatalf(".. escape must be left untouched, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf(".. escape must not be an edge: %+v", links)
	}
	if len(*warns) != 1 {
		t.Fatalf(".. escape should warn once, got %v", *warns)
	}
}

func TestFileURLSymlinkCannotEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real target outside the root, and a symlink inside the root pointing at it.
	targetFile := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(targetFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "link.md")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	// srcToOut *would* resolve link.md if the symlink guard failed.
	r, warns := newFileResolver(root, map[string]string{"link.md": "link.md"})
	target := fileURL(linkPath)
	body := "[x](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")
	if out != body {
		t.Fatalf("symlink escape must be left untouched, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("symlink escape must not be an edge: %+v", links)
	}
	if len(*warns) != 1 {
		t.Fatalf("symlink escape should warn once, got %v", *warns)
	}
}

func TestFileURLInRootUnknownIsUnresolvedEdge(t *testing.T) {
	root := t.TempDir()
	// Empty corpus map: the target is inside the root but is not a known concept.
	r, warns := newFileResolver(root, map[string]string{})

	target := fileURL(filepath.Join(root, "docs", "ghost.md"))
	body := "[g](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")
	if out != body {
		t.Fatalf("unresolved in-root file:// must be left untouched, got %q", out)
	}
	if len(links) != 1 || links[0].Resolved {
		t.Fatalf("in-root unknown target should be a recorded, unresolved edge: %+v", links)
	}
	if len(*warns) != 0 {
		t.Fatalf("in-root unknown target is a tolerated broken link, not a warning: %v", *warns)
	}
}

func TestFileURLNonMarkdownStaysExternal(t *testing.T) {
	root := t.TempDir()
	r, _ := newFileResolver(root, map[string]string{})

	target := fileURL(filepath.Join(root, "image.png"))
	body := "[img](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")
	if out != body {
		t.Fatalf("non-.md file:// must be left untouched, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("non-.md file:// must not be an edge: %+v", links)
	}
}

func TestFileURLDisabledWithoutRoot(t *testing.T) {
	// The historical helper leaves srcRoot empty: file:// stays external.
	body := "[d](file:///abs/docs/doc.md)\n"
	out, links := rewriteLinks(body, "intro.md", map[string]string{"docs/doc.md": "docs/doc.md"})
	if out != body {
		t.Fatalf("file:// resolution must be disabled without a root, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("no edges expected when resolution is disabled: %+v", links)
	}
}
