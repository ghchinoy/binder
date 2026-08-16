package native

import (
	"strings"
	"testing"
)

// This file is the PERMANENT, table-driven record of the shape space probed for
// the flow/mapping sibling-preservation work (see the Part B ruling). Each row is
// one authored `verified` container shape; the append is the real
// convert.applyVerifiedBy pattern (normalize to []any, append a stamp). The
// twenty rows split in two:
//
//   - TestByteFaithfulShapeMatrix pins the shapes whose pre-existing entries
//     SURVIVE the append: it asserts the entry's source BYTES/LINES are present
//     verbatim, the pre-existing scalar TAG is unchanged (the !!timestamp->!!str
//     retype cannot hide behind identical-looking text), AND the whole document
//     REPARSES. These are invariants; a red here is a regression.
//   - TestCharacterize_ChangedContainerLosesInterleavedFormatting and
//     TestCharacterize_EmptyFlowMapReshapedOnAppend pin the shapes whose entries
//     survive but whose INTERLEAVED content (comments, an empty-map reshape) does
//     NOT. Following the TestCharacterize_ convention already used in this
//     codebase, they record CURRENT behaviour, not a guarantee: comment loss is a
//     yaml.v3 node-model limit on the changed path, not a design choice. A future
//     change that carries comments should update these to match, not read a red
//     here as a bug it caused.
//
// Together these twenty rows are the evidence for exactly what docs/user_guide.md
// section 3 is permitted to claim.

