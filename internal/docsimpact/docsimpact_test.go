package docsimpact_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/ghchinoy/binder/internal/docsimpact"
)

// templatePath is the artifact under test. Every control below is DERIVED from
// this file at test time by structural mutation — there is not one transcribed
// fixture in this file. A hand-written fixture is a second artifact maintained
// in parallel with the template; when it goes stale it fails exactly as designed
// while real PRs sail through, and the agreement between the two wrong halves
// hides the fault (issue #104; process-practices P15).
const templatePath = "../../.github/pull_request_template.md"

func readTemplate(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(templatePath))
	if err != nil {
		t.Fatalf("cannot read PR template %s: %v", templatePath, err)
	}
	if len(b) == 0 {
		t.Fatalf("PR template %s is empty", templatePath)
	}
	return string(b)
}

// The mutators below deliberately use their OWN structural regexes, NOT any
// function from the docsimpact package. If a mutation located its target using
// the matcher's detection code, a renamed heading would make the mutator emit
// the template unmutated, the matcher would fail on it for the wrong reason, and
// the control would pass vacuously (issue #104, "Guard against circularity").
//
// The mustChange() assertion is the load-bearing guard: it is checkable rather
// than a discipline. If any transform produces output identical to its input,
// the job fails and NAMES mutation drift as the cause, so a heading/checkbox
// rename can never silently turn a control into a no-op.

// docsHeadingRE locates the docs-impact heading line. Independent literal from
// the matcher's own headingRE.
var docsHeadingRE = regexp.MustCompile(`(?im)^##[ \t]+docs impact\b.*$`)

// sectionBoundRE locates the next level-2 heading that bounds the section.
var sectionBoundRE = regexp.MustCompile(`(?m)^#{1,2}[ \t]+\S`)

// uncheckedBoxLineRE matches a whole unchecked task-list line, e.g. "- [ ] No…".
var uncheckedBoxLineRE = regexp.MustCompile(`(?m)^[ \t]*[-*][ \t]+\[[ \t]*\].*$`)

// firstUncheckedBoxRE matches just the empty brackets of an unchecked box.
var firstUncheckedBoxRE = regexp.MustCompile(`\[[ \t]*\]`)

// checklistHeadingRE locates the "## Checklist" heading that bounds the end of
// the docs-impact section in the default template. Independent literal, used only
// to delete that heading structurally (the section-bounding-leak control).
var checklistHeadingRE = regexp.MustCompile(`(?m)^##[ \t]+Checklist[ \t]*$`)

// docsTaskPlaceholderRE locates the underscore placeholder on the "Docs task"
// line. Independent literal; keys on the placeholder's shape, not on the matcher.
var docsTaskPlaceholderRE = regexp.MustCompile(`_{3,}`)

// mustChange fails the job, naming mutation drift, when a transform was a no-op.
func mustChange(t *testing.T, name, before, after string) {
	t.Helper()
	if before == after {
		t.Fatalf("mutation drift: control %q produced output identical to its "+
			"input — the mutator's structural pattern no longer matches the "+
			"template, so this control is vacuous. Update the mutator in "+
			"docsimpact_test.go to track the template's current shape.", name)
	}
}

// deleteSection removes the docs-impact heading through to (but not including)
// the next heading. Models an author deleting the whole section.
func deleteSection(t *testing.T, tmpl string) string {
	t.Helper()
	loc := docsHeadingRE.FindStringIndex(tmpl)
	if loc == nil {
		return tmpl // no-op; mustChange will flag it as drift
	}
	rest := tmpl[loc[1]:]
	end := len(rest)
	if next := sectionBoundRE.FindStringIndex(rest); next != nil {
		end = next[0]
	}
	return tmpl[:loc[0]] + rest[end:]
}

// emptyAnswer removes the checkbox option lines entirely: the section and its
// question remain, but there is no longer anything to check.
func emptyAnswer(t *testing.T, tmpl string) string {
	t.Helper()
	return uncheckedBoxLineRE.ReplaceAllString(tmpl, "")
}

// whitespaceAnswer replaces each checkbox option line with a whitespace-only
// line: the lines are present but carry no answer.
func whitespaceAnswer(t *testing.T, tmpl string) string {
	t.Helper()
	return uncheckedBoxLineRE.ReplaceAllString(tmpl, "    ")
}

// validAnswer checks the first box (e.g. selecting "No"). This is a valid,
// answered PR body.
func validAnswer(t *testing.T, tmpl string) string {
	t.Helper()
	replaced := false
	return firstUncheckedBoxRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		if replaced {
			return m
		}
		replaced = true
		return "[x]"
	})
}

// checkNthBoxes checks the boxes at the given 1-based positions among the
// template's unchecked task-list boxes, located POSITIONALLY with the test's own
// firstUncheckedBoxRE — no docsimpact function is consulted, so the guard stays
// non-circular. In the default template position 1 is the "No" option, position 2
// is "Yes", and 3+ are the checklist boxes.
func checkNthBoxes(tmpl string, positions ...int) string {
	want := make(map[int]bool, len(positions))
	for _, p := range positions {
		want[p] = true
	}
	n := 0
	return firstUncheckedBoxRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		n++
		if want[n] {
			return "[x]"
		}
		return m
	})
}

