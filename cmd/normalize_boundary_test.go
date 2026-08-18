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
