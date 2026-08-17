package native

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// Regression wall for issue #123 — the lone-CR codec defect.
//
// binder normalises CRLF to LF at native.go:51 but not a LONE carriage return
// (`\r`, classic-Mac). yaml.v3 treats a lone CR as a line break; spliceFrontmatter
// builds its line array with strings.SplitAfter(fmText, "\n"), which does NOT. So
// every yaml.v3 line index the splice uses is shifted by the number of preceding
// lone CRs, and joinLines silently clamps out-of-range indices to "". That clamp is
// why the damage is silent.
//
// THE CLOCK NOTE. These are CODEC-LEVEL tests: they call ParseConcept, mutate the
// Frontmatter map with values chosen HERE, and Serialize. They never invoke enrich,
// never read a config origin, and never consult SOURCE_DATE_EPOCH — the timestamps
// are literal test data, so the dedup/collision trap that only bites the enrich
// stamp path (see the reach tests) cannot make these pass vacuously. The clock is
// irrelevant to this file by construction; the reach-level tests state it per case.
//
// Every test in this file EXCEPT the two controls (TestByteFaithful_LFAndCRLF_
// Unregressed and TestLoneCRBody_PreservedUnderC2) is expected to FAIL on the
// pre-fix code — that failure is the evidence the fix is measured against.

// appendVerifiedStamp mutates con's `verified` value exactly as
// convert.applyVerifiedBy does: normalise the existing value to []any, then append
// one stamp. This is the real first-key-rewrite the destroy needs — a pre-existing
// non-last key whose value changes.
func appendVerifiedStamp(t *testing.T, con *okf.Concept, by, at string) {
	t.Helper()
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
	con.Frontmatter.Set("verified", append(list, map[string]any{"by": by, "at": at}))
}

// serializeReparse serializes con and re-parses the output, returning both the
// output bytes and the reparsed concept (nil on a parse error).
func serializeReparse(t *testing.T, c *Codec, con *okf.Concept) (out []byte, re *okf.Concept, reErr error) {
	t.Helper()
	out, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	re, reErr = c.ParseConcept("doc.md", out)
	return out, re, reErr
}

// Test 1 — outcome 1, the silent destroy. Fixture S1 from the repro samples:
// `verified` is the FIRST key with a lone-CR-delimited value, followed by `owner`
// and `canary`. Rewriting the first key (appending a stamp) drops every subsequent
// lone-CR-delimited key, silently, exit 0.
//
// FAILS TODAY: the output re-parses to `verified` only — `owner` and `canary` are
// gone.
func TestLoneCR_FirstKeyRewrite_PreservesAllKeys(t *testing.T) {
	raw := "---\n" +
		"verified:\r- {by: human:alice, at: 2024-01-01T00:00:00Z}\r" +
		"owner: human:alice\r" +
		"canary: KEEPME\n" +
		"---\n# Body\n"

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	// Anti-vacuity: all three keys are present before the write.
	for _, k := range []string{"verified", "owner", "canary"} {
		if !con.Frontmatter.Has(k) {
			t.Fatalf("setup: fixture is missing key %q before the write", k)
		}
	}

	appendVerifiedStamp(t, con, "human:alice", "2026-01-01T00:00:00Z")
	out, re, reErr := serializeReparse(t, c, con)
	if reErr != nil {
		t.Fatalf("output does not re-parse (%v):\n%q", reErr, out)
	}

	if v, ok := re.Frontmatter.Get("owner"); !ok || v != "human:alice" {
		t.Errorf("owner destroyed or altered: got %#v present=%v\noutput:\n%q", v, ok, out)
	}
	if v, ok := re.Frontmatter.Get("canary"); !ok || v != "KEEPME" {
		t.Errorf("canary destroyed or altered: got %#v present=%v\noutput:\n%q", v, ok, out)
	}
}

