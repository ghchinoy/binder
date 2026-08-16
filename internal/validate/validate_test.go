package validate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
	"github.com/ghchinoy/binder/internal/validate"
)

const goldenBundles = "../../testdata/okf-bundles"

// TestValidateGoldenBundles is the Phase-1 conformance gate: binder MUST accept
// every spec-provided golden bundle (they are, by construction, valid OKF v0.2).
func TestValidateGoldenBundles(t *testing.T) {
	entries, err := os.ReadDir(goldenBundles)
	if err != nil {
		t.Fatal(err)
	}
	var bundles int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		bundles++
		root := filepath.Join(goldenBundles, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			res, err := validate.Bundle(root, native.New(), okf.SpecV02)
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if !res.Conformant() {
				for _, f := range res.Errors() {
					t.Errorf("unexpected error finding: %s", f)
				}
				t.Fatalf("golden bundle %q must be conformant", e.Name())
			}
			if res.NumConcepts == 0 {
				t.Fatalf("bundle %q reported zero concepts", e.Name())
			}
		})
	}
	if bundles == 0 {
		t.Fatal("no golden bundles found")
	}
}

func writeBundle(t *testing.T, files map[string]string) string {
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

func TestValidateHardFailures(t *testing.T) {
	t.Run("missing type is an error (§11.2)", func(t *testing.T) {
		dir := writeBundle(t, map[string]string{
			"a.md": "---\ntitle: No Type\n---\n\nbody\n",
		})
		res, err := validate.Bundle(dir, native.New(), okf.SpecV02)
		if err != nil {
			t.Fatal(err)
		}
		if res.Conformant() {
			t.Fatal("bundle with a typeless concept must not be conformant")
		}
	})

	t.Run("unparseable frontmatter is an error (§11.1)", func(t *testing.T) {
		dir := writeBundle(t, map[string]string{
			"a.md": "no frontmatter at all\n",
		})
		res, err := validate.Bundle(dir, native.New(), okf.SpecV02)
		if err != nil {
			t.Fatal(err)
		}
		if res.Conformant() {
			t.Fatal("bundle with unparseable frontmatter must not be conformant")
		}
	})
}

// TestReservedStructureUnchecked pins the scope contract for #77 item 1: the
// validator counts reserved files but does not examine their structure, so the
// Result must report that fact (ReservedStructureChecked=false) rather than let
// a `conformant` verdict imply a surface it never inspected. When structural
// validation of reserved files lands (#77 item 2, v0.4.0) this expectation is
// what flips.
func TestReservedStructureUnchecked(t *testing.T) {
	dir := writeBundle(t, map[string]string{
		"a.md":     "---\ntype: concept\ntitle: A\n---\n\nBody\n",
		"index.md": "arbitrary garbage, no structure\n",
		"log.md":   "arbitrary garbage, no structure\n",
	})
	res, err := validate.Bundle(dir, native.New(), okf.SpecV02)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Conformant() {
		t.Fatalf("garbage reserved files must not reject (never-reject): %v", res.Errors())
	}
	if res.NumReserved != 2 {
		t.Fatalf("NumReserved = %d, want 2", res.NumReserved)
	}
	if res.ReservedStructureChecked {
		t.Error("ReservedStructureChecked = true, want false: reserved-file structure is not examined in this release")
	}
}

func TestValidateNeverRejectsOptional(t *testing.T) {
	// Unknown keys, unknown type values, broken links, and total trust absence
	// are all tolerated (spec §11): a minimal but well-formed concept is conformant.
	dir := writeBundle(t, map[string]string{
		"a.md":     "---\ntype: SomethingNovel\nweird_key: 1\n---\n\n[broken](nowhere.md)\n",
		"index.md": "---\nokf_version: \"0.2\"\n---\n\n# Concepts\n",
	})
	res, err := validate.Bundle(dir, native.New(), okf.SpecV02)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Conformant() {
		t.Fatalf("unknown keys/types/broken links must not reject: %v", res.Errors())
	}
}
