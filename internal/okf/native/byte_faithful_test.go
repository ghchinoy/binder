package native

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// scalarTags walks a YAML frontmatter block and records "tag|style" for every
// scalar node, keyed by a dotted/indexed path. It asserts on the YAML TAG, not
// just the bytes, because a future change could preserve bytes by luck while
// silently re-typing a value (the exact defect: !!timestamp -> !!str).
func scalarTags(t *testing.T, fmBlock string) map[string]string {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fmBlock), &doc); err != nil {
		t.Fatalf("re-parsing frontmatter for tag check: %v", err)
	}
	out := map[string]string{}
	var walk func(n *yaml.Node, path string)
	walk = func(n *yaml.Node, path string) {
		switch n.Kind {
		case yaml.DocumentNode:
			for _, c := range n.Content {
				walk(c, path)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				walk(n.Content[i+1], path+"."+n.Content[i].Value)
			}
		case yaml.SequenceNode:
			for i, c := range n.Content {
				walk(c, fmt.Sprintf("%s[%d]", path, i))
			}
		default:
			out[path] = fmt.Sprintf("%s|%d", n.Tag, n.Style)
		}
	}
	walk(&doc, "")
	return out
}

// frontmatterOf extracts the text between the first two "---" fences.
func frontmatterOf(t *testing.T, doc string) string {
	t.Helper()
	if !strings.HasPrefix(doc, "---\n") {
		t.Fatalf("document does not open with frontmatter fence")
	}
	rest := doc[4:]
	i := strings.Index(rest, "\n---")
	if i < 0 {
		t.Fatalf("frontmatter fence not closed")
	}
	return rest[:i+1]
}

