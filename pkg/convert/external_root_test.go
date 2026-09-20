package convert

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// newFileResolverWithExternal builds a linkResolver rooted at root with a set of
// declared external roots (issue #25) and a warning collector.
func newFileResolverWithExternal(root string, srcToOut map[string]string, external ...string) (*linkResolver, *[]string) {
	var warnings []string
	cleaned := make([]string, 0, len(external))
	for _, e := range external {
		cleaned = append(cleaned, filepath.Clean(e))
	}
	r := &linkResolver{
		srcToOut:      srcToOut,
		srcRoot:       filepath.Clean(root),
		wsRoot:        filepath.Clean(root),
		externalRoots: cleaned,
		warn:          func(f string, a ...any) { warnings = append(warnings, f) },
	}
	return r, &warnings
}

// TestExternalRootSuppressesAdvisory: a file:// link outside the workspace but
// under a declared external root stays external (byte-identical) yet no longer
// warns.
func TestExternalRootSuppressesAdvisory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	sibling := filepath.Join(base, "sibling")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	r, warns := newFileResolverWithExternal(root, map[string]string{"a.md": "a.md"}, sibling)

	target := fileURL(filepath.Join(sibling, "docs", "a.md"))
	body := "[a](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")

	if out != body {
		t.Fatalf("declared-external link must be left untouched (external), got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("declared-external link must NOT become an edge: %+v", links)
	}
	if len(*warns) != 0 {
		t.Fatalf("declared external root should suppress the advisory, got %v", *warns)
	}
}

// TestExternalRootDoesNotInternalize: even when the target under the declared
// root would match a known concept id, it must NOT be internalized/rewritten —
// --external-root only suppresses the advisory.
func TestExternalRootDoesNotInternalize(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	sibling := filepath.Join(base, "sibling")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// srcToOut deliberately contains an entry that the sibling path would map to
	// were the boundary ignored; it must be irrelevant.
	r, warns := newFileResolverWithExternal(root, map[string]string{"docs/a.md": "docs/a.md"}, sibling)

	target := fileURL(filepath.Join(sibling, "docs", "a.md"))
	body := "[a](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")

	if out != body {
		t.Fatalf("declared-external link must stay external, not internalized: got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("declared-external link must not be recorded as an edge: %+v", links)
	}
	if len(*warns) != 0 {
		t.Fatalf("advisory should be suppressed, got %v", *warns)
	}
}

// TestExternalRootNonMatchingStillWarns: an external link NOT under any declared
// root still emits its advisory (guardrail 2).
func TestExternalRootNonMatchingStillWarns(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	declared := filepath.Join(base, "declared")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	r, warns := newFileResolverWithExternal(root, map[string]string{"a.md": "a.md"}, declared)

	// Under a DIFFERENT sibling, not the declared one.
	target := fileURL(filepath.Join(base, "elsewhere", "a.md"))
	body := "[a](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")

	if out != body {
		t.Fatalf("non-declared external link must be left untouched, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("non-declared external link must not be an edge: %+v", links)
	}
	if len(*warns) != 1 {
		t.Fatalf("non-declared external link should still warn once, got %v", *warns)
	}
}

// TestExternalRootSegmentSafe: "/projects/jib" must NOT suppress a link under
// "/projects/jibo/..." (guardrail: match at a path-segment boundary).
func TestExternalRootSegmentSafe(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	projects := filepath.Join(base, "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Declare the sibling "jib" (a prefix of "jibo" at the string level).
	declared := filepath.Join(projects, "jib")

	r, warns := newFileResolverWithExternal(root, map[string]string{"a.md": "a.md"}, declared)

	// Link lives under "jibo", which shares the "jib" string prefix but is a
	// different directory. It must still warn.
	target := fileURL(filepath.Join(projects, "jibo", "docs", "a.md"))
	body := "[a](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")

	if out != body {
		t.Fatalf("segment-mismatched link must be left untouched, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("segment-mismatched link must not be an edge: %+v", links)
	}
	if len(*warns) != 1 {
		t.Fatalf("prefix-but-not-segment match must still warn once, got %v", *warns)
	}

	// Sanity: the exact declared segment IS suppressed.
	r2, warns2 := newFileResolverWithExternal(root, map[string]string{"a.md": "a.md"}, declared)
	target2 := fileURL(filepath.Join(projects, "jib", "docs", "a.md"))
	if _, _ = r2.rewrite("[a]("+target2+")\n", "intro.md"); len(*warns2) != 0 {
		t.Fatalf("link under the declared segment should be suppressed, got %v", *warns2)
	}
}

// TestExternalRootOrderingDeterministic: the set of declared roots is an
// any-match; the ordering must not change whether a link is suppressed.
func TestExternalRootOrderingDeterministic(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	a := filepath.Join(base, "a")
	b := filepath.Join(base, "b")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	target := fileURL(filepath.Join(b, "docs", "x.md"))
	body := "[x](" + target + ")\n"

	r1, w1 := newFileResolverWithExternal(root, map[string]string{}, a, b)
	out1, _ := r1.rewrite(body, "intro.md")
	r2, w2 := newFileResolverWithExternal(root, map[string]string{}, b, a)
	out2, _ := r2.rewrite(body, "intro.md")

	if out1 != out2 || out1 != body {
		t.Fatalf("output must be identical and unrewritten regardless of root order: %q vs %q", out1, out2)
	}
	if len(*w1) != 0 || len(*w2) != 0 {
		t.Fatalf("both orderings should suppress: %v / %v", *w1, *w2)
	}
}

// TestExternalRootSymlinkCoherent: a symlink inside the workspace whose real
// target lives under a declared external root has its symlink advisory
// suppressed too, while the link stays external.
func TestExternalRootSymlinkCoherent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	sibling := filepath.Join(base, "sibling")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(sibling, "secret.md")
	if err := os.WriteFile(targetFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "link.md")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	// EvalSymlinks canonicalizes tmp paths (e.g. /var -> /private/var on macOS);
	// declare the resolved sibling so the real-path match is exact.
	realSibling := sibling
	if rs, err := filepath.EvalSymlinks(sibling); err == nil {
		realSibling = rs
	}

	r, warns := newFileResolverWithExternal(root, map[string]string{"link.md": "link.md"}, realSibling)
	target := fileURL(linkPath)
	body := "[x](" + target + ")\n"
	out, links := r.rewrite(body, "intro.md")

	if out != body {
		t.Fatalf("symlink-to-declared-root must stay external, got %q", out)
	}
	if len(links) != 0 {
		t.Fatalf("symlink-to-declared-root must not be an edge: %+v", links)
	}
	if len(*warns) != 0 {
		t.Fatalf("declared root should suppress the symlink advisory, got %v", *warns)
	}
}

// TestExternalRootEmptyPreservesBehavior: with no declared roots the advisory
// fires exactly as before (regression against guardrail 2 / criterion 2).
func TestExternalRootEmptyPreservesBehavior(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	r, warns := newFileResolverWithExternal(root, map[string]string{"a.md": "a.md"})

	target := fileURL(filepath.Join(base, "other", "a.md"))
	out, _ := r.rewrite("[a]("+target+")\n", "intro.md")
	if out != "[a]("+target+")\n" {
		t.Fatalf("no-flag behavior changed: %q", out)
	}
	if len(*warns) != 1 {
		t.Fatalf("without declared roots the advisory must still fire once, got %v", *warns)
	}
}
