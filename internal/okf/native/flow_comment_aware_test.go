package native

import (
	"strings"
	"testing"
)

// This file is the permanent record for the comment-aware flow scanner (the R1
// fix). Before it, a multi-line flow collection with ANY interior YAML comment
// produced UNPARSEABLE frontmatter: matchFlow/splitFlowSeqItems scanned the
// comment's bytes as structure, so a '#' comment carrying a ',' or a ']'/'}' broke
// the split or the depth count. The fix teaches both scanners the YAML comment
// rule — a '#' begins a comment only OUTSIDE a quoted scalar AND at line-start or
// after whitespace — via the commentEnd helper.
//
// The tests split three ways:
//   - TestCommentEnd and TestSplitFlowSeqItems_CommentAware pin the RULE in
//     isolation, so "outside quotes AND at line-start or after whitespace" is
//     readable off the tests without reading commentEnd. The quoted-hash and
//     "a#b" cases are the ones most likely to be broken by a future refactor.
//   - TestByteFaithfulMultiLineFlowSequenceWithCommentReparses is the invariant:
//     a multi-line flow SEQUENCE with an interior comment (with or without a
//     stray bracket) must RE-PARSE and keep its entries. A red here is a
//     regression of the R1 defect.
//   - TestCharacterize_MultiLineFlowMappingWithComment records CURRENT behaviour
//     for the flow MAPPING shape, which was never unparseable but is re-emitted
//     un-indented (not byte-faithful). Recorded, not guaranteed.

// TestCommentEnd pins the low-level comment rule that both flow scanners rely on.
// commentEnd deliberately does NOT check quote state (its callers only invoke it
// outside quotes), so the "inside quotes" half of the rule is pinned at the
// scanner level by TestSplitFlowSeqItems_CommentAware.
func TestCommentEnd(t *testing.T) {
	cases := []struct {
		name    string
		s       string
		i       int  // index of the '#'
		wantOK  bool // is it a comment?
		wantEnd int  // index of the terminating '\n', or len(s)
	}{
		{"at_line_start", "#c\nx", 0, true, 2},
		{"after_space", "a #c\nx", 2, true, 4},
		{"after_tab", "a\t#c\nx", 2, true, 4},
		{"after_newline", "a\n#c\nx", 2, true, 4},
		{"runs_to_end_of_string", "a #c", 2, true, 4},
		{"no_preceding_whitespace_is_ordinary", "a#b", 1, false, 0},
		{"not_a_hash", "abc", 1, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			end, ok := commentEnd(tc.s, tc.i)
			if ok != tc.wantOK {
				t.Fatalf("commentEnd(%q, %d) ok = %v, want %v", tc.s, tc.i, ok, tc.wantOK)
			}
			if ok && end != tc.wantEnd {
				t.Errorf("commentEnd(%q, %d) end = %d, want %d", tc.s, tc.i, end, tc.wantEnd)
			}
		})
	}
}

