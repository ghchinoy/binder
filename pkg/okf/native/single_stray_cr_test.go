package native

import (
	"strings"
	"testing"
)

// Regression wall for issue #123 — the SINGLE-STRAY-CR population.
//
// The earlier 64-cell matrix (line_ending_matrix_test.go) is {LF, CR}^4 over the
// four interior line breaks: every cell is built from LF and lone CR only, so there
// is NO CRLF anywhere in it. That grid therefore does NOT cover the most common
// real-world shape: an ordinary CRLF (or pure-LF) file that happens to contain ONE
// stray lone CR somewhere in the frontmatter interior.
//
// Measurement (EM, post-brief) established that this single stray CR is enough to
// destroy a key on the unfixed code — the trigger is not a CR-delimited "classic
// Mac" file, it is a single stray CR anywhere in the frontmatter interior of an
// otherwise ordinary file. These tests pin exactly that.
//
// CLOCK NOTE: as with the rest of the codec wall, these mutate the Frontmatter map
// with values chosen HERE and Serialize; they never touch enrich, a config origin,
// or SOURCE_DATE_EPOCH, so they cannot pass vacuously via the dedup/collision trap.
//
// countLoneCR returns the number of CR bytes that are NOT part of a CRLF pair — the
// stray CRs. It is the anti-vacuity check for these fixtures: each must contain
// exactly one.
func countLoneCR(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && (i+1 >= len(s) || s[i+1] != '\n') {
			n++
		}
	}
	return n
}

// EM-required fixture ONE — a CRLF file (CRLF fences, CRLF interior) with exactly
// ONE stray lone CR in the interior. The key whose value line the stray CR
// terminates (`owner`) is rewritten in place; the key IMMEDIATELY AFTER the stray CR
// (`team`) must survive with its value.
//
// MECHANISM (measured, not predicted): CRLF is normalised to LF at native.go:51,
// leaving the single stray CR, so the frontmatter region becomes
// `... owner: human:alice\rteam: eng\n ...`. strings.SplitAfter(fmText, "\n") keeps
// `owner:...\rteam:...` as ONE line while yaml.v3 counts the \r as a break, so the
// key that follows the CR is folded onto the rewritten key's source line and is lost
// when that line is re-emitted. Rewriting a key two positions ahead of the CR (e.g.
// `type`) does NOT trigger it — the rewritten key must be the one adjacent to the CR.
//
// EXPECTED TODAY: DESTROY — `team` disappears while `owner` keeps its new value.
func TestSingleStrayCR_InCRLFFile_FollowingKeySurvives(t *testing.T) {
	const CRLF = "\r\n"
	raw := "---" + CRLF +
		"type: Metric" + CRLF +
		"owner: human:alice" + "\r" + // the one stray lone CR (NOT a CRLF)
		"team: eng" + CRLF +
		"canary: KEEPME" + CRLF +
		"---" + CRLF +
		"# Body" + CRLF

	// Anti-vacuity: this is a CRLF file with EXACTLY ONE stray lone CR.
	if got := countLoneCR(raw); got != 1 {
		t.Fatalf("setup: fixture must have exactly one stray lone CR, got %d\n%q", got, raw)
	}

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	for _, k := range []string{"type", "owner", "team", "canary"} {
		if !con.Frontmatter.Has(k) {
			t.Fatalf("setup: key %q missing before the write; keys=%v", k, con.Frontmatter.Keys())
		}
	}

	// Rewrite the key whose value line the stray CR terminates.
	con.Frontmatter.Set("owner", "human:bob")
	out, re, reErr := serializeReparse(t, c, con)
	if reErr != nil {
		t.Fatalf("output does not re-parse (%v):\n%q", reErr, out)
	}
	// Anti-vacuity: the rewrite of `owner` actually landed.
	if v, ok := re.Frontmatter.Get("owner"); !ok || v != "human:bob" {
		t.Fatalf("setup: owner rewrite did not land: got %#v present=%v\noutput:\n%q", v, ok, out)
	}

	// The key IMMEDIATELY AFTER the stray CR must survive with its value.
	if v, ok := re.Frontmatter.Get("team"); !ok || v != "eng" {
		t.Errorf("team destroyed or altered by a single stray CR: got %#v present=%v, want %q\noutput:\n%q", v, ok, "eng", out)
	}
	if v, ok := re.Frontmatter.Get("canary"); !ok || v != "KEEPME" {
		t.Errorf("canary destroyed or altered: got %#v present=%v, want %q\noutput:\n%q", v, ok, "KEEPME", out)
	}
}

