package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// Reach coverage for issue #123 — which config origins REACH the codec write path
// (and therefore the lone-CR destroy), and which are gated out before they can.
//
// The origin-authorization matrix itself (flag writes; global config writes but
// skips a different identity; repo-local and BINDER_VERIFIED_BY never write) is
// already pinned on LF corpora by verifiedby_test.go and ambient_verified_stamp_
// test.go. What those do NOT pin is the #123 INTERSECTION: on a lone-CR fixture a
// WRITING origin silently DESTROYS pre-existing keys, while a NON-writing origin
// never touches the file so it cannot destroy. These tests pin exactly that
// intersection, end to end through the real CLI (resolveVerifiedBy origin
// resolution + the native codec write).
//
// CLOCK DISCIPLINE (brief's warning made visible per test): a stamp that
// deduplicates writes nothing, leaves every key intact, and would make a
// destroy-proving test pass while proving nothing. SOURCE_DATE_EPOCH is therefore
// pinned EXPLICITLY in every writing test, to a value chosen so the new (by,at)
// CANNOT dedup against the fixture's existing stamp, and each writing test asserts
// the fresh stamp actually landed (anti-vacuity) BEFORE asserting survival. Pinning
// is never treated as a mitigation — it is set so the write path is genuinely
// exercised.

// pinnedEpoch is 1700000000 = 2023-11-14T22:13:20Z.
const pinnedEpoch = "1700000000"
const pinnedAt = "2023-11-14T22:13:20Z"

