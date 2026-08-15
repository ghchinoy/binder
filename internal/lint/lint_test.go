package lint_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/linkcheck"
	"github.com/ghchinoy/binder/internal/lint"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// fixedNow / fixedToday keep staleness deterministic (2023-11-14).
var fixedNow = time.Unix(1700000000, 0).UTC()

const fixedToday = "2023-11-14"

func lintCorpus(t *testing.T, src string) *lint.Report {
	t.Helper()
	concepts, facts, _, err := convert.Analyze(src, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNow,
	})
	if err != nil {
		t.Fatalf("Analyze(%s): %v", src, err)
	}
	rep := lint.Lint(concepts, facts, fixedToday)
	rep.Src = src
	return rep
}

// TestLintBrokenLinks: an unresolved internal .md ref and a residual wikilink are
// broken; a resolved link and an external URL are not.
func TestLintBrokenLinks(t *testing.T) {
	rep := lintCorpus(t, "../../testdata/corpus-lint-links")

	want := []lint.Finding{
		{Concept: "a", Detail: "[[Ghost]]"},
		{Concept: "a", Detail: "nope.md"},
	}
	if len(rep.BrokenLinks) != len(want) {
		t.Fatalf("broken links = %+v, want %+v", rep.BrokenLinks, want)
	}
	for i, f := range rep.BrokenLinks {
		if f != want[i] {
			t.Errorf("broken[%d] = %+v, want %+v", i, f, want[i])
		}
	}
	// The external link and the resolved a<->b links are never flagged.
	for _, f := range rep.BrokenLinks {
		if strings.Contains(f.Detail, "example.com") || f.Detail == "b.md" || f.Detail == "a.md" {
			t.Errorf("unexpected broken link: %+v", f)
		}
	}
}

// TestLintClean: a fully-resolvable, well-formed corpus yields zero findings.
func TestLintClean(t *testing.T) {
	rep := lintCorpus(t, "../../testdata/corpus-lint-clean")
	if n := rep.NumFindings(); n != 0 {
		t.Errorf("clean corpus has %d finding(s): %s", n, rep.String())
	}
}

// TestBrokenLinkParity: lint's broken CONCEPT references are exactly the
// converter's unresolved concept references on the same corpus (no drift).
func TestBrokenLinkParity(t *testing.T) {
	src := "../../testdata/corpus-lint-links"
	concepts, facts, rep, err := convert.Analyze(src, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Converter's unresolved concept refs, keyed by concept id (strip .md from the
	// source-relative From) + raw target.
	convSet := map[string]bool{}
	for _, u := range rep.Unresolved {
		if linkcheck.IsBrokenConceptRef(u.RawTarget) {
			id := strings.TrimSuffix(u.From, ".md")
			convSet[id+"|"+u.RawTarget] = true
		}
	}

	// Lint's broken concept refs.
	lintRep := lint.Lint(concepts, facts, fixedToday)
	lintSet := map[string]bool{}
	for _, f := range lintRep.BrokenLinks {
		if linkcheck.IsBrokenConceptRef(f.Detail) {
			lintSet[f.Concept+"|"+f.Detail] = true
		}
	}

	if len(convSet) == 0 {
		t.Fatal("test corpus has no unresolved concept refs; parity is vacuous")
	}
	for k := range convSet {
		if !lintSet[k] {
			t.Errorf("converter unresolved %q missing from lint broken links", k)
		}
	}
	for k := range lintSet {
		if !convSet[k] {
			t.Errorf("lint broken link %q not in converter unresolved set", k)
		}
	}
}

// TestReportSlicesInitialized: every bucket is a non-nil slice so --json emits []
// not null (#13).
func TestReportSlicesInitialized(t *testing.T) {
	rep := lintCorpus(t, "../../testdata/corpus-lint-clean")
	if rep.BrokenLinks == nil || rep.MissingTitles == nil || rep.Orphans == nil ||
		rep.Stale == nil || rep.SchemaViolations == nil {
		t.Error("a report bucket is nil; empty buckets must be initialized to []")
	}
}