// appendStamp runs the applyVerifiedBy append against a frontmatter interior and
// returns the serialized document plus the error from re-parsing that output.
// Re-parseability is a first-class property here: the multi-line-flow defect
// produced byte-plausible output that did not parse, so a bytes-only check could
// pass while the file was broken.
func appendStamp(t *testing.T, fmBody string) (out string, reparseErr error) {
	t.Helper()
	raw := "---\n" + fmBody + "---\n\n# Body\n"
	c := New()
	con, err := c.ParseConcept("x.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	v, _ := con.Frontmatter.Get("verified")
	var list []any
	switch vv := v.(type) {
	case []any:
		list = vv
	case map[string]any:
		list = []any{vv}
	case nil:
		list = nil
	default:
		t.Fatalf("unexpected verified shape %T", v)
	}
	con.Frontmatter.Set("verified", append(list,
		map[string]any{"by": "human:ghchinoy", "at": "2023-11-14T22:13:20Z"}))
	outB, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	_, reparseErr = c.ParseConcept("x.md", outB)
	return string(outB), reparseErr
}

// firstTag returns the "tag|style" of the first of paths present in m.
func firstTag(m map[string]string, paths ...string) (string, bool) {
	for _, p := range paths {
		if v, ok := m[p]; ok {
			return v, true
		}
	}
	return "", false
}

// TestByteFaithfulShapeMatrix pins every container shape whose pre-existing
// entries survive an append verbatim. atTag is the tag the pre-existing timestamp
// must keep ("" for shapes with no scalar timestamp at verified[0].at, e.g. an
// alias node or a null sub-value, where entry-byte survival is the property).
func TestByteFaithfulShapeMatrix(t *testing.T) {
	cases := []struct {
		name     string
		fm       string
		sentinel string // pre-existing entry text that must survive verbatim
		atTag    string // required tag prefix of verified[0].at, or "" to skip
	}{
		{
			"A_blockseq_flow_item",
			"type: Metric\nverified:\n  - { by: human:x, at: 2024-02-01T09:30:00Z }\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "!!timestamp",
		},
		{
			"B_blockseq_block_item",
			"type: Metric\nverified:\n  - by: human:x\n    at: 2024-02-01T09:30:00Z\n",
			"  - by: human:x\n    at: 2024-02-01T09:30:00Z\n", "!!timestamp",
		},
		{
			"C_blockseq_deep_nested",
			"type: Metric\nverified:\n  - by: human:x\n    meta:\n      role: reviewer\n    at: 2024-02-01T09:30:00Z\n",
			"  - by: human:x\n    meta:\n      role: reviewer\n    at: 2024-02-01T09:30:00Z\n", "!!timestamp",
		},
		{
			"D_blockseq_block_scalar",
			"type: Metric\nverified:\n  - by: human:x\n    note: |\n      multi\n      line\n    at: 2024-02-01T09:30:00Z\n",
			"    note: |\n      multi\n      line\n", "!!timestamp",
		},
		{
			"G_blockseq_anchor",
			"type: Metric\nverified:\n  - &att { by: human:x, at: 2024-02-01T09:30:00Z }\n",
			"  - &att { by: human:x, at: 2024-02-01T09:30:00Z }\n", "!!timestamp",
		},
		{
			"H_flowseq_singleline_two",
			"type: Metric\nverified: [{ by: human:a, at: 2024-02-01T09:30:00Z }, { by: human:b, at: 2024-03-01T09:30:00Z }]\n",
			"  - { by: human:a, at: 2024-02-01T09:30:00Z }\n", "!!timestamp",
		},
		{
			"I_flowmap_bare",
			"type: Metric\nverified: { by: human:x, at: 2024-02-01T09:30:00Z }\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "!!timestamp",
		},
		{
			"J_flowmap_double_quoted",
			"type: Metric\nverified: { by: \"human:x\", at: \"2024-02-01T09:30:00Z\" }\n",
			"  - { by: \"human:x\", at: \"2024-02-01T09:30:00Z\" }\n", "!!str",
		},
		{
			"K_blockmap_nested",
			"type: Metric\nverified:\n  by: human:x\n  at: 2024-02-01T09:30:00Z\n",
			"  - by: human:x\n    at: 2024-02-01T09:30:00Z\n", "!!timestamp",
		},
		{
			"L_blockmap_deep",
			"type: Metric\nverified:\n  by: human:x\n  meta:\n    role: reviewer\n  at: 2024-02-01T09:30:00Z\n",
			"  - by: human:x\n    meta:\n      role: reviewer\n    at: 2024-02-01T09:30:00Z\n", "!!timestamp",
		},
		{
			"M_flowseq_nested_collection",
			"type: Metric\nverified: [{ by: human:x, at: 2024-02-01T09:30:00Z, tags: [a, b] }]\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z, tags: [a, b] }\n", "!!timestamp",
		},
		{
			"O_null_sub_value",
			"type: Metric\nverified:\n  - by: human:x\n    at:\n",
			"  - by: human:x\n    at:\n", "",
		},
		{
			"Q_alias_item",
			"type: Metric\nanchors:\n  base: &b { by: human:x, at: 2024-02-01T09:30:00Z }\nverified:\n  - *b\n",
			"  - *b\n", "",
		},
		{
			"R_merge_key_item",
			"type: Metric\nverified:\n  - <<: &m { by: human:x }\n    at: 2024-02-01T09:30:00Z\n",
			"  - <<: &m { by: human:x }\n    at: 2024-02-01T09:30:00Z\n", "!!timestamp",
		},
		{
			"S_flowseq_multiline",
			"type: Metric\nverified: [\n  { by: human:x, at: 2024-02-01T09:30:00Z },\n  { by: human:y, at: 2024-03-01T09:30:00Z },\n]\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "!!timestamp",
		},
		{
			"T_blockseq_blank_separated",
			"type: Metric\nverified:\n  - { by: human:x, at: 2024-02-01T09:30:00Z }\n\n  - { by: human:y, at: 2024-03-01T09:30:00Z }\n",
			"  - { by: human:y, at: 2024-03-01T09:30:00Z }\n", "!!timestamp",
		},
		{
			// A '#' inside a QUOTED scalar is not a comment. This is the single most
			// likely place the comment-aware scanner regresses: it pins that a quoted
			// hash survives an append verbatim (bytes + !!str tag) rather than being
			// mistaken for a comment and truncated. See also TestCommentEnd and
			// TestSplitFlowSeqItems_CommentAware for the rule in isolation.
			"U_flowseq_multiline_quoted_hash",
			"type: Metric\nverified: [\n  { by: \"human:x #1\", at: \"2024-02-01T09:30:00Z\" },\n  { by: human:y, at: 2024-03-01T09:30:00Z },\n]\n",
			"  - { by: \"human:x #1\", at: \"2024-02-01T09:30:00Z\" }\n", "!!str",
		},
		{
			// A double-quoted scalar with an ODD number of backslash-escaped double
			// quotes: the quote tracker used to treat the escaped '"' as a close, so
			// the parity flipped and the entry's own closing '}' (then the flow ']')
			// was scanned as ordinary text — producing UNPARSEABLE output. Pins that
			// the entry survives verbatim, tag intact, and the document re-parses.
			"W_flowseq_multiline_escaped_dquote",
			"type: Metric\nverified: [\n  { by: \"he said \\\"hi\", at: 2024-02-01T09:30:00Z },\n  { by: human:y, at: 2024-03-01T09:30:00Z },\n]\n",
			"  - { by: \"he said \\\"hi\", at: 2024-02-01T09:30:00Z }\n", "!!timestamp",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := scalarTags(t, frontmatterOf(t, "---\n"+tc.fm+"---\n\n# Body\n"))
			out, reparseErr := appendStamp(t, tc.fm)

			// Anti-vacuity: the append actually landed, so the container changed and
			// the test is not "preserving" a no-op.
			if !strings.Contains(out, "human:ghchinoy") {
				t.Fatalf("appended stamp missing — container did not change:\n%s", out)
			}
			// The property that failed for multi-line flow was PARSEABILITY, so it is
			// asserted for every row, not just S.
			if reparseErr != nil {
				t.Errorf("output does not re-parse (%v):\n%s", reparseErr, out)
			}
			// Byte/line survival of the pre-existing entry.
			if !strings.Contains(out, tc.sentinel) {
				t.Errorf("pre-existing entry not preserved verbatim; want to contain:\n%q\ngot:\n%s", tc.sentinel, out)
			}
			// Tag survival (the !!timestamp->!!str retype detector). Skipped for rows
			// with no scalar timestamp at verified[0].at.
			if tc.atTag != "" {
				bt, ok := firstTag(before, ".verified[0].at", ".verified.at")
				if !ok || !strings.HasPrefix(bt, tc.atTag) {
					t.Fatalf("setup: pre-existing timestamp tag is %q, want %s* — tag assertion would be vacuous", bt, tc.atTag)
				}
				after := scalarTags(t, frontmatterOf(t, out))
				if at := after[".verified[0].at"]; !strings.HasPrefix(at, tc.atTag) {
					t.Errorf("verified[0].at retyped to %q, want %s*:\n%s", at, tc.atTag, out)
				}
			}
		})
	}
}

