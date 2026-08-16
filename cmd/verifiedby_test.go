package cmd

import (
	"bytes"
	"encoding/json"
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

// TestVerifiedByRepoLocalConfigDoesNotStamp pins Option A (owner-ruled): a
// repo-local ./.binder.yaml verified_by does NOT evidence THIS user's per-run
// decision, so it does NOT authorize a stamp. The value is not silently dropped —
// it is disclosed in the report as an ignored repo-local verifier (Residual B).
func TestVerifiedByRepoLocalConfigDoesNotStamp(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".binder.yaml"),
		[]byte("verified_by: process:ci-bot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := mkCorpus(t)
	out := t.TempDir()
	stdout, code := runCLI(t, "convert", src, "-o", out) // no flag; only a repo-local config
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if contains(string(b), "verified:") {
		t.Errorf("repo-local ./.binder.yaml authorized a stamp (Option A violated):\n%s", b)
	}
	// Residual B: the ignored repo-local verifier is disclosed, not silently dropped.
	if !contains(stdout, "ignored repo-local") || !contains(stdout, "process:ci-bot") {
		t.Errorf("repo-local verifier was not disclosed in the report:\n%s", stdout)
	}
}

// TestVerifiedByGlobalConfigStampsAndDiscloses is the criterion-3 companion and
// the anti-vacuity control for the Option-A skip above: the SAME value in a GLOBAL
// home config (XDG) DOES stamp (the user set their own default) and the stamp is
// disclosed in the report (Residual B, prose).
func TestVerifiedByGlobalConfigStampsAndDiscloses(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	dir := isolateConfig(t)
	gdir := filepath.Join(dir, "xdg", "binder")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gdir, "config.yaml"),
		[]byte("verified_by: process:ci-bot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := mkCorpus(t)
	out := t.TempDir()
	stdout, code := runCLI(t, "convert", src, "-o", out) // no flag → global config default
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if !contains(string(b), "by: process:ci-bot") {
		t.Errorf("global config default verified_by not applied:\n%s", b)
	}
	if !contains(stdout, "Trust (verified stamps)") || !contains(stdout, "source: config") {
		t.Errorf("stamp was not disclosed in the report:\n%s", stdout)
	}
}

// TestVerifiedByConfigDefaultSkipsDifferentIdentity pins criterion 4: a global
// config default must NOT co-sign a document a DIFFERENT identity has already
// attested. It is a SKIP (exit 0, file not stamped, prior attestation intact),
// disclosed in the report — never an error and never a drop.
func TestVerifiedByConfigDefaultSkipsDifferentIdentity(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	dir := isolateConfig(t)
	gdir := filepath.Join(dir, "xdg", "binder")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gdir, "config.yaml"),
		[]byte("verified_by: human:ghchinoy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// enrich in place over a source already attested by a DIFFERENT identity.
	src := t.TempDir()
	p := filepath.Join(src, "a.md")
	doc := "---\ntype: Note\ntitle: A\n" +
		"verified:\n  - by: human:ahormati\n    at: '2020-01-01T00:00:00Z'\n---\n\n# A\n\nBody.\n"
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, code := runCLI(t, "enrich", src)
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0 (skip is not a reject)", code)
	}
	got, _ := os.ReadFile(p)
	if contains(string(got), "human:ghchinoy") {
		t.Errorf("config default co-signed a different identity (criterion 4 violated):\n%s", got)
	}
	if !contains(string(got), "human:ahormati") {
		t.Errorf("prior attestation was dropped:\n%s", got)
	}
	if !contains(stdout, "skipped") || !contains(stdout, "already attested by human:ahormati") {
		t.Errorf("skip was not disclosed in the report:\n%s", stdout)
	}
}

