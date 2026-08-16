package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if !contains(out, `gemini_model: "gemini-3.5-flash-lite" (source: default)`) {
		t.Errorf("missing gemini_model default line:\n%s", out)
	}
}

func TestConfigListCmdAlias(t *testing.T) {
	isolateConfig(t)
	out, code := runCLI(t, "config", "list")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	if !contains(out, `default_type: "Note" (source: default)`) {
		t.Errorf("missing default_type default line:\n%s", out)
	}
}

func TestConfigGetCmd(t *testing.T) {
	isolateConfig(t)
	out, code := runCLI(t, "config", "get", "default_type")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	if strings.TrimSpace(out) != "Note" {
		t.Errorf("output = %q, want Note", strings.TrimSpace(out))
	}

	// Dotted key normalization
	outDot, codeDot := runCLI(t, "config", "get", "gemini.model")
	if codeDot != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", codeDot, outDot)
	}
	if strings.TrimSpace(outDot) != "gemini-3.5-flash-lite" {
		t.Errorf("output = %q, want gemini-3.5-flash-lite", strings.TrimSpace(outDot))
	}
}

func TestConfigGetCmdUnknownKeyExit2(t *testing.T) {
	isolateConfig(t)
	_, code := runCLI(t, "config", "get", "nonexistent_key")
	if code != clijson.ExitUsage {
		t.Errorf("exit = %d, want %d (ExitUsage)", code, clijson.ExitUsage)
	}
}

func TestConfigCmdStrayArgsExit2(t *testing.T) {
	isolateConfig(t)

	// A stray positional arg on the parent `config` command is a usage error
	// (exit 2), not an IO/internal error (exit 3). Regression guard for the
	// exactArgs(0) vs cobra.NoArgs exit-code contract.
	_, code := runCLI(t, "config", "bogus")
	if code != clijson.ExitUsage {
		t.Errorf("config bogus exit = %d, want %d (ExitUsage)", code, clijson.ExitUsage)
	}

	// Same contract for `config list`.
	_, codeList := runCLI(t, "config", "list", "bogus")
	if codeList != clijson.ExitUsage {
		t.Errorf("config list bogus exit = %d, want %d (ExitUsage)", codeList, clijson.ExitUsage)
	}
}

func TestConfigSetStoresStringValues(t *testing.T) {
	dir := isolateConfig(t)

	// A numeric-looking value (e.g. a GCP project id) must stay a string, not
	// become a YAML int.
	if _, code := runCLI(t, "config", "set", "gemini_project", "1234567890"); code != clijson.ExitSuccess {
		t.Fatalf("set gemini_project exit = %d, want 0", code)
	}
	// A "true"-looking value must stay a string, not become a YAML bool.
	if _, code := runCLI(t, "config", "set", "gemini_model", "true"); code != clijson.ExitSuccess {
		t.Fatalf("set gemini_model exit = %d, want 0", code)
	}

	settings, err := config.ReadConfigFile(filepath.Join(dir, ".binder.yaml"))
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if v, ok := settings["gemini_project"].(string); !ok || v != "1234567890" {
		t.Errorf("gemini_project = %#v (%T), want string \"1234567890\"", settings["gemini_project"], settings["gemini_project"])
	}
	if v, ok := settings["gemini_model"].(string); !ok || v != "true" {
		t.Errorf("gemini_model = %#v (%T), want string \"true\"", settings["gemini_model"], settings["gemini_model"])
	}
}

func TestConfigSetAndGetWorkflow(t *testing.T) {
	dir := isolateConfig(t)

	// Set gemini.project locally in .binder.yaml
	out, code := runCLI(t, "config", "set", "gemini.project", "my-gcp-project")
	if code != clijson.ExitSuccess {
		t.Fatalf("set exit = %d, want 0; out:\n%s", code, out)
	}
	if !contains(out, "Set gemini_project = \"my-gcp-project\" in .binder.yaml") {
		t.Errorf("unexpected set output:\n%s", out)
	}

	// Verify local file exists and only has gemini_project
	data, err := os.ReadFile(filepath.Join(dir, ".binder.yaml"))
	if err != nil {
		t.Fatalf("reading .binder.yaml: %v", err)
	}
	if !contains(string(data), "gemini_project: my-gcp-project") {
		t.Errorf(".binder.yaml content = %q, want gemini_project: my-gcp-project", string(data))
	}

	// Unset gemini.project
	outUnset, codeUnset := runCLI(t, "config", "unset", "gemini.project")
	if codeUnset != clijson.ExitSuccess {
		t.Fatalf("unset exit = %d, want 0; out:\n%s", codeUnset, outUnset)
	}
	if !contains(outUnset, "Unset gemini_project in .binder.yaml (reverted to default)") {
		t.Errorf("unexpected unset output:\n%s", outUnset)
	}
}

func TestConfigSetGlobalWorkflow(t *testing.T) {
	dir := isolateConfig(t)

	// Set global config
	out, code := runCLI(t, "config", "set", "--global", "gemini.model", "gemini-2.5-pro")
	if code != clijson.ExitSuccess {
		t.Fatalf("set global exit = %d, want 0; out:\n%s", code, out)
	}

	globalPath := filepath.Join(dir, "xdg", "binder", "config.yaml")
	data, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("reading global config: %v", err)
	}
	if !contains(string(data), "gemini_model: gemini-2.5-pro") {
		t.Errorf("global config content = %q, want gemini_model: gemini-2.5-pro", string(data))
	}
}

func TestConfigSetValidationExit2(t *testing.T) {
	isolateConfig(t)

	// Bad key
	_, code := runCLI(t, "config", "set", "bad_key", "value")
	if code != clijson.ExitUsage {
		t.Errorf("exit = %d, want ExitUsage for bad key", code)
	}

	// Bad actor in verified_by
	_, code = runCLI(t, "config", "set", "verified_by", "invalid-actor")
	if code != clijson.ExitUsage {
		t.Errorf("exit = %d, want ExitUsage for bad actor", code)
	}

	// Bad backend in gemini_backend
	_, code = runCLI(t, "config", "set", "gemini_backend", "invalid-backend")
	if code != clijson.ExitUsage {
		t.Errorf("exit = %d, want ExitUsage for bad backend", code)
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
