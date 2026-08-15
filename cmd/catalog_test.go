package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// TestConvertCatalogFlags proves `binder convert` exposes the three #9 flags and
// that they append a root-only "# Catalog"; non-root indexes stay catalog-free.
func TestConvertCatalogFlags(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out := t.TempDir()
	_, code := runCLI(t, "convert", "../testdata/corpus-rich", "-o", out,
		"--group-by-type", "--include-backlinks", "--include-graph")
	if code != clijson.ExitSuccess {
		t.Fatalf("convert exit = %d, want 0", code)
	}
	root, err := os.ReadFile(filepath.Join(out, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(root), "# Catalog") {
		t.Error("root index.md missing # Catalog")
	}
	nonRoot, err := os.ReadFile(filepath.Join(out, "guides", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(nonRoot), "# Catalog") {
		t.Error("non-root index.md unexpectedly contains a catalog")
	}
}

// TestIndexCatalogFlags proves `binder index` exposes the flags too and can add
// the catalog to an already-converted bundle (the second command that generates
// indexes), leaving validate-relevant structure intact.
func TestIndexCatalogFlags(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out := t.TempDir()
	if _, code := runCLI(t, "convert", "../testdata/corpus-rich", "-o", out); code != clijson.ExitSuccess {
		t.Fatalf("convert exit = %d, want 0", code)
	}
	// Baseline: no catalog before `index --group-by-type`.
	before, _ := os.ReadFile(filepath.Join(out, "index.md"))
	if strings.Contains(string(before), "# Catalog") {
		t.Fatal("catalog present before index --group-by-type")
	}
	if _, code := runCLI(t, "index", out, "--group-by-type"); code != clijson.ExitSuccess {
		t.Fatalf("index exit = %d, want 0", code)
	}
	after, _ := os.ReadFile(filepath.Join(out, "index.md"))
	if !strings.Contains(string(after), "# Catalog") {
		t.Error("index --group-by-type did not add a catalog to the root index")
	}
}
