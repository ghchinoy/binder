package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// writeTree writes rel→content files under a fresh temp dir and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestValidateStrict: a trust advisory gates only under --strict; a hard
// non-conformance gates regardless.
func TestValidateStrict(t *testing.T) {
	// Conformant (has type) but carries a trust advisory (bad status value).
	advisory := writeTree(t, map[string]string{
		"a.md": "---\ntype: Note\nstatus: bogus\n---\n# A\n",
	})
	if _, code := runCLI(t, "validate", advisory); code != clijson.ExitSuccess {
		t.Errorf("advisory without --strict: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "validate", advisory, "--strict"); code != clijson.ExitFindings {
		t.Errorf("advisory with --strict: exit = %d, want 1", code)
	}

	// Hard non-conformance (missing type) gates with OR without --strict.
	hard := writeTree(t, map[string]string{
		"a.md": "---\ntitle: no type here\n---\n# A\n",
	})
	if _, code := runCLI(t, "validate", hard); code != clijson.ExitFindings {
		t.Errorf("hard non-conformance without --strict: exit = %d, want 1", code)
	}
	if _, code := runCLI(t, "validate", hard, "--strict"); code != clijson.ExitFindings {
		t.Errorf("hard non-conformance with --strict: exit = %d, want 1", code)
	}
}

// TestReviewStrict: a review finding (orphan) gates only under --strict.
func TestReviewStrict(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	bundle := writeTree(t, map[string]string{
		"a.md": "---\ntype: Note\n---\n# A\n",
	})
	if _, code := runCLI(t, "review", bundle); code != clijson.ExitSuccess {
		t.Errorf("finding without --strict: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "review", bundle, "--strict"); code != clijson.ExitFindings {
		t.Errorf("finding with --strict: exit = %d, want 1", code)
	}
}

// TestConvertStrict: unresolved links gate only under --strict.
func TestConvertStrict(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	corpus := writeTree(t, map[string]string{
		"a.md": "# A\n\n[missing](./nope.md)\n",
	})
	if _, code := runCLI(t, "convert", corpus, "--dry-run"); code != clijson.ExitSuccess {
		t.Errorf("unresolved without --strict: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "convert", corpus, "--dry-run", "--strict"); code != clijson.ExitFindings {
		t.Errorf("unresolved with --strict: exit = %d, want 1", code)
	}

	// A clean corpus does not gate even under --strict.
	if _, code := runCLI(t, "convert", "../testdata/corpus-clean", "--dry-run", "--strict"); code != clijson.ExitSuccess {
		t.Errorf("clean corpus with --strict: exit = %d, want 0", code)
	}
}
