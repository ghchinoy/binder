package convert_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// writeCorpus lays out a small corpus under a fresh temp dir and returns its
// absolute root. intro.md links to docs/doc.md via a file:// URI built from the
// corpus's own absolute path (as an IDE would emit).
func writeFileURLCorpus(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	docURI := "file://" + filepath.ToSlash(filepath.Join(root, "docs", "doc.md"))
	outsideURI := "file://" + filepath.ToSlash(filepath.Join(filepath.Dir(root), "elsewhere.md"))
	intro := "# Intro\n\nInternal [doc](" + docURI + ") and external [away](" + outsideURI + ").\n"
	if err := os.WriteFile(filepath.Join(root, "intro.md"), []byte(intro), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "doc.md"), []byte("# Doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestConvertFileURLResolvesInternalEdge(t *testing.T) {
	root := writeFileURLCorpus(t)
	out := t.TempDir()

	rep, err := convert.Convert(root, out, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	// The internal file:// edge is counted and resolved (fixes links: 0).
	if rep.NumLinks < 1 || rep.NumResolved < 1 {
		t.Fatalf("expected at least one resolved edge, got %+v", rep)
	}

	body, err := os.ReadFile(filepath.Join(out, "intro.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "[doc](/docs/doc.md)") {
		t.Fatalf("internal file:// not rewritten to /docs/doc.md:\n%s", got)
	}
	// Portability: the RESOLVED internal edge must not leak its absolute path.
	docURI := "file://" + filepath.ToSlash(filepath.Join(root, "docs", "doc.md"))
	if strings.Contains(got, docURI) {
		t.Fatalf("resolved edge leaked an absolute machine path:\n%s", got)
	}
	// The outside-root target stays external (left exactly as written) and
	// produced a warning advisory — a broken/external link is tolerated, not
	// rewritten, so its own path legitimately remains.
	if !strings.Contains(got, "elsewhere.md") {
		t.Fatalf("outside-root target should be left external in the body:\n%s", got)
	}
	if len(rep.Warnings) == 0 {
		t.Fatalf("outside-root file:// should record a warning advisory")
	}
}

func TestConvertFileURLDeterministic(t *testing.T) {
	root := writeFileURLCorpus(t)
	now := time.Unix(1700000000, 0).UTC()

	run := func() string {
		out := t.TempDir()
		if _, err := convert.Convert(root, out, convert.Options{
			Codec:   native.New(),
			Version: "0.1.0",
			Now:     now,
		}); err != nil {
			t.Fatalf("convert: %v", err)
		}
		b, err := os.ReadFile(filepath.Join(out, "intro.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	if a, b := run(), run(); a != b {
		t.Fatalf("two runs must be byte-identical:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

func TestConvertFileURLOutsideRootExit(t *testing.T) {
	// A file:// target outside the root must not fail the conversion (exit 0).
	root := t.TempDir()
	outsideURI := "file://" + filepath.ToSlash(filepath.Join(filepath.Dir(root), "gone.md"))
	if err := os.WriteFile(filepath.Join(root, "a.md"),
		[]byte("[x]("+outsideURI+")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := convert.Convert(root, t.TempDir(), convert.Options{
		Codec: native.New(), Version: "0.1.0", Now: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("outside-root file:// must not error: %v", err)
	}
	if len(rep.Warnings) == 0 {
		t.Fatalf("expected a warning advisory for the outside-root target")
	}
}

func TestConvertWorkspaceRootWidensBoundary(t *testing.T) {
	// With --workspace-root set to the corpus parent, a file:// under the parent
	// but outside the corpus is inside the boundary; since it is not a corpus
	// concept it is a tolerated unresolved edge rather than an external link.
	parent := t.TempDir()
	root := filepath.Join(parent, "corpus")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	siblingURI := "file://" + filepath.ToSlash(filepath.Join(parent, "sibling.md"))
	if err := os.WriteFile(filepath.Join(root, "a.md"),
		[]byte("[s]("+siblingURI+")\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := convert.Convert(root, t.TempDir(), convert.Options{
		Codec: native.New(), Version: "0.1.0",
		Now:           time.Unix(1700000000, 0).UTC(),
		WorkspaceRoot: parent,
	})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if rep.NumUnresolved < 1 {
		t.Fatalf("in-boundary non-concept should be a recorded unresolved edge, got %+v", rep)
	}
}
