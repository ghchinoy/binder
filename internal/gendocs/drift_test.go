package gendocs

import (
	"os"
	"path/filepath"
	"testing"
)

// committedDir is the checked-in command reference, relative to this test's
// package directory (internal/gendocs).
const committedDir = "../../docs/commands"

// TestCommandReference_NoDrift regenerates the command reference into a temp
// dir and asserts byte-equality with the committed docs/commands/. A new flag
// or command that has not been regenerated (via `make docs`) fails here — and
// therefore in `make check` / CI (`go test ./...`). This makes the generated
// reference part of binder's single-source-of-truth guarantee: the Cobra
// command tree is the source, docs/commands/ is its checked-in projection.
func TestCommandReference_NoDrift(t *testing.T) {
	tmp := t.TempDir()
	if err := Generate(tmp); err != nil {
		t.Fatalf("Generate into temp dir: %v", err)
	}

	got := readTree(t, tmp)
	want := readTree(t, committedDir)

	// Report files present on only one side first — that is the shape of a
	// drift caused by an added or removed command.
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("generated file %q is not committed; run `make docs`", name)
		}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("committed file %q is no longer generated; run `make docs`", name)
		}
	}

	for name, gotBody := range got {
		wantBody, ok := want[name]
		if !ok {
			continue
		}
		if gotBody != wantBody {
			t.Errorf("docs/commands/%s is out of sync with the command tree; run `make docs` to regenerate", name)
		}
	}
}

// TestGenerate_Idempotent asserts a second generation over an already-generated
// directory produces identical bytes, so `make docs` run twice yields no diff.
func TestGenerate_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	if err := Generate(tmp); err != nil {
		t.Fatalf("first Generate: %v", err)
	}
	first := readTree(t, tmp)
	if err := Generate(tmp); err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	second := readTree(t, tmp)

	if len(first) != len(second) {
		t.Fatalf("file count changed on regeneration: %d then %d", len(first), len(second))
	}
	for name, body := range first {
		if second[name] != body {
			t.Errorf("regeneration changed %q; output is not deterministic", name)
		}
	}
}

// readTree reads every *.md file in dir into a map keyed by base filename.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = string(b)
	}
	return out
}
