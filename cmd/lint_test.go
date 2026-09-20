package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/clijson"
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

// TestLintTodayFlag: --today drives staleness deterministically, and under
// --strict an orphan/stale finding gates exit 1 while a clean date does not gate
// away the orphan.
func TestLintTodayFlag(t *testing.T) {
	// After stale_after: orphan + stale both present → --strict gates.
	if _, code := runCLI(t, "lint", "../testdata/corpus-lint-graph", "--today", "2023-11-14", "--strict"); code != clijson.ExitFindings {
		t.Errorf("strict with findings: exit = %d, want 1", code)
	}
	// The orphan (island) persists regardless of date, so --strict still gates
	// even before any stale_after.
	out, code := runCLI(t, "lint", "../testdata/corpus-lint-graph", "--today", "2019-01-01")
	if code != clijson.ExitSuccess {
		t.Fatalf("bare lint exit = %d, want 0; %s", code, out)
	}
	if !strings.Contains(out, "stale: 0") {
		t.Errorf("expected no stale concepts as of 2019:\n%s", out)
	}
	if !strings.Contains(out, "orphans (no inbound or outbound links): 1") {
		t.Errorf("expected the orphan regardless of date:\n%s", out)
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
	if env.Binder != "binder/"+binder.Version {
		t.Errorf("binder = %q, want binder/%s", env.Binder, binder.Version)
	}
	result, ok := env.Result.(map[string]any)
	if !ok {
		t.Fatalf("result not an object: %T", env.Result)
	}
	for _, key := range []string{
		"src", "num_concepts", "broken_links", "missing_titles",
		"orphans", "entrypoints", "stale", "schema_violations",
		"colon_space_scalars",
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

// TestLintColonSpaceAdvisory exercises issue #93 end to end: the advisory is
// surfaced in prose and JSON on the positive-control corpus, stays silent on the
// negative-control corpus, and gates nothing in either — not even under --strict.
func TestLintColonSpaceAdvisory(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	out, code := runCLI(t, "lint", "../testdata/corpus-lint-schema")
	if code != clijson.ExitSuccess {
		t.Fatalf("bare lint exit = %d, want 0 (never-reject); output:\n%s", code, out)
	}
	if !strings.Contains(out, "unquoted colon-space scalars (advisory, never gates): 2") {
		t.Errorf("advisory count missing from prose:\n%s", out)
	}
	for _, want := range []string{
		`badyaml: title: unquoted value contains a colon followed by a space or tab — quote it`,
		`badyaml: goal: unquoted value contains a colon followed by a space or tab — quote it`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prose missing %q:\n%s", want, out)
		}
	}

	// The negative controls (quoted scalars, https:// URLs, 12:30 timestamps,
	// block-scalar interiors, flow collections) stay clean, and --strict over them
	// still exits 0.
	out, code = runCLI(t, "lint", "../testdata/corpus-lint-colonspace", "--strict")
	if code != clijson.ExitSuccess {
		t.Fatalf("negative-control corpus under --strict: exit = %d, want 0; output:\n%s", code, out)
	}
	if !strings.Contains(out, "unquoted colon-space scalars (advisory, never gates): 0") {
		t.Errorf("negative controls triggered the advisory:\n%s", out)
	}

	// JSON parity: the advisory rides in the same report payload.
	out, code = runCLI(t, "lint", "../testdata/corpus-lint-schema", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("lint --json exit = %d, want 0; output:\n%s", code, out)
	}
	var env struct {
		Result struct {
			ColonSpaceScalars []struct {
				Concept string `json:"concept"`
				Detail  string `json:"detail"`
			} `json:"colon_space_scalars"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("lint json parse: %v\n%s", err, out)
	}
	if len(env.Result.ColonSpaceScalars) != 2 {
		t.Errorf("colon_space_scalars = %+v, want 2 entries", env.Result.ColonSpaceScalars)
	}
	for _, f := range env.Result.ColonSpaceScalars {
		if f.Concept != "badyaml" {
			t.Errorf("unexpected concept in advisory: %+v", f)
		}
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
