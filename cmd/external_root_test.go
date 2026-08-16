package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// writeExternalRootCLICorpus lays out a corpus that links (via file://) to a doc
// under a sibling directory outside the corpus root. It returns the corpus root
// and the absolute sibling directory to declare with --external-root.
func writeExternalRootCLICorpus(t *testing.T) (root, sibling string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "corpus")
	sibling = filepath.Join(base, "sibling")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + filepath.ToSlash(filepath.Join(sibling, "a.md"))
	if err := os.WriteFile(filepath.Join(root, "a.md"),
		[]byte("# A\n\nCross-repo [x]("+uri+") link.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, sibling
}

// TestConvertExternalRootCLISuppresses: the advisory shows without the flag and
// is suppressed with it (criterion 3, text output path).
func TestConvertExternalRootCLISuppresses(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	root, sibling := writeExternalRootCLICorpus(t)

	out, code := runCLI(t, "convert", root, "--dry-run")
	if code != clijson.ExitSuccess {
		t.Fatalf("without flag: exit = %d, want 0; out:\n%s", code, out)
	}
	if !strings.Contains(out, "resolves outside the workspace root") {
		t.Fatalf("without flag the advisory should appear:\n%s", out)
	}

	out, code = runCLI(t, "convert", root, "--dry-run", "--external-root", sibling)
	if code != clijson.ExitSuccess {
		t.Fatalf("with flag: exit = %d, want 0; out:\n%s", code, out)
	}
	if strings.Contains(out, "resolves outside the workspace root") {
		t.Fatalf("with --external-root the advisory must be suppressed:\n%s", out)
	}
}

// TestConvertExternalRootRepeatable: the flag is repeatable; multiple declared
// roots each suppress their own links.
func TestConvertExternalRootRepeatable(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	sibA := filepath.Join(base, "sibA")
	sibB := filepath.Join(base, "sibB")
	for _, d := range []string{root, sibA, sibB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	uriA := "file://" + filepath.ToSlash(filepath.Join(sibA, "a.md"))
	uriB := "file://" + filepath.ToSlash(filepath.Join(sibB, "b.md"))
	if err := os.WriteFile(filepath.Join(root, "a.md"),
		[]byte("[a]("+uriA+") [b]("+uriB+")\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, "convert", root, "--dry-run",
		"--external-root", sibA, "--external-root", sibB)
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	if strings.Contains(out, "resolves outside the workspace root") {
		t.Fatalf("both declared roots should suppress their advisories:\n%s", out)
	}
}

// TestConvertExternalRootEmptyIsUsageError: criterion 7 — an empty flag value is
// a usage error (exit 2). A well-formed nonexistent path is accepted (covered by
// convert-package tests).
func TestConvertExternalRootEmptyIsUsageError(t *testing.T) {
	root, _ := writeExternalRootCLICorpus(t)
	_, code := runCLI(t, "convert", root, "--dry-run", "--external-root", "")
	if code != clijson.ExitUsage {
		t.Fatalf("empty --external-root: exit = %d, want %d (usage)", code, clijson.ExitUsage)
	}
}

// TestConvertExternalRootStrictStory: criterion 8 — declared roots do not gate
// under --strict; a genuinely unresolved internal link still gates exactly as
// before (external links never gated, flag or not).
func TestConvertExternalRootStrictStory(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	root, sibling := writeExternalRootCLICorpus(t)

	// External link + --strict, no flag: does not gate today (external links are
	// not unresolved edges).
	if _, code := runCLI(t, "convert", root, "--dry-run", "--strict"); code != clijson.ExitSuccess {
		t.Errorf("external link under --strict without flag: exit = %d, want 0", code)
	}
	// Declared root + --strict: still does not gate.
	if _, code := runCLI(t, "convert", root, "--dry-run", "--strict",
		"--external-root", sibling); code != clijson.ExitSuccess {
		t.Errorf("declared root under --strict: exit = %d, want 0", code)
	}

	// Unchanged gating: a genuinely unresolved INTERNAL link still gates under
	// --strict whether or not --external-root is present.
	base := t.TempDir()
	corpus := filepath.Join(base, "c")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corpus, "a.md"),
		[]byte("# A\n\n[missing](./nope.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := runCLI(t, "convert", corpus, "--dry-run", "--strict"); code != clijson.ExitFindings {
		t.Errorf("unresolved internal link under --strict: exit = %d, want 1", code)
	}
	if _, code := runCLI(t, "convert", corpus, "--dry-run", "--strict",
		"--external-root", sibling); code != clijson.ExitFindings {
		t.Errorf("--external-root must not change gating for unknown internal links: exit = %d, want 1", code)
	}
}
