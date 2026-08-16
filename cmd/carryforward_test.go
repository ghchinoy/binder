package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// runCLISplit runs the root command with separate stdout/stderr buffers so a
// test can assert which stream a message lands on (used to prove the catalog-flag
// hint goes to stderr and never corrupts a --json payload on stdout).
func runCLISplit(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), clijson.ExitCode(err)
}

// TestCatalogHintToStderr proves (#9 polish) that --include-backlinks /
// --include-graph without --group-by-type emit a hint to STDERR only: stdout
// stays valid JSON with no hint, and the exit code is unchanged (0).
func TestCatalogHintToStderr(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	stdout, stderr, code := runCLISplit(t, "convert", "../testdata/corpus-basic",
		"--dry-run", "--json", "--include-backlinks", "--include-graph")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0 (hint must not gate); stderr:\n%s", code, stderr)
	}
	// stdout must be the untouched JSON envelope — no hint text leaked onto it.
	if strings.Contains(stdout, "hint:") {
		t.Errorf("hint leaked onto stdout:\n%s", stdout)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not valid JSON (hint corrupted it?): %v\n%s", err, stdout)
	}
	// stderr must carry a hint for each flag passed without --group-by-type.
	if !strings.Contains(stderr, "hint: --include-backlinks has no effect without --group-by-type") {
		t.Errorf("missing backlinks hint on stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "hint: --include-graph has no effect without --group-by-type") {
		t.Errorf("missing graph hint on stderr:\n%s", stderr)
	}
}

// TestCatalogHintSuppressedWithGroupByType proves the hint does NOT appear when
// --group-by-type is present (the flags are no longer a no-op).
func TestCatalogHintSuppressedWithGroupByType(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	_, stderr, code := runCLISplit(t, "convert", "../testdata/corpus-basic",
		"-o", t.TempDir(), "--group-by-type", "--include-backlinks", "--include-graph")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	if strings.Contains(stderr, "hint:") {
		t.Errorf("hint emitted despite --group-by-type:\n%s", stderr)
	}
}

// TestIndexCatalogHintToStderr proves the same hint fires on `binder index`,
// which also carries the flags.
func TestIndexCatalogHintToStderr(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out := t.TempDir()
	if _, code := runCLI(t, "convert", "../testdata/corpus-basic", "-o", out); code != clijson.ExitSuccess {
		t.Fatalf("convert setup exit = %d, want 0", code)
	}
	_, stderr, code := runCLISplit(t, "index", out, "--include-graph")
	if code != clijson.ExitSuccess {
		t.Fatalf("index exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "hint: --include-graph has no effect without --group-by-type") {
		t.Errorf("index did not emit the catalog-flag hint to stderr:\n%s", stderr)
	}
}

// TestTodayValidationReview proves (#8/#4 polish) that a valid --today is
// accepted unchanged (exit 0) while a malformed --today is a usage error (exit
// 2) with a message naming the expected format — for `binder review`.
func TestTodayValidationReview(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	bundle := t.TempDir()
	if _, code := runCLI(t, "convert", "../testdata/corpus-basic", "-o", bundle); code != clijson.ExitSuccess {
		t.Fatalf("convert setup exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "review", bundle, "--today", "2024-01-01"); code != clijson.ExitSuccess {
		t.Fatalf("valid --today: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "review", bundle, "--today", "not-a-date"); code != clijson.ExitUsage {
		t.Fatalf("malformed --today: exit = %d, want %d", code, clijson.ExitUsage)
	}
	err := runCLIErr(t, "review", bundle, "--today", "not-a-date")
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("malformed --today error = %v, want it to name YYYY-MM-DD", err)
	}
}

// TestTodayValidationLint proves the same --today validation for `binder lint`.
func TestTodayValidationLint(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	if _, code := runCLI(t, "lint", "../testdata/corpus-basic", "--today", "2024-01-01"); code != clijson.ExitSuccess {
		t.Fatalf("valid --today: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "lint", "../testdata/corpus-basic", "--today", "not-a-date"); code != clijson.ExitUsage {
		t.Fatalf("malformed --today: exit = %d, want %d", code, clijson.ExitUsage)
	}
	err := runCLIErr(t, "lint", "../testdata/corpus-basic", "--today", "not-a-date")
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("malformed --today error = %v, want it to name YYYY-MM-DD", err)
	}
}

// TestTodayValidationGraph proves the same --today validation for `binder graph`
// (previously the outlier that silently accepted a malformed date and emitted
// wrong staleness at exit 0). The message must be indistinguishable from
// lint/review so the three commands validate identically.
func TestTodayValidationGraph(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	if _, code := runCLI(t, "graph", "../testdata/expected-rich", "--today", "2024-01-01"); code != clijson.ExitSuccess {
		t.Fatalf("valid --today: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "graph", "../testdata/expected-rich", "--today", "not-a-date"); code != clijson.ExitUsage {
		t.Fatalf("malformed --today: exit = %d, want %d", code, clijson.ExitUsage)
	}
	err := runCLIErr(t, "graph", "../testdata/expected-rich", "--today", "not-a-date")
	want := `--today "not-a-date" is not a valid date (expected YYYY-MM-DD)`
	if err == nil || err.Error() != want {
		t.Fatalf("malformed --today error = %v, want exactly %q", err, want)
	}
}
