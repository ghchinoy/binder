package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// runCLI executes the binder root command with args, capturing stdout, and
// returns the captured output and the exit code the contract maps the error to.
// SOURCE_DATE_EPOCH is set by callers that need deterministic timestamps.
func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), clijson.ExitCode(err)
}

// TestConvertJSONGolden exercises convert --json end to end and asserts the
// envelope shape, provenance, and a stable set of report fields. It parses via
// encoding/json so the assertion is on the contract, not on formatting.
func TestConvertJSONGolden(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out, code := runCLI(t, "convert", "../testdata/corpus-basic", "--dry-run", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want %d; output:\n%s", code, clijson.ExitSuccess, out)
	}

	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if env.Binder != "binder/"+Version {
		t.Errorf("binder = %q, want %q", env.Binder, "binder/"+Version)
	}
	if env.Command != "convert" {
		t.Errorf("command = %q, want convert", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}

	result, ok := env.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %T", env.Result)
	}
	// Counts/booleans and slices are always present (stable-schema policy).
	for _, key := range []string{
		"src", "out", "concepts", "warnings", "unresolved",
		"num_concepts", "num_links", "num_resolved", "num_unresolved",
		"num_recovered", "dry_run",
	} {
		if _, present := result[key]; !present {
			t.Errorf("result missing key %q", key)
		}
	}
	if got := result["num_concepts"]; got != float64(4) {
		t.Errorf("num_concepts = %v, want 4", got)
	}
	if got := result["dry_run"]; got != true {
		t.Errorf("dry_run = %v, want true", got)
	}
}

// TestConvertJSONDeterministic asserts two runs with a fixed SOURCE_DATE_EPOCH
// are byte-identical.
func TestConvertJSONDeterministic(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	a, ca := runCLI(t, "convert", "../testdata/corpus-basic", "--dry-run", "--json")
	b, cb := runCLI(t, "convert", "../testdata/corpus-basic", "--dry-run", "--json")
	if ca != 0 || cb != 0 {
		t.Fatalf("exit codes = %d, %d; want 0", ca, cb)
	}
	if a != b {
		t.Errorf("JSON not byte-identical across runs:\n%s\n---\n%s", a, b)
	}
}

// TestConvertEmptySlicesAreArrays asserts empty report slices serialize as []
// (the documented empty-slice policy), not null.
func TestConvertEmptySlicesAreArrays(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	// corpus-clean has no unresolved links or warnings.
	out, code := runCLI(t, "convert", "../testdata/corpus-clean", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("exit = %d; output:\n%s", code, out)
	}
	if strings.Contains(out, ": null") {
		t.Errorf("report contains a null slice; empty slices must be []:\n%s", out)
	}
	if !strings.Contains(out, `"unresolved": []`) && !strings.Contains(out, `"unresolved": [`) {
		t.Errorf("unresolved should be an array:\n%s", out)
	}
}

// TestConvertProseUnchangedWithoutJSON is a guard that omitting --json still
// yields the prose report (byte-unchanged path).
func TestConvertProseUnchangedWithoutJSON(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out, code := runCLI(t, "convert", "../testdata/corpus-basic", "--dry-run")
	if code != 0 {
		t.Fatalf("exit = %d; output:\n%s", code, out)
	}
	if !strings.HasPrefix(out, "binder convert --dry-run") {
		t.Errorf("prose output changed; got:\n%s", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("prose path emitted JSON:\n%s", out)
	}
}