// insertInSection inserts block on its own lines immediately after the
// docs-impact heading, i.e. inside the section, located with the test's own
// docsHeadingRE. Returns the input unchanged if the heading is not found, so
// mustChange flags the drift.
func insertInSection(tmpl, block string) string {
	loc := docsHeadingRE.FindStringIndex(tmpl)
	if loc == nil {
		return tmpl
	}
	return tmpl[:loc[1]] + "\n" + block + tmpl[loc[1]:]
}

// deleteChecklistHeading removes the "## Checklist" heading line that bounds the
// section's end, so the checklist boxes fall inside the docs-impact bound —
// modelling the section-bounding leak.
func deleteChecklistHeading(tmpl string) string {
	return checklistHeadingRE.ReplaceAllString(tmpl, "")
}

// fillDocsTask writes a task name over the underscore placeholder on the
// "Docs task" line.
func fillDocsTask(tmpl string) string {
	return docsTaskPlaceholderRE.ReplaceAllString(tmpl, "write the widget migration guide")
}

// insertIndentedCheckbox inserts a four-space-indented checkbox line inside the
// section, preceded and followed by blank lines so CommonMark reads it as an
// INDENTED code block (the construct a fence-only stripper misses). Located with
// the test's own docsHeadingRE — no docsimpact function is consulted.
func insertIndentedCheckbox(tmpl string) string {
	loc := docsHeadingRE.FindStringIndex(tmpl)
	if loc == nil {
		return tmpl
	}
	return tmpl[:loc[1]] + "\n\n    - [x] No — indented code, not a real answer\n" + tmpl[loc[1]:]
}

// TestNegativeControls_MustFail: four bodies derived from the template that the
// matcher MUST reject on every CI run. If any of these ever passes, the matcher
// has drifted permissive — the invisible, indefinite failure direction — and
// this test breaks loudly (issue #104; process-practices P15).
func TestNegativeControls_MustFail(t *testing.T) {
	tmpl := readTemplate(t)

	tests := []struct {
		name    string
		derive  func(*testing.T, string) string
		mutated bool // whether derive is a mutation that must change its input
	}{
		{"placeholder-intact", func(_ *testing.T, s string) string { return s }, false},
		{"section-deleted", deleteSection, true},
		{"answer-empty", emptyAnswer, true},
		{"answer-whitespace-only", whitespaceAnswer, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.derive(t, tmpl)
			if tc.mutated {
				mustChange(t, tc.name, tmpl, body)
			}
			if err := docsimpact.Check(body); err == nil {
				t.Fatalf("negative control %q PASSED but must fail: the matcher "+
					"accepted an unanswered docs-impact section. The matcher has "+
					"drifted too permissive.\n--- derived body ---\n%s", tc.name, body)
			}
		})
	}
}

// TestPositiveControl_MustPass: a body derived from the template with a box
// checked, which the matcher MUST accept. This is what catches too-strict drift
// IN CI rather than on a contributor's PR — if a template change breaks the
// matcher's detection, this fails here instead of spuriously blocking a real PR.
func TestPositiveControl_MustPass(t *testing.T) {
	tmpl := readTemplate(t)
	body := validAnswer(t, tmpl)
	mustChange(t, "valid-answer", tmpl, body)

	if err := docsimpact.Check(body); err != nil {
		t.Fatalf("positive control \"valid-answer\" FAILED but must pass: the "+
			"matcher rejected an answered docs-impact section (%v). The matcher "+
			"has drifted too strict.\n--- derived body ---\n%s", err, body)
	}
}

// TestFenceControls covers the fence-awareness fix in BOTH directions. Not one of
// the original five controls contained a fenced code block, which is exactly why
// the suite was structurally unable to see this class. Both bodies are the
// mutation-derived spirit of the existing suite: a checkbox-shaped and a
// heading-shaped line placed inside a fence within the section.
func TestFenceControls(t *testing.T) {
	tmpl := readTemplate(t)

	// FALSE-PASS direction: both real boxes are left unchecked, but a
	// checkbox-shaped line sits inside a fenced block — an author quoting the
	// template. GitHub renders it as literal text, so this body is unanswered and
	// the gate MUST reject it.
	t.Run("fenced-checkbox-must-fail", func(t *testing.T) {
		body := insertInSection(tmpl, "```md\n- [x] No — quoted from the template\n```")
		mustChange(t, "fenced-checkbox", tmpl, body)
		if err := docsimpact.Check(body); err == nil {
			t.Fatalf("fenced-checkbox control PASSED but must fail: a checkbox "+
				"inside a fenced block was read as a real answer.\n--- body ---\n%s", body)
		}
	})

	// FALSE-FAIL direction: the "No" box is genuinely checked, but a "##"-shaped
	// line inside a fenced block precedes it. A non-fence-aware matcher truncates
	// the section before the real box and rejects an answered body. The gate MUST
	// accept it.
	t.Run("fenced-heading-must-pass", func(t *testing.T) {
		answered := checkNthBoxes(tmpl, 1)
		mustChange(t, "fenced-heading/answer", tmpl, answered)
		body := insertInSection(answered, "```sh\n## not a real heading, just output\n```")
		mustChange(t, "fenced-heading/insert", answered, body)
		if err := docsimpact.Check(body); err != nil {
			t.Fatalf("fenced-heading control FAILED but must pass: a fenced "+
				"\"##\" line truncated the section before the real box (%v).\n--- body ---\n%s", err, body)
		}
	})
}

