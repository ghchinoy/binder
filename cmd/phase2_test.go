package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// TestValidateJSON asserts validate --json emits the enveloped Result and that
// the exit code tracks conformance (0 conformant, 1 non-conformant) identically
// to prose mode.
func TestValidateJSON(t *testing.T) {
	out, code := runCLI(t, "validate", "../testdata/expected-basic", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("conformant validate exit = %d, want 0; out:\n%s", code, out)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("validate --json not valid JSON: %v\n%s", err, out)
	}
	if env.Command != "validate" || env.Schema != clijson.SchemaVersion {
		t.Errorf("envelope command/schema = %q/%q", env.Command, env.Schema)
	}
	result := env.Result.(map[string]any)
	for _, key := range []string{"root", "num_concepts", "num_reserved", "findings"} {
		if _, ok := result[key]; !ok {
			t.Errorf("validate result missing key %q", key)
		}
	}

	// Non-conformant bundle: exit 1, and JSON still emitted (findings present).
	out, code = runCLI(t, "validate", "../testdata/malformed", "--json")
	if code != clijson.ExitFindings {
		t.Fatalf("non-conformant validate exit = %d, want 1; out:\n%s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("non-conformant validate --json not valid JSON: %v\n%s", err, out)
	}
	findings, _ := env.Result.(map[string]any)["findings"].([]any)
	if len(findings) == 0 {
		t.Errorf("expected at least one finding for a non-conformant bundle")
	}
	if sev := findings[0].(map[string]any)["severity"]; sev != "error" {
		t.Errorf("first finding severity = %v, want error", sev)
	}
}

// TestValidateProseUnchanged guards the prose stdout path (exit still tracks
// conformance).
func TestValidateProseUnchanged(t *testing.T) {
	out, code := runCLI(t, "validate", "../testdata/expected-basic")
	if code != 0 {
		t.Fatalf("exit = %d; out:\n%s", code, out)
	}
	if !strings.HasPrefix(out, "bundle: ") || !strings.Contains(out, "RESULT: conformant") {
		t.Errorf("validate prose changed:\n%s", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("prose path emitted JSON:\n%s", out)
	}
}

// TestReviewJSON asserts review --json emits an enveloped, deterministic report
// and always exits 0 even with orphans and unresolved links (never-reject).
func TestReviewJSON(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	a, code := runCLI(t, "review", "../testdata/expected-basic", "--json", "--today", "2026-08-15")
	if code != clijson.ExitSuccess {
		t.Fatalf("review exit = %d, want 0; out:\n%s", code, a)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(a), &env); err != nil {
		t.Fatalf("review --json not valid JSON: %v\n%s", err, a)
	}
	if env.Command != "review" {
		t.Errorf("command = %q, want review", env.Command)
	}
	result := env.Result.(map[string]any)
	// A corpus with orphans/unresolved links is reported but never gates.
	if orphans, _ := result["orphans"].([]any); len(orphans) == 0 {
		t.Errorf("expected orphans reported in review of expected-basic")
	}

	// Determinism: byte-identical across runs.
	b, _ := runCLI(t, "review", "../testdata/expected-basic", "--json", "--today", "2026-08-15")
	if a != b {
		t.Errorf("review --json not deterministic:\n%s\n---\n%s", a, b)
	}
}

// TestGraphJSONAliasNoEnvelope asserts graph --json is an alias for --format
// json (raw {nodes,edges}, NOT the report envelope) and that a conflicting
// --format is a usage error.
func TestGraphJSONAliasNoEnvelope(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out, code := runCLI(t, "graph", "../testdata/expected-rich", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("graph --json exit = %d, want 0; out:\n%s", code, out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("graph --json not valid JSON: %v\n%s", err, out)
	}
	// Must be the raw graph export, not the report envelope.
	if _, hasNodes := m["nodes"]; !hasNodes {
		t.Errorf("graph --json missing nodes; got keys %v", keys(m))
	}
	if _, wrapped := m["schema"]; wrapped {
		t.Errorf("graph --json must NOT be wrapped in the report envelope:\n%s", out)
	}

	// --json + --format json is redundant-but-fine (exit 0).
	if _, code := runCLI(t, "graph", "../testdata/expected-rich", "--json", "--format", "json"); code != 0 {
		t.Errorf("graph --json --format json exit = %d, want 0", code)
	}
	// --json + a conflicting --format is a usage error (exit 2).
	if _, code := runCLI(t, "graph", "../testdata/expected-rich", "--json", "--format", "dot"); code != clijson.ExitUsage {
		t.Errorf("graph --json --format dot exit = %d, want 2", code)
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