// writeLoneCRDoc writes a crall-shape fixture (LF fences so the block is recognised
// as frontmatter, lone-CR interior so the splice defect is live) with `verified` as
// the FIRST key carrying one existing stamp, followed by three ordinary keys. It
// returns the containing dir and the file path. The trailing keys are what a
// first-key rewrite destroys.
func writeLoneCRDoc(t *testing.T, dir, existingBy, existingAt string) string {
	t.Helper()
	// `generated` is present so enrich's set-when-absent StampGenerated has nothing to
	// add — a refused verified origin then produces a genuine no-write (matches the
	// §A reach matrix, whose fixtures also carried generated).
	doc := "---\n" +
		"verified:\r- {by: " + existingBy + ", at: '" + existingAt + "'}\r" +
		"type: Metric\r" +
		"title: Alpha\r" +
		"generated:\r  by: binder/0.3.0\r  at: '2020-01-01T00:00:00Z'\r" +
		"canary_last: KEEPME\n" +
		"---\n# Alpha\n"
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeLFDoc is the byte-for-byte LF counterpart of writeLoneCRDoc (the necessary-
// condition control): identical keys and stamp, pure LF interior, no lone CR.
func writeLFDoc(t *testing.T, dir, existingBy, existingAt string) string {
	t.Helper()
	doc := "---\n" +
		"verified:\n  - {by: " + existingBy + ", at: '" + existingAt + "'}\n" +
		"type: Metric\n" +
		"title: Alpha\n" +
		"generated:\n  by: binder/0.3.0\n  at: '2020-01-01T00:00:00Z'\n" +
		"canary_last: KEEPME\n" +
		"---\n# Alpha\n"
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertKeysSurvive is the survival assertion shared by the destroy-proving tests.
// It reports every lost key rather than stopping at the first.
func assertKeysSurvive(t *testing.T, got string) {
	t.Helper()
	for _, k := range []string{"type: Metric", "title: Alpha", "canary_last: KEEPME"} {
		if !contains(got, k) {
			t.Errorf("pre-existing key %q destroyed by a lone-CR write:\n%q", k, got)
		}
	}
}

// --- WRITING origins: they REACH the defect and DESTROY today ---

// TestReach_FlagCoSignDifferentIdentity_DestroysOnLoneCR — the flag origin writes
// and co-signs even a different identity, so it reaches the first-key rewrite on a
// lone-CR file and destroys the trailing keys.
//
// CLOCK: pinned, but the pin is irrelevant to whether this writes — a co-sign of a
// DIFFERENT identity ALWAYS appends a fresh stamp (there is no same-(by,at) to dedup
// against), so this cell destroys under any clock. That is exactly why it is the
// safest writing-destroy proof: it cannot vacuously pass via dedup.
//
// FAILS TODAY: trailing keys are gone after the co-sign stamp is written.
func TestReach_FlagCoSignDifferentIdentity_DestroysOnLoneCR(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", pinnedEpoch)
	isolateConfig(t)
	src := t.TempDir()
	p := writeLoneCRDoc(t, src, "human:bob", "2024-01-01T00:00:00Z")

	_, code := runCLI(t, "enrich", src, "--verified-by", "human:alice")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := readFile(t, p)

	// Anti-vacuity: the co-sign actually wrote a fresh alice stamp (the write path ran).
	if !contains(got, "human:alice") {
		t.Fatalf("co-sign stamp was not written — the write path was not exercised:\n%q", got)
	}
	assertKeysSurvive(t, got)
}

// TestReach_GlobalConfigSameIdentity_DestroysOnLoneCR — a global (home) config
// verified_by writes without a flag (the user's own default), so it too reaches the
// first-key rewrite and destroys.
//
// CLOCK: pinned to 2023-11-14T22:13:20Z, which is DELIBERATELY DIFFERENT from the
// fixture's existing stamp at 2024-01-01T00:00:00Z. A pin EQUAL to the existing at
// would dedup, write nothing, leave every key intact, and pass this test while
// proving nothing — the exact trap the brief warns about. The anti-vacuity check
// below (the fresh at must be present) fails loudly if a dedup ever sneaks in.
//
// FAILS TODAY: trailing keys are gone after the same-identity stamp is written.
func TestReach_GlobalConfigSameIdentity_DestroysOnLoneCR(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", pinnedEpoch)
	dir := isolateConfig(t)
	gdir := filepath.Join(dir, "xdg", "binder")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gdir, "config.yaml"),
		[]byte("verified_by: human:alice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	p := writeLoneCRDoc(t, src, "human:alice", "2024-01-01T00:00:00Z") // same identity, DIFFERENT at

	_, code := runCLI(t, "enrich", src) // no flag → global config default
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := readFile(t, p)

	// Anti-vacuity: a SECOND, freshly-timestamped stamp was written (no dedup, no
	// refusal). If this fails, the clock deduped and the test proved nothing.
	if !contains(got, pinnedAt) {
		t.Fatalf("no fresh stamp at %s — the write path deduped or refused, so this "+
			"test did not exercise the destroy (clock trap):\n%q", pinnedAt, got)
	}
	assertKeysSurvive(t, got)
}

// --- NON-writing origins: the reach gate stops them BEFORE the defect ---
// These PASS today and must keep passing after the fix: a refused/skipped origin
// never writes, so the lone-CR file is left byte-intact and nothing is destroyed.
// They are the load-bearing controls that prove the destroy is GATED by origin, not
// merely by the presence of a lone CR.

// TestReach_RepoLocalConfig_NoWrite_NoDestroy — a repo-local ./.binder.yaml is not
// authorised (Option A), so no stamp is written and the lone-CR file is untouched.
func TestReach_RepoLocalConfig_NoWrite_NoDestroy(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", pinnedEpoch)
	dir := isolateConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".binder.yaml"),
		[]byte("verified_by: process:ci-bot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	p := writeLoneCRDoc(t, src, "human:bob", "2024-01-01T00:00:00Z")
	before := readFile(t, p)

	stdout, code := runCLI(t, "enrich", src) // repo-local config only, no flag
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	// The reach gate refused: no ci-bot stamp, and the file is byte-identical.
	if contains(readFile(t, p), "process:ci-bot") {
		t.Errorf("repo-local config reached the write path (Option A violated)")
	}
	if readFile(t, p) != before {
		t.Errorf("unauthorised origin still rewrote the file:\nbefore:\n%q\nafter:\n%q", before, readFile(t, p))
	}
	if !contains(stdout, "ignored repo-local") {
		t.Errorf("refused repo-local origin was not disclosed:\n%s", stdout)
	}
}

// TestReach_EnvVar_NoWrite_NoDestroy — BINDER_VERIFIED_BY is not authorised (owner
// ruling), so no stamp is written and the lone-CR file is untouched.
func TestReach_EnvVar_NoWrite_NoDestroy(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", pinnedEpoch)
	isolateConfig(t)
	t.Setenv("BINDER_VERIFIED_BY", "human:envguy")
	src := t.TempDir()
	p := writeLoneCRDoc(t, src, "human:bob", "2024-01-01T00:00:00Z")
	before := readFile(t, p)

	stdout, code := runCLI(t, "enrich", src) // env set, no flag
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	if contains(readFile(t, p), "human:envguy") {
		t.Errorf("BINDER_VERIFIED_BY reached the write path (owner ruling violated)")
	}
	if readFile(t, p) != before {
		t.Errorf("unauthorised env origin still rewrote the file:\nbefore:\n%q\nafter:\n%q", before, readFile(t, p))
	}
	if !contains(stdout, "ignored BINDER_VERIFIED_BY") {
		t.Errorf("refused env origin was not disclosed:\n%s", stdout)
	}
}

// TestReach_GlobalConfigDifferentIdentity_Skips_NoDestroy — a global config default
// must NOT co-sign a document a DIFFERENT identity already attested (Residual A
// skip). The skip means no write, so the lone-CR file is untouched — a non-writing
// origin cannot destroy even though it is otherwise a writing origin.
func TestReach_GlobalConfigDifferentIdentity_Skips_NoDestroy(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", pinnedEpoch)
	dir := isolateConfig(t)
	gdir := filepath.Join(dir, "xdg", "binder")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gdir, "config.yaml"),
		[]byte("verified_by: human:alice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	p := writeLoneCRDoc(t, src, "human:bob", "2024-01-01T00:00:00Z") // DIFFERENT identity already attests
	before := readFile(t, p)

	stdout, code := runCLI(t, "enrich", src) // global config default, different identity present
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0 (a skip is not a reject)", code)
	}
	after := readFile(t, p)
	if contains(after, "human:alice") {
		t.Errorf("global config co-signed a different identity (Residual A violated)")
	}
	if after != before {
		t.Errorf("a skip still rewrote the file:\nbefore:\n%q\nafter:\n%q", before, after)
	}
	if !contains(stdout, "skipped") || !contains(stdout, "human:bob") {
		t.Errorf("Residual A skip was not disclosed:\n%s", stdout)
	}
}

// --- The necessary-condition control: same writing op, LF instead of lone CR ---

// TestReach_FlagCoSign_LFControl_SafeWrite is the anti-vacuity twin of the flag
// destroy test: the SAME co-sign operation over the byte-identical LF fixture must
// SAFE-WRITE — stamp appended, every pre-existing key intact. It proves the destroy
// in TestReach_FlagCoSignDifferentIdentity_DestroysOnLoneCR is caused by the lone CR
// and not by the co-sign operation itself. GREEN TODAY and after the fix.
func TestReach_FlagCoSign_LFControl_SafeWrite(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", pinnedEpoch)
	isolateConfig(t)
	src := t.TempDir()
	p := writeLFDoc(t, src, "human:bob", "2024-01-01T00:00:00Z")

	_, code := runCLI(t, "enrich", src, "--verified-by", "human:alice")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := readFile(t, p)
	if !contains(got, "human:alice") {
		t.Fatalf("control did not write the co-sign stamp — would be vacuous:\n%q", got)
	}
	assertKeysSurvive(t, got) // LF: all keys survive today
}
