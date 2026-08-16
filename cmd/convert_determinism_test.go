package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// stripAtLines drops every line carrying a timestamp ("at:"). With no verifier the
// only such line is the generated provenance stamp, so what remains is the
// clock-invariant part of the output.
func stripAtLines(s string) string {
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.Contains(ln, "at:") {
			continue
		}
		keep = append(keep, ln)
	}
	return strings.Join(keep, "\n")
}

// TestConvertDeterminism_SingleTimeVaryingField pins the corrected determinism
// claim: convert output is byte-identical for identical inputs UNDER A PINNED CLOCK
// (SOURCE_DATE_EPOCH), and the ONLY field that varies with the clock is the
// generated provenance timestamp (generated.at). The former blanket "is
// deterministic" claim was false because generated.at records wall-clock time.
//
// DEMONSTRATED RED (observed on this head): asserting the old blanket claim — that
// two runs under DIFFERENT clocks produce byte-identical output — FAILS; the outputs
// differ, and differ ONLY at generated.at.
func TestConvertDeterminism_SingleTimeVaryingField(t *testing.T) {
	convertOnce := func(t *testing.T, epoch, src, out string) string {
		t.Helper()
		t.Setenv("SOURCE_DATE_EPOCH", epoch)
		if _, code := runCLI(t, "convert", src, "-o", out); code != clijson.ExitSuccess {
			t.Fatalf("convert exit = %d, want 0", code)
		}
		b, err := os.ReadFile(filepath.Join(out, "a.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	src := mkCorpus(t)

	// Same clock → byte-identical (the true guarantee).
	a := convertOnce(t, "1700000000", src, t.TempDir())
	b := convertOnce(t, "1700000000", src, t.TempDir())
	if a != b {
		t.Errorf("same SOURCE_DATE_EPOCH produced non-identical output:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}

	// Different clock → outputs DIFFER (the blanket determinism claim is false) ...
	c := convertOnce(t, "1700000002", src, t.TempDir())
	if a == c {
		t.Errorf("different clocks produced byte-identical output; expected generated.at to differ")
	}
	// ... and the ONLY difference is the generated provenance timestamp.
	if stripAtLines(a) != stripAtLines(c) {
		t.Errorf("clock-varying output differs beyond generated.at:\n--- pinned ---\n%s\n--- advanced ---\n%s", a, c)
	}
}