// TestSplitFlowSeqItems_CommentAware pins the full rule through the splitter: a
// '#' inside quotes or with no preceding whitespace is ORDINARY (kept in the item
// text); a '#' at line-start or after whitespace is a COMMENT (dropped, and its
// bytes — including any ',' or ']' it carries — never affect the split).
func TestSplitFlowSeqItems_CommentAware(t *testing.T) {
	cases := []struct {
		name string
		seq  string
		want []string
	}{
		{
			"hash_in_double_quotes_is_not_a_comment",
			`[{ by: "human:x #1", at: "2024-02-01T09:30:00Z" }]`,
			[]string{`{ by: "human:x #1", at: "2024-02-01T09:30:00Z" }`},
		},
		{
			"hash_in_single_quotes_is_not_a_comment",
			`[{ by: 'a #b', at: v }]`,
			[]string{`{ by: 'a #b', at: v }`},
		},
		{
			"hash_with_no_preceding_whitespace_is_ordinary",
			`[{ by: a#b, at: v }]`,
			[]string{`{ by: a#b, at: v }`},
		},
		{
			"comment_after_whitespace_is_dropped",
			"[\n  { by: x }, # note\n  { by: y },\n]",
			[]string{"{ by: x }", "{ by: y }"},
		},
		{
			"comment_carrying_a_comma_does_not_split",
			"[\n  { by: x }, # a, b, c\n  { by: y },\n]",
			[]string{"{ by: x }", "{ by: y }"},
		},
		{
			"comment_carrying_a_bracket_does_not_break_depth",
			"[\n  { by: x }, # note ]\n  { by: y },\n]",
			[]string{"{ by: x }", "{ by: y }"},
		},
		{
			"line_start_comment_is_dropped",
			"[\n  # header\n  { by: x },\n]",
			[]string{"{ by: x }"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := splitFlowSeqItems(tc.seq)
			if !ok {
				t.Fatalf("splitFlowSeqItems(%q) ok = false, want true", tc.seq)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("splitFlowSeqItems(%q) = %q (%d items), want %q (%d items)",
					tc.seq, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("item %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestByteFaithfulMultiLineFlowSequenceWithCommentReparses is the invariant
// regression for the R1 defect. Both an interior comment WITHOUT a bracket and one
// WITH a stray ']' used to produce unparseable output (the reviewer flagged the
// bracket case; the plain-comment case is broader and equally broke). Both must now
// re-parse with the pre-existing entries preserved and untyped-changed, and with no
// orphan closer leaking onto its own line.
func TestByteFaithfulMultiLineFlowSequenceWithCommentReparses(t *testing.T) {
	cases := []struct {
		name string
		fm   string
	}{
		{
			"plain_interior_comment",
			"type: Metric\nverified: [\n" +
				"  { by: human:x, at: 2024-02-01T09:30:00Z }, # who\n" +
				"  { by: human:y, at: 2024-03-01T09:30:00Z },\n" +
				"]\n",
		},
		{
			"interior_comment_with_stray_bracket",
			"type: Metric\nverified: [\n" +
				"  { by: human:x, at: 2024-02-01T09:30:00Z }, # note ]\n" +
				"  { by: human:y, at: 2024-03-01T09:30:00Z },\n" +
				"]\n",
		},
		{
			"standalone_comment_line",
			"type: Metric\nverified: [\n" +
				"  # provenance\n" +
				"  { by: human:x, at: 2024-02-01T09:30:00Z },\n" +
				"  { by: human:y, at: 2024-03-01T09:30:00Z },\n" +
				"]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, reparseErr := appendStamp(t, tc.fm)
			if !strings.Contains(out, "human:ghchinoy") {
				t.Fatalf("appended stamp missing — container did not change:\n%s", out)
			}
			// THE property that failed: the output must parse.
			if reparseErr != nil {
				t.Fatalf("multi-line flow sequence with comment produced UNPARSEABLE output (%v):\n%s", reparseErr, out)
			}
			// No stray closer leaked onto its own line.
			for _, line := range strings.Split(out, "\n") {
				if s := strings.TrimSpace(line); s == "]" || s == "}" {
					t.Errorf("orphan flow closer leaked as a standalone line:\n%s", out)
				}
			}
			// Both pre-existing entries survive verbatim, not retyped.
			for _, want := range []string{
				"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n",
				"  - { by: human:y, at: 2024-03-01T09:30:00Z }\n",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("pre-existing entry not preserved; want to contain %q:\n%s", want, out)
				}
			}
			after := scalarTags(t, frontmatterOf(t, out))
			for _, p := range []string{".verified[0].at", ".verified[1].at"} {
				if at := after[p]; !strings.HasPrefix(at, "!!timestamp") {
					t.Errorf("%s retyped to %q, want !!timestamp:\n%s", p, at, out)
				}
			}
		})
	}
}

// TestCharacterize_MultiLineFlowMappingWithComment records CURRENT behaviour for
// the shape that the sequence defect is often confused with. A multi-line flow
// MAPPING (`verified: {` … `}`) with an interior comment was NEVER unparseable —
// unlike the sequence, it re-parses today. But it is not byte-faithful: the whole
// mapping value (comment included) is copied into a single block-sequence item
// WITHOUT re-indentation, so the interior lines and the closing `}` land at
// column 0 — valid YAML, ugly output. This differs from the sequence path, which
// splits per entry and drops interior comments. Recorded, not guaranteed: a future
// change that re-indents this (or that turns it into the sequence case) should
// update this test rather than read a red as a regression.
func TestCharacterize_MultiLineFlowMappingWithComment(t *testing.T) {
	fm := "type: Metric\nverified: {\n  by: human:x, # who\n  at: 2024-02-01T09:30:00Z,\n}\n"
	out, reparseErr := appendStamp(t, fm)
	if reparseErr != nil {
		t.Fatalf("multi-line flow mapping with comment should re-parse today, got %v:\n%s", reparseErr, out)
	}
	if !strings.Contains(out, "human:ghchinoy") {
		t.Fatalf("appended stamp missing — container did not change:\n%s", out)
	}
	// CURRENT behaviour: the value is copied verbatim (comment carried) rather than
	// re-indented as a clean block item. The interior lines keep their ORIGINAL
	// source columns instead of being re-indented under the `- ` marker, so the
	// comment survives (the opposite of the sequence path) and the closing `}` lands
	// at column 0 — valid YAML, but not byte-faithful re-indentation.
	if !strings.Contains(out, "  by: human:x, # who\n") {
		t.Errorf("expected the comment carried verbatim in the copied mapping; update this characterization:\n%s", out)
	}
	if !strings.Contains(out, "\n}\n") {
		t.Errorf("expected the closing '}' un-indented at column 0 (not re-indented under the '- ' marker); update this characterization:\n%s", out)
	}
}
