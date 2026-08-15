package bundle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/okf/native"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadReadsConceptsSkipsReservedAndReadsRootVersion(t *testing.T) {
	root := t.TempDir()
	write(t, root, "index.md", "---\nokf_version: \"0.2\"\n---\n# Bundle\n")
	write(t, root, "intro.md", "---\ntype: Note\ntitle: Intro\n---\n# Intro\nSee [orders](tables/orders.md).\n")
	write(t, root, "tables/orders.md", "---\ntype: BigQuery Table\ntitle: Orders\n---\n# Orders\n")
	write(t, root, "tables/index.md", "# Tables\n") // reserved: not a concept

	b, err := bundle.Load(root, native.New())
	if err != nil {
		t.Fatal(err)
	}
	if b.OKFVersion != "0.2" {
		t.Errorf("OKFVersion = %q, want 0.2", b.OKFVersion)
	}
	if len(b.Concepts) != 2 {
		t.Fatalf("concepts = %d, want 2 (index.md files skipped)", len(b.Concepts))
	}
	// Sorted by RelPath: intro.md, tables/orders.md.
	if b.Concepts[0].ID != "intro" || b.Concepts[1].ID != "tables/orders" {
		t.Errorf("concepts = %q, %q", b.Concepts[0].ID, b.Concepts[1].ID)
	}
	// Links extracted via LinkGraph.
	if len(b.Concepts[0].Links) != 1 || b.Concepts[0].Links[0].TargetID != "tables/orders" {
		t.Errorf("intro links = %+v", b.Concepts[0].Links)
	}
}

func TestLoadNeverRejectsUnparseableConcept(t *testing.T) {
	root := t.TempDir()
	write(t, root, "good.md", "---\ntype: Note\n---\n# Good\n")
	// Broken YAML frontmatter: must be skipped, not fatal.
	write(t, root, "bad.md", "---\ntype: Note\n  : : bad yaml :\n---\n# Bad\n")

	b, err := bundle.Load(root, native.New())
	if err != nil {
		t.Fatalf("Load should not error on unparseable frontmatter: %v", err)
	}
	for _, c := range b.Concepts {
		if c.ID == "bad" {
			// Tolerated either way, but it must not have crashed the load.
			return
		}
	}
	if len(b.Concepts) < 1 {
		t.Fatal("expected the good concept to load")
	}
}

func TestLoadErrorsOnMissingRoot(t *testing.T) {
	if _, err := bundle.Load(filepath.Join(t.TempDir(), "nope"), native.New()); err == nil {
		t.Fatal("expected error for missing root")
	}
}