// EM-required fixture TWO — a pure-LF file with exactly ONE stray lone CR in the
// interior. Same rewrite-adjacent-key, assert-following-survives shape as fixture
// ONE, proving the trigger is the stray CR itself, not the surrounding line ending.
//
// EXPECTED TODAY: DESTROY — `team` disappears while `owner` keeps its new value.
func TestSingleStrayCR_InLFFile_FollowingKeySurvives(t *testing.T) {
	raw := "---\n" +
		"type: Metric\n" +
		"owner: human:alice" + "\r" + // the one stray lone CR
		"team: eng\n" +
		"canary: KEEPME\n" +
		"---\n# Body\n"

	// Anti-vacuity: pure-LF file (no CRLF at all) with EXACTLY ONE stray lone CR.
	if strings.Contains(raw, "\r\n") {
		t.Fatalf("setup: fixture must be pure-LF with no CRLF\n%q", raw)
	}
	if got := countLoneCR(raw); got != 1 {
		t.Fatalf("setup: fixture must have exactly one stray lone CR, got %d\n%q", got, raw)
	}

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}
	for _, k := range []string{"type", "owner", "team", "canary"} {
		if !con.Frontmatter.Has(k) {
			t.Fatalf("setup: key %q missing before the write; keys=%v", k, con.Frontmatter.Keys())
		}
	}

	con.Frontmatter.Set("owner", "human:bob") // rewrite the key adjacent to the stray CR
	out, re, reErr := serializeReparse(t, c, con)
	if reErr != nil {
		t.Fatalf("output does not re-parse (%v):\n%q", reErr, out)
	}
	if v, ok := re.Frontmatter.Get("owner"); !ok || v != "human:bob" {
		t.Fatalf("setup: owner rewrite did not land: got %#v present=%v\noutput:\n%q", v, ok, out)
	}

	if v, ok := re.Frontmatter.Get("team"); !ok || v != "eng" {
		t.Errorf("team destroyed or altered by a single stray CR: got %#v present=%v, want %q\noutput:\n%q", v, ok, "eng", out)
	}
	if v, ok := re.Frontmatter.Get("canary"); !ok || v != "KEEPME" {
		t.Errorf("canary destroyed or altered: got %#v present=%v, want %q\noutput:\n%q", v, ok, "KEEPME", out)
	}
}

// EM-required fixture THREE — the CRLF control with NO stray CR. CRLF is normalised
// at native.go:51, so this is in the SAFE direction and must round-trip cleanly.
//
// GREEN TODAY and must stay green: a control that fires in the safe direction is
// worth having precisely because CRLF safety comes from the line-51 normalisation
// the fix is extending to lone CR. If this ever goes red the fix broke the case it
// was modelled on.
func TestSingleStrayCR_CRLFControl_NoStrayCR_Unaffected(t *testing.T) {
	const CRLF = "\r\n"
	raw := "---" + CRLF +
		"type: Metric" + CRLF +
		"owner: human:alice" + CRLF +
		"team: eng" + CRLF +
		"canary: KEEPME" + CRLF +
		"---" + CRLF +
		"# Body" + CRLF

	// Anti-vacuity: a genuine CRLF file with ZERO stray CRs.
	if !strings.Contains(raw, "\r\n") {
		t.Fatalf("setup: control must actually be CRLF\n%q", raw)
	}
	if got := countLoneCR(raw); got != 0 {
		t.Fatalf("setup: control must have no stray lone CR, got %d\n%q", got, raw)
	}

	c := New()
	con, err := c.ParseConcept("doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("ParseConcept: %v", err)
	}

	con.Frontmatter.Set("owner", "human:bob") // SAME operation as the failing cases; only the stray CR differs
	out, re, reErr := serializeReparse(t, c, con)
	if reErr != nil {
		t.Fatalf("control regressed: output does not re-parse (%v):\n%q", reErr, out)
	}
	for _, kv := range []struct{ k, v string }{
		{"owner", "human:bob"}, {"team", "eng"}, {"canary", "KEEPME"},
	} {
		if v, ok := re.Frontmatter.Get(kv.k); !ok || v != kv.v {
			t.Errorf("control regressed: %s = %#v present=%v, want %q\noutput:\n%q", kv.k, v, ok, kv.v, out)
		}
	}
}
