package enrich_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/enrich"
	"github.com/ghchinoy/binder/internal/okf/native"
)

var fixedNow = time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

func opts(src string) enrich.Options {
	return enrich.Options{Codec: native.New(), Version: "0.1.0", Now: fixedNow}
}

// writeFile writes rel (slash path) under root with content, creating dirs.
func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func find(t *testing.T, rep *enrich.Report, path string) enrich.FileResult {
	t.Helper()
	for _, f := range rep.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no file result for %q in report", path)
	return enrich.FileResult{}
}

// TestResidualASkipDoesNotWriteOrReshape proves the never-fabricate-trust ruling
// at the enrich boundary: a NON-explicit (global config) verifier must NOT co-sign a
// document a DIFFERENT identity has already attested. The skip must (a) write
// nothing — the file is byte-identical afterward, including a single-line FLOW
// sequence that a whole-value re-encode would reshape (the PR #115 residual) — and
// (b) be disclosed in the report (Residual B).
func TestResidualASkipDoesNotWriteOrReshape(t *testing.T) {
	src := t.TempDir()
	// All required keys already present, so the ONLY candidate change is the verified
	// stamp; the prior attestation is a compact FLOW sequence by a DIFFERENT identity.
	doc := "---\n" +
		"type: Note\n" +
		"title: A\n" +
		"generated:\n  by: human:me\n  at: '2020-01-01T00:00:00Z'\n" +
		"verified: [{by: human:ahormati, at: '2020-01-01T00:00:00Z'}]\n" +
		"---\n\n# A\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)

	o := opts(src)
	o.VerifiedBy = "human:ghchinoy" // resolved from config-default (NOT explicit)
	o.VerifiedByExplicit = false
	o.VerifiedBySource = "config"

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}

	// (a) No write, no reshape: bytes identical, flow sequence intact.
	if got := read(t, p); got != doc {
		t.Errorf("skip wrote/reshaped the file.\n--- want ---\n%s\n--- got ---\n%s", doc, got)
	}
	if st := find(t, rep, "a.md").Status; st != enrich.StatusUnchanged {
		t.Errorf("status = %q, want unchanged (Residual A skip writes nothing)", st)
	}

	// (b) Disclosed: one skip naming the existing actor, nothing stamped.
	if rep.Verified.NumStamped != 0 {
		t.Errorf("NumStamped = %d, want 0 (must not co-sign)", rep.Verified.NumStamped)
	}
	if rep.Verified.NumSkipped != 1 {
		t.Fatalf("NumSkipped = %d, want 1 (skip must be disclosed)", rep.Verified.NumSkipped)
	}
	if got := rep.Verified.Skipped[0].ExistingActor; got != "human:ahormati" {
		t.Errorf("skipped existing_actor = %q, want human:ahormati", got)
	}
	// Prose disclosure carries the skip too (Residual B, prose AND JSON).
	if prose := rep.String(); !bytes.Contains([]byte(prose), []byte("already attested by human:ahormati")) {
		t.Errorf("prose did not disclose the skip:\n%s", prose)
	}
}

// TestResidualAExplicitCoSignsAndDiscloses is the anti-vacuity control: the SAME
// different-identity document, enriched with an EXPLICIT verifier, DOES co-sign and
// the write is disclosed as a stamp — proving the skip above keys on explicitness.
func TestResidualAExplicitCoSignsAndDiscloses(t *testing.T) {
	src := t.TempDir()
	doc := "---\n" +
		"type: Note\n" +
		"title: A\n" +
		"generated:\n  by: human:me\n  at: '2020-01-01T00:00:00Z'\n" +
		"verified: [{by: human:ahormati, at: '2020-01-01T00:00:00Z'}]\n" +
		"---\n\n# A\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)

	o := opts(src)
	o.VerifiedBy = "human:ghchinoy"
	o.VerifiedByExplicit = true
	o.VerifiedBySource = "flag"

	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verified.NumStamped != 1 {
		t.Fatalf("NumStamped = %d, want 1 (explicit co-sign)", rep.Verified.NumStamped)
	}
	if rep.Verified.NumSkipped != 0 {
		t.Errorf("NumSkipped = %d, want 0 (explicit path must not skip)", rep.Verified.NumSkipped)
	}
	got := read(t, p)
	if !bytes.Contains([]byte(got), []byte("human:ahormati")) {
		t.Error("prior attestation was dropped by the co-sign")
	}
	if !bytes.Contains([]byte(got), []byte("human:ghchinoy")) {
		t.Error("explicit co-sign stamp was not written")
	}
}

