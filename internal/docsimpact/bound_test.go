package docsimpact

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This file tests the DECLARED section bound (sectionBound) as a property in its
// own right — white-box, in-package — so that a future template reorder that
// widens or narrows the scanned region produces a RED here rather than a silent
// change in what the gate reads. It complements the black-box behaviour controls
// in docsimpact_test.go; the bound is derived from the live template, never a
// transcribed fixture.

const boundTemplatePath = "../../.github/pull_request_template.md"

// bnChecklistHeadingRE locates the "## Checklist" heading that follows the
// docs-impact section in the default template. Independent structural literal;
// used only to model a reorder that makes Docs impact the last section.
var bnChecklistHeadingRE = regexp.MustCompile(`(?m)^##[ \t]+Checklist[ \t]*$`)

func readBoundTemplate(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(boundTemplatePath))
	if err != nil {
		t.Fatalf("cannot read PR template %s: %v", boundTemplatePath, err)
	}
	if len(b) == 0 {
		t.Fatalf("PR template %s is empty", boundTemplatePath)
	}
	return string(b)
}

// TestSectionBound pins the scanned region in both directions: what it EXCLUDES
// while a level-2 heading follows Docs impact, and what it INCLUDES once Docs
// impact is the last section. The second half is the load-bearing one — it makes
// the bound's widening to end-of-body a visible, tested behaviour instead of a
// silent consequence of where headings happen to fall.
func TestSectionBound(t *testing.T) {
	tmpl := readBoundTemplate(t)

	// Positive control: the section is present and the real "No" option is inside
	// it. If this fails the rest of the test is meaningless, so assert it first —
	// this also distinguishes "bound excludes X" from "the test never ran".
	sec, ok := sectionBound(tmpl)
	if !ok {
		t.Fatal("sectionBound reported no Docs impact section in the live template")
	}
	if !strings.Contains(sec, "none of the above changed") {
		t.Fatalf("section bound too narrow: the real \"No\" option must be INSIDE "+
			"the scanned region.\n--- section ---\n%s", sec)
	}

	// Bounded case: with the "## Checklist" heading following, the checklist boxes
	// are OUTSIDE the region. The checklist "No" line's own text carries the marker
	// "Release-As"; here it must not appear in the scanned region.
	if strings.Contains(sec, "Release-As") {
		t.Fatalf("section bound too wide: with a following level-2 heading the "+
			"Checklist \"Release-As\" line must be OUTSIDE the scanned region.\n"+
			"--- section ---\n%s", sec)
	}

	// Widened case: make Docs impact the last section by removing the only heading
	// that follows it. The bound MUST widen to end-of-body and pull the checklist
	// boxes INTO the region. Correctness under this state is carried by the
	// option-text matchers, proved by TestChecklistNoLineDoesNotAnswerGate; here we
	// only pin that the widening happens and is observable.
	last := bnChecklistHeadingRE.ReplaceAllString(tmpl, "")
	if last == tmpl {
		t.Fatal("mutation drift: the \"## Checklist\" heading was not found/removed; " +
			"update bnChecklistHeadingRE in bound_test.go.")
	}
	secLast, ok := sectionBound(last)
	if !ok {
		t.Fatal("sectionBound reported no Docs impact section after the reorder")
	}
	if !strings.Contains(secLast, "Release-As") {
		t.Fatalf("section bound did not widen to end-of-body when Docs impact is "+
			"the last section: the Checklist \"Release-As\" line must fall INSIDE the "+
			"region, so the widening is a tested behaviour rather than a silent "+
			"consequence of layout.\n--- section ---\n%s", secLast)
	}
}

// TestSectionBoundMissing pins the section-missing signal at the bound level: a
// body with no Docs impact heading yields no section. The positive control that
// this is not vacuous is TestSectionBound, which asserts the live template DOES
// yield a section in the same package run.
func TestSectionBoundMissing(t *testing.T) {
	if _, ok := sectionBound("## Something else\n\nno docs impact heading here\n"); ok {
		t.Fatal("sectionBound reported a section for a body with no Docs impact heading")
	}
}