// Test 2 — outcome 2, the stale duplicate. Rewriting a NON-first key whose value
// has lone CRs BEFORE it re-emits that key's source bytes in a preceding span AND
// appends the fresh encode, so the key appears twice in the written file.
//
// The output still parses (yaml.v3 dedups, keeping the stale value or the fresh one
// depending on order), so the property under test is the SERIALIZED bytes: the
// rewritten key must appear exactly once in the frontmatter block.
//
// FAILS TODAY: `status:` appears twice in the output.
func TestLoneCR_NonFirstKeyRewrite_NoStaleDuplicate(t *testing.T) {
	raw := "---\n" +
		"type: Metric\r" +
		"title: Alpha\r" +
		"status: draft\n" +
		"tags: [a, b]\n" +
		"---\n# Body\n"

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	if !con.Frontmatter.Has("status") {
		t.Fatalf("setup: fixture missing `status` before the write")
	}

	con.Frontmatter.Set("status", "stable") // rewrite a non-first key in place
	out, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	fm := frontmatterOf(t, string(out))

	if n := strings.Count(fm, "status:"); n != 1 {
		t.Errorf("stale duplicate: `status:` appears %d times, want exactly 1\noutput frontmatter:\n%q", n, fm)
	}
}

// Test 3 — outcome 3, silent value absorption. A scalar key (`status`) with a lone
// CR after its value is followed by a block-sequence key (`tags`). Rewriting the
// scalar destroys `tags` and absorbs its orphaned block lines into the scalar's
// value: `status` becomes the single string "stable - x - y". This is a SURVIVING
// key whose meaning changed, so the assertion is on the value, not on presence.
//
// FAILS TODAY: status == "stable - x - y" and `tags` is gone.
func TestLoneCR_BlockValueNotAbsorbed(t *testing.T) {
	raw := "---\n" +
		"status: draft\r" +
		"tags:\n  - x\n  - y\n" +
		"---\n# Body\n"

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	// Anti-vacuity: `tags` really is a two-item list before the write.
	tv, _ := con.Frontmatter.Get("tags")
	if list, ok := tv.([]any); !ok || len(list) != 2 {
		t.Fatalf("setup: `tags` is not the expected 2-item list (got %#v)", tv)
	}

	con.Frontmatter.Set("status", "stable") // rewrite the preceding scalar in place
	out, re, reErr := serializeReparse(t, c, con)
	if reErr != nil {
		t.Fatalf("output does not re-parse (%v):\n%q", reErr, out)
	}

	// The rewritten scalar must be exactly the new value — not a merged string.
	if v, _ := re.Frontmatter.Get("status"); v != "stable" {
		t.Errorf("value absorption: status = %#v, want %q (the block value was absorbed)\noutput:\n%q", v, "stable", out)
	}
	// The block-valued key must survive with its list intact.
	v, ok := re.Frontmatter.Get("tags")
	if !ok {
		t.Errorf("`tags` was destroyed\noutput:\n%q", out)
	} else if list, isList := v.([]any); !isList || len(list) != 2 {
		t.Errorf("`tags` value changed: got %#v, want a 2-item list\noutput:\n%q", v, out)
	}
}

