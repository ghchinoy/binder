package convert_test

import (
	"crypto/sha256"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf/native"
)

var update = flag.Bool("update", false, "regenerate the expected-basic golden fixture")

const (
	corpusBasic   = "../../testdata/corpus-basic"
	expectedBasic = "../../testdata/expected-basic"
)

// fixedNow makes generated.at deterministic; matches SOURCE_DATE_EPOCH=1700000000.
var fixedNow = time.Unix(1700000000, 0).UTC()

func convertBasic(t *testing.T, out string) *convert.Report {
	t.Helper()
	rep, err := convert.Convert(corpusBasic, out, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNow,
	})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	return rep
}

// TestConvertGolden converts corpus-basic and compares the whole output tree to
// the committed expected-basic fixture, byte for byte. Run with -update to
// regenerate the fixture after an intentional change.
func TestConvertGolden(t *testing.T) {
	if *update {
		if err := os.RemoveAll(expectedBasic); err != nil {
			t.Fatal(err)
		}
		convertBasic(t, expectedBasic)
		t.Logf("regenerated golden fixture at %s", expectedBasic)
		return
	}

	out := t.TempDir()
	convertBasic(t, out)

	got := readTree(t, out)
	want := readTree(t, expectedBasic)

	for _, name := range union(keys(got), keys(want)) {
		g, gok := got[name]
		w, wok := want[name]
		switch {
		case !wok:
			t.Errorf("unexpected output file %q:\n%s", name, g)
		case !gok:
			t.Errorf("missing expected output file %q", name)
		case g != w:
			t.Errorf("file %q differs from golden:\n--- want ---\n%s\n--- got ---\n%s", name, w, g)
		}
	}
}

func TestConvertIsDeterministicAndIdempotent(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	convertBasic(t, a)
	convertBasic(t, b)
	ta, tb := readTree(t, a), readTree(t, b)
	if len(ta) != len(tb) {
		t.Fatalf("different file counts: %d vs %d", len(ta), len(tb))
	}
	for name, ca := range ta {
		if ca != tb[name] {
			t.Errorf("non-deterministic output for %q", name)
		}
	}
}

func TestConvertDoesNotMutateSource(t *testing.T) {
	before := treeHash(t, corpusBasic)
	convertBasic(t, t.TempDir())
	after := treeHash(t, corpusBasic)
	if before != after {
		t.Fatal("convert mutated the source corpus")
	}
}

func TestConvertDryRunWritesNothing(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	rep, err := convert.Convert(corpusBasic, out, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNow,
		DryRun:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.NumConcepts == 0 {
		t.Fatal("dry-run should still report the conversion plan")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote to %s (err=%v)", out, err)
	}
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func treeHash(t *testing.T, root string) string {
	t.Helper()
	tree := readTree(t, root)
	h := sha256.New()
	for _, name := range sortedKeys(tree) {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(tree[name]))
		h.Write([]byte{0})
	}
	return string(h.Sum(nil))
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := keys(m)
	sort.Strings(out)
	return out
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(a, b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