// TestInjectsGenerated: a file WITH frontmatter (type+title) but no generated
// gets a generated stamp; the body and existing keys are byte-faithful.
func TestInjectsGenerated(t *testing.T) {
	src := t.TempDir()
	doc := "---\ntype: Note\ntitle: Alpha\n---\n\n# Alpha\n\nBody text.\n"
	p := writeFile(t, src, "a.md", doc)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "a.md")
	if res.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched", res.Status)
	}
	if len(res.Added) != 1 || res.Added[0] != "generated" {
		t.Fatalf("added = %v, want [generated]", res.Added)
	}

	got := read(t, p)
	// Existing keys + body preserved verbatim; generated appended.
	if !bytes.Contains([]byte(got), []byte("type: Note\ntitle: Alpha\n")) {
		t.Errorf("existing keys not preserved:\n%s", got)
	}
	if !bytes.Contains([]byte(got), []byte("# Alpha\n\nBody text.\n")) {
		t.Errorf("body not preserved:\n%s", got)
	}
	if !bytes.Contains([]byte(got), []byte("generated:")) || !bytes.Contains([]byte(got), []byte("by: binder/0.1.0")) {
		t.Errorf("generated stamp missing:\n%s", got)
	}
}

// TestIdempotent: a second run finds every key present → unchanged, and writes
// nothing (verified by mtime + byte identity).
func TestIdempotent(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "a.md", "---\ntype: Note\ntitle: Alpha\n---\n\n# Alpha\n\nBody.\n")

	if _, err := enrich.Enrich(src, opts(src)); err != nil {
		t.Fatal(err)
	}
	after1 := read(t, p)
	info1, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}

	// Small sleep so a spurious rewrite would change mtime detectably.
	time.Sleep(10 * time.Millisecond)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "a.md")
	if res.Status != enrich.StatusUnchanged {
		t.Fatalf("2nd run status = %q, want unchanged", res.Status)
	}
	if rep.NumEnriched != 0 {
		t.Fatalf("2nd run NumEnriched = %d, want 0", rep.NumEnriched)
	}
	after2 := read(t, p)
	if after1 != after2 {
		t.Errorf("2nd run changed file content")
	}
	info2, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("2nd run rewrote the file (mtime changed): no-write invariant violated")
	}
}

// TestUnchangedFileNotWritten: a file already carrying all keys is not written
// on the FIRST run either (no git churn).
func TestUnchangedFileNotWritten(t *testing.T) {
	src := t.TempDir()
	doc := "---\ntype: Note\ntitle: Alpha\ngenerated:\n  by: human:me\n  at: '2020-01-01T00:00:00Z'\n---\n\n# Alpha\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)
	info0, _ := os.Stat(p)
	time.Sleep(10 * time.Millisecond)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	if find(t, rep, "a.md").Status != enrich.StatusUnchanged {
		t.Fatalf("want unchanged")
	}
	if read(t, p) != doc {
		t.Errorf("file with all keys was rewritten")
	}
	info1, _ := os.Stat(p)
	if !info0.ModTime().Equal(info1.ModTime()) {
		t.Errorf("mtime changed on an all-keys-present file")
	}
}