// TestByteFaithfulUnchangedKeysEnumeration is the permanent evidence for the
// preservation enumeration in docs/user_guide.md section 3: when the codec changes
// ONE key (here `verified`), every OTHER top-level key is re-emitted from its
// source bytes verbatim. Each row exercises one property from the enumeration —
// interior spacing, scalar quoting (incl. a '#' inside quotes), a FOLDED scalar
// (`>`, not literal `|`: a folded scalar reflows under a full re-encode, so the row
// reddens if byte-copy breaks; a literal `|` survives re-encode and would be a
// vacuous guard), nested-map key order, a YAML tag, a top-level head comment, and a
// same-line comment on an unchanged key. The last two pin the TRUE half of the
// comments claim (comments on UNCHANGED keys survive); the container-kind
// asymmetry for CHANGED containers is pinned separately by the Characterize tests
// and TestByteFaithfulMultiLineFlowSequenceWithCommentReparses. If a future change
// re-encodes unchanged keys instead of byte-copying them, this goes red — which is
// the signal the enumeration is no longer safe to assert.
func TestByteFaithfulUnchangedKeysEnumeration(t *testing.T) {
	fm := "# top head comment\n" +
		"type: Metric\n" +
		"flowmap: { a: 1,  b: 2 }\n" + // interior spacing (double space)
		"quoted: \"a: b # not-a-comment\"\n" + // quoting + a '#' inside quotes
		"folded: >\n  wrapped\n  text\n" + // folded scalar: reflows under re-encode, so a non-vacuous guard
		"nested:\n  z: 1\n  a: 2\n" + // nested-map key order (non-alpha)
		"tagged: 2024-02-01T09:30:00Z\n" + // YAML tag (!!timestamp)
		"withcomment: 42 # keep me\n" + // same-line comment on an unchanged key
		"verified: [\n  { by: human:x, at: 2024-02-01T09:30:00Z },\n]\n"

	out, reparseErr := appendStamp(t, fm)
	if reparseErr != nil {
		t.Fatalf("output does not re-parse (%v):\n%s", reparseErr, out)
	}
	// Anti-vacuity: the change actually landed on `verified`.
	if !strings.Contains(out, "human:ghchinoy") {
		t.Fatalf("appended stamp missing — nothing changed, so unchanged-key preservation is vacuous:\n%s", out)
	}
	for _, tc := range []struct{ prop, want string }{
		{"head comment", "# top head comment\n"},
		{"interior spacing", "flowmap: { a: 1,  b: 2 }\n"},
		{"quoting + hash-in-quotes", "quoted: \"a: b # not-a-comment\"\n"},
		{"folded scalar", "folded: >\n  wrapped\n  text\n"},
		{"nested-map key order", "nested:\n  z: 1\n  a: 2\n"},
		{"YAML tag", "tagged: 2024-02-01T09:30:00Z\n"},
		{"same-line comment on unchanged key", "withcomment: 42 # keep me\n"},
	} {
		if !strings.Contains(out, tc.want) {
			t.Errorf("unchanged key not byte-faithful (%s); want to contain:\n%q\ngot:\n%s", tc.prop, tc.want, out)
		}
	}
}