// writeAddsKey parses raw, injects one new top-level key (forcing the codec's
// rebuild path — the same path enrich/convert take when they add a key), and
// returns the serialized document.
func writeAddsKey(t *testing.T, raw, key, val string) string {
	t.Helper()
	c := New()
	con, err := c.ParseConcept("x.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	con.Frontmatter.Set(key, val)
	out, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return string(out)
}

// Defect B: a flow MAPPING in a value must survive a write that adds an
// unrelated key — interior spacing, scalar quoting, AND the !!timestamp tag.
// The free discriminator (a flow SEQUENCE) pins the other direction: a correct
// fix leaves BOTH untouched. This is the two-sided control (criterion C).
func TestByteFaithfulFlowMappingAndSequence(t *testing.T) {
	raw := "---\n" +
		"type: Metric\n" +
		"tags: [finance, margin, deprecated]\n" +
		"generated: { by: human:jsmith@acme, at: 2024-01-15T10:00:00Z }\n" +
		"verified:\n" +
		"  - { by: human:jsmith@acme, at: 2024-01-15T10:00:00Z }\n" +
		"---\n\n# Body\n"

	before := scalarTags(t, frontmatterOf(t, raw))
	out := writeAddsKey(t, raw, "status", "deprecated")

	// The intended change happened.
	if !strings.Contains(out, "\nstatus: deprecated\n") {
		t.Fatalf("added key not present:\n%s", out)
	}
	// Flow mapping preserved byte-for-byte (interior spacing + bare scalars).
	if !strings.Contains(out, "generated: { by: human:jsmith@acme, at: 2024-01-15T10:00:00Z }\n") {
		t.Errorf("flow mapping was re-serialized:\n%s", out)
	}
	if !strings.Contains(out, "  - { by: human:jsmith@acme, at: 2024-01-15T10:00:00Z }\n") {
		t.Errorf("flow mapping inside sequence was re-serialized:\n%s", out)
	}
	// Free discriminator: flow SEQUENCE untouched.
	if !strings.Contains(out, "tags: [finance, margin, deprecated]\n") {
		t.Errorf("flow sequence was re-serialized:\n%s", out)
	}
	// The quoting is a TYPE change: assert on the YAML tag, not just bytes.
	after := scalarTags(t, frontmatterOf(t, out))
	for _, p := range []string{".generated.at", ".verified[0].at"} {
		if before[p] != after[p] {
			t.Errorf("%s tag/style changed: before=%s after=%s", p, before[p], after[p])
		}
		if !strings.HasPrefix(after[p], "!!timestamp") {
			t.Errorf("%s is no longer !!timestamp: %s", p, after[p])
		}
	}
}

// Criterion B — the special-casing detector. A flow mapping in an ORDINARY key
// (not generated/verified/any attestation key) must be preserved too. If this
// fails while the trust-key cases pass, the fix special-cased the trust keys and
// the guarantee is still false for everyone else.
func TestByteFaithfulNonTrustFlowMapping(t *testing.T) {
	raw := "---\n" +
		"type: Note\n" +
		"window: { from: 2024-01-15T10:00:00Z, to: 2024-06-30T00:00:00Z }\n" +
		"owner: { name: alice, team: data }\n" +
		"---\n\n# Body\n"

	before := scalarTags(t, frontmatterOf(t, raw))
	out := writeAddsKey(t, raw, "status", "active")

	if !strings.Contains(out, "window: { from: 2024-01-15T10:00:00Z, to: 2024-06-30T00:00:00Z }\n") {
		t.Errorf("ordinary-key flow mapping was re-serialized:\n%s", out)
	}
	if !strings.Contains(out, "owner: { name: alice, team: data }\n") {
		t.Errorf("ordinary-key flow mapping was re-serialized:\n%s", out)
	}
	after := scalarTags(t, frontmatterOf(t, out))
	for _, p := range []string{".window.from", ".window.to"} {
		if before[p] != after[p] {
			t.Errorf("%s tag/style changed: before=%s after=%s", p, before[p], after[p])
		}
		if !strings.HasPrefix(after[p], "!!timestamp") {
			t.Errorf("%s should remain !!timestamp, got %s", p, after[p])
		}
	}
}

// Defect A: top-level head comments are dropped while nested comments survive.
// Pin the asymmetry — both must survive after the fix.
func TestByteFaithfulTopLevelComment(t *testing.T) {
	raw := "---\n" +
		"# top-level head comment\n" +
		"type: Note\n" +
		"meta:\n" +
		"  # nested comment\n" +
		"  owner: alice\n" +
		"generated: { by: human:x, at: 2024-01-15T10:00:00Z }\n" +
		"---\n\n# Body\n"

	out := writeAddsKey(t, raw, "status", "active")

	if !strings.Contains(out, "# top-level head comment\n") {
		t.Errorf("top-level head comment was dropped:\n%s", out)
	}
	if !strings.Contains(out, "  # nested comment\n") {
		t.Errorf("nested comment was dropped:\n%s", out)
	}
}

// Criterion D — sibling-level preservation inside a CHANGED container. When a
// second `verified` entry is appended, the container legitimately changes, but
// the PRE-EXISTING entry must stay byte-identical: same flow-mapping style, same
// spacing, same scalar quoting, same !!timestamp tag. This is the append path,
// where the bytes being silently rewritten are a named human's attestation.
// (The append shape mirrors convert.applyVerifiedBy: a map[string]any appended
// to the []any list.)
func TestByteFaithfulSiblingInChangedContainer(t *testing.T) {
	raw := "---\n" +
		"type: Metric\n" +
		"generated: { by: human:jsmith@acme, at: 2024-01-15T10:00:00Z }\n" +
		"verified:\n" +
		"  - { by: human:ahormati, at: 2024-02-01T09:30:00Z }\n" +
		"---\n\n# Body\n"

	c := New()
	con, err := c.ParseConcept("x.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	before := scalarTags(t, frontmatterOf(t, raw))

	// Append a second stamp — the container changes, the first entry must not.
	list, _ := con.Frontmatter.Get("verified")
	con.Frontmatter.Set("verified", append(list.([]any),
		map[string]any{"by": "human:ghchinoy", "at": "2023-11-14T22:13:20Z"}))
	outB, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	out := string(outB)

	// Pre-existing sibling: byte-identical flow mapping.
	if !strings.Contains(out, "  - { by: human:ahormati, at: 2024-02-01T09:30:00Z }\n") {
		t.Errorf("pre-existing verified sibling was rewritten:\n%s", out)
	}
	// Subtree control in the same write: untouched `generated` preserved.
	if !strings.Contains(out, "generated: { by: human:jsmith@acme, at: 2024-01-15T10:00:00Z }\n") {
		t.Errorf("untouched generated was rewritten alongside the changed sibling:\n%s", out)
	}
	// The intended second entry landed.
	if !strings.Contains(out, "human:ghchinoy") {
		t.Errorf("appended verified entry missing:\n%s", out)
	}
	// Assert on the YAML tag, not just bytes.
	after := scalarTags(t, frontmatterOf(t, out))
	for _, p := range []string{".verified[0].at", ".generated.at"} {
		if before[p] != after[p] {
			t.Errorf("%s tag/style changed: before=%s after=%s", p, before[p], after[p])
		}
		if !strings.HasPrefix(after[p], "!!timestamp") {
			t.Errorf("%s is no longer !!timestamp: %s", p, after[p])
		}
	}
}

// TestByteFaithfulFlowSeqAndBareMapSibling is a PERMANENT invariant — the
// companion to TestByteFaithfulSiblingInChangedContainer (block sequence) for the
// other two attestation container shapes. When a `verified` value is written as a
// single-line FLOW SEQUENCE (`verified: [{ … }]`) or as a BARE/nested flow
// MAPPING (`verified: { … }`) and a stamp is appended, the pre-existing entry
// stays byte-identical: its flow style, interior spacing, {by,at} sub-key order,
// and `!!timestamp` tag all survive. spliceFrontmatter re-emits it from its
// source bytes (spliceFlowContainerToBlock) instead of re-encoding the whole
// value through yaml.v3. This was previously the KNOWN residual pinned by a
// characterization test; the splice extension closed it, so the test now pins the
// promise, not the symptom, and must never flip back.
//
// Anti-vacuity: each case first asserts the pre-existing `at` really is a
// !!timestamp today (so the preserved-!!timestamp assertion is not vacuously true
// of a value that was already a string) and asserts the appended stamp landed (so
// the container genuinely changed and the test is not "preserving" a no-op).
func TestByteFaithfulFlowSeqAndBareMapSibling(t *testing.T) {
	cases := []struct {
		name, verified, beforePath string
	}{
		{"flow_sequence", "verified: [{ by: human:ahormati, at: 2024-02-01T09:30:00Z }]\n", ".verified[0].at"},
		{"bare_mapping", "verified: { by: human:ahormati, at: 2024-02-01T09:30:00Z }\n", ".verified.at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := "---\n" +
				"type: Metric\n" +
				tc.verified +
				"---\n\n# Body\n"

			c := New()
			con, err := c.ParseConcept("x.md", []byte(raw))
			if err != nil {
				t.Fatalf("ParseConcept: %v", err)
			}
			before := scalarTags(t, frontmatterOf(t, raw))
			// Anti-vacuity 1: the authored value really is a !!timestamp today.
			if got := before[tc.beforePath]; !strings.HasPrefix(got, "!!timestamp") {
				t.Fatalf("setup: pre-existing %s is not !!timestamp (got %q); the preservation "+
					"assertion below would be vacuous", tc.beforePath, got)
			}

			// Append a stamp exactly as convert.applyVerifiedBy does: normalize the
			// existing value to []any (a bare mapping becomes a one-element list),
			// then append.
			v, _ := con.Frontmatter.Get("verified")
			var list []any
			switch vv := v.(type) {
			case []any:
				list = vv
			case map[string]any:
				list = []any{vv}
			default:
				t.Fatalf("unexpected verified shape %T", v)
			}
			con.Frontmatter.Set("verified", append(list,
				map[string]any{"by": "human:ghchinoy", "at": "2023-11-14T22:13:20Z"}))
			outB, err := c.Serialize(con)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			out := string(outB)

			// Anti-vacuity 2: the append actually landed, so the container changed.
			if !strings.Contains(out, "human:ghchinoy") {
				t.Fatalf("appended stamp missing — container did not change:\n%s", out)
			}

			// INVARIANT: the pre-existing entry is byte-identical — flow style,
			// interior spacing, and {by,at} sub-key order all intact.
			if !strings.Contains(out, "  - { by: human:ahormati, at: 2024-02-01T09:30:00Z }\n") {
				t.Errorf("pre-existing %s entry was reshaped, not preserved verbatim:\n%s", tc.name, out)
			}
			// INVARIANT: assert on the YAML TAG, not only bytes — the exact defect
			// was a !!timestamp -> !!str retype that identical-looking text can hide.
			after := scalarTags(t, frontmatterOf(t, out))
			if got := after[".verified[0].at"]; !strings.HasPrefix(got, "!!timestamp") {
				t.Errorf("pre-existing verified[0].at was retyped to %q, want !!timestamp:\n%s", got, out)
			}
		})
	}
}

// TestByteFaithfulNonTrustSiblingOnAppend is the special-casing detector for the
// append path (criterion 4): the same sibling preservation must hold for an
// ORDINARY key of the flow-sequence and bare-mapping shapes, not just for
// `verified`. The splice never inspects the key name; if this fails while the
// `verified` cases pass, the fix special-cased the trust key.
func TestByteFaithfulNonTrustSiblingOnAppend(t *testing.T) {
	cases := []struct{ name, value string }{
		{"flow_sequence", "history: [{ by: alice, at: 2024-02-01T09:30:00Z }]\n"},
		{"bare_mapping", "history: { by: alice, at: 2024-02-01T09:30:00Z }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := "---\ntype: Note\n" + tc.value + "---\n\n# Body\n"
			c := New()
			con, err := c.ParseConcept("x.md", []byte(raw))
			if err != nil {
				t.Fatalf("ParseConcept: %v", err)
			}
			before := scalarTags(t, frontmatterOf(t, raw))
			bp := ".history[0].at"
			if _, ok := before[bp]; !ok {
				bp = ".history.at"
			}
			if got := before[bp]; !strings.HasPrefix(got, "!!timestamp") {
				t.Fatalf("setup: pre-existing %s is not !!timestamp (got %q)", bp, got)
			}
			v, _ := con.Frontmatter.Get("history")
			var list []any
			switch vv := v.(type) {
			case []any:
				list = vv
			case map[string]any:
				list = []any{vv}
			default:
				t.Fatalf("unexpected history shape %T", v)
			}
			con.Frontmatter.Set("history", append(list,
				map[string]any{"by": "bob", "at": "2023-11-14T22:13:20Z"}))
			outB, err := c.Serialize(con)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			out := string(outB)
			if !strings.Contains(out, "bob") {
				t.Fatalf("appended entry missing — container did not change:\n%s", out)
			}
			if !strings.Contains(out, "  - { by: alice, at: 2024-02-01T09:30:00Z }\n") {
				t.Errorf("ordinary-key pre-existing entry was reshaped, not preserved:\n%s", out)
			}
			after := scalarTags(t, frontmatterOf(t, out))
			if got := after[".history[0].at"]; !strings.HasPrefix(got, "!!timestamp") {
				t.Errorf("ordinary-key history[0].at was retyped to %q, want !!timestamp:\n%s", got, out)
			}
		})
	}
}

