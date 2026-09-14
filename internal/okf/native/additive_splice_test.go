package native

import (
	"strings"
	"testing"
)

// Issue #142: appending a stamp to a verified sequence must be STRICTLY
// ADDITIVE — every pre-existing byte of the document survives, in order, and the
// only new bytes are the appended stamp. The verified sequence is trust-bearing,
// so a splice that rewrites prior bytes (here: silently dropping the blank line
// that separated two entries) breaks the invariant the trust guarantee rests on,
// however cosmetic the dropped bytes look in a terminal.
//
// The assertions below are BYTE-level on purpose. A missing blank line is exactly
// the kind of difference a visual check waves through, so each case diffs the
// serialized output against an untouched copy of the input and requires the
// difference to be a single contiguous INSERTION.

// singleInsertion reports whether out is orig with exactly one contiguous run of
// bytes inserted, and returns that run. It is the byte-level statement of "strictly
// additive": it computes the longest common prefix and the longest common suffix
// of the two byte strings and requires them to account for ALL of orig. Any
// deleted, reordered or rewritten pre-existing byte makes prefix+suffix fall short
// of len(orig) and the check fails.
func singleInsertion(orig, out string) (inserted string, ok bool) {
	if len(out) < len(orig) {
		return "", false
	}
	p := 0
	for p < len(orig) && orig[p] == out[p] {
		p++
	}
	s := 0
	for s < len(orig)-p && orig[len(orig)-1-s] == out[len(out)-1-s] {
		s++
	}
	if p+s != len(orig) {
		return "", false
	}
	return out[p : len(out)-s], true
}

// TestSingleInsertion_PositiveControl is the positive control for the detector
// used by the tests below: it proves the check CAN show a difference, so a green
// additivity assertion is evidence and not a vacuous pass. The controls are the
// two failure modes #142 is about — a pre-existing blank line dropped, and a
// pre-existing byte rewritten in place.
func TestSingleInsertion_PositiveControl(t *testing.T) {
	orig := "a: 1\n\nb: 2\n"
	cases := []struct {
		name         string
		out          string
		want         bool
		wantInserted string // the run the detector must report (checked when want)
	}{
		{"identical", orig, true, ""},
		{"pure_append", "a: 1\n\nb: 2\nc: 3\n", true, "c: 3\n"},
		// A genuine INTERIOR insertion: the run does not land at the end of the
		// document, so this row is not `pure_append` by another route, and it is the
		// only row that exercises the suffix loop and its `s < len(orig)-p` cap. An
		// append to a sequence lands mid-document whenever another key follows it.
		// wantInserted is asserted so the row cannot silently degenerate into a
		// duplicate of pure_append again.
		{"insert_in_middle", "a: 1\nX\n\nb: 2\n", true, "X\n"},
		{"blank_line_dropped", "a: 1\nb: 2\nc: 3\n", false, ""},
		{"byte_rewritten", "a: 9\n\nb: 2\nc: 3\n", false, ""},
		{"line_reordered", "\na: 1\nb: 2\n", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := singleInsertion(orig, tc.out)
			if ok != tc.want {
				t.Fatalf("singleInsertion(%q, %q) ok = %v, want %v", orig, tc.out, ok, tc.want)
			}
			if tc.wantInserted != "" && got != tc.wantInserted {
				t.Errorf("inserted run = %q, want %q", got, tc.wantInserted)
			}
		})
	}
}