// TestDryRunWritesNothing: --dry-run reports would-enrich but writes nothing.
func TestDryRunWritesNothing(t *testing.T) {
	src := t.TempDir()
	doc := "---\ntype: Note\ntitle: Alpha\n---\n\n# Alpha\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)

	o := opts(src)
	o.DryRun = true
	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "a.md")
	if res.Status != enrich.StatusWouldEnrich {
		t.Fatalf("status = %q, want would-enrich", res.Status)
	}
	if !rep.DryRun {
		t.Error("report DryRun = false")
	}
	if read(t, p) != doc {
		t.Errorf("dry-run wrote to the file:\n%s", read(t, p))
	}
}

// TestNeverClobber: authored type/title/generated are never overwritten.
func TestNeverClobber(t *testing.T) {
	src := t.TempDir()
	doc := "---\ntype: Decision\ntitle: My Title\ngenerated:\n  by: human:alice\n  at: '2019-05-05T00:00:00Z'\n---\n\n# Heading\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	if find(t, rep, "a.md").Status != enrich.StatusUnchanged {
		t.Fatalf("want unchanged (all present)")
	}
	got := read(t, p)
	if got != doc {
		t.Errorf("authored values were clobbered:\n%s", got)
	}
}

// TestNoFrontmatterGetsFreshBlock: a plain-markdown file (no fence) gets a fresh
// valid block with type/title/generated; the body is preserved.
func TestNoFrontmatterGetsFreshBlock(t *testing.T) {
	src := t.TempDir()
	body := "# Getting Started\n\nSome intro.\n"
	p := writeFile(t, src, "getting-started.md", body)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "getting-started.md")
	if res.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched", res.Status)
	}
	want := map[string]bool{"type": true, "title": true, "generated": true}
	for _, k := range res.Added {
		delete(want, k)
	}
	if len(want) != 0 {
		t.Fatalf("added = %v, missing %v", res.Added, want)
	}

	got := read(t, p)
	if !bytes.HasPrefix([]byte(got), []byte("---\n")) {
		t.Errorf("no fresh frontmatter fence:\n%s", got)
	}
	// Title derives from the first H1; body preserved.
	if !bytes.Contains([]byte(got), []byte("title: Getting Started")) {
		t.Errorf("title not derived from H1:\n%s", got)
	}
	if !bytes.Contains([]byte(got), []byte("# Getting Started\n\nSome intro.\n")) {
		t.Errorf("body not preserved:\n%s", got)
	}

	// Re-parse to confirm it is now valid, enrichable-clean (idempotent).
	rep2, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	if find(t, rep2, "getting-started.md").Status != enrich.StatusUnchanged {
		t.Errorf("fresh block not idempotent on re-run")
	}
}

// TestTitleFromHumanizedFilename: no H1 in the body → humanized filename title.
func TestTitleFromHumanizedFilename(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "my-note.md", "Just prose, no heading.\n")

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(src, "my-note.md"))
	if !bytes.Contains([]byte(got), []byte("title: My Note")) {
		t.Errorf("humanized title missing:\n%s", got)
		_ = rep
	}
}

// TestTypeMapPrefix: --type-map assigns type by directory prefix; default-type
// applies elsewhere.
func TestTypeMapPrefix(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "adr/0001.md", "# Decision\n\nBody.\n")
	writeFile(t, src, "notes/x.md", "# Note\n\nBody.\n")

	o := opts(src)
	o.TypeMap = map[string]string{"adr": "Decision"}
	o.DefaultType = "Note"
	if _, err := enrich.Enrich(src, o); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(read(t, filepath.Join(src, "adr", "0001.md"))), []byte("type: Decision")) {
		t.Errorf("type-map prefix not applied to adr/0001.md")
	}
	if !bytes.Contains([]byte(read(t, filepath.Join(src, "notes", "x.md"))), []byte("type: Note")) {
		t.Errorf("default-type not applied to notes/x.md")
	}
}

