// Package docsimpact is the CI gate that enforces the "Docs impact" field in
// the PR template (.github/pull_request_template.md, added in #103). It answers
// exactly one question about a PR body: was the docs-impact question ANSWERED?
//
// It deliberately does NOT judge whether the answer is correct. A "No" on a PR
// that does change user-visible output is a reviewer finding, not a CI finding
// (see issue #104). The gate fails only when the section is missing, or is
// present but no box is checked — the machine equivalent of the reviewer
// instruction the template already carries.
//
// The matcher here is the SAME code path exercised by the derived controls in
// docsimpact_test.go and by the real-PR command in cmd/gate. There is no
// second, parallel implementation to drift out of sync.
package docsimpact

import (
	"errors"
	"regexp"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// Heading is the human-readable name of the section this gate enforces. Used in
// actionable error messages.
const Heading = "Docs impact"

// headingRE matches the "## Docs impact ..." heading line that opens the
// section. Case-insensitive so a stray capitalisation change does not turn a
// present-but-differently-cased heading into a missing one; a genuine rename is
// caught loudly by the positive control in CI.
var headingRE = regexp.MustCompile(`(?im)^##[ \t]+docs impact\b.*$`)

// nextHeadingRE matches any level-2 (or shallower) markdown heading, used to
// bound the docs-impact section.
var nextHeadingRE = regexp.MustCompile(`(?m)^#{1,2}[ \t]+\S`)

// noOptionRE and yesOptionRE match the two mutually-exclusive docs-impact option
// lines, capturing the checkbox contents so the gate can tell whether each
// specific option is checked. Keying on the two named options — rather than "any
// checked box in the region" — is what makes the gate reject a contradictory
// both-checked answer and ignore unrelated checklist boxes that fall inside the
// bound.
//
// The match keys on the REAL option text — the "— " that opens each option's
// description ("No — none of the above changed.", "Yes — and the docs task is
// named below.") — not on a bare leading "No"/"Yes" fragment. A bare fragment also
// matches unrelated checklist lines such as "- [ ] No `Release-As:` footer.". A
// matcher keyed on it stayed correct for every reachable template layout, but only
// through two incidental invariants: the section bound kept the checklist line out
// of the scanned region, and — because optionChecked reads only the first matching
// line — the real (unchecked) option always sat ahead of any checklist line a
// reorder could sweep in. That was a latent fragility, not an open hole. Keying on
// the option's own text removes reliance on both invariants: an unrelated checklist
// "No"/"Yes" line is never read as an option, wherever it sits and wherever the
// section bound falls.
var (
	noOptionRE  = regexp.MustCompile(`(?m)^[ \t]*[-*][ \t]+\[([^\]]*)\][ \t]+No[ \t]+—`)
	yesOptionRE = regexp.MustCompile(`(?m)^[ \t]*[-*][ \t]+\[([^\]]*)\][ \t]+Yes[ \t]+—`)
)

// docsTaskRE captures the value written on the "Docs task (required when Yes)"
// line, used to enforce that a "Yes" answer actually names a task.
var docsTaskRE = regexp.MustCompile(`(?im)^[ \t]*\*\*Docs task\b[^:]*:\*\*[ \t]*(.*)$`)

// ErrSectionMissing is returned when the docs-impact heading is absent from the
// PR body entirely.
var ErrSectionMissing = errors.New(
	"the \"" + Heading + "\" section is missing from the PR description: " +
		"restore it from .github/pull_request_template.md and check the \"No\" " +
		"or \"Yes\" box that describes this PR")

// ErrUnanswered is returned when the docs-impact section is present but neither
// checkbox is checked (the raw placeholder, an emptied answer, etc.).
var ErrUnanswered = errors.New(
	"the \"" + Heading + "\" section is present but unanswered: check either " +
		"the \"No\" box (no user-visible output changed) or the \"Yes\" box " +
		"(and name the docs task) in the PR description")

// ErrBothChecked is returned when both the "No" and "Yes" boxes are checked. The
// options are mutually exclusive, so a both-checked answer records no decision;
// it is malformed, not answered. This is a well-formedness check on the document,
// not a judgement about the answer's correctness.
var ErrBothChecked = errors.New(
	"the \"" + Heading + "\" section has both the \"No\" and \"Yes\" boxes " +
		"checked: they are mutually exclusive, so check exactly one in the PR " +
		"description")

// ErrDocsTaskMissing is returned when the "Yes" box is checked but the "Docs
// task (required when Yes)" line is left blank (placeholder underscores or
// whitespace only). The template's own text makes the task "required when Yes",
// and the Yes option asserts "the docs task is named below"; a blank line makes
// the body contradict itself. This enforces the artifact's stated rule and reads
// only the form, never whether the named task is a good one.
var ErrDocsTaskMissing = errors.New(
	"the \"" + Heading + "\" section has the \"Yes\" box checked but the " +
		"\"Docs task (required when \\\"Yes\\\")\" line is blank: name the docs " +
		"task on that line in the PR description")

// sectionBound isolates the docs-impact section — the region the option matchers
// scan — and reports whether the section is present at all. The bound is a
// DECLARED property of the gate, deliberately stated here rather than left to
// emerge from wherever headings happen to fall:
//
//	the section runs from just after the "## Docs impact" heading to the start of
//	the next level-2-or-shallower heading, or to the end of the body when Docs
//	impact is the last section.
//
// The end-of-body case is the one worth naming. When Docs impact is the last
// section the bound widens to end-of-body and pulls the "## Checklist" boxes into
// the scanned region. Declaring and testing the bound (see TestSectionBound) makes
// that widening a visible, tested behaviour rather than a silent consequence of a
// template reorder. The bound does not, on its own, decide the answer: correctness
// under the widened bound rests on the option matchers keying on the real option
// text, so a checklist "No"/"Yes" line swept into the region is not read as an
// option. The bound is declared here for clarity and drift-detection, not because
// the gate's correctness hangs on where it falls.
func sectionBound(body string) (string, bool) {
	loc := headingRE.FindStringIndex(body)
	if loc == nil {
		return "", false
	}
	rest := body[loc[1]:]
	if next := nextHeadingRE.FindStringIndex(rest); next != nil {
		return rest[:next[0]], true
	}
	return rest, true
}

// optionChecked reports whether the option line matched by re is present and has
// its checkbox checked ("[x]" / "[X]").
func optionChecked(re *regexp.Regexp, section string) bool {
	m := re.FindStringSubmatch(section)
	if m == nil {
		return false
	}
	return strings.ContainsAny(m[1], "xX")
}

// taskNamed reports whether the "Docs task" line carries a real value once the
// placeholder underscores and surrounding whitespace are stripped. A missing line
// counts as not named.
func taskNamed(section string) bool {
	m := docsTaskRE.FindStringSubmatch(section)
	if m == nil {
		return false
	}
	v := strings.TrimSpace(m[1])
	v = strings.Trim(v, "_")
	return strings.TrimSpace(v) != ""
}

// Check reports whether the docs-impact question is answered in the given PR
// body. Code regions — fenced blocks, indented blocks, and inline spans — are
// masked first via the shared, CommonMark-aware okf.MaskCode, so a checkbox- or
// heading-shaped line that is really quoted code is never read as structure (as
// GitHub renders it literally). Delegating to the parser rather than enumerating
// code constructs by hand is deliberate: a line scanner that lists "fences" would
// miss indented blocks, the next scanner would miss inline spans, and so on.
//
// It returns nil when exactly one of the "No"/"Yes" boxes is checked (and, for
// "Yes", the docs-task line names a task); ErrSectionMissing when the section is
// absent; ErrUnanswered when neither box is checked; ErrBothChecked when both are
// checked; and ErrDocsTaskMissing when "Yes" is checked with a blank docs-task
// line. Callers distinguish a finding (these errors) from a failure to run (which
// never originates here — Check always runs to a definite verdict on whatever
// string it is given).
func Check(body string) error {
	body = okf.MaskCode(body)

	section, ok := sectionBound(body)
	if !ok {
		return ErrSectionMissing
	}

	no := optionChecked(noOptionRE, section)
	yes := optionChecked(yesOptionRE, section)
	switch {
	case no && yes:
		return ErrBothChecked
	case !no && !yes:
		return ErrUnanswered
	case yes && !taskNamed(section):
		return ErrDocsTaskMissing
	}
	return nil
}