// TestVerifiedAppendIsStrictlyAdditive is the #142 regression test. Each row is a
// `verified` sequence whose entries are separated by a blank line; a third stamp
// is appended with the real convert.applyVerifiedBy pattern (see appendStamp).
// The document is then diffed byte-for-byte against an untouched copy of the
// input: the ONLY difference may be the inserted stamp.
func TestVerifiedAppendIsStrictlyAdditive(t *testing.T) {
	cases := []struct {
		name string
		fm   string
	}{
		{
			// The plain-scalar variant. It is the row that proves the defect is in
			// the SEQUENCE SPLICE and not in block-scalar span math (issue #132):
			// there is no block scalar anywhere in it, and the blank line is still
			// dropped.
			"plain_scalars_blank_separated",
			"type: Metric\nverified:\n  - by: human:x\n    at: 2024-02-01T09:30:00Z\n\n  - by: human:y\n    at: 2024-03-01T09:30:00Z\n",
		},
		{
			// The shape #142 was first reported with: a keep-chomping block scalar
			// (`|+`, which PRESERVES trailing newlines) ends an entry, so the blank
			// line that follows is both part of that scalar's value and the visual
			// separator before the next entry. Dropping it silently rewrites a
			// trust-bearing value, not just whitespace.
			"keep_chomping_block_scalar",
			"type: Metric\nverified:\n  - by: human:x\n    note: |+\n      kept\n\n  - by: human:y\n    at: 2024-03-01T09:30:00Z\n",
		},
		{
			// Flow-item entries (the shape T_blockseq_blank_separated already pins for
			// entry survival) with the blank separator.
			"flow_items_blank_separated",
			"type: Metric\nverified:\n  - { by: human:x, at: 2024-02-01T09:30:00Z }\n\n  - { by: human:y, at: 2024-03-01T09:30:00Z }\n",
		},
		{
			// A clamping block scalar in a MIDDLE field, with the blank separator
			// after the entry: the two span models (block-scalar body, inter-entry
			// gap) have to compose.
			"block_scalar_midfield_blank_separated",
			"type: Metric\nverified:\n  - by: human:x\n    note: |\n      multi\n      line\n    at: 2024-02-01T09:30:00Z\n\n  - by: human:y\n    at: 2024-03-01T09:30:00Z\n",
		},
		{
			// Three entries, two separators: the gap must be carried for EVERY
			// pre-existing pair, not just the first.
			"three_entries_two_blanks",
			"type: Metric\nverified:\n  - by: human:x\n    at: 2024-02-01T09:30:00Z\n\n  - by: human:y\n    at: 2024-03-01T09:30:00Z\n\n  - by: human:z\n    at: 2024-04-01T09:30:00Z\n",
		},
		{
			// TWO blank lines between entries. It carries verbatim — and it is what
			// keeps the doubled-blank assertion below honest: that assertion compares
			// the "\n\n\n" count against the INPUT, so a row that actually contains
			// one is what proves the comparison is not a disguised "must be zero".
			"two_blank_separator",
			"type: Metric\nverified:\n  - by: human:x\n    at: 2024-02-01T09:30:00Z\n\n\n  - by: human:y\n    at: 2024-03-01T09:30:00Z\n",
		},
		{
			// A key AFTER verified: the appended stamp must not disturb the bytes that
			// follow the sequence either.
			"trailing_key_after_sequence",
			"type: Metric\nverified:\n  - by: human:x\n    at: 2024-02-01T09:30:00Z\n\n  - by: human:y\n    at: 2024-03-01T09:30:00Z\nstatus: draft\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := "---\n" + tc.fm + "---\n\n# Body\n" // the untouched copy
			out, reparseErr := appendStamp(t, tc.fm)
			if reparseErr != nil {
				t.Fatalf("output does not re-parse (%v):\n%s", reparseErr, out)
			}
			// Anti-vacuity: the append actually landed, so this is a real splice and
			// not a no-op that trivially preserves the input.
			if !strings.Contains(out, "human:ghchinoy") {
				t.Fatalf("appended stamp missing — the sequence did not change:\n%s", out)
			}
			inserted, ok := singleInsertion(orig, out)
			if !ok {
				t.Fatalf("append was NOT strictly additive — pre-existing bytes changed.\n"+
					"--- want (untouched copy, %d bytes) ---\n%s\n--- got (%d bytes) ---\n%s\n--- line diff ---\n%s",
					len(orig), orig, len(out), out, lineDiff(orig, out))
			}
			// The inserted run is the stamp and nothing else.
			if !strings.Contains(inserted, "human:ghchinoy") {
				t.Errorf("inserted run does not contain the stamp: %q", inserted)
			}
			for _, sentinel := range []string{"human:x", "human:y", "human:z", "2024-"} {
				if strings.Contains(inserted, sentinel) {
					t.Errorf("inserted run re-emits pre-existing content %q: %q", sentinel, inserted)
				}
			}
			// The inverse failure: a fix that restores the missing separator by
			// EMITTING one would double the blank lines somewhere. Two assertions,
			// because singleInsertion has already established that out is orig with
			// exactly ONE contiguous run inserted — so a blank line can only be
			// invented by that run itself, or at one of its two seams.
			//
			// (i) the seams: the number of doubled-blank runs is unchanged. Compared
			// against the INPUT, not against zero, because a source that separates its
			// entries with two blank lines is carried through correctly (see the
			// two_blank_separator row) and an assertion stricter than the property
			// would red on that correct output.
			if got, want := strings.Count(out, "\n\n\n"), strings.Count(orig, "\n\n\n"); got != want {
				t.Errorf("blank-line structure changed: input has %d doubled-blank run(s), output has %d:\n%s",
					want, got, out)
			}
			// (ii) the run: the appended stamp brings no blank line of its own.
			if strings.Contains(inserted, "\n\n") {
				t.Errorf("inserted run introduces a blank line: %q", inserted)
			}
		})
	}
}

