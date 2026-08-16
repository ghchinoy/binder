package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/lint"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// TestIssue99EndToEnd is the surface the issue was filed against: a 3-file corpus
// where sub/a.md carries a stray "[" inside a code region ahead of a real link
// that is byte-identical to sub/b.md's link. On main the extractor swallows
// sub/a's link, so lint reports only ONE broken link and misclassifies sub/a as
// an orphan, AND convert emits sub/a's link UN-rewritten into the bundle.
//
// This test asserts both halves that the fix must restore:
//   - convert OUTPUT correctness: BOTH sub/a and sub/b bodies carry the rewritten
//     bundle-absolute target (the more serious, user-shipped half).
//   - lint graph: exactly 2 broken links AND 0 orphans (the orphan assertion
//     locks the graph-side half — a fix that recovered the link while corrupting
//     the graph would still fail here).
func TestIssue99EndToEnd(t *testing.T) {
	src := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// target.md has headings "Target"/"Heading" but no "nope", so the #nope
	// anchor is broken for BOTH linking files — that is what yields 2 broken links.
	write("docs/target.md", "---\ntype: Note\ntitle: Target\n---\n\n# Target\n\n## Heading\n")
	// sub/a.md: stray unmatched "[" inside an inline code span, THEN the real link.
	write("sub/a.md", "---\ntype: Note\ntitle: A\n---\n\n# A\n\nRegex `[` here.\n\nsee [x](../docs/target.md#nope)\n")
	// sub/b.md: the byte-identical link, with no code-region trigger.
	write("sub/b.md", "---\ntype: Note\ntitle: B\n---\n\n# B\n\nsee [x](../docs/target.md#nope)\n")

	concepts, facts, _, err := convert.Analyze(src, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNow,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// --- convert OUTPUT correctness: both bodies must carry the rewritten link ---
	bodies := map[string]string{}
	for _, c := range concepts {
		bodies[c.ID] = c.Body
	}
	for _, id := range []string{"sub/a", "sub/b"} {
		b, ok := bodies[id]
		if !ok {
			t.Fatalf("concept %q missing from bundle; got %v", id, keys(bodies))
		}
		if !strings.Contains(b, "(/docs/target.md#nope)") {
			t.Errorf("convert emitted an un-rewritten link for %q: body = %q", id, b)
		}
		if strings.Contains(b, "(../docs/target.md#nope)") {
			t.Errorf("convert left %q's link relative (spec §6 non-conformant): body = %q", id, b)
		}
	}

	// --- lint graph: 2 broken links AND 0 orphans ---
	rep := lint.Lint(concepts, facts, fixedToday, nil)
	if len(rep.BrokenLinks) != 2 {
		t.Fatalf("broken links = %+v, want exactly 2 (sub/a and sub/b, both #nope)", rep.BrokenLinks)
	}
	// Both broken links must be the #nope anchor from the two distinct concepts.
	seen := map[string]bool{}
	for _, f := range rep.BrokenLinks {
		if !strings.Contains(f.Detail, "#nope") {
			t.Errorf("unexpected broken link: %+v", f)
		}
		seen[f.Concept] = true
	}
	if !seen["sub/a"] || !seen["sub/b"] {
		t.Errorf("broken links must come from BOTH sub/a and sub/b; got %+v", rep.BrokenLinks)
	}
	if len(rep.Orphans) != 0 {
		t.Errorf("orphans = %v, want none (sub/a's recovered link makes it non-orphan)", rep.Orphans)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