// TestVerifiedByEnvDoesNotStamp pins the owner ruling on BINDER_VERIFIED_BY: an
// inherited environment export is NOT a per-invocation decision to attest, so it
// does NOT authorize a `verified` stamp without an explicit --verified-by. It gets
// the same treatment as a repo-local .binder.yaml (Option A). The refusal is
// DISCLOSED with a note parallel to the repo-local one — never silently ignored.
//
// This carries a DEMONSTRATED RED: before the OriginEnv arm of
// config.PermitsStampWithoutFlag was flipped to false, an ambient
// BINDER_VERIFIED_BY DID stamp, and this assertion failed. It now passes.
func TestVerifiedByEnvDoesNotStamp(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	isolateConfig(t)
	t.Setenv("BINDER_VERIFIED_BY", "human:envguy")
	src := mkCorpus(t)
	out := t.TempDir()
	stdout, code := runCLI(t, "convert", src, "-o", out) // env set, NO flag
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if contains(string(b), "verified:") {
		t.Errorf("BINDER_VERIFIED_BY authorized a stamp without --verified-by (owner ruling violated):\n%s", b)
	}
	// The refused env value is disclosed (prose), not silently ignored.
	if !contains(stdout, "ignored BINDER_VERIFIED_BY") || !contains(stdout, "human:envguy") {
		t.Errorf("refused env verifier was not disclosed in the report:\n%s", stdout)
	}
}

// TestVerifiedByEnvRefusalDisclosedInJSON pins that the refused-env disclosure is
// present in the JSON note field too (Residual B: prose AND JSON), matching the
// repo-local shape.
func TestVerifiedByEnvRefusalDisclosedInJSON(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	isolateConfig(t)
	t.Setenv("BINDER_VERIFIED_BY", "human:envguy")
	src := mkCorpus(t)
	out := t.TempDir()
	stdout, code := runCLI(t, "convert", src, "-o", out, "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	var env struct {
		Result struct {
			Verified struct {
				Actor string `json:"actor"`
				Note  string `json:"note"`
			} `json:"verified"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout)
	}
	if env.Result.Verified.Actor != "" {
		t.Errorf("verified.actor = %q, want empty (nothing stamped)", env.Result.Verified.Actor)
	}
	if !contains(env.Result.Verified.Note, "ignored BINDER_VERIFIED_BY") ||
		!contains(env.Result.Verified.Note, "human:envguy") {
		t.Errorf("JSON note did not disclose the refused env verifier: %q", env.Result.Verified.Note)
	}
}

// TestVerifiedByEnvWithExplicitFlagStamps is the NON-VACUITY control for the env
// refusal above: an env value present alongside an EXPLICIT --verified-by must
// still stamp. Without this, the refusal test could pass vacuously (e.g. if env
// were being dropped entirely rather than refused only as a no-flag authority).
func TestVerifiedByEnvWithExplicitFlagStamps(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	isolateConfig(t)
	t.Setenv("BINDER_VERIFIED_BY", "human:envguy")
	src := mkCorpus(t)
	out := t.TempDir()
	_, code := runCLI(t, "convert", src, "-o", out, "--verified-by", "human:envguy")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if !contains(string(b), "by: human:envguy") {
		t.Errorf("env value + explicit --verified-by did not stamp (control would be vacuous):\n%s", b)
	}
}

// TestVerifiedByEnvOutranksRepoLocalStillNoStamp pins the precedence edge case AND
// the shadowing-disclosure fix: env outranks a repo-local ./.binder.yaml in viper
// resolution, so with BOTH present and NO flag, the resolved origin is env — which
// now refuses. The result must be NO stamp; a repo-local value must not sneak a
// stamp through underneath. Critically, setting env must NOT suppress disclosure:
// the refusal is still surfaced (via the env note), never silent — otherwise an
// environment variable would silence a note the repo-local case would have fired.
func TestVerifiedByEnvOutranksRepoLocalStillNoStamp(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".binder.yaml"),
		[]byte("verified_by: human:repoguy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BINDER_VERIFIED_BY", "human:envguy")
	src := mkCorpus(t)
	out := t.TempDir()
	stdout, code := runCLI(t, "convert", src, "-o", out) // env + repo-local, NO flag
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	b, _ := os.ReadFile(filepath.Join(out, "a.md"))
	if contains(string(b), "verified:") {
		t.Errorf("env outranks repo-local but a stamp still slipped through (must be NO stamp):\n%s", b)
	}
	// Shadowing fix: the refusal is disclosed (the env value that actually resolved),
	// so setting env does not silence the disclosure the repo-local case would fire.
	if !contains(stdout, "ignored BINDER_VERIFIED_BY") || !contains(stdout, "human:envguy") {
		t.Errorf("env+repo-local refusal was silent — env suppressed disclosure:\n%s", stdout)
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
