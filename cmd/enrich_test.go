package cmd

import (
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