// TestReservedFilesSkipped: index.md/log.md are never touched or reported.
func TestReservedFilesSkipped(t *testing.T) {
	src := t.TempDir()
	idx := "# Index\n\nlisting.\n"
	pidx := writeFile(t, src, "index.md", idx)
	plog := writeFile(t, src, "log.md", "# Log\n")
	writeFile(t, src, "a.md", "# A\n")

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	if rep.NumFiles != 1 {
		t.Fatalf("NumFiles = %d, want 1 (reserved excluded)", rep.NumFiles)
	}
	for _, f := range rep.Files {
		if f.Path == "index.md" || f.Path == "log.md" {
			t.Errorf("reserved file %q appeared in report", f.Path)
		}
	}
	if read(t, pidx) != idx {
		t.Errorf("index.md was mutated")
	}
	if read(t, plog) != "# Log\n" {
		t.Errorf("log.md was mutated")
	}
}

// TestReportSlicesInitialized: an empty corpus yields an initialized Files slice
// (serializes to [] not null) and zero counts.
func TestReportSlicesInitialized(t *testing.T) {
	src := t.TempDir()
	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files == nil {
		t.Error("Files is nil; want initialized [] for empty-slice JSON policy")
	}
	if rep.Warnings == nil {
		t.Error("Warnings is nil; want initialized [] for empty-slice JSON policy")
	}
	if rep.NumFiles != 0 || rep.NumFindings() != 0 {
		t.Errorf("empty corpus: NumFiles=%d NumFindings=%d, want 0/0", rep.NumFiles, rep.NumFindings())
	}
}

// TestNumFindingsEqualsSkipped keeps the advisory contract explicit.
func TestNumFindingsEqualsSkipped(t *testing.T) {
	rep := &enrich.Report{NumSkipped: 3}
	if rep.NumFindings() != 3 {
		t.Fatalf("NumFindings = %d, want 3 (== NumSkipped)", rep.NumFindings())
	}
}

// TestUnparseableSkippedAndUntouched: a file that opens a fence but whose YAML
// will not parse is skipped + reported (with a reason) and left BYTE-IDENTICAL on
// disk — enrich never mutates what it cannot safely parse.
func TestUnparseableSkippedAndUntouched(t *testing.T) {
	src := t.TempDir()
	bad := "---\ntype: Note\n  : : :\n---\n\n# Bad\n\nBody.\n"
	unterminated := "---\ntype: Note\nno closing fence here\n"
	pbad := writeFile(t, src, "bad.md", bad)
	punt := writeFile(t, src, "unterminated.md", unterminated)
	writeFile(t, src, "good.md", "# Good\n\nBody.\n")

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"bad.md", "unterminated.md"} {
		res := find(t, rep, name)
		if res.Status != enrich.StatusSkipped {
			t.Errorf("%s status = %q, want skipped", name, res.Status)
		}
		if !bytes.Contains([]byte(res.Reason), []byte("unparseable frontmatter")) {
			t.Errorf("%s reason = %q, want unparseable-frontmatter reason", name, res.Reason)
		}
	}
	if rep.NumSkipped != 2 || rep.NumFindings() != 2 {
		t.Errorf("NumSkipped=%d NumFindings=%d, want 2/2", rep.NumSkipped, rep.NumFindings())
	}

	if read(t, pbad) != bad {
		t.Errorf("unparseable bad.md was mutated on disk")
	}
	if read(t, punt) != unterminated {
		t.Errorf("unterminated.md was mutated on disk")
	}
	// The good file was still enriched (skip is per-file, run completes).
	if find(t, rep, "good.md").Status != enrich.StatusEnriched {
		t.Errorf("good.md should have been enriched alongside the skipped files")
	}
}

