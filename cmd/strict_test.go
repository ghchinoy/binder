package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/pkg/clijson"
)

// writeTree writes rel→content files under a fresh temp dir and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestValidateStrict: a trust advisory gates only under --strict; a hard
// non-conformance gates regardless.
func TestValidateStrict(t *testing.T) {
	// Conformant (has type) but carries a trust advisory (bad status value).
	advisory := writeTree(t, map[string]string{
		"a.md": "---\ntype: Note\nstatus: bogus\n---\n# A\n",
	})
	if _, code := runCLI(t, "validate", advisory); code != clijson.ExitSuccess {
		t.Errorf("advisory without --strict: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "validate", advisory, "--strict"); code != clijson.ExitFindings {
		t.Errorf("advisory with --strict: exit = %d, want 1", code)
	}

	// Hard non-conformance (missing type) gates with OR without --strict.
	hard := writeTree(t, map[string]string{
		"a.md": "---\ntitle: no type here\n---\n# A\n",
	})
	if _, code := runCLI(t, "validate", hard); code != clijson.ExitFindings {
		t.Errorf("hard non-conformance without --strict: exit = %d, want 1", code)
	}
	if _, code := runCLI(t, "validate", hard, "--strict"); code != clijson.ExitFindings {
		t.Errorf("hard non-conformance with --strict: exit = %d, want 1", code)
	}
}

// TestReviewStrict: a review finding (orphan) gates only under --strict.
func TestReviewStrict(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	bundle := writeTree(t, map[string]string{
		"a.md": "---\ntype: Note\n---\n# A\n",
	})
	if _, code := runCLI(t, "review", bundle); code != clijson.ExitSuccess {
		t.Errorf("finding without --strict: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "review", bundle, "--strict"); code != clijson.ExitFindings {
		t.Errorf("finding with --strict: exit = %d, want 1", code)
	}
}

// TestReviewEntrypointDoesNotGateStrict: an entrypoint (issue #24) is advisory
// signal, not a gating finding — a corpus whose only "no inbound" node is a root
// README that links out passes --strict, whereas a true orphan still gates.
func TestReviewEntrypointDoesNotGateStrict(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	// README links out to guide; guide is linked-to. No orphans, so nothing gates.
	entrypoint := writeTree(t, map[string]string{
		"README.md": "---\ntype: Note\ntitle: Home\n---\n# Home\n\nSee the [guide](guide.md).\n",
		"guide.md":  "---\ntype: Guide\ntitle: Guide\n---\n# Guide\n",
	})
	if _, code := runCLI(t, "review", entrypoint); code != clijson.ExitSuccess {
		t.Errorf("entrypoint corpus bare: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "review", entrypoint, "--strict"); code != clijson.ExitSuccess {
		t.Errorf("entrypoint must not gate under --strict: exit = %d, want 0", code)
	}

	// A true orphan (no inbound AND no outbound) still gates under --strict.
	orphan := writeTree(t, map[string]string{
		"lonely.md": "---\ntype: Note\ntitle: Lonely\n---\n# Lonely\n",
	})
	if _, code := runCLI(t, "review", orphan, "--strict"); code != clijson.ExitFindings {
		t.Errorf("true orphan must still gate under --strict: exit = %d, want 1", code)
	}
}

// TestLintEntrypointDoesNotGateStrict: the same never-reject posture for `binder
// lint` over a SOURCE corpus.
func TestLintEntrypointDoesNotGateStrict(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	entrypoint := writeTree(t, map[string]string{
		"README.md": "---\ntype: Note\ntitle: Home\n---\n# Home\n\nSee the [guide](guide.md).\n",
		"guide.md":  "---\ntype: Guide\ntitle: Guide\n---\n# Guide\n",
	})
	if _, code := runCLI(t, "lint", entrypoint, "--strict"); code != clijson.ExitSuccess {
		t.Errorf("entrypoint corpus must not gate lint --strict: exit = %d, want 0", code)
	}

	// A true orphan still gates.
	orphan := writeTree(t, map[string]string{
		"lonely.md": "---\ntype: Note\ntitle: Lonely\n---\n# Lonely\n",
	})
	if _, code := runCLI(t, "lint", orphan, "--strict"); code != clijson.ExitFindings {
		t.Errorf("true orphan must still gate lint --strict: exit = %d, want 1", code)
	}

	// The designation flag turns that orphan into a non-gating entrypoint.
	if _, code := runCLI(t, "lint", orphan, "--strict", "--entrypoint", "lonely"); code != clijson.ExitSuccess {
		t.Errorf("designated entrypoint must not gate lint --strict: exit = %d, want 0", code)
	}
}

// TestConvertStrict: unresolved links gate only under --strict.
func TestConvertStrict(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	corpus := writeTree(t, map[string]string{
		"a.md": "# A\n\n[missing](./nope.md)\n",
	})
	if _, code := runCLI(t, "convert", corpus, "--dry-run"); code != clijson.ExitSuccess {
		t.Errorf("unresolved without --strict: exit = %d, want 0", code)
	}
	if _, code := runCLI(t, "convert", corpus, "--dry-run", "--strict"); code != clijson.ExitFindings {
		t.Errorf("unresolved with --strict: exit = %d, want 1", code)
	}

	// A clean corpus does not gate even under --strict.
	if _, code := runCLI(t, "convert", "../testdata/corpus-clean", "--dry-run", "--strict"); code != clijson.ExitSuccess {
		t.Errorf("clean corpus with --strict: exit = %d, want 0", code)
	}
}

// TestReviewRequireTierShortfallReasons: --require-tier accepts all three spec
// tier names ("unverified", "machine-confirmed", "human-reviewed") and names why
// each concept fell short ("no verified entry" vs. "verified only by non-human
// actor(s)") in both prose and --json (issues #243, #247).
func TestReviewRequireTierShortfallReasons(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	bundle := writeTree(t, map[string]string{
		"README.md":  "---\ntype: Note\ntitle: Home\nverified:\n  - by: human:alice\n    at: \"2026-01-01T00:00:00Z\"\n---\n# Home\n\nSee [orders](orders.md) and [scratch](scratch.md).\n",
		"orders.md":  "---\ntype: Table\ntitle: Orders\nverified:\n  - by: binder/0.8.0\n    at: \"2026-01-01T00:00:00Z\"\n---\n# Orders\n",
		"scratch.md": "---\ntype: Note\ntitle: Scratch\n---\n# Scratch\n",
	})

	// "unverified" is a valid spec tier and passes (exit 0).
	if out, code := runCLI(t, "review", bundle, "--require-tier", "unverified"); code != clijson.ExitSuccess {
		t.Errorf("require-tier unverified: exit = %d, want 0; out:\n%s", code, out)
	}

	// "machine-confirmed" fails only on scratch (no verified entry).
	outMC, codeMC := runCLI(t, "review", bundle, "--require-tier", "machine-confirmed")
	if codeMC != clijson.ExitFindings {
		t.Errorf("require-tier machine-confirmed: exit = %d, want 1", codeMC)
	}
	if !strings.Contains(outMC, "scratch (unverified: no verified entry)") {
		t.Errorf("expected scratch shortfall reason in output, got:\n%s", outMC)
	}
	if strings.Contains(outMC, "orders (machine-confirmed") {
		t.Errorf("orders meets machine-confirmed and must not be listed in shortfalls:\n%s", outMC)
	}

	// "human-reviewed" fails on both orders (non-human actor) and scratch (no verified entry).
	outHR, codeHR := runCLI(t, "review", bundle, "--require-tier", "human-reviewed")
	if codeHR != clijson.ExitFindings {
		t.Errorf("require-tier human-reviewed: exit = %d, want 1", codeHR)
	}
	for _, want := range []string{
		"below required tier (human-reviewed): 2",
		"orders (machine-confirmed: verified only by non-human actor(s): binder/0.8.0)",
		"scratch (unverified: no verified entry)",
	} {
		if !strings.Contains(outHR, want) {
			t.Errorf("require-tier human-reviewed output missing %q:\n%s", want, outHR)
		}
	}
}
