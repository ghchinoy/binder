package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/lint"
)

// TestColonSpacePositiveControl: the pre-existing #93 positive control,
// testdata/corpus-lint-schema/badyaml.md, triggers the advisory — once per
// offending key, naming the key so the author knows what to quote. Its title: is
// the exact shape the #93 comment measured in the wild (a title:, not a
// description:), which is why the rule is keyed on no particular key.
func TestColonSpacePositiveControl(t *testing.T) {
	rep := lintCorpus(t, "../../testdata/corpus-lint-schema")

	want := []lint.Finding{
		{Concept: "badyaml", Detail: `goal: unquoted value contains ": " — quote it`},
		{Concept: "badyaml", Detail: `title: unquoted value contains ": " — quote it`},
	}
	if len(rep.ColonSpaceScalars) != len(want) {
		t.Fatalf("colon-space scalars = %+v, want %+v", rep.ColonSpaceScalars, want)
	}
	for i, f := range rep.ColonSpaceScalars {
		if f != want[i] {
			t.Errorf("colon-space[%d] = %+v, want %+v", i, f, want[i])
		}
	}

	// The advisory is ADDITIVE to the invalid-frontmatter violation, not a
	// replacement: the violation says the block did not parse, the advisory says
	// which key to quote.
	var sawViolation bool
	for _, f := range rep.SchemaViolations {
		if f.Concept == "badyaml" && strings.HasPrefix(f.Detail, "invalid frontmatter") {
			sawViolation = true
		}
	}
	if !sawViolation {
		t.Error("badyaml lost its invalid-frontmatter schema violation")
	}

	// It appears in the prose report, labelled as advisory.
	if !strings.Contains(rep.String(), "unquoted colon-space scalars (advisory, never gates): 2") {
		t.Errorf("advisory missing from prose report:\n%s", rep.String())
	}
}

// TestColonSpaceNegativeControls: quoted scalars, colons NOT followed by a space
// (URLs, timestamps, ratios), block-scalar interiors and flow collections must
// all stay clean. URL-like values are the classic false positive for this rule.
func TestColonSpaceNegativeControls(t *testing.T) {
	// This test's subject is an ABSENCE, so CALIBRATE THE INSTRUMENT FIRST. Two
	// different no-ops would otherwise satisfy every assertion below: a corpus with
	// nothing in it, and a detector that has stopped reporting anything at all.
	// Neither has to be deliberate — a broken detector makes this test greener, not
	// redder, which is exactly the failure mode worth spending four lines on.
	if pos := lintCorpus(t, "../../testdata/corpus-lint-schema"); len(pos.ColonSpaceScalars) == 0 {
		t.Fatal("vacuous: the advisory reports nothing even on the positive control, so a clean " +
			"result below would say nothing about false positives")
	}

	rep := lintCorpus(t, "../../testdata/corpus-lint-colonspace")
	if rep.NumConcepts < 2 {
		t.Fatalf("vacuous: negative-control corpus has %d concept(s); the false-positive "+
			"shapes it is supposed to carry are not being scanned", rep.NumConcepts)
	}

	if len(rep.ColonSpaceScalars) != 0 {
		t.Errorf("negative-control corpus triggered the advisory: %+v", rep.ColonSpaceScalars)
	}
	// The corpus is clean outright, so a false positive here could not be hiding
	// behind some other finding.
	if n := rep.NumFindings(); n != 0 {
		t.Errorf("negative-control corpus has %d other finding(s): %s", n, rep.String())
	}
}

// TestColonSpaceAlwaysAccompaniedByViolation is the load-bearing half of the
// never-gates rationale, and the half that is a UNIVERSAL claim rather than an
// observation: an advisory is safe to leave out of NumFindings because the file
// it names ALWAYS also produces the invalid-frontmatter SchemaViolation that IS
// counted, so nothing goes unreported and no defect is double-counted.
//
// The claim held for every case that was tried by hand and was still false for
// one that was not — a multi-line quoted scalar whose continuation line reads
// `note: a: b` is valid YAML, produced no violation, and yet drew a finding
// against a phantom key. That was a detector bug, not a legitimate lone firing,
// and it is fixed; this test is what stops it (or anything like it) from coming
// back silently. It sweeps every corpus fixture in the repo rather than a
// curated list.
func TestColonSpaceAlwaysAccompaniedByViolation(t *testing.T) {
	corpora, err := filepath.Glob("../../testdata/corpus-*")
	if err != nil {
		t.Fatalf("globbing corpora: %v", err)
	}
	if len(corpora) == 0 {
		t.Fatal("no corpus fixtures found; this test would be vacuous")
	}

	var fired int
	for _, src := range corpora {
		info, serr := os.Stat(src)
		if serr != nil || !info.IsDir() {
			continue
		}
		rep := lintCorpus(t, src)

		violated := map[string]bool{}
		for _, f := range rep.SchemaViolations {
			if strings.HasPrefix(f.Detail, "invalid frontmatter") {
				violated[f.Concept] = true
			}
		}
		for _, f := range rep.ColonSpaceScalars {
			fired++
			if !violated[f.Concept] {
				t.Errorf("%s: %q drew the colon-space advisory but produced NO "+
					"invalid-frontmatter schema violation. The advisory is excluded from "+
					"NumFindings on the grounds that a counted violation always subsumes "+
					"it — that is now untrue, so either this is a false positive or the "+
					"exclusion needs a different justification.", src, f.Concept)
			}
		}
	}
	if fired == 0 {
		t.Fatal("the advisory fired on no corpus; the invariant was never exercised")
	}
}

// TestColonSpaceNeverGates is the never-reject guarantee of issue #93, asserted
// rather than argued: the advisory is excluded from NumFindings, which is the
// single number `binder lint --strict` gates on. On the positive-control corpus
// the advisory fires twice and contributes exactly zero to that total, so the
// rule cannot change any exit code — there is no code path by which it rejects.
func TestColonSpaceNeverGates(t *testing.T) {
	rep := lintCorpus(t, "../../testdata/corpus-lint-schema")
	if len(rep.ColonSpaceScalars) == 0 {
		t.Fatal("positive control produced no advisory; this test would be vacuous")
	}

	gating := len(rep.BrokenLinks) + len(rep.MissingTitles) + len(rep.Orphans) +
		len(rep.Stale) + len(rep.SchemaViolations)
	if rep.NumFindings() != gating {
		t.Errorf("NumFindings() = %d, want %d — the colon-space advisory must not be counted "+
			"in the total --strict gates on", rep.NumFindings(), gating)
	}

	// Zeroing the advisory cannot change the gate either way.
	before := rep.NumFindings()
	rep.ColonSpaceScalars = nil
	if after := rep.NumFindings(); after != before {
		t.Errorf("NumFindings changed when the advisory was removed: %d -> %d", before, after)
	}
}