// TestByteFaithfulBlockMapSiblingOnAppend covers the third attestation shape a
// bare {by,at} can take: a BLOCK (dash-less) nested mapping. Appending here
// cannot be byte-identical — a lone mapping value must acquire a `- ` marker and
// one indent level to become a sequence item — but the pre-existing entry's
// TOKENS, {by,at} sub-key order, and `!!timestamp` tag must be preserved: only
// the block framing changes, never the attestation's data or type. (CJK content
// is used to confirm the byte-based re-indent is multi-byte safe.)
func TestByteFaithfulBlockMapSiblingOnAppend(t *testing.T) {
	raw := "---\n" +
		"type: Metric\n" +
		"verified:\n" +
		"  by: human:あほまてぃ\n" +
		"  at: 2024-02-01T09:30:00Z\n" +
		"---\n\n# Body\n"
	c := New()
	con, err := c.ParseConcept("x.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	before := scalarTags(t, frontmatterOf(t, raw))
	if got := before[".verified.at"]; !strings.HasPrefix(got, "!!timestamp") {
		t.Fatalf("setup: pre-existing .verified.at is not !!timestamp (got %q)", got)
	}
	v, _ := con.Frontmatter.Get("verified")
	list := []any{v.(map[string]any)}
	con.Frontmatter.Set("verified", append(list,
		map[string]any{"by": "human:ghchinoy", "at": "2023-11-14T22:13:20Z"}))
	outB, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	out := string(outB)
	if !strings.Contains(out, "human:ghchinoy") {
		t.Fatalf("appended stamp missing — container did not change:\n%s", out)
	}
	// Tokens and sub-key order preserved (re-indented into a sequence item).
	if !strings.Contains(out, "  - by: human:あほまてぃ\n    at: 2024-02-01T09:30:00Z\n") {
		t.Errorf("pre-existing block-mapping entry was reshaped or retyped:\n%s", out)
	}
	// Tag preserved: no !!timestamp -> !!str retype.
	after := scalarTags(t, frontmatterOf(t, out))
	if got := after[".verified[0].at"]; !strings.HasPrefix(got, "!!timestamp") {
		t.Errorf("pre-existing verified[0].at was retyped to %q, want !!timestamp:\n%s", got, out)
	}
}

// Criterion A — corpus-wide positive control. Every tracked frontmatter that
// carries a flow mapping must be tag-stable AND byte-stable for its pre-existing
// keys across a write that adds a key. This is the general property: no key is
// special. The positive control is built in — the walk asserts it actually
// visited files carrying the trigger, so "no matches" cannot masquerade as pass.
func TestByteFaithfulCorpusTagStable(t *testing.T) {
	root := "../../../testdata"
	c := New()
	var checked, withFlow int
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		raw, e := os.ReadFile(p)
		if e != nil {
			return nil
		}
		con, e := c.ParseConcept(p, raw)
		if e != nil {
			return nil
		}
		fmBefore := frontmatterOf(t, strings.ReplaceAll(string(raw), "\r\n", "\n"))
		hasFlow := strings.Contains(fmBefore, "{") // flow mapping present
		checked++
		before := scalarTags(t, fmBefore)
		con.Frontmatter.Set("__probe__", "x")
		out, e := c.Serialize(con)
		if e != nil {
			t.Errorf("%s: Serialize: %v", p, e)
			return nil
		}
		after := scalarTags(t, frontmatterOf(t, string(out)))
		for k, bv := range before {
			if av, ok := after[k]; ok && av != bv {
				t.Errorf("%s: %s tag/style changed across write: before=%s after=%s", p, k, bv, av)
			}
		}
		// Stronger byte-level control: an additive write appends its key, so the
		// entire original frontmatter block must survive as an exact prefix.
		outFM := frontmatterOf(t, string(out))
		if !strings.HasPrefix(outFM, fmBefore) {
			t.Errorf("%s: original frontmatter not preserved byte-for-byte.\n--- orig ---\n%s\n--- got ---\n%s", p, fmBefore, outFM)
		}
		if hasFlow {
			withFlow++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Positive control: prove the walk actually saw trigger files.
	if withFlow == 0 {
		t.Fatalf("positive control failed: no flow-mapping frontmatter found under %s (checked=%d)", root, checked)
	}
	t.Logf("corpus tag-stability: checked=%d files-with-flow-mapping=%d", checked, withFlow)
}