// TestExactlyOneBox covers F3: a contradictory answer with both the "No" and
// "Yes" boxes checked records no decision and MUST be rejected. The matcher keys
// on the two named options, not on "any checked box in the region".
func TestExactlyOneBox(t *testing.T) {
	tmpl := readTemplate(t)
	both := checkNthBoxes(tmpl, 1, 2)
	mustChange(t, "both-checked", tmpl, both)
	// Guard against partial drift: if the template lost its second option box,
	// only one would flip and the control would silently test the wrong state.
	if both == checkNthBoxes(tmpl, 1) || both == checkNthBoxes(tmpl, 2) {
		t.Fatalf("mutation drift: both-checked control flipped fewer than two " +
			"boxes — the template no longer has two option boxes where expected. " +
			"Update checkNthBoxes positions in docsimpact_test.go.")
	}
	if err := docsimpact.Check(both); err == nil {
		t.Fatalf("both-checked control PASSED but must fail: a contradictory "+
			"answer with both boxes checked was accepted.\n--- body ---\n%s", both)
	}
}

// TestYesRequiresDocsTask covers F4: "Yes" checked with a blank docs-task line is
// the #97 state — a real docs impact with no docs task named — and MUST be
// rejected; the same body with the task filled MUST pass. This reads only the
// form of the line (placeholder underscores stripped), never whether the named
// task is a good one.
func TestYesRequiresDocsTask(t *testing.T) {
	tmpl := readTemplate(t)

	t.Run("yes-blank-task-must-fail", func(t *testing.T) {
		body := checkNthBoxes(tmpl, 2) // Yes checked, placeholder left intact
		mustChange(t, "yes-blank", tmpl, body)
		if err := docsimpact.Check(body); err == nil {
			t.Fatalf("yes-blank control PASSED but must fail: \"Yes\" was "+
				"accepted with the docs-task line blank.\n--- body ---\n%s", body)
		}
	})

	t.Run("yes-filled-task-must-pass", func(t *testing.T) {
		filled := fillDocsTask(tmpl)
		mustChange(t, "yes-filled/task", tmpl, filled)
		body := checkNthBoxes(filled, 2)
		mustChange(t, "yes-filled/box", filled, body)
		if err := docsimpact.Check(body); err != nil {
			t.Fatalf("yes-filled control FAILED but must pass: \"Yes\" with a "+
				"named docs task was rejected (%v).\n--- body ---\n%s", err, body)
		}
	})
}

// TestSectionBoundingLeak covers F5: with the "## Checklist" heading deleted, a
// checked checklist box falls inside the docs-impact bound. Because F3 keys on
// the two named options, such an unrelated checked box no longer counts, so this
// body — both docs boxes unchecked — MUST be rejected.
func TestSectionBoundingLeak(t *testing.T) {
	tmpl := readTemplate(t)
	noHeading := deleteChecklistHeading(tmpl)
	mustChange(t, "leak/delete-heading", tmpl, noHeading)
	body := checkNthBoxes(noHeading, 3) // check a checklist box; docs boxes untouched
	mustChange(t, "leak/check-box", noHeading, body)
	if err := docsimpact.Check(body); err == nil {
		t.Fatalf("section-bounding-leak control PASSED but must fail: a checked "+
			"checklist box leaked into the docs-impact bound and was accepted.\n--- body ---\n%s", body)
	}
}

// TestIndentedCodeControl covers M8 (REQ-1): a four-space-indented checkbox line
// is an INDENTED code block — GitHub renders it literally, exactly like a fenced
// block — so a checkbox inside it is not a real answer. With both real boxes left
// unchecked this body MUST be rejected. This is the control the fence-only
// stripper could not have: it exercises the construct that a code-region
// enumeration (fences only) misses, which the markdown-aware okf.MaskCode covers.
func TestIndentedCodeControl(t *testing.T) {
	tmpl := readTemplate(t)
	body := insertIndentedCheckbox(tmpl)
	mustChange(t, "indented-code-checkbox", tmpl, body)
	if err := docsimpact.Check(body); err == nil {
		t.Fatalf("indented-code control PASSED but must fail: a checkbox inside a "+
			"four-space-indented code block was read as a real answer.\n--- body ---\n%s", body)
	}
}
