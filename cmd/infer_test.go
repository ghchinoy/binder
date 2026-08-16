package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

func setupInferCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	sub := filepath.Join(dir, "subsystems")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "audio.md"), []byte("# Audio\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rb := filepath.Join(dir, "runbooks")
	if err := os.MkdirAll(rb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rb, "deploy.md"), []byte("# Deploy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestInferCmdProse(t *testing.T) {
	dir := setupInferCorpus(t)
	out, code := runCLI(t, "infer", dir)
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	want := "runbooks=Runbook,subsystems=Subsystem"
	if strings.TrimSpace(out) != want {
		t.Errorf("output = %q, want %q", strings.TrimSpace(out), want)
	}
}

// TestInferCmdZeroMappingDiagnosticToStderr pins the output-routing contract for
// the zero-mapping case: stdout must be empty (so the documented
// `--type-map "$(binder infer SRC)"` idiom substitutes "" rather than prose),
// the human-readable diagnostic must land on stderr, and the exit code stays 0.
// We assert stdout *emptiness* rather than the absence of the message text, so the
// test survives any later rewording of the diagnostic.
func TestInferCmdZeroMappingDiagnosticToStderr(t *testing.T) {
	// An empty (but readable) corpus yields no mappings.
	dir := t.TempDir()
	stdout, stderr, code := runCLISplit(t, "infer", dir)
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout must be empty for the zero-mapping case, got:\n%q", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Errorf("diagnostic must be written to stderr, but stderr was empty")
	}
}

func TestInferCmdJSONEnvelope(t *testing.T) {
	dir := setupInferCorpus(t)
	out, code := runCLI(t, "infer", dir, "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}

	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}

	if env.Command != "infer" {
		t.Errorf("command = %q, want infer", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}
	if !strings.HasPrefix(env.Binder, "binder/") {
		t.Errorf("binder = %q, want prefix binder/", env.Binder)
	}

	res, ok := env.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not map[string]any: %T", env.Result)
	}
	if res["type_map"] != "runbooks=Runbook,subsystems=Subsystem" {
		t.Errorf("type_map = %v, want runbooks=Runbook,subsystems=Subsystem", res["type_map"])
	}
	proposals, ok := res["mappings"].([]any)
	if !ok || len(proposals) != 2 {
		t.Fatalf("mappings len = %d, want 2", len(proposals))
	}
}

// TestInferCmdJSONWarningsStable locks the stable-envelope guarantee: a
// warning-free run must still emit "warnings" as an empty array, never as null
// and never as an omitted key. Every sibling command emits [] and the docs
// promise a stable output shape.
func TestInferCmdJSONWarningsStable(t *testing.T) {
	dir := setupInferCorpus(t)
	out, code := runCLI(t, "infer", dir, "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	if !strings.Contains(out, `"warnings": []`) {
		t.Errorf("warnings not emitted as []; output:\n%s", out)
	}

	// Distinguish "present and empty" from "null" and "absent": decode into a
	// map that only carries a key when it was present in the JSON.
	var env struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	raw, present := env.Result["warnings"]
	if !present {
		t.Fatalf("warnings key omitted; output:\n%s", out)
	}
	if string(raw) != "[]" {
		t.Errorf("warnings = %s, want []", raw)
	}
}

// TestInferCmdJSONMappingsStable locks the same stable-envelope guarantee for
// mappings: an empty corpus (no directory type mappings) must emit "mappings"
// as an empty array, never null and never an omitted key — the identical defect
// class as warnings (U8), in the same envelope.
func TestInferCmdJSONMappingsStable(t *testing.T) {
	// An empty (but readable) corpus yields no mappings and no warnings.
	dir := t.TempDir()
	out, code := runCLI(t, "infer", dir, "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	if !strings.Contains(out, `"mappings": []`) {
		t.Errorf("mappings not emitted as []; output:\n%s", out)
	}

	var env struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	for _, key := range []string{"mappings", "warnings"} {
		raw, present := env.Result[key]
		if !present {
			t.Fatalf("%s key omitted; output:\n%s", key, out)
		}
		if string(raw) != "[]" {
			t.Errorf("%s = %s, want []", key, raw)
		}
	}
}

func TestInferCmdJSONDeterministic(t *testing.T) {
	dir := setupInferCorpus(t)
	out1, c1 := runCLI(t, "infer", dir, "--json")
	out2, c2 := runCLI(t, "infer", dir, "--json")
	if c1 != 0 || c2 != 0 {
		t.Fatalf("exit codes = %d, %d", c1, c2)
	}
	if out1 != out2 {
		t.Errorf("infer --json not byte-identical:\n%s\n---\n%s", out1, out2)
	}
}

func TestInferCmdBadPathExit2(t *testing.T) {
	_, code := runCLI(t, "infer", "/nonexistent/path/that/does/not/exist")
	if code != clijson.ExitUsage {
		t.Errorf("exit = %d, want %d (ExitUsage)", code, clijson.ExitUsage)
	}
}
