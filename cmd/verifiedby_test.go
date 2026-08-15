package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// runCLIErr runs the CLI and returns the raw command error (which main.go prints
// to stderr). Used to assert on error messages the root silences.
func runCLIErr(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	return root.Execute()
}

// mkCorpus writes a one-file corpus and returns its dir.
func mkCorpus(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte("# A\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestVerifiedByFlagStamps(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	isolateConfig(t)
	src := mkCorpus(t)
	out := t.TempDir()
	_, code := runCLI(t, "convert", src, "-o", out, "--verified-by", "human:ghchinoy")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, err := os.ReadFile(filepath.Join(out, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !contains(got, "verified:") || !contains(got, "by: human:ghchinoy") {
		t.Errorf("a.md missing verified stamp:\n%s", got)
	}
	if !contains(got, "2023-11-14T22:13:20Z") {
		t.Errorf("a.md missing deterministic verified.at:\n%s", got)
	}
}

func TestVerifiedByFromConfigDefault(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".binder.yaml"),
		[]byte("verified_by: process:ci-bot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := mkCorpus(t)
	out := t.TempDir()
	_, code := runCLI(t, "convert", src, "-o", out) // no flag → config default
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if !contains(string(b), "by: process:ci-bot") {
		t.Errorf("config default verified_by not applied:\n%s", b)
	}
}

func TestVerifiedByNoneWritesNoStamp(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	isolateConfig(t)
	src := mkCorpus(t)
	out := t.TempDir()
	_, code := runCLI(t, "convert", src, "-o", out) // no flag, no config
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if contains(string(b), "verified:") {
		t.Errorf("verified stamp written with no flag/config (never auto-stamp):\n%s", b)
	}
}

func TestVerifiedByInvalidFlagExit2(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	isolateConfig(t)
	src := mkCorpus(t)
	err := runCLIErr(t, "convert", src, "--dry-run", "--verified-by", "agent:benchmarking-bot")
	if code := clijson.ExitCode(err); code != clijson.ExitUsage {
		t.Fatalf("exit = %d, want %d (usage)", code, clijson.ExitUsage)
	}
	// Message must list the valid forms so the user can fix it.
	msg := err.Error()
	if !contains(msg, "human:") || !contains(msg, "process:") || !contains(msg, "team:") || !contains(msg, "<producer>/<version>") {
		t.Errorf("error does not list valid actor forms: %q", msg)
	}
}

func TestVerifiedByFlagBeatsConfig(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".binder.yaml"),
		[]byte("verified_by: process:ci-bot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := mkCorpus(t)
	out := t.TempDir()
	_, code := runCLI(t, "convert", src, "-o", out, "--verified-by", "human:override")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if !contains(string(b), "by: human:override") || contains(string(b), "process:ci-bot") {
		t.Errorf("flag did not override config verified_by:\n%s", b)
	}
}
