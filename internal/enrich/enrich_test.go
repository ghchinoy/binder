package enrich_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
