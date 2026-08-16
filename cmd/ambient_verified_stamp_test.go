package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// TestCharacterize_AmbientConfigStampsConvertAndEnrich is a CHARACTERIZATION
// test: it records that BOTH `convert` and `enrich` resolve verified_by from an
// ambient GLOBAL user config (XDG_CONFIG_HOME) and write that verified stamp —
// convert into the output bundle, enrich in place into git-trackable source —
// when no --verified-by flag is passed. convert and enrich do NOT share a code
// path (separate commands over internal/convert and internal/enrich), so each
// verb is covered separately here, as is the widened coverage the finding asked
// for: the original report was about enrich, and this pins convert too.
//
// The owner has now RULED on this behaviour (it is no longer under review): a
// GLOBAL home config verified_by IS the user's own decision to set a default, so
// stamping from it without a flag is PERMITTED (config.PermitsStampWithoutFlag).
// A repo-local ./.binder.yaml does NOT satisfy the exception (Option A) and is
// covered separately. This test therefore pins settled, ruled behaviour; its name
// stays non-invariant-shaped so a later policy change reads as a DECISION, not a
// silent regression.
//
// Each verb carries an anti-vacuity control (a "_no_config" subtest): with no
// config the same run writes NO verified stamp, so the positive assertion cannot
// quietly decay into pinning nothing.
func TestCharacterize_AmbientConfigStampsConvertAndEnrich(t *testing.T) {
	// armGlobalConfig points config discovery at a clean XDG dir (optionally
	// carrying verified_by) with no ./.binder.yaml in reach, so the only config in
	// play is the ambient global one.
	armGlobalConfig := func(t *testing.T, verifiedBy string) {
		t.Helper()
		home := t.TempDir()
		t.Chdir(home) // no ./.binder.yaml here — isolates from any repo config
		xdg := filepath.Join(home, "xdg")
		if verifiedBy != "" {
			if err := os.MkdirAll(filepath.Join(xdg, "binder"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(xdg, "binder", "config.yaml"),
				[]byte("verified_by: "+verifiedBy+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("XDG_CONFIG_HOME", xdg)
	}

	read := func(t *testing.T, p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	t.Run("convert", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		armGlobalConfig(t, "human:alice")
		src := mkCorpus(t)
		out := t.TempDir()
		if _, code := runCLI(t, "convert", src, "-o", out); code != clijson.ExitSuccess {
			t.Fatalf("convert exit = %d, want 0", code)
		}
		if got := read(t, filepath.Join(out, "a.md")); !contains(got, "by: human:alice") {
			t.Errorf("convert did not stamp verified_by from ambient config:\n%s", got)
		}
	})

	t.Run("convert_no_config", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		armGlobalConfig(t, "")
		src := mkCorpus(t)
		out := t.TempDir()
		if _, code := runCLI(t, "convert", src, "-o", out); code != clijson.ExitSuccess {
			t.Fatalf("convert exit = %d, want 0", code)
		}
		if got := read(t, filepath.Join(out, "a.md")); contains(got, "verified:") {
			t.Errorf("anti-vacuity: convert stamped with no config (never-auto-stamp):\n%s", got)
		}
	})

	t.Run("enrich", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		armGlobalConfig(t, "human:alice")
		src := t.TempDir()
		p := filepath.Join(src, "a.md")
		mustWrite(t, p, "---\ntype: Note\ntitle: A\n---\n\n# A\n\nBody.\n")
		if _, code := runCLI(t, "enrich", src); code != clijson.ExitSuccess {
			t.Fatalf("enrich exit = %d, want 0", code)
		}
		if got := read(t, p); !contains(got, "by: human:alice") {
			t.Errorf("enrich did not stamp verified_by from ambient config into source:\n%s", got)
		}
	})

	t.Run("enrich_no_config", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		armGlobalConfig(t, "")
		src := t.TempDir()
		p := filepath.Join(src, "a.md")
		mustWrite(t, p, "---\ntype: Note\ntitle: A\n---\n\n# A\n\nBody.\n")
		if _, code := runCLI(t, "enrich", src); code != clijson.ExitSuccess {
			t.Fatalf("enrich exit = %d, want 0", code)
		}
		if got := read(t, p); contains(got, "verified:") {
			t.Errorf("anti-vacuity: enrich stamped with no config:\n%s", got)
		}
	})
}
