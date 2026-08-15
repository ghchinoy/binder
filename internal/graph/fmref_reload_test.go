package graph_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf/native"
	"github.com/ghchinoy/binder/internal/review"
)

// TestFMRefEdgeSurvivesReload is the definitive R1a regression: a frontmatter-ref
// edge whose target has NO coincident body link must still appear as a graph edge
// and keep its target off the orphan list after the bundle is reloaded — while the
// original frontmatter key/value is preserved. `binder graph`/`binder review`
// rebuild edges only from persisted body links, so the resolved fm-ref must be
// materialized into the body (a "## Related" section) at convert time.
func TestFMRefEdgeSurvivesReload(t *testing.T) {
	src := t.TempDir()
	// a.md declares parent: [[Beta]] but has NO body link to Beta.
	writeFile(t, src, "a.md", "---\ntype: Note\ntitle: Alpha\nparent: \"[[Beta]]\"\n---\n\n# Alpha\n\nNo body link here.\n")
	writeFile(t, src, "b.md", "---\ntype: Note\ntitle: Beta\n---\n\n# Beta\n")

	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := convert.Convert(src, out, convert.Options{
		Codec:     native.New(),
		Version:   "0.1.0",
		Now:       time.Unix(1700000000, 0),
		FMRefKeys: []string{"parent"},
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	// The original frontmatter key/value is preserved verbatim.
	aBytes, err := os.ReadFile(filepath.Join(out, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(aBytes); !strings.Contains(got, `parent: "[[Beta]]"`) {
		t.Errorf("frontmatter key must be preserved:\n%s", got)
	}

	// Reload the bundle the way graph/review do (edges rebuilt from body links).
	b, err := bundle.Load(out, native.New())
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	m := graph.Build(b, "2026-08-15")
	var found bool
	for _, e := range m.Edges {
		if e.From == "a" && e.To == "b" {
			found = true
		}
	}
	if !found {
		t.Errorf("graph edges must include a->b (fm-ref materialized as body link): %+v", m.Edges)
	}

	r := review.Review(b, "2026-08-15")
	for _, o := range r.Orphans {
		if o == "b" {
			t.Errorf("Beta must not be an orphan; a declares it as parent: %v", r.Orphans)
		}
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
