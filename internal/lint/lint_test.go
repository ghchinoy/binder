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

// TestLintMissingTitlesAndSchema: source-fact checks see the pre-default state —
// a file with no authored title/H1 is a missing title; a file with no authored
// type: and one with invalid YAML are schema violations. A recovered file is not
// double-listed as "missing type".
func TestLintMissingTitlesAndSchema(t *testing.T) {
	rep := lintCorpus(t, "../../testdata/corpus-lint-schema")

	if want := []string{"notitle"}; !equalStrings(rep.MissingTitles, want) {
		t.Errorf("missing titles = %v, want %v", rep.MissingTitles, want)
	}

	if len(rep.SchemaViolations) != 2 {
		t.Fatalf("schema violations = %+v, want 2", rep.SchemaViolations)
	}
	if rep.SchemaViolations[0].Concept != "badyaml" ||
		!strings.HasPrefix(rep.SchemaViolations[0].Detail, "invalid frontmatter: ") {
		t.Errorf("schema[0] = %+v, want badyaml invalid-frontmatter", rep.SchemaViolations[0])
	}
	// The prefix must appear exactly once (codec error already carries it).
	if strings.Count(rep.SchemaViolations[0].Detail, "invalid frontmatter") != 1 {
		t.Errorf("invalid-frontmatter prefix duplicated: %q", rep.SchemaViolations[0].Detail)
	}
	if rep.SchemaViolations[1] != (lint.Finding{Concept: "notype", Detail: "missing type"}) {
		t.Errorf("schema[1] = %+v, want notype missing type", rep.SchemaViolations[1])
	}
	// The recovered file (badyaml) must NOT also appear as "missing type".
	for _, f := range rep.SchemaViolations {
		if f.Concept == "badyaml" && f.Detail == "missing type" {
			t.Errorf("recovered file double-listed as missing type: %+v", f)
		}
	}
	// badyaml has a first H1 in its preserved body, so it is not a missing title.
	for _, id := range rep.MissingTitles {
		if id == "badyaml" {
			t.Errorf("badyaml should not be a missing title (its body has an H1)")
		}
	}
}

// TestLintOrphansAndStale: an orphan has 0 inbound AND 0 outbound edges; a
// connected-but-past-stale_after concept is stale, not an orphan; staleness is
// deterministic in `today`.
func TestLintOrphansAndStale(t *testing.T) {
	src := "../../testdata/corpus-lint-graph"
	concepts, facts, _, err := convert.Analyze(src, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	rep := lint.Lint(concepts, facts, "2023-11-14")
	if want := []string{"island"}; !equalStrings(rep.Orphans, want) {
		t.Errorf("orphans = %v, want %v", rep.Orphans, want)
	}
	if want := []string{"stale"}; !equalStrings(rep.Stale, want) {
		t.Errorf("stale = %v, want %v", rep.Stale, want)
	}
	// The stale concept is connected (links to/from a), so it is not an orphan.
	for _, id := range rep.Orphans {
		if id == "stale" {
			t.Errorf("connected 'stale' concept wrongly flagged orphan")
		}
	}

	// Determinism: before stale_after, the same corpus reports no stale concepts.
	early := lint.Lint(concepts, facts, "2019-01-01")
	if len(early.Stale) != 0 {
		t.Errorf("stale as of 2019-01-01 = %v, want none", early.Stale)
	}
	// Orphan detection is independent of today.
	if !equalStrings(early.Orphans, []string{"island"}) {
		t.Errorf("orphans as of 2019 = %v, want [island]", early.Orphans)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
