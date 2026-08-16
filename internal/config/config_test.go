package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile writes content to path, creating parent dirs, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// isolate points config discovery at a clean temp dir: cwd for ./.binder.yaml
// and XDG_CONFIG_HOME for the user config, so tests never read a real config.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	return dir
}

func TestLoadDefaultsNoFile(t *testing.T) {
	isolate(t)
	c := &Config{}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.GetString(KeyDefaultType); got != "Note" {
		t.Errorf("default_type = %q, want Note", got)
	}
	if got := c.GetString(KeyVerifiedBy); got != "" {
		t.Errorf("verified_by = %q, want empty", got)
	}
	if c.ConfigFile() != "" {
		t.Errorf("config file = %q, want empty (no file)", c.ConfigFile())
	}
	if s := c.Source(KeyDefaultType); s != "default" {
		t.Errorf("default_type source = %q, want default", s)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := isolate(t)
	writeFile(t, filepath.Join(dir, ".binder.yaml"), "verified_by: process:ci-bot\ndefault_type: Guide\n")
	c := &Config{}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.GetString(KeyVerifiedBy); got != "process:ci-bot" {
		t.Errorf("verified_by = %q, want process:ci-bot", got)
	}
	if got := c.GetString(KeyDefaultType); got != "Guide" {
		t.Errorf("default_type = %q, want Guide", got)
	}
	if s := c.Source(KeyVerifiedBy); s != "file" {
		t.Errorf("verified_by source = %q, want file", s)
	}
}

func TestEnvBeatsFile(t *testing.T) {
	dir := isolate(t)
	writeFile(t, filepath.Join(dir, ".binder.yaml"), "verified_by: process:ci-bot\n")
	t.Setenv("BINDER_VERIFIED_BY", "team:core")
	c := &Config{}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.GetString(KeyVerifiedBy); got != "team:core" {
		t.Errorf("verified_by = %q, want team:core (env over file)", got)
	}
	if s := c.Source(KeyVerifiedBy); s != "env" {
		t.Errorf("source = %q, want env", s)
	}
}

func TestXDGConfigSearch(t *testing.T) {
	dir := isolate(t)
	writeFile(t, filepath.Join(dir, "xdg", "binder", "config.yaml"), "default_type: Decision\n")
	c := &Config{}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.GetString(KeyDefaultType); got != "Decision" {
		t.Errorf("default_type = %q, want Decision (from XDG)", got)
	}
}

func TestLocalFileBeatsXDG(t *testing.T) {
	dir := isolate(t)
	writeFile(t, filepath.Join(dir, "xdg", "binder", "config.yaml"), "default_type: FromXDG\n")
	writeFile(t, filepath.Join(dir, ".binder.yaml"), "default_type: FromLocal\n")
	c := &Config{}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.GetString(KeyDefaultType); got != "FromLocal" {
		t.Errorf("default_type = %q, want FromLocal (local .binder.yaml wins)", got)
	}
}

func TestBadConfigActorFailsFast(t *testing.T) {
	dir := isolate(t)
	writeFile(t, filepath.Join(dir, ".binder.yaml"), "verified_by: not-an-actor\n")
	c := &Config{}
	err := c.Load()
	if err == nil {
		t.Fatal("Load succeeded; want usage error for malformed verified_by")
	}
	// The message must list the valid forms so the user can fix it.
	if got := err.Error(); got == "" || !containsAll(got, "human:", "process:", "team:", "<producer>/<version>") {
		t.Errorf("error %q does not list valid actor forms", got)
	}
}

func TestBadEnvActorFailsFast(t *testing.T) {
	isolate(t)
	t.Setenv("BINDER_VERIFIED_BY", "bogus")
	c := &Config{}
	if err := c.Load(); err == nil {
		t.Fatal("Load succeeded; want usage error for malformed env verified_by")
	}
}

func TestValidActorForms(t *testing.T) {
	isolate(t)
	for _, actor := range []string{"human:ghchinoy", "process:benchmarking-bot", "team:core", "binder/0.1.0"} {
		t.Setenv("BINDER_VERIFIED_BY", actor)
		c := &Config{}
		if err := c.Load(); err != nil {
			t.Errorf("Load rejected valid actor %q: %v", actor, err)
		}
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestGeminiConfigDefaults(t *testing.T) {
	isolate(t)
	c := &Config{}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.GetString(KeyGeminiModel); got != "gemini-3.5-flash-lite" {
		t.Errorf("gemini_model = %q, want gemini-3.5-flash-lite", got)
	}
	if got := c.GetString(KeyGeminiLocation); got != "global" {
		t.Errorf("gemini_location = %q, want global", got)
	}
	if got := c.GetString(KeyGeminiProject); got != "" {
		t.Errorf("gemini_project = %q, want empty", got)
	}
	if got := c.GetString(KeyGeminiBackend); got != "auto" {
		t.Errorf("gemini_backend = %q, want auto", got)
	}
}

func TestGeminiConfigFromEnv(t *testing.T) {
	isolate(t)
	t.Setenv("BINDER_GEMINI_MODEL", "custom-model")
	t.Setenv("BINDER_GEMINI_PROJECT", "test-project")
	t.Setenv("BINDER_GEMINI_LOCATION", "us-central1")
	t.Setenv("BINDER_GEMINI_BACKEND", "vertex")
	c := &Config{}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.GetString(KeyGeminiModel); got != "custom-model" {
		t.Errorf("gemini_model = %q, want custom-model", got)
	}
	if got := c.GetString(KeyGeminiProject); got != "test-project" {
		t.Errorf("gemini_project = %q, want test-project", got)
	}
	if got := c.GetString(KeyGeminiLocation); got != "us-central1" {
		t.Errorf("gemini_location = %q, want us-central1", got)
	}
	if got := c.GetString(KeyGeminiBackend); got != "vertex" {
		t.Errorf("gemini_backend = %q, want vertex", got)
	}
}
