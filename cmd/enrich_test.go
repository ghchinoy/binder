package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// enrichCorpus writes a small corpus under a temp dir and returns its path.
func enrichCorpus(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "a.md"), "---\ntype: Note\ntitle: A\n---\n\n# A\n\nBody.\n")
	mustWrite(t, filepath.Join(src, "plain.md"), "# Plain\n\nProse.\n")
	return src
}

func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEnrichJSONEnvelope: enrich --json emits the shared envelope with
// command:"enrich" and schema binder.report/v1, and the expected result fields.
func TestEnrichJSONEnvelope(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	src := enrichCorpus(t)
	out, code := runCLI(t, "enrich", src, "--dry-run", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; output:\n%s", code, out)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if env.Command != "enrich" {
		t.Errorf("command = %q, want enrich", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}
	if env.Binder != "binder/"+Version {
		t.Errorf("binder = %q, want binder/%s", env.Binder, Version)
	}
	result, ok := env.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not an object: %T", env.Result)
	}
	for _, key := range []string{"src", "dry_run", "num_files", "num_enriched", "num_unchanged", "num_skipped", "files"} {
		if _, present := result[key]; !present {
			t.Errorf("result missing key %q", key)
		}
	}
	if _, ok := result["files"].([]any); !ok {
		t.Errorf("files is not an array (empty-slice policy): %T", result["files"])
	}
}

// TestEnrichBadPathExit2: a missing/non-directory source is a usage error → 2.
func TestEnrichBadPathExit2(t *testing.T) {
	_, code := runCLI(t, "enrich", filepath.Join(t.TempDir(), "does-not-exist"))
	if code != clijson.ExitUsage {
		t.Fatalf("exit = %d, want %d (usage)", code, clijson.ExitUsage)
	}
}

// TestEnrichCleanExit0: a run that enriches without skips exits 0.
func TestEnrichCleanExit0(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	src := enrichCorpus(t)
	_, code := runCLI(t, "enrich", src)
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	// Idempotent second run also exits 0.
	if _, code := runCLI(t, "enrich", src); code != clijson.ExitSuccess {
		t.Fatalf("2nd run exit = %d, want 0", code)
	}
}

// TestEnrichStrictGatesOnSkipped: an unparseable file is skipped; bare enrich
// exits 0, but --strict gates at exit 1.
func TestEnrichStrictGatesOnSkipped(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	src := t.TempDir()
	// Opens a fence but the YAML is invalid → unparseable → skipped.
	mustWrite(t, filepath.Join(src, "bad.md"), "---\ntype: Note\n  : : :\n---\n\n# Bad\n")

	out, code := runCLI(t, "enrich", src)
	if code != clijson.ExitSuccess {
		t.Fatalf("bare enrich exit = %d, want 0; out:\n%s", code, out)
	}
	if !strings.Contains(out, "skipped") {
		t.Errorf("expected a skipped report line:\n%s", out)
	}

	_, code = runCLI(t, "enrich", src, "--strict")
	if code != clijson.ExitFindings {
		t.Fatalf("--strict exit = %d, want %d (findings)", code, clijson.ExitFindings)
	}
}

// TestEnrichInvalidVerifiedByExit2: a malformed --verified-by actor is a usage
// error (exit 2), mirroring convert.
func TestEnrichInvalidVerifiedByExit2(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	src := enrichCorpus(t)
	_, code := runCLI(t, "enrich", src, "--verified-by", "not-an-actor", "--dry-run")
	if code != clijson.ExitUsage {
		t.Fatalf("exit = %d, want %d (usage) for a bad --verified-by", code, clijson.ExitUsage)
	}
}

// TestEnrichBadStatusMapExit2: a malformed --status-map is a usage error (exit 2).
func TestEnrichBadStatusMapExit2(t *testing.T) {
	src := enrichCorpus(t)
	_, code := runCLI(t, "enrich", src, "--status-map", "no-equals-sign", "--dry-run")
	if code != clijson.ExitUsage {
		t.Fatalf("exit = %d, want %d (usage) for a bad --status-map", code, clijson.ExitUsage)
	}
}

// TestEnrichOverwriteTrustKeyExit2 is acceptance criterion 4: naming a
// trust-provenance key in --overwrite-keys is a usage error (exit 2) whose
// message names the offending key, and NO file is modified.
func TestEnrichOverwriteTrustKeyExit2(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	src := t.TempDir()
	doc := "---\ntype: Note\ntitle: A\nverified:\n  - by: human:me\n    at: '2020-01-01T00:00:00Z'\n---\n\n# A\n\nBody.\n"
	p := filepath.Join(src, "a.md")
	mustWrite(t, p, doc)

	for _, key := range []string{"verified", "verified_by", "sources", "generated"} {
		// The root silences error text (SilenceErrors), so assert on the returned
		// error itself: it must be a usage error (exit 2) naming the offending key.
		root := NewRootCmd()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"enrich", src, "--overwrite-keys", "status," + key})
		err := root.Execute()
		if code := clijson.ExitCode(err); code != clijson.ExitUsage {
			t.Fatalf("%s: exit = %d, want %d (usage)", key, code, clijson.ExitUsage)
		}
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s: refusal message does not name the key: %v", key, err)
		}
	}
	if got, err := os.ReadFile(p); err != nil || string(got) != doc {
		t.Errorf("file modified by a refused overwrite (want byte-identical):\n%s", string(got))
	}
}

// TestEnrichOverwriteEndToEnd is criteria 3/5 through the CLI: --overwrite-keys
// status,stale_after refreshes the two keys in place (and --dry-run previews
// without writing).
func TestEnrichOverwriteEndToEnd(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	src := t.TempDir()
	doc := "---\ntype: Note\ntitle: A\nstatus: stable\nstale_after: 2019-05-05\n---\n\n# A\n\nBody.\n"
	p := filepath.Join(src, "notes", "a.md")
	mustWrite(t, p, doc)

	// --dry-run writes nothing.
	out, code := runCLI(t, "enrich", src,
		"--status-map", "notes=deprecated", "--stale-after-map", "notes=+6m",
		"--overwrite-keys", "status,stale_after", "--dry-run")
	if code != clijson.ExitSuccess {
		t.Fatalf("dry-run exit = %d, want 0; out:\n%s", code, out)
	}
	if !strings.Contains(out, "overwrite") {
		t.Errorf("dry-run report does not mention overwrite:\n%s", out)
	}
	if got, _ := os.ReadFile(p); string(got) != doc {
		t.Fatalf("dry-run modified the file")
	}

	// Real run refreshes the two keys.
	if _, code := runCLI(t, "enrich", src,
		"--status-map", "notes=deprecated", "--stale-after-map", "notes=+6m",
		"--overwrite-keys", "status,stale_after"); code != clijson.ExitSuccess {
		t.Fatalf("run exit = %d, want 0", code)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "status: deprecated") {
		t.Errorf("status not refreshed:\n%s", got)
	}
	if strings.Contains(string(got), "stale_after: 2019-05-05") {
		t.Errorf("stale_after not refreshed:\n%s", got)
	}
	if !strings.Contains(string(got), "title: A") {
		t.Errorf("unrelated key not preserved:\n%s", got)
	}
}
