package convert_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// writeExternalRootCorpus lays out a corpus whose intro.md links to a doc under
// a SIBLING directory via a file:// URI (outside the corpus/workspace root). It
// returns the corpus root and the absolute sibling directory an author would
// declare with --external-root.
func writeExternalRootCorpus(t *testing.T) (root, sibling string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "corpus")
	sibling = filepath.Join(base, "sibling")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sibling, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	siblingURI := "file://" + filepath.ToSlash(filepath.Join(sibling, "docs", "a.md"))
	intro := "# Intro\n\nCross-repo [away](" + siblingURI + ") link.\n"
	if err := os.WriteFile(filepath.Join(root, "intro.md"), []byte(intro), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "docs", "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, sibling
}

// readBundle reads every file under dir into a path→bytes map for byte-level
// comparison of two conversion runs.
func readBundle(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	got := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		got[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestConvertExternalRootByteIdenticalBundle: guardrail 1 / criterion 4. The
// emitted bundle bytes are IDENTICAL with and without --external-root; the link
// is still external and unrewritten. Only the advisory differs.
func TestConvertExternalRootByteIdenticalBundle(t *testing.T) {
	root, sibling := writeExternalRootCorpus(t)
	now := time.Unix(1700000000, 0).UTC()

	outNoFlag := t.TempDir()
	repNo, err := convert.Convert(root, outNoFlag, convert.Options{
		Codec: native.New(), Version: "0.1.0", Now: now,
	})
	if err != nil {
		t.Fatalf("convert (no flag): %v", err)
	}

	outFlag := t.TempDir()
	repFlag, err := convert.Convert(root, outFlag, convert.Options{
		Codec: native.New(), Version: "0.1.0", Now: now,
		ExternalRoots: []string{sibling},
	})
	if err != nil {
		t.Fatalf("convert (flag): %v", err)
	}

	a := readBundle(t, outNoFlag)
	b := readBundle(t, outFlag)
	if len(a) != len(b) {
		t.Fatalf("bundle file set differs: %d vs %d files", len(a), len(b))
	}
	for name, ab := range a {
		bb, ok := b[name]
		if !ok {
			t.Fatalf("file %q present without flag but missing with flag", name)
		}
		if !bytes.Equal(ab, bb) {
			t.Fatalf("bundle bytes differ for %q with vs without --external-root:\n--- without ---\n%s\n--- with ---\n%s",
				name, ab, bb)
		}
	}

	// The external link must remain verbatim in the emitted body under both runs.
	siblingURI := "file://" + filepath.ToSlash(filepath.Join(sibling, "docs", "a.md"))
	if !bytes.Contains(a["intro.md"], []byte(siblingURI)) {
		t.Fatalf("external link should remain verbatim in the body:\n%s", a["intro.md"])
	}

	// Advisory present WITHOUT the flag, suppressed WITH it (criterion 3).
	if !hasOutsideAdvisory(repNo.Warnings) {
		t.Fatalf("expected outside-root advisory without the flag, got %v", repNo.Warnings)
	}
	if hasOutsideAdvisory(repFlag.Warnings) {
		t.Fatalf("advisory must be suppressed with --external-root, got %v", repFlag.Warnings)
	}
}

// TestConvertExternalRootSuppressedInJSON: criterion 3 — suppression is
// consistent in the JSON envelope too (warnings surface there via the same
// Report).
func TestConvertExternalRootSuppressedInJSON(t *testing.T) {
	root, sibling := writeExternalRootCorpus(t)
	now := time.Unix(1700000000, 0).UTC()

	encode := func(opts convert.Options) string {
		rep, err := convert.Convert(root, t.TempDir(), opts)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		var buf bytes.Buffer
		if err := clijson.Encode(&buf, "0.1.0", "convert", rep); err != nil {
			t.Fatalf("encode json: %v", err)
		}
		return buf.String()
	}

	base := convert.Options{Codec: native.New(), Version: "0.1.0", Now: now}
	jsonNo := encode(base)
	withFlag := base
	withFlag.ExternalRoots = []string{sibling}
	jsonFlag := encode(withFlag)

	if !strings.Contains(jsonNo, "resolves outside the workspace root") {
		t.Fatalf("JSON without flag should carry the advisory:\n%s", jsonNo)
	}
	if strings.Contains(jsonFlag, "resolves outside the workspace root") {
		t.Fatalf("JSON with --external-root must not carry the advisory:\n%s", jsonFlag)
	}
}

// TestConvertExternalRootSegmentSafeE2E: criterion 6 end-to-end — declaring a
// root that is a string-prefix (but not a path-segment prefix) of the link's
// directory must NOT suppress the advisory.
func TestConvertExternalRootSegmentSafeE2E(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Link points under projects/jibo; we declare projects/jib.
	linkURI := "file://" + filepath.ToSlash(filepath.Join(base, "projects", "jibo", "a.md"))
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("[x]("+linkURI+")\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := convert.Convert(root, t.TempDir(), convert.Options{
		Codec: native.New(), Version: "0.1.0", Now: time.Unix(1700000000, 0).UTC(),
		ExternalRoots: []string{filepath.Join(base, "projects", "jib")},
	})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !hasOutsideAdvisory(rep.Warnings) {
		t.Fatalf("/projects/jib must not suppress /projects/jibo/...; expected advisory, got %v", rep.Warnings)
	}
}

// TestConvertExternalRootNonexistentAccepted: criterion 7 — a well-formed but
// nonexistent declared root is accepted (no error) and still suppresses links
// under it. Declared siblings need not exist on the converting machine.
func TestConvertExternalRootNonexistentAccepted(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "corpus")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// The link and the declared root both live under a directory that does NOT
	// exist on disk.
	ghost := filepath.Join(base, "ghost-sibling")
	linkURI := "file://" + filepath.ToSlash(filepath.Join(ghost, "docs", "a.md"))
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("[x]("+linkURI+")\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := convert.Convert(root, t.TempDir(), convert.Options{
		Codec: native.New(), Version: "0.1.0", Now: time.Unix(1700000000, 0).UTC(),
		ExternalRoots: []string{ghost}, // does not exist
	})
	if err != nil {
		t.Fatalf("nonexistent external root must be accepted, got error: %v", err)
	}
	if hasOutsideAdvisory(rep.Warnings) {
		t.Fatalf("nonexistent-but-declared root should still suppress, got %v", rep.Warnings)
	}
}

func hasOutsideAdvisory(warnings []string) bool {
	for _, w := range warnings {
		if strings.Contains(w, "resolves outside the workspace root") {
			return true
		}
	}
	return false
}