// TestJSONDeterministic: the report marshals identically across two runs on the
// same input (deterministic, generated.at fixed via opts.Now).
func TestJSONDeterministic(t *testing.T) {
	build := func() []byte {
		src := t.TempDir()
		writeFile(t, src, "a.md", "---\ntype: Note\ntitle: A\n---\n\n# A\n\nBody.\n")
		writeFile(t, src, "b.md", "# B\n\nBody.\n")
		o := opts(src)
		o.DryRun = true // avoid mutating so both runs see identical inputs
		rep, err := enrich.Enrich(src, o)
		if err != nil {
			t.Fatal(err)
		}
		// Zero out the tempdir-specific Src so only the deterministic payload
		// (files, statuses, added keys, counts) is compared.
		rep.Src = ""
		b, err := json.Marshal(rep)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if a, b := build(), build(); !bytes.Equal(a, b) {
		t.Errorf("report JSON not deterministic:\n%s\n---\n%s", a, b)
	}
}

// TestAtomicWriteFailureLeavesOriginalIntact: when the atomic write cannot
// complete (target directory not writable, so the temp file cannot be created),
// Enrich returns an error and the original file is left byte-identical — the
// temp+rename design never corrupts a source file.
func TestAtomicWriteFailureLeavesOriginalIntact(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory mode does not prevent writes")
	}
	src := t.TempDir()
	dir := filepath.Join(src, "ro")
	orig := "---\ntype: Note\ntitle: A\n---\n\n# A\n\nBody.\n" // needs `generated`, so a write is attempted
	p := writeFile(t, dir, "a.md", orig)

	// Make the containing directory read-only so CreateTemp (temp in same dir)
	// fails; restore afterward so t.TempDir cleanup can remove it.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)

	_, err := enrich.Enrich(src, opts(src))
	if err == nil {
		t.Fatalf("expected an IO error when the target dir is not writable")
	}
	// Temporarily restore read access to inspect the original.
	os.Chmod(dir, 0o755)
	if got := read(t, p); got != orig {
		t.Errorf("original file was modified after a failed write:\n%s", got)
	}
}

// TestLifecycleInjectors: --status-map / --stale-after-map inject status and
// stale_after by directory prefix, set-when-absent.
func TestLifecycleInjectors(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "archive/old.md", "---\ntype: Note\ntitle: Old\n---\n\n# Old\n\nBody.\n")

	o := opts(src)
	o.StatusMap = map[string]string{"archive": "deprecated"}
	o.StaleAfterMap = map[string]string{"archive": "+0d"}
	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "archive/old.md")
	if res.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched", res.Status)
	}
	got := read(t, filepath.Join(src, "archive", "old.md"))
	if !bytes.Contains([]byte(got), []byte("status: deprecated")) {
		t.Errorf("status not injected:\n%s", got)
	}
	if !bytes.Contains([]byte(got), []byte("stale_after:")) {
		t.Errorf("stale_after not injected:\n%s", got)
	}
	// generated is the fixed date; +0d stale_after resolves to that date.
	if !bytes.Contains([]byte(got), []byte("stale_after: \"2026-08-15\"")) &&
		!bytes.Contains([]byte(got), []byte("stale_after: 2026-08-15")) {
		t.Errorf("stale_after not resolved to opts.Now date:\n%s", got)
	}
}

// TestVerifiedByAppendsDetectedAndWritten: --verified-by appends a stamp to an
// EXISTING verified list — a value change of an existing key. The value-aware
// change detection must see it and write the file (not report unchanged).
func TestVerifiedByAppendsDetectedAndWritten(t *testing.T) {
	src := t.TempDir()
	doc := "---\ntype: Note\ntitle: A\ngenerated:\n  by: human:me\n  at: '2020-01-01T00:00:00Z'\n" +
		"verified:\n  - by: process:old-bot\n    at: '2020-01-01T00:00:00Z'\n---\n\n# A\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)

	o := opts(src)
	o.VerifiedBy = "human:ghchinoy"
	// Explicit --verified-by is the co-sign path (Residual A permits appending over a
	// different prior identity); the mechanic under test is value-change detection.
	o.VerifiedByExplicit = true
	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "a.md")
	if res.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched (verified append is a change)", res.Status)
	}
	containsVerified := false
	for _, k := range res.Added {
		if k == "verified" {
			containsVerified = true
		}
	}
	if !containsVerified {
		t.Errorf("added = %v, want to include 'verified' (value changed)", res.Added)
	}
	got := read(t, p)
	if !bytes.Contains([]byte(got), []byte("process:old-bot")) {
		t.Errorf("prior verified stamp not preserved:\n%s", got)
	}
	if !bytes.Contains([]byte(got), []byte("human:ghchinoy")) {
		t.Errorf("new verified stamp not appended:\n%s", got)
	}

	// Idempotent: a second run with the same clock dedups → unchanged, no write.
	rep2, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if find(t, rep2, "a.md").Status != enrich.StatusUnchanged {
		t.Errorf("verified append not idempotent on re-run")
	}
}

