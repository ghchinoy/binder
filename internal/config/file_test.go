package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalKey(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"gemini_project", KeyGeminiProject, false},
		{"gemini.project", KeyGeminiProject, false},
		{"GEMINI.MODEL", KeyGeminiModel, false},
		{"verified_by", KeyVerifiedBy, false},
		{"verified.by", KeyVerifiedBy, false},
		{"default_type", KeyDefaultType, false},
		{"unknown_key", "", true},
		{"", "", true},
	}

	for _, tc := range cases {
		got, err := CanonicalKey(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("CanonicalKey(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("CanonicalKey(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestValidateValue(t *testing.T) {
	// 1. verified_by
	if err := ValidateValue("verified_by", "human:ghchinoy"); err != nil {
		t.Errorf("ValidateValue(verified_by, human:ghchinoy) = %v, want nil", err)
	}
	if err := ValidateValue("verified_by", "invalid-actor"); err == nil {
		t.Error("ValidateValue(verified_by, invalid-actor) succeeded, want error")
	}

	// 2. gemini_backend
	if err := ValidateValue("gemini_backend", "vertex"); err != nil {
		t.Errorf("ValidateValue(gemini_backend, vertex) = %v, want nil", err)
	}
	if err := ValidateValue("gemini_backend", "invalid-backend"); err == nil {
		t.Error("ValidateValue(gemini_backend, invalid-backend) succeeded, want error")
	}

	// 3. default_type
	if err := ValidateValue("default_type", ""); err == nil {
		t.Error("ValidateValue(default_type, '') succeeded, want error")
	}
}

func TestSetAndUnsetKeyInFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".binder.yaml")

	// 1. Set key in non-existent file
	k, err := SetKeyInFile(cfgPath, "gemini.project", "my-project")
	if err != nil {
		t.Fatalf("SetKeyInFile: %v", err)
	}
	if k != KeyGeminiProject {
		t.Errorf("SetKeyInFile returned canonical key %q, want %q", k, KeyGeminiProject)
	}

	// Verify file content has ONLY gemini_project: my-project
	settings, err := ReadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if len(settings) != 1 || settings[KeyGeminiProject] != "my-project" {
		t.Errorf("settings = %+v, want only gemini_project: my-project", settings)
	}

	// 2. Set another key
	if _, err := SetKeyInFile(cfgPath, "gemini_model", "gemini-3.5-flash-lite"); err != nil {
		t.Fatalf("SetKeyInFile gemini_model: %v", err)
	}
	settings, err = ReadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if len(settings) != 2 {
		t.Errorf("settings len = %d, want 2", len(settings))
	}

	// 3. Unset key
	k, existed, err := UnsetKeyInFile(cfgPath, "gemini.project")
	if err != nil {
		t.Fatalf("UnsetKeyInFile: %v", err)
	}
	if !existed || k != KeyGeminiProject {
		t.Errorf("UnsetKeyInFile = (%q, %v), want (%q, true)", k, existed, KeyGeminiProject)
	}

	settings, err = ReadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if _, ok := settings[KeyGeminiProject]; ok {
		t.Errorf("gemini_project still present after unset: %+v", settings)
	}
	if settings[KeyGeminiModel] != "gemini-3.5-flash-lite" {
		t.Errorf("gemini_model corrupted: %+v", settings)
	}

	// 4. Unset non-existent key
	_, existed, err = UnsetKeyInFile(cfgPath, "verified_by")
	if err != nil {
		t.Fatalf("UnsetKeyInFile non-existent: %v", err)
	}
	if existed {
		t.Errorf("existed = true, want false for unset of absent key")
	}
}

func TestSetKeyInFileStoresStrings(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".binder.yaml")

	// Numeric-looking and bool-looking values must be stored as strings, not
	// coerced to YAML int/bool.
	if _, err := SetKeyInFile(cfgPath, "gemini_project", "1234567890"); err != nil {
		t.Fatalf("SetKeyInFile gemini_project: %v", err)
	}
	if _, err := SetKeyInFile(cfgPath, "gemini_model", "true"); err != nil {
		t.Fatalf("SetKeyInFile gemini_model: %v", err)
	}

	settings, err := ReadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if v, ok := settings[KeyGeminiProject].(string); !ok || v != "1234567890" {
		t.Errorf("gemini_project = %#v (%T), want string \"1234567890\"", settings[KeyGeminiProject], settings[KeyGeminiProject])
	}
	if v, ok := settings[KeyGeminiModel].(string); !ok || v != "true" {
		t.Errorf("gemini_model = %#v (%T), want string \"true\"", settings[KeyGeminiModel], settings[KeyGeminiModel])
	}

	// The on-disk YAML must quote these so they re-read as strings.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	if !strings.Contains(string(data), `gemini_project: "1234567890"`) {
		t.Errorf("YAML did not quote numeric value:\n%s", data)
	}
	if !strings.Contains(string(data), `gemini_model: "true"`) {
		t.Errorf("YAML did not quote bool-like value:\n%s", data)
	}
}

func TestUnsetLastKeyRemovesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".binder.yaml")

	if _, err := SetKeyInFile(cfgPath, "gemini_project", "my-project"); err != nil {
		t.Fatalf("SetKeyInFile: %v", err)
	}
	if _, _, err := UnsetKeyInFile(cfgPath, "gemini_project"); err != nil {
		t.Fatalf("UnsetKeyInFile: %v", err)
	}

	// Removing the last key must leave no file behind (no lingering "{}").
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		data, _ := os.ReadFile(cfgPath)
		t.Errorf("config file still exists after unsetting last key (err=%v), content:\n%s", err, data)
	}
}

func TestWriteConfigFilePreservesMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".binder.yaml")

	// Create an existing file with a distinctive mode.
	if err := os.WriteFile(cfgPath, []byte("gemini_model: old\n"), 0o600); err != nil {
		t.Fatalf("seeding config file: %v", err)
	}

	if _, err := SetKeyInFile(cfgPath, "gemini_project", "my-project"); err != nil {
		t.Fatalf("SetKeyInFile: %v", err)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat after write: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600 (atomic write must preserve existing mode)", info.Mode().Perm())
	}
}

func TestTargetFilePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))

	local, err := TargetFilePath(false)
	if err != nil {
		t.Fatalf("TargetFilePath(false): %v", err)
	}
	if local != ".binder.yaml" && local != filepath.Join(".", ".binder.yaml") {
		t.Errorf("TargetFilePath(false) = %q, want ./.binder.yaml", local)
	}

	global, err := TargetFilePath(true)
	if err != nil {
		t.Fatalf("TargetFilePath(true): %v", err)
	}
	wantGlobal := filepath.Join(dir, "xdg", "binder", "config.yaml")
	if global != wantGlobal {
		t.Errorf("TargetFilePath(true) = %q, want %q", global, wantGlobal)
	}
	// Verify global directory was created
	if info, err := os.Stat(filepath.Dir(global)); err != nil || !info.IsDir() {
		t.Errorf("global config dir not created: %v", err)
	}
}