// Test 4 — outcome 4, the brick. Fixture BR_d: a lone CR sits INSIDE the multi-line
// block-sequence value of the key being rewritten (`verified`). binder writes a file
// it can no longer read. The real property is that binder can READ BACK WHAT IT JUST
// WROTE — asserting only that a later read errors would pass for the wrong reason
// (e.g. if the write had refused up front). So: Serialize must succeed AND its output
// must re-parse.
//
// FAILS TODAY: the output is written but re-parsing it errors with
// `mapping values are not allowed in this context`.
func TestLoneCR_ChangedBlockSequence_OutputParses(t *testing.T) {
	raw := "---\n" +
		"verified:\n  - by: human:alice\r    at: 2024-01-01T00:00:00Z\n" +
		"owner: human:alice\r" +
		"team: eng\n" +
		"---\n# Body\n"

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}

	appendVerifiedStamp(t, con, "human:bob", "2026-01-01T00:00:00Z")
	out, err := c.Serialize(con)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	// Anti-vacuity: the write actually happened (the stamp is in the bytes), so this
	// is a real "read back what you wrote" and not a no-op.
	if !strings.Contains(string(out), "human:bob") {
		t.Fatalf("appended stamp missing — nothing was written:\n%q", out)
	}

	// The property is that binder can READ BACK what it just wrote. This is the BRICK
	// (a distinct defect from the locus-2 fence refusal in Test 5): splitFrontmatter
	// SUCCEEDED on the input and the splice reached the write path, so the failure is
	// on the second read, in parseFrontmatterNode, as `mapping values are not allowed
	// in this context`. Asserting merely that "a later read errors" would pass for the
	// wrong reason and would also pass on a locus-2 file — so the failure below is
	// checked to be the brick error, not the unterminated-fence error.
	_, reErr := c.ParseConcept("doc.md", out)
	if reErr != nil {
		if strings.Contains(reErr.Error(), "unterminated") {
			t.Fatalf("unexpected: read-back failed with the locus-2 fence error, not the brick "+
				"(this test must exercise the write-path brick, not a read refusal): %v", reErr)
		}
		t.Errorf("brick: binder wrote a file it cannot read back (%v):\n%q", reErr, out)
	}
}

// Test 5 — outcome 5, the second locus. A lone CR immediately before the closing
// fence makes splitFrontmatter fail to find the fence (it splits on "\n" and
// TrimRights only "\r\n"), so binder refuses the file on read.
//
// FAILS TODAY: ParseConcept returns `unterminated '---' block`.
func TestLoneCR_BeforeClosingFence_FileIsAccepted(t *testing.T) {
	raw := "---\n" +
		"type: Note\n" +
		"status: draft\r" + // lone CR immediately before the closing fence
		"---\n# Body\n"

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		// This is LOCUS 2 — a different defect from the brick (Test 4). It fails on
		// reading the ORIGINAL file as authored, in splitFrontmatter's fence scan;
		// binder never reaches the write path and produces nothing. Confirm the
		// failure is that specific fence-scan refusal, not the brick's parse error, so
		// the two loci are not conflated. After the fix the fence scan becomes
		// CR-aware, ParseConcept succeeds, and this test passes.
		if !strings.Contains(err.Error(), "unterminated") {
			t.Fatalf("expected the locus-2 fence-scan refusal (`unterminated '---' block`), "+
				"got a different error: %v", err)
		}
		t.Fatalf("outcome 5 (locus 2): original file refused on read with %v — "+
			"should be accepted after the CR-aware fence-scan fix", err)
	}
	// If accepted, the keys must be intact.
	for _, k := range []string{"type", "status"} {
		if !con.Frontmatter.Has(k) {
			t.Errorf("accepted but key %q missing\nkeys: %v", k, con.Frontmatter.Keys())
		}
	}
}

// Test 7 — defence in depth. Hand spliceFrontmatter a `\r`-bearing fmText directly
// (with one key changed so the splice path actually runs) and assert it ERRORS
// rather than silently writing damaged bytes. The plan's recommendation adds exactly
// this guard: "spliceFrontmatter should return an error if fmText still contains a
// \r". It is unreachable once ParseConcept normalises the frontmatter region, but it
// converts any future regression at the normalisation boundary from silent
// destruction into a refusal to write (P46-compliant: loud, not quiet).
//
// FAILS TODAY (expected and correct): there is no guard, so spliceFrontmatter
// returns a nil error and produces (damaged) output. This test goes green only when
// the fix adds the guard.
func TestSpliceRefusesCarriageReturn(t *testing.T) {
	fmText := "verified:\r- {by: human:alice, at: 2024-01-01T00:00:00Z}\r" +
		"owner: human:alice\rcanary: KEEPME\n"

	origRoot, origOM, err := parseFrontmatterNode(fmText)
	if err != nil {
		t.Fatalf("parseFrontmatterNode setup: %v", err)
	}
	// Build the desired map from the source, changing one key so the splice does not
	// take a pure-verbatim path.
	fm := okf.NewOrderedMap()
	for _, k := range origOM.Keys() {
		v, _ := origOM.Get(k)
		fm.Set(k, v)
	}
	fm.Set("owner", "human:bob") // a change, so a rewrite path is exercised

	// Anti-vacuity: confirm the input we are guarding against really contains a lone
	// CR (and not a CRLF).
	if !strings.Contains(fmText, "\r") || strings.Contains(fmText, "\r\n") {
		t.Fatalf("setup: fmText must contain a lone CR and no CRLF")
	}

	if _, err := spliceFrontmatter(fmText, origRoot, origOM, fm); err == nil {
		t.Errorf("spliceFrontmatter accepted a \\r-bearing fmText and did not error; " +
			"the defence-in-depth guard is missing (expected to fail until the fix adds it)")
	}
}

