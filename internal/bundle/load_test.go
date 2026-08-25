package bundle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/okf"
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

// badYAML is a frontmatter block that no YAML parser accepts (an unquoted plain
// scalar containing ": "). It is the same defect shape as
// testdata/corpus-lint-schema/badyaml.md, inlined so the bundle package has no
// cross-tree fixture dependency.
const badYAML = "---\ntitle: thing: with an unquoted colon\ngoal: another: bad line\n---\n\n# Bad YAML\n\nLink to [good](good.md).\n"

// TestLoadRecoversAndDisclosesUnparseableConcept is the #161 negative fixture at
// the loader boundary: a file whose frontmatter will not parse is NOT dropped —
// it is recovered as a body-only concept (so it stays in the node set) AND
// recorded in Bundle.Unparsed (so every read-side surface can disclose it). The
// pre-fix loader did a bare `continue`, so the concept vanished with no record;
// this test fails against that loader (the concept is absent and Unparsed is
// empty) and passes after.
func TestLoadRecoversAndDisclosesUnparseableConcept(t *testing.T) {
	root := t.TempDir()
	write(t, root, "good.md", "---\ntype: Note\ntitle: Good\n---\n# Good\n")
	write(t, root, "bad.md", badYAML)

	b, err := bundle.Load(root, native.New())
	if err != nil {
		t.Fatalf("Load must not error on unparseable frontmatter (never-reject): %v", err)
	}

	// The bad concept is recovered into the node set, not dropped.
	var bad *okf.Concept
	for _, c := range b.Concepts {
		if c.ID == "bad" {
			bad = c
		}
	}
	if bad == nil {
		t.Fatalf("bad.md must be recovered into b.Concepts, not dropped; got %d concepts", len(b.Concepts))
	}
	// Recovered concept carries the same marker `binder convert` stamps, so review
	// reports it uniformly.
	if !okf.IsRecovered(bad.Frontmatter) {
		t.Errorf("recovered concept must carry the recovery marker so review discloses it")
	}
	// The raw text (fence and all) is preserved as body under never-reject.
	if !strings.Contains(bad.Body, "thing: with an unquoted colon") {
		t.Errorf("recovered concept must preserve the original text as body; got %q", bad.Body)
	}

	// The drop is disclosed on the bundle with the path and the parse error.
	if len(b.Unparsed) != 1 {
		t.Fatalf("Bundle.Unparsed = %d entries, want 1", len(b.Unparsed))
	}
	u := b.Unparsed[0]
	if u.RelPath != "bad.md" || u.ID != "bad" {
		t.Errorf("Unparsed[0] = %+v, want RelPath=bad.md ID=bad", u)
	}
	if u.Err == "" {
		t.Errorf("Unparsed[0].Err must carry the codec parse error, got empty")
	}
}

// TestRootVersionNotAdoptedFromInvalidFrontmatter is the #163 negative fixture:
// an index.md whose frontmatter is invalid YAML must NOT contribute an
// okf_version — the pre-fix line-prefix scrape adopted "v0.9-bogus" from a
// document it never parsed. The default version stands and the drop is disclosed.
func TestRootVersionNotAdoptedFromInvalidFrontmatter(t *testing.T) {
	root := t.TempDir()
	write(t, root, "good.md", "---\ntype: Note\ntitle: Good\n---\n# Good\n")
	// Invalid YAML frontmatter (unquoted colon) carrying a bogus version.
	write(t, root, "index.md", "---\ntitle: Index: with a colon\nokf_version: v0.9-bogus\n---\n\n# Index\n")

	b, err := bundle.Load(root, native.New())
	if err != nil {
		t.Fatal(err)
	}
	if b.OKFVersion == "v0.9-bogus" {
		t.Errorf("okf_version was adopted from unparseable frontmatter (#163); want the default %q", okf.DefaultSpecVersion)
	}
	if b.OKFVersion != okf.DefaultSpecVersion {
		t.Errorf("OKFVersion = %q, want default %q", b.OKFVersion, okf.DefaultSpecVersion)
	}
	if b.RootVersionUnparsed == nil {
		t.Fatalf("RootVersionUnparsed must disclose that index.md frontmatter did not parse")
	}
	if b.RootVersionUnparsed.RelPath != "index.md" || b.RootVersionUnparsed.Err == "" {
		t.Errorf("RootVersionUnparsed = %+v, want index.md with a parse error", b.RootVersionUnparsed)
	}
}

// TestRootVersionNotAdoptedFromBody is the second #163 negative fixture: an
// okf_version that appears only in the BODY, including inside a fenced code
// block, must NOT be adopted — the pre-fix scrape matched any line prefix
// anywhere in the file. Here the frontmatter parses cleanly and declares no
// version, so the default stands and there is no disclosure (nothing failed).
func TestRootVersionNotAdoptedFromBody(t *testing.T) {
	root := t.TempDir()
	write(t, root, "good.md", "---\ntype: Note\ntitle: Good\n---\n# Good\n")
	write(t, root, "index.md", "---\ntype: index\n---\n\n# Index\n\n```yaml\nokf_version: v9.9-from-body-codeblock\n```\n")

	b, err := bundle.Load(root, native.New())
	if err != nil {
		t.Fatal(err)
	}
	if string(b.OKFVersion) == "v9.9-from-body-codeblock" {
		t.Errorf("okf_version was scraped from the body/code block (#163); want the default %q", okf.DefaultSpecVersion)
	}
	if b.OKFVersion != okf.DefaultSpecVersion {
		t.Errorf("OKFVersion = %q, want default %q", b.OKFVersion, okf.DefaultSpecVersion)
	}
	if b.RootVersionUnparsed != nil {
		t.Errorf("frontmatter parsed cleanly; RootVersionUnparsed must be nil, got %+v", b.RootVersionUnparsed)
	}
}

// TestRootVersionAdoptedFromValidFrontmatter guards the happy path: a version
// declared in frontmatter that parses is still adopted (the fix must not stop
// reading legitimately-declared versions).
func TestRootVersionAdoptedFromValidFrontmatter(t *testing.T) {
	root := t.TempDir()
	write(t, root, "good.md", "---\ntype: Note\ntitle: Good\n---\n# Good\n")
	write(t, root, "index.md", "---\ntype: index\nokf_version: \"0.2\"\n---\n\n# Index\n")

	b, err := bundle.Load(root, native.New())
	if err != nil {
		t.Fatal(err)
	}
	if b.OKFVersion != "0.2" {
		t.Errorf("OKFVersion = %q, want 0.2 (declared in valid frontmatter)", b.OKFVersion)
	}
	if b.RootVersionUnparsed != nil {
		t.Errorf("RootVersionUnparsed must be nil when frontmatter parses, got %+v", b.RootVersionUnparsed)
	}
}

func TestLoadErrorsOnMissingRoot(t *testing.T) {
	if _, err := bundle.Load(filepath.Join(t.TempDir(), "nope"), native.New()); err == nil {
		t.Fatal("expected error for missing root")
	}
}