// TestSequenceRemovalDoesNotMisattributeTrivia pins the guard that keeps the
// gap-carrying rule honest when the desired list is SHORTER than the source.
//
// Gap lines are attached to the entry that FOLLOWS them, which is right for an
// append and wrong for a removal: the comment introducing a removed entry would
// be re-emitted in front of a different entry, leaving it annotating an
// attestation it was never about. Mis-attributed trivia in a trust-bearing
// sequence is a worse failure than dropped trivia, so a shrinking list falls back
// to the whole-value re-encode (spliceSequenceItems returns ok=false) and the
// trivia is dropped rather than moved.
//
// No caller shrinks a frontmatter list today — every writer is
// append-preserving-prefix — so this pins a contract of the general helper, not a
// reachable code path. Reported as PR #189 review finding 1.
func TestSequenceRemovalDoesNotMisattributeTrivia(t *testing.T) {
	fm := "type: Metric\n" +
		"verified:\n" +
		"  - by: human:x\n" +
		"    at: 2024-02-01T09:30:00Z\n" +
		"  # this comment is ABOUT y\n" +
		"  - by: human:y\n" +
		"    at: 2024-03-01T09:30:00Z\n" +
		"  - by: human:z\n" +
		"    at: 2024-04-01T09:30:00Z\n"
	raw := "---\n" + fm + "---\n\n# Body\n"

	c := New()
	con, err := c.ParseConcept("x.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	v, _ := con.Frontmatter.Get("verified")
	list, ok := v.([]any)
	if !ok || len(list) != 3 {
		t.Fatalf("fixture did not parse as a 3-entry list: %T %v", v, v)
	}
	// Remove the MIDDLE entry (the one the comment is about).
	con.Frontmatter.Set("verified", []any{list[0], list[2]})
	outB, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	out := string(outB)
	if _, err := c.ParseConcept("x.md", outB); err != nil {
		t.Fatalf("output does not re-parse (%v):\n%s", err, out)
	}
	// Anti-vacuity: the removal really happened.
	if strings.Contains(out, "human:y") {
		t.Fatalf("removed entry still present — this run does not exercise a removal:\n%s", out)
	}
	if !strings.Contains(out, "human:x") || !strings.Contains(out, "human:z") {
		t.Fatalf("surviving entries lost:\n%s", out)
	}
	// The property: the comment is NOT carried onto a neighbour it was never about.
	if strings.Contains(out, "this comment is ABOUT y") {
		t.Errorf("trivia for the REMOVED entry survived — it now annotates a different "+
			"attestation, which is the mis-attribution the removal guard exists to prevent:\n%s", out)
	}
}

// lineDiff renders a compact line-level side-by-side of two documents, used only
// to make a failure of the byte-level check readable.
func lineDiff(a, b string) string {
	al, bl := strings.SplitAfter(a, "\n"), strings.SplitAfter(b, "\n")
	var sb strings.Builder
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		mark := "  "
		if x != y {
			mark = "!!"
		}
		sb.WriteString(mark + " orig=" + quote(x) + " out=" + quote(y) + "\n")
	}
	return sb.String()
}

func quote(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\t", "\\t")
}

// TestVerifiedAppendEdgeCases covers the start and end of the sequence, where
// this class of fix usually breaks: there is no pre-existing gap to carry, so
// nothing may be invented. Empty and single-entry sequences must still append
// correctly and must not gain a blank line.
func TestVerifiedAppendEdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		fm       string
		additive bool // whether the shape is expected to be byte-additive at all
	}{
		// An empty sequence has no pre-existing entry: the value is re-encoded, so
		// byte-additivity is not the property (only correctness + no stray blanks).
		{"empty_flow_sequence", "type: Metric\nverified: []\n", false},
		{"no_verified_key", "type: Metric\n", false},
		// A single BLOCK entry, no separators anywhere: the append must be additive
		// and must not introduce a gap that was never there.
		{"single_block_entry", "type: Metric\nverified:\n  - by: human:x\n    at: 2024-02-01T09:30:00Z\n", true},
		{"single_flow_item_entry", "type: Metric\nverified:\n  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", true},
		// Two adjacent entries with NO blank between them: none may appear.
		{"two_entries_no_blank", "type: Metric\nverified:\n  - by: human:x\n    at: 2024-02-01T09:30:00Z\n  - by: human:y\n    at: 2024-03-01T09:30:00Z\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := "---\n" + tc.fm + "---\n\n# Body\n"
			out, reparseErr := appendStamp(t, tc.fm)
			if reparseErr != nil {
				t.Fatalf("output does not re-parse (%v):\n%s", reparseErr, out)
			}
			if !strings.Contains(out, "human:ghchinoy") {
				t.Fatalf("appended stamp missing — the sequence did not change:\n%s", out)
			}
			// No blank line may be invented INSIDE the frontmatter (the blank between
			// the closing fence and the body is part of every fixture here).
			fmOut := frontmatterRegion(t, out)
			if strings.Contains(fmOut, "\n\n") {
				t.Errorf("blank line invented inside frontmatter:\n%s", fmOut)
			}
			if tc.additive {
				if _, ok := singleInsertion(orig, out); !ok {
					t.Errorf("append was NOT strictly additive — pre-existing bytes changed.\n"+
						"--- orig ---\n%s\n--- got ---\n%s", orig, out)
				}
			}
		})
	}
}

// frontmatterRegion returns the text between the two "---" fences of doc.
func frontmatterRegion(t *testing.T, doc string) string {
	t.Helper()
	rest, ok := strings.CutPrefix(doc, "---\n")
	if !ok {
		t.Fatalf("document has no opening fence:\n%s", doc)
	}
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		t.Fatalf("document has no closing fence:\n%s", doc)
	}
	return rest[:i+1]
}