// TestByteFaithfulMultiLineFlowSequenceReparses is the dedicated regression for
// the corruption defect (case S). A multi-line flow sequence (its "[" on the key
// line, items on their own lines, "]" on a trailing line) used to fall through to
// a whole-value re-encode that left the orphan "]" behind, producing UNPARSEABLE
// output. The decisive assertion is that the result RE-PARSES; the byte and tag
// checks pin that the pre-existing entries were not retyped in the process.
func TestByteFaithfulMultiLineFlowSequenceReparses(t *testing.T) {
	fm := "type: Metric\nverified: [\n" +
		"  { by: human:x, at: 2024-02-01T09:30:00Z },\n" +
		"  { by: human:y, at: 2024-03-01T09:30:00Z },\n" +
		"]\n"

	out, reparseErr := appendStamp(t, fm)

	if !strings.Contains(out, "human:ghchinoy") {
		t.Fatalf("appended stamp missing — container did not change:\n%s", out)
	}
	// THE property that failed: the output must parse. A bytes-only check could
	// pass while the file is still broken, so this is asserted first and loudly.
	if reparseErr != nil {
		t.Fatalf("multi-line flow sequence produced UNPARSEABLE output (%v):\n%s", reparseErr, out)
	}
	// No stray closing bracket leaked onto its own line.
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "]" || strings.TrimSpace(line) == "}" {
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
}

// TestCharacterize_ChangedContainerLosesInterleavedFormatting records CURRENT
// behaviour, NOT a guarantee: when a container is CHANGED (a stamp appended), the
// pre-existing ENTRIES survive but INTERLEAVED formatting between or around them —
// YAML comments and blank separator lines — is not carried onto the rebuilt
// value. This is a yaml.v3 node-model limitation on the changed path (those trivia
// are not attached to the entry nodes the splice copies), not a design choice. A
// future change that preserves them should update this test to match rather than
// read a red here as a regression it caused. The name is deliberately not
// invariant-shaped so it cannot launder this limit into a promise.
//
// Anti-vacuity: each row first asserts the pre-existing ENTRY did survive, so the
// test pins "entry kept, trivia dropped" and not a wholesale re-encode.
func TestCharacterize_ChangedContainerLosesInterleavedFormatting(t *testing.T) {
	cases := []struct {
		name    string
		fm      string
		entry   string // must survive
		dropped string // currently NOT carried
	}{
		{
			"E_comment_before_first_item",
			"type: Metric\nverified:\n  # leading comment\n  - { by: human:x, at: 2024-02-01T09:30:00Z }\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "# leading comment",
		},
		{
			"F_comment_between_items",
			"type: Metric\nverified:\n  - { by: human:x, at: 2024-02-01T09:30:00Z }\n  # mid comment\n  - { by: human:y, at: 2024-03-01T09:30:00Z }\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "# mid comment",
		},
		{
			"N_flowmap_trailing_comment",
			"type: Metric\nverified: { by: human:x, at: 2024-02-01T09:30:00Z } # trailing\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "# trailing",
		},
		{
			// Previously UNPARSEABLE: the interior comment's bytes were scanned into the
			// flow value, so the rebuilt collection did not parse. The comment-aware
			// scanner now drops the interior comment and preserves the entries; comment
			// loss on a CHANGED container is the same limit recorded by E/F/N.
			"P_flowseq_multiline_comment_between",
			"type: Metric\nverified: [\n  { by: human:x, at: 2024-02-01T09:30:00Z }, # mid\n  { by: human:y, at: 2024-03-01T09:30:00Z },\n]\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "# mid",
		},
		{
			// The comment carries a stray ']' — the exact byte that used to be counted
			// into the flow depth and corrupt the scan. It is dropped whole.
			"V_flowseq_multiline_comment_with_bracket",
			"type: Metric\nverified: [\n  { by: human:x, at: 2024-02-01T09:30:00Z }, # note ]\n  { by: human:y, at: 2024-03-01T09:30:00Z },\n]\n",
			"  - { by: human:x, at: 2024-02-01T09:30:00Z }\n", "# note ]",
		},
		{
			"T_blank_line_between_items",
			"type: Metric\nverified:\n  - { by: human:x, at: 2024-02-01T09:30:00Z }\n\n  - { by: human:y, at: 2024-03-01T09:30:00Z }\n",
			"  - { by: human:y, at: 2024-03-01T09:30:00Z }\n", "}\n\n  -",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, reparseErr := appendStamp(t, tc.fm)
			if reparseErr != nil {
				t.Fatalf("output does not re-parse (%v):\n%s", reparseErr, out)
			}
			// Anti-vacuity: the entry itself survived (this is not a re-encode).
			if !strings.Contains(out, tc.entry) {
				t.Fatalf("pre-existing entry was NOT preserved (so this row does not isolate a trivia loss):\n%s", out)
			}
			// CURRENT behaviour: the interleaved trivia is dropped.
			if strings.Contains(out, tc.dropped) {
				t.Errorf("this test records trivia LOSS, but %q survived — behaviour changed; update this characterization:\n%s", tc.dropped, out)
			}
		})
	}
}

// TestCharacterize_EmptyFlowMapReshapedOnAppend records CURRENT behaviour: an
// EMPTY flow mapping (`verified: {}`) has no pre-existing entry to preserve, and
// on append its `{}` is reshaped into a block-sequence item (`- {}`) rather than
// kept verbatim. Recorded, not guaranteed; a future change may keep it verbatim.
func TestCharacterize_EmptyFlowMapReshapedOnAppend(t *testing.T) {
	out, reparseErr := appendStamp(t, "type: Metric\nverified: {}\n")
	if reparseErr != nil {
		t.Fatalf("output does not re-parse (%v):\n%s", reparseErr, out)
	}
	if !strings.Contains(out, "human:ghchinoy") {
		t.Fatalf("appended stamp missing — container did not change:\n%s", out)
	}
	if strings.Contains(out, "verified: {}") {
		t.Errorf("this test records the {} -> '- {}' reshape, but 'verified: {}' survived; update this characterization:\n%s", out)
	}
	if !strings.Contains(out, "- {}") {
		t.Errorf("expected the empty map reshaped to a block item '- {}':\n%s", out)
	}
}