// TestVerifiedByScalarPreservedAdvisory: a spec-invalid scalar verified value is
// preserved unchanged and surfaced as an advisory finding (never dropped). It
// gates under --strict via NumFindings but the file is not skipped.
func TestVerifiedByScalarPreservedAdvisory(t *testing.T) {
	src := t.TempDir()
	// All required keys present so the ONLY potential change is verified.
	doc := "---\ntype: Note\ntitle: A\ngenerated:\n  by: human:me\n  at: '2020-01-01T00:00:00Z'\n" +
		"verified: reviewed by bob\n---\n\n# A\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)

	o := opts(src)
	o.VerifiedBy = "human:ghchinoy"
	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want 1 preserve-or-advise finding", rep.Warnings)
	}
	if rep.NumFindings() != 1 {
		t.Errorf("NumFindings = %d, want 1 (advisory counts)", rep.NumFindings())
	}
	// The authored scalar is preserved and no stamp appended → file unchanged.
	if find(t, rep, "a.md").Status != enrich.StatusUnchanged {
		t.Errorf("want unchanged (scalar preserved, nothing appended)")
	}
	if read(t, p) != doc {
		t.Errorf("spec-invalid scalar verified value was mutated on disk:\n%s", read(t, p))
	}
}

// TestNoTempFilesLeftBehind: after a successful run no .binder-enrich-*.tmp files
// remain (atomic-write cleanup).
func TestNoTempFilesLeftBehind(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "a.md", "# A\n")
	writeFile(t, src, "sub/b.md", "# B\n")
	if _, err := enrich.Enrich(src, opts(src)); err != nil {
		t.Fatal(err)
	}
	var leftover []string
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && bytes.Contains([]byte(filepath.Base(p)), []byte(".binder-enrich-")) {
			leftover = append(leftover, p)
		}
		return nil
	})
	if len(leftover) != 0 {
		t.Errorf("temp files left behind: %v", leftover)
	}
}

// TestIdempotent_BreaksWhenStampAdvances measures the BOUNDARY of the help claim
// "idempotent (a second run writes nothing)". It holds with no verifier
// (TestIdempotent) and with a verifier under a pinned clock (the second run in
// TestVerifiedByAppendsDetectedAndWritten). It does NOT hold with a live verifier
// and an advancing clock: a `verified` stamp dedups on (by,at), so a later run
// appends a NEW stamp and the second run DOES write. The corrected help states the
// idempotence guarantee conditionally (pin SOURCE_DATE_EPOCH / no stamp advancing).
//
// DEMONSTRATED RED (observed on this head): asserting the old blanket claim — that
// the second run is StatusUnchanged and rewrites nothing — FAILS here; the file
// gains a second stamp. This test pins the measured reality instead.
func TestIdempotent_BreaksWhenStampAdvances(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "a.md", "---\ntype: Note\ntitle: A\n---\n\n# A\n\nBody.\n")

	o1 := opts(src)
	o1.VerifiedBy = "human:alice"
	o1.VerifiedByExplicit = true
	o1.VerifiedBySource = "flag"
	if _, err := enrich.Enrich(src, o1); err != nil {
		t.Fatal(err)
	}
	first := read(t, p)
	if n := strings.Count(first, "human:alice"); n != 1 {
		t.Fatalf("precondition: first run wrote %d stamps, want 1:\n%s", n, first)
	}

	o2 := o1
	o2.Now = fixedNow.Add(time.Hour) // clock advanced → (by,at) differs → appends
	rep, err := enrich.Enrich(src, o2)
	if err != nil {
		t.Fatal(err)
	}
	if res := find(t, rep, "a.md"); res.Status != enrich.StatusEnriched {
		t.Errorf("advancing clock: 2nd run status = %q, want enriched (a new stamp is written)", res.Status)
	}
	second := read(t, p)
	if second == first {
		t.Errorf("advancing clock: 2nd run wrote nothing; expected an appended stamp:\n%s", second)
	}
	if n := strings.Count(second, "human:alice"); n != 2 {
		t.Errorf("expected 2 verified stamps after advancing-clock rerun, got %d:\n%s", n, second)
	}
}

