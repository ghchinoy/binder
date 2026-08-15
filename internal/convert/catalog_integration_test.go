package convert_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
	"github.com/ghchinoy/binder/internal/validate"
)

// catalogOptions enables the full #9 catalog surface on top of the rich signals.
func catalogOptions() convert.Options {
	return convert.Options{
		Codec:            native.New(),
		Version:          "0.1.0",
		Now:              fixedNow,
		FMRefKeys:        []string{"related"},
		GroupByType:      true,
		IncludeBacklinks: true,
		IncludeGraph:     true,
	}
}

// TestCatalogValidatePasses converts corpus-rich with the catalog flags on and
// asserts the resulting bundle still validates: the additive catalog must not
// break spec §8/§12 conformance (root index stays the sole okf_version carrier).
func TestCatalogValidatePasses(t *testing.T) {
	out := t.TempDir()
	if _, err := convert.Convert(corpusRich, out, catalogOptions()); err != nil {
		t.Fatalf("convert: %v", err)
	}

	// The catalog only lands in the root index, and only when grouped.
	root, err := os.ReadFile(filepath.Join(out, "index.md"))
	if err != nil {
		t.Fatalf("read root index: %v", err)
	}
	if !bytes.Contains(root, []byte("# Catalog")) {
		t.Fatal("root index.md missing # Catalog with --group-by-type")
	}

	res, err := validate.Bundle(out, native.New(), okf.SpecV02)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !res.Conformant() {
		t.Errorf("catalog bundle is not conformant; errors: %v", res.Errors())
	}
}

// TestCatalogConvertIdempotent proves re-running convert with the catalog flags
// yields a byte-identical root index (deterministic + idempotent end to end).
func TestCatalogConvertIdempotent(t *testing.T) {
	out1, out2 := t.TempDir(), t.TempDir()
	if _, err := convert.Convert(corpusRich, out1, catalogOptions()); err != nil {
		t.Fatalf("convert run 1: %v", err)
	}
	if _, err := convert.Convert(corpusRich, out2, catalogOptions()); err != nil {
		t.Fatalf("convert run 2: %v", err)
	}
	a, err := os.ReadFile(filepath.Join(out1, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out2, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("root index not idempotent:\n--- run1 ---\n%s\n--- run2 ---\n%s", a, b)
	}
}

// TestCatalogDefaultOutputUnchanged proves that without the flags the generated
// index.md is byte-identical to a plain rich conversion (no catalog leakage).
func TestCatalogDefaultOutputUnchanged(t *testing.T) {
	withFlags, plain := t.TempDir(), t.TempDir()

	flagged := catalogOptions()
	flagged.GroupByType = false
	flagged.IncludeBacklinks = false
	flagged.IncludeGraph = false
	if _, err := convert.Convert(corpusRich, withFlags, flagged); err != nil {
		t.Fatalf("convert flags-off: %v", err)
	}

	base := catalogOptions()
	base.GroupByType, base.IncludeBacklinks, base.IncludeGraph = false, false, false
	if _, err := convert.Convert(corpusRich, plain, base); err != nil {
		t.Fatalf("convert plain: %v", err)
	}

	a, _ := os.ReadFile(filepath.Join(withFlags, "index.md"))
	b, _ := os.ReadFile(filepath.Join(plain, "index.md"))
	if !bytes.Equal(a, b) {
		t.Errorf("flags-off output differs from plain:\n%s\n---\n%s", a, b)
	}
	if bytes.Contains(a, []byte("# Catalog")) {
		t.Error("flags-off output unexpectedly contains a catalog")
	}
}
