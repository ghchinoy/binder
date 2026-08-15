package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
)

// isolateConfig points config discovery at a clean temp dir so `binder config`
// tests never read a developer's real config file.
func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	return dir
}

func TestConfigCmdDefaults(t *testing.T) {
	isolateConfig(t)
	out, code := runCLI(t, "config")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	if !contains(out, `default_type: "Note" (source: default)`) {
		t.Errorf("missing default_type default line:\n%s", out)
	}
}

func TestConfigCmdJSONEnvelope(t *testing.T) {
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".binder.yaml"),
		[]byte("verified_by: human:ghchinoy\ndefault_type: Guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runCLI(t, "config", "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if env.Command != "config" {
		t.Errorf("command = %q, want config", env.Command)
	}
	if env.Schema != config.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, config.SchemaVersion)
	}
	result, ok := env.Result.(map[string]any)
	if !ok {
		t.Fatalf("result not an object: %T", env.Result)
	}
	values, ok := result["values"].(map[string]any)
	if !ok {
		t.Fatalf("values not an object: %T", result["values"])
	}
	vb, ok := values["verified_by"].(map[string]any)
	if !ok {
		t.Fatalf("verified_by not an object: %T", values["verified_by"])
	}
	if vb["value"] != "human:ghchinoy" {
		t.Errorf("verified_by.value = %v, want human:ghchinoy", vb["value"])
	}
	if vb["source"] != "file" {
		t.Errorf("verified_by.source = %v, want file", vb["source"])
	}
}

func TestConfigCmdJSONDeterministic(t *testing.T) {
	isolateConfig(t)
	a, ca := runCLI(t, "config", "--json")
	b, cb := runCLI(t, "config", "--json")
	if ca != 0 || cb != 0 {
		t.Fatalf("exit codes = %d, %d; want 0", ca, cb)
	}
	if a != b {
		t.Errorf("config --json not byte-identical:\n%s\n---\n%s", a, b)
	}
}

func TestConfigCmdBadActorExit2(t *testing.T) {
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".binder.yaml"),
		[]byte("verified_by: not-an-actor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, code := runCLI(t, "config")
	if code != clijson.ExitUsage {
		t.Errorf("exit = %d, want %d (usage) for malformed config verified_by", code, clijson.ExitUsage)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