// TestAdditive_AuthorizedStampModifiesPresentVerifiedKey pins the corrected
// "additive/never-clobber" claim. The old wording "only ABSENT keys are added" is
// false: an authorized `verified` stamp is APPENDED to an already-PRESENT `verified`
// list, so a present key IS modified. never-clobber still holds — the prior
// attestation is preserved, never overwritten.
//
// DEMONSTRATED RED (observed on this head): asserting the old claim — that a present
// `verified` key is left unchanged and never appears in Added — FAILS; the key gains
// a second entry and Added lists "verified".
func TestAdditive_AuthorizedStampModifiesPresentVerifiedKey(t *testing.T) {
	src := t.TempDir()
	doc := "---\ntype: Note\ntitle: A\n" +
		"verified:\n  - by: human:alice\n    at: '2020-01-01T00:00:00Z'\n---\n\n# A\n\nBody.\n"
	p := writeFile(t, src, "a.md", doc)

	o := opts(src)
	o.VerifiedBy = "human:alice"
	o.VerifiedByExplicit = true // same actor, later time → appends (not a Residual-A skip)
	o.VerifiedBySource = "flag"
	rep, err := enrich.Enrich(src, o)
	if err != nil {
		t.Fatal(err)
	}
	res := find(t, rep, "a.md")
	// A PRESENT key was modified: the file was WRITTEN (enriched), not left unchanged.
	if res.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched (a PRESENT key was modified)", res.Status)
	}
	got := read(t, p)
	if n := strings.Count(got, "human:alice"); n != 2 {
		t.Errorf("present verified key did not gain a 2nd entry, got %d:\n%s", n, got)
	}
	// never-clobber: the prior attestation survives untouched.
	if !strings.Contains(got, "2020-01-01T00:00:00Z") {
		t.Errorf("prior attestation clobbered:\n%s", got)
	}
	// NOTE: res.Added currently lists "verified" here even though the `verified`
	// key was already PRESENT — the report-envelope symptom of this same behaviour.
	// That is a SEPARATE tracked defect (added: for a modified-not-added key);
	// deliberately NOT asserted here so this test does not cement it as correct.
}

// TestAtomic_ExecutedControls backs the "atomic (temp file + rename)" claim with
// EXECUTION rather than a code read: on a successful enrich the source file's mode
// is preserved, the file is REPLACED by rename (a different underlying file) rather
// than truncated in place, and no temp file is left behind. Replace-by-rename is the
// property that makes an interrupt leave either the old or the new file, never a
// partial one.
func TestAtomic_ExecutedControls(t *testing.T) {
	src := t.TempDir()
	p := writeFile(t, src, "a.md", "---\ntype: Note\n---\n\n# A\n\nBody.\n") // missing title → enriched
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	if res := find(t, rep, "a.md"); res.Status != enrich.StatusEnriched {
		t.Fatalf("precondition: file not enriched (status %q) — control would be vacuous", res.Status)
	}

	after, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// (1) mode preserved across the rewrite.
	if after.Mode().Perm() != 0o640 {
		t.Errorf("mode not preserved across atomic rewrite: got %o, want 640", after.Mode().Perm())
	}
	// (2) replaced by rename, not truncated in place (different underlying file).
	if os.SameFile(before, after) {
		t.Errorf("file written in place (same inode); expected replace-by-rename")
	}
	// (3) no temp litter after a successful write.
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".binder-enrich-") {
			t.Errorf("temp file left behind after a successful write: %s", e.Name())
		}
	}
}
