package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// TestLintExitCodes exercises the decided exit posture (option (a), unified
// never-reject): bare lint always exits 0; --strict gates exit 1 on any finding;
// a bad path is a usage error (exit 2); a clean corpus is 0 even under --strict.
func TestLintExitCodes(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"clean", []string{"lint", "../testdata/corpus-lint-clean"}, clijson.ExitSuccess},
		{"clean-strict", []string{"lint", "../testdata/corpus-lint-clean", "--strict"}, clijson.ExitSuccess},
		{"findings-bare", []string{"lint", "../testdata/corpus-lint-links"}, clijson.ExitSuccess},
		{"findings-strict", []string{"lint", "../testdata/corpus-lint-links", "--strict"}, clijson.ExitFindings},
		{"bad-path", []string{"lint", "../testdata/does-not-exist"}, clijson.ExitUsage},
		{"path-is-file", []string{"lint", "../testdata/corpus-lint-clean/a.md"}, clijson.ExitUsage},
		{"unknown-flag", []string{"lint", "../testdata/corpus-lint-clean", "--nope"}, clijson.ExitUsage},
		{"missing-arg", []string{"lint"}, clijson.ExitUsage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, code := runCLI(t, c.args...)
			if code != c.want {
				t.Errorf("args %v: exit = %d, want %d", c.args, code, c.want)
			}
		})
	}
}

// TestLintProse: default output is the deterministic prose report.
func TestLintProse(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out, code := runCLI(t, "lint", "../testdata/corpus-lint-links")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d; output:\n%s", code, out)
	}
	if !strings.HasPrefix(out, "binder lint\n") {
		t.Errorf("prose output changed; got:\n%s", out)
	}
	if !strings.Contains(out, "broken links: 2") {
		t.Errorf("expected two broken links in prose:\n%s", out)
	}
	if !strings.Contains(out, "a -> nope.md") || !strings.Contains(out, "a -> [[Ghost]]") {
		t.Errorf("broken links missing from prose:\n%s", out)
	}
}

// TestLintJSONEnvelope: --json emits the shared envelope with command "lint" and
// the shared report schema, and is deterministic across runs.
func TestLintJSONEnvelope(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out, code := runCLI(t, "lint", "../testdata/corpus-lint-links", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d; output:\n%s", code, out)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if env.Command != "lint" {
		t.Errorf("command = %q, want lint", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}
	if env.Binder != "binder/"+Version {
		t.Errorf("binder = %q, want binder/%s", env.Binder, Version)
	}
	result, ok := env.Result.(map[string]any)
	if !ok {
		t.Fatalf("result not an object: %T", env.Result)
	}
	for _, key := range []string{
		"src", "num_concepts", "broken_links", "missing_titles",
		"orphans", "stale", "schema_violations",
	} {
		if _, present := result[key]; !present {
			t.Errorf("result missing key %q", key)
		}
	}

	// Deterministic across runs.
	out2, _ := runCLI(t, "lint", "../testdata/corpus-lint-links", "--json")
	if out != out2 {
		t.Errorf("JSON not byte-identical across runs:\n%s\n---\n%s", out, out2)
	}
}

// TestLintJSONEmptySlices: a clean corpus serializes empty buckets as [] not null.
func TestLintJSONEmptySlices(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out, code := runCLI(t, "lint", "../testdata/corpus-lint-clean", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d; output:\n%s", code, out)
	}
	if strings.Contains(out, ": null") {
		t.Errorf("report contains a null slice; empty buckets must be []:\n%s", out)
	}
	if !strings.Contains(out, `"broken_links": []`) {
		t.Errorf("broken_links should be an empty array:\n%s", out)
	}
}
