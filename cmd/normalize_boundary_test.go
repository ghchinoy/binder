package cmd

// AC4 (design §9): the trust tier a human authored survives all the way through
// the repaired output. A BOM-prefixed source carrying a human `verified` block
// is converted (repaired), and then `review` and `validate --strict` run over the
// REPAIRED bundle — the tier they derive must be human-reviewed and the bundle
// must be strictly conformant. Before the #124 fix the BOM demoted the file to a
// synthetic `type: Note` body with no verified block, so review would have
// derived `unverified` — a silent trust downgrade.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

func TestAC4_ReviewAndValidateOnRepairedOutput(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	bom := string([]byte{0xEF, 0xBB, 0xBF})
	src := t.TempDir()
	doc := bom + "---\n" +
		"type: Guide\ntitle: Real Title\nowner: alice\n" +
		"verified:\n  - by: human:ghchinoy\n    at: \"2026-01-01T00:00:00Z\"\n" +
		"---\nbody\n"
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Repair by converting into a bundle.
	out := filepath.Join(t.TempDir(), "bundle")
	if _, code := runCLI(t, "convert", src, "-o", out); code != clijson.ExitSuccess {
		t.Fatalf("convert of BOM source: exit = %d, want 0", code)
	}

	// review derives the tier on the repaired output.
	rev, code := runCLI(t, "review", out)
	if code != clijson.ExitSuccess {
		t.Fatalf("review of repaired bundle: exit = %d, want 0; output:\n%s", code, rev)
	}
	if !strings.Contains(rev, "human-reviewed: 1") {
		t.Errorf("review did not derive the human-reviewed tier from repaired output:\n%s", rev)
	}
	// Positive control: no phantom unverified concept (the #124 demotion would
	// have produced exactly one unverified concept instead).
	if !strings.Contains(rev, "unverified: 0") {
		t.Errorf("expected unverified: 0 (the verified block must survive):\n%s", rev)
	}

	// validate --strict must pass on the repaired, valid-trust output.
	if _, code := runCLI(t, "validate", out, "--strict"); code != clijson.ExitSuccess {
		t.Errorf("validate --strict on repaired bundle: exit = %d, want 0", code)
	}
}

// TestEnrichStrictDoesNotGateOnNormalizationAdvisory pins the issue-#154
// exit-code semantics end to end: a BOM-prefixed file is enriched and its
// read-boundary normalization is disclosed, but the advisory alone no longer
// escalates `enrich --strict` to exit 1. The disclosure must still be printed —
// the advisory stopped gating, it did not stop being reported.
func TestEnrichStrictDoesNotGateOnNormalizationAdvisory(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	bom := string([]byte{0xEF, 0xBB, 0xBF})
	src := t.TempDir()
	doc := bom + "---\ntype: Guide\ntitle: Real Title\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, "enrich", src, "--strict")
	if code != clijson.ExitSuccess {
		t.Fatalf("enrich --strict on a normalized file: exit = %d, want 0; output:\n%s", code, out)
	}
	// Positive control: the normalization really happened and was disclosed, so
	// the exit-0 result is not vacuous.
	if !strings.Contains(out, "input normalized before frontmatter recognition") {
		t.Errorf("the normalization advisory was not reported:\n%s", out)
	}
	if !strings.Contains(out, "stripped-utf8-bom") {
		t.Errorf("the advisory did not name the normalization applied:\n%s", out)
	}
	// convert --strict already exits 0 on the same advisory; that behavior is
	// unchanged and is asserted here so the two surfaces stay aligned.
	bundle := filepath.Join(t.TempDir(), "bundle")
	if _, code := runCLI(t, "convert", src, "-o", bundle, "--strict"); code != clijson.ExitSuccess {
		t.Errorf("convert --strict on a normalized file: exit = %d, want 0 (unchanged)", code)
	}
}

// TestEnrichStrictMessageNamesRealQuantities is the issue-#154 regression guard
// on the wording: the gate line reported the FINDINGS total as "skipped N
// file(s)", so a run with zero skips announced skips that never happened. The
// skipped-file count and the warning count are now named separately, and each
// must match the report body.
func TestEnrichStrictMessageNamesRealQuantities(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	src := t.TempDir()
	// Opens a fence but the YAML is invalid → unparseable → skipped (a genuine
	// finding, so --strict must still gate).
	if err := os.WriteFile(filepath.Join(src, "bad.md"),
		[]byte("---\ntype: Note\n  : : :\n---\n\n# Bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The gate message is carried on the returned error (the root silences it and
	// main prints it), so assert on the error text.
	err := runCLIErr(t, "enrich", src, "--strict")
	if clijson.ExitCode(err) != clijson.ExitFindings {
		t.Fatalf("exit = %d, want %d (a real skip still gates); err = %v",
			clijson.ExitCode(err), clijson.ExitFindings, err)
	}
	const want = "enrich skipped 1 file(s) and raised 0 warning(s) (--strict)"
	if err.Error() != want {
		t.Errorf("gate message = %q, want %q", err.Error(), want)
	}
	// The message must agree with the report body it summarizes.
	out, _ := runCLI(t, "enrich", src)
	if !strings.Contains(out, "1 file(s): 0 enriched, 0 unchanged, 1 skipped") {
		t.Errorf("report body does not corroborate the gate message:\n%s", out)
	}
}

// TestEnrichStrictMessageCountsWarningsSeparately covers the other half of the
// #154 wording fix: a preserve-or-advise warning with ZERO skipped files gates,
// and the message says so instead of claiming a skipped file.
func TestEnrichStrictMessageCountsWarningsSeparately(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	src := t.TempDir()
	// A spec-invalid scalar `verified` value: preserved unchanged and reported as
	// a warning (issue #7), never a skip.
	doc := "---\ntype: Note\ntitle: A\ngenerated:\n  by: human:me\n  at: '2020-01-01T00:00:00Z'\n" +
		"verified: reviewed by bob\n---\n\n# A\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(src, "a.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runCLIErr(t, "enrich", src, "--verified-by", "human:ghchinoy", "--strict")
	if clijson.ExitCode(err) != clijson.ExitFindings {
		t.Fatalf("exit = %d, want %d (a warning still gates); err = %v",
			clijson.ExitCode(err), clijson.ExitFindings, err)
	}
	const want = "enrich skipped 0 file(s) and raised 1 warning(s) (--strict)"
	if err.Error() != want {
		t.Errorf("gate message = %q, want %q", err.Error(), want)
	}
}