// Test 8 — THE CONTROL. It must be GREEN today and stay green throughout. The two
// line-ending shapes binder already handles correctly — pure LF, and CRLF
// (normalised at native.go:51) — must round-trip and accept a first-key rewrite with
// no key lost and no unreadable output. If this is red before anyone has changed the
// production code, something else is wrong and the whole wall is suspect.
//
// This is the named companion to the existing byte-faithful battery in
// byte_faithful_test.go (from 427503e); it isolates the "unregressed for the
// endings we DO handle" half of the claim next to the lone-CR failures.
func TestByteFaithful_LFAndCRLF_Unregressed(t *testing.T) {
	cases := []struct{ name, nl string }{
		{"LF", "\n"},
		{"CRLF", "\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nl := tc.nl
			raw := "---" + nl +
				"verified:" + nl + "  - {by: human:alice, at: 2024-01-01T00:00:00Z}" + nl +
				"owner: human:alice" + nl +
				"canary: KEEPME" + nl +
				"---" + nl + "# Body" + nl

			c := New()
			con, err := c.ParseConcept("doc.md", []byte(raw))
			if err != nil {
				t.Fatalf("ParseConcept: %v", err)
			}
			appendVerifiedStamp(t, con, "human:alice", "2026-01-01T00:00:00Z")
			out, re, reErr := serializeReparse(t, c, con)
			if reErr != nil {
				t.Fatalf("control regressed: output does not re-parse (%v):\n%q", reErr, out)
			}
			// Anti-vacuity: the write landed.
			if !strings.Contains(string(out), "2026-01-01T00:00:00Z") {
				t.Fatalf("control: stamp not written — nothing to preserve:\n%q", out)
			}
			for _, kv := range []struct{ k, v string }{{"owner", "human:alice"}, {"canary", "KEEPME"}} {
				if v, ok := re.Frontmatter.Get(kv.k); !ok || v != kv.v {
					t.Errorf("control regressed: %s = %#v present=%v, want %q\noutput:\n%q", kv.k, v, ok, kv.v, out)
				}
			}
		})
	}
}

// Test 9 — pins the C2 decision. The fix must normalise lone CR to LF only INSIDE
// the frontmatter region, never the body. A body lone CR must survive a frontmatter
// write untouched.
//
// GREEN TODAY: the body is already emitted verbatim (Serialize collapses only CRLF
// in the body), so this passes now. It is worth having because it PINS the decision:
// a future C1-style whole-document normalisation would rewrite the body's lone CRs
// and turn this red. It guards the boundary the fix must not cross.
func TestLoneCRBody_PreservedUnderC2(t *testing.T) {
	body := "body line one\rbody line two\rbody line three\n"
	raw := "---\ntype: Note\n---\n" + body

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	if con.Body != body {
		t.Fatalf("setup: parsed body already differs from source: got %q want %q", con.Body, body)
	}

	// A frontmatter write (append a key) must not touch the body's lone CRs.
	con.Frontmatter.Set("status", "stable")
	out, re, reErr := serializeReparse(t, c, con)
	if reErr != nil {
		t.Fatalf("output does not re-parse (%v):\n%q", reErr, out)
	}
	if re.Body != body {
		t.Errorf("body lone CRs were rewritten by a frontmatter write:\n want %q\n  got %q", body, re.Body)
	}
}
