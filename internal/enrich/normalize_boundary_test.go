package enrich_test

// Boundary-normalization tests for issue #124 (design §4.4 Option A + §9 "T1"
// acceptance criteria). These pin, by execution, the transition the trust
// findings identified: before the fix a BOM-prefixed or lone-CR-fenced file
// failed frontmatter recognition and had its real (often human-verified)
// frontmatter silently demoted to body under a synthetic `type: Note` block;
// after the fix it is recognised, its trust block preserved, and the
// normalization disclosed.
//
// The PRIMARY instrument (per the brief, C11/C23) is a RAW-byte comparison of
// the surviving `verified` attestation against a copy binder never touched, with
// a positive control proving the instrument can show a difference — a probe that
// normalized the axis it checks could not check it, so the diff is on raw bytes.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/enrich"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// bom is the UTF-8 byte-order mark (U+FEFF → EF BB BF), written as an escape so
// the test source itself carries no literal BOM (which Go rejects).
var bom = string([]byte{0xEF, 0xBB, 0xBF})

// The exact three lines of the human attestation. Its bytes must survive
// verbatim (LF newlines) in the written output for the BOM case (AC1).
const verifiedBlockLF = "verified:\n  - by: human:ghchinoy\n    at: \"2026-01-01T00:00:00Z\"\n"

// tierOf re-parses written bytes through the SAME codec review/validate use and
// returns the derived trust tier — the substantive "tier preserved" claim.
func tierOf(t *testing.T, raw []byte) okf.Tier {
	t.Helper()
	c, err := native.New().ParseConcept("a.md", raw)
	if err != nil {
		t.Fatalf("re-parsing enriched output failed: %v\n%q", err, raw)
	}
	c.Trust = okf.ProjectTrust(c.Frontmatter, "")
	return okf.TrustTier(c)
}

// TestAC1_BOM_EnrichPreservesVerifiedAndDiscloses is design §9 AC1 on the enrich
// surface: a BOM-prefixed file with a real human `verified` block is recognised,
// the block survives byte-for-byte against an untouched copy, no synthetic
// `type: Note` is invented, the tier is preserved, and the envelope carries
// normalized: ["stripped-utf8-bom"].
func TestAC1_BOM_EnrichPreservesVerifiedAndDiscloses(t *testing.T) {
	src := t.TempDir()
	doc := bom + "---\n" +
		"type: Guide\n" +
		"title: Real Title\n" +
		"owner: alice\n" +
		verifiedBlockLF +
		"---\nbody\n"
	p := writeFile(t, src, "a.md", doc)

	// The untouched copy binder never writes — the reference for the raw-byte diff.
	untouched := []byte(doc)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	out := []byte(read(t, p))

	// --- PRIMARY INSTRUMENT: raw-byte survival of the attestation, + control ---
	// Expect: the attestation's raw bytes appear verbatim in BOTH the untouched
	// copy and the enriched output (the BOM sits before the block, so LF newlines
	// are identical on both sides — this is a literal byte diff, not through a
	// normalizer).
	if !bytes.Contains(untouched, []byte(verifiedBlockLF)) {
		t.Fatalf("test setup: untouched copy lacks the attestation block")
	}
	if !bytes.Contains(out, []byte(verifiedBlockLF)) {
		t.Errorf("attestation NOT preserved byte-for-byte in enriched output:\n%q", out)
	}
	// Positive control (C11/C23/C24): the instrument MUST be able to show a
	// difference. A block with a mutated actor must NOT be found — expect false.
	mutated := strings.Replace(verifiedBlockLF, "human:ghchinoy", "human:attacker", 1)
	if bytes.Contains(out, []byte(mutated)) {
		t.Fatalf("positive control impossible: a mutated actor should never appear")
	}
	if !bytes.Contains([]byte(verifiedBlockLF+"x"), []byte(verifiedBlockLF)) {
		t.Fatalf("positive control failed: substring instrument is vacuous")
	}

	// --- No demotion: no synthetic type invented; the real type survives. ---
	if bytes.Contains(out, []byte("type: Note")) {
		t.Errorf("synthetic `type: Note` was invented (the #124 demotion):\n%q", out)
	}
	if !bytes.Contains(out, []byte("type: Guide")) {
		t.Errorf("authored `type: Guide` did not survive:\n%q", out)
	}
	if bytes.HasPrefix(out, []byte(bom)) {
		t.Errorf("leading UTF-8 BOM was not stripped:\n%q", out)
	}

	// --- Tier preserved (re-parsed through the codec). ---
	if got := tierOf(t, out); got != okf.TierHumanReviewed {
		t.Errorf("tier = %q, want human-reviewed (the verified block must still derive the tier)", got)
	}

	// --- Disclosure: per-file signal + top-level advisory (AC5, non-optional). ---
	fr := find(t, rep, "a.md")
	if fr.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched", fr.Status)
	}
	if len(fr.Normalized) != 1 || fr.Normalized[0] != "stripped-utf8-bom" {
		t.Errorf("per-file normalized = %v, want [stripped-utf8-bom]", fr.Normalized)
	}
	if !anyContains(rep.Normalizations, "normalized before frontmatter recognition") ||
		!anyContains(rep.Normalizations, "stripped-utf8-bom") {
		t.Errorf("top-level advisory missing; normalizations = %v", rep.Normalizations)
	}
	// The advisory is disclosed on a NON-gating channel (issue #154): it must not
	// land in Warnings, which --strict counts.
	if len(rep.Warnings) != 0 {
		t.Errorf("normalization advisory leaked into the gating Warnings: %v", rep.Warnings)
	}
}

// TestAC2_LoneCR_EnrichPreservesVerifiedAndDiscloses is design §9 AC2: a lone-CR
// (classic-Mac) frontmatter file with a real human `verified` block is
// recognised, the attestation content survives, tier is preserved, and the
// envelope carries normalized: ["translated-lone-cr"]. The CR->LF translation IS
// the disclosed change, so the survival claim is on the attestation CONTENT (the
// axis under test is "did the attestation survive", not the line ending).
func TestAC2_LoneCR_EnrichPreservesVerifiedAndDiscloses(t *testing.T) {
	src := t.TempDir()
	cr := "\r"
	doc := "---" + cr +
		"type: Guide" + cr +
		"title: Real Title" + cr +
		"owner: alice" + cr +
		"verified:" + cr +
		"  - by: human:ghchinoy" + cr +
		"    at: \"2026-01-01T00:00:00Z\"" + cr +
		"---" + cr + "body" + cr
	p := writeFile(t, src, "a.md", doc)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	out := []byte(read(t, p))

	// The attestation content survives (as LF, the disclosed normalization).
	if !bytes.Contains(out, []byte(verifiedBlockLF)) {
		t.Errorf("attestation content not preserved after lone-CR translation:\n%q", out)
	}
	// Positive control: a mutated timestamp must not appear.
	if bytes.Contains(out, []byte("2099-12-31")) {
		t.Fatalf("positive control impossible: a value never written should be absent")
	}
	if bytes.Contains(out, []byte("type: Note")) {
		t.Errorf("synthetic `type: Note` was invented (the #124 demotion):\n%q", out)
	}
	if bytes.IndexByte(out, '\r') >= 0 {
		t.Errorf("a lone CR survived into the written output (should be LF):\n%q", out)
	}
	if got := tierOf(t, out); got != okf.TierHumanReviewed {
		t.Errorf("tier = %q, want human-reviewed", got)
	}

	fr := find(t, rep, "a.md")
	if len(fr.Normalized) != 1 || fr.Normalized[0] != "translated-lone-cr" {
		t.Errorf("per-file normalized = %v, want [translated-lone-cr]", fr.Normalized)
	}
	if !anyContains(rep.Normalizations, "translated-lone-cr") {
		t.Errorf("top-level advisory missing; normalizations = %v", rep.Normalizations)
	}
}

// TestAC3_SkipAndDiscloseFiresAfterNormalization is the load-bearing,
// trust-SME-pinned transition (design §9 AC3): after NormalizeInput strips the
// BOM the fence OPENS, ParseConcept fails on genuinely-broken YAML, and the
// EXISTING skip-and-disclose path fires (status: skipped + reason) — a path that
// was BYPASSED for these inputs before the fix (the fence never opened). Two
// fixtures: (a) BOM + invalid YAML, (b) BOM + unterminated fence.
func TestAC3_SkipAndDiscloseFiresAfterNormalization(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{
			name: "bom_invalid_yaml",
			// A closed fence whose YAML is genuinely invalid (a mapping value that is
			// also a bare sequence at the same indent).
			doc: bom + "---\ntype: Guide\n  - broken: [unclosed\nverified: : :\n---\nbody\n",
		},
		{
			name: "bom_unterminated_fence",
			doc:  bom + "---\ntype: Guide\ntitle: Real\nverified:\n  - by: human:x\n(no closing fence, ever)\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := t.TempDir()
			p := writeFile(t, src, "a.md", tc.doc)
			before := read(t, p)

			rep, err := enrich.Enrich(src, opts(src))
			if err != nil {
				t.Fatal(err)
			}
			fr := find(t, rep, "a.md")
			if fr.Status != enrich.StatusSkipped {
				t.Fatalf("status = %q, want skipped (the skip path must now be REACHED)", fr.Status)
			}
			if fr.Reason == "" || !strings.Contains(fr.Reason, "unparseable frontmatter") {
				t.Errorf("skip reason = %q, want an 'unparseable frontmatter' disclosure", fr.Reason)
			}
			// Never mutate what we cannot parse: the file is left byte-identical
			// (BOM included) — no synthetic block, no demotion.
			if read(t, p) != before {
				t.Errorf("a skipped file was rewritten:\n--- want ---\n%q\n--- got ---\n%q", before, read(t, p))
			}
			if rep.NumSkipped != 1 {
				t.Errorf("num_skipped = %d, want 1", rep.NumSkipped)
			}
		})
	}
}

// TestAC5_DisclosureIsNonOptional pins that a SUCCESSFUL enrich of a normalized
// file is never silent (design §9 AC5): status enriched AND a normalized signal
// AND a top-level advisory, and the JSON envelope carries the field.
func TestAC5_DisclosureIsNonOptional(t *testing.T) {
	src := t.TempDir()
	doc := bom + "---\ntype: Guide\ntitle: T\n---\nbody\n"
	writeFile(t, src, "a.md", doc)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	fr := find(t, rep, "a.md")
	if fr.Status != enrich.StatusEnriched {
		t.Fatalf("status = %q, want enriched", fr.Status)
	}
	if len(fr.Normalized) == 0 {
		t.Fatalf("silent-success: an enriched normalized file carried no `normalized` signal")
	}
	if len(rep.Normalizations) == 0 {
		t.Fatalf("silent-success: no top-level advisory for a normalized enrich")
	}
	// The disclosure also stays on the PROSE surface a human reads (issue #154
	// moved the channel, not the disclosure).
	if !strings.Contains(rep.String(), "advisory: a.md: input normalized before frontmatter recognition") {
		t.Errorf("prose report dropped the normalization advisory:\n%s", rep.String())
	}
	// The field marshals in binder.report/v1.
	b, err := json.Marshal(fr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\"normalized\":[\"stripped-utf8-bom\"]") {
		t.Errorf("JSON envelope missing the normalized field: %s", b)
	}
}

// TestAC6_DeterministicAndPlainUnchanged is the regression guard (design §9 AC6):
// a plain (no BOM, no lone CR) file's behaviour is byte-identical to before, the
// same input yields byte-identical output across runs (SOURCE_DATE_EPOCH pinned
// via a fixed clock), and no `normalized` signal or advisory is raised for it.
func TestAC6_DeterministicAndPlainUnchanged(t *testing.T) {
	plain := "---\ntype: Guide\ntitle: Plain\n---\nbody\n"

	run := func() (string, enrich.FileResult, *enrich.Report) {
		src := t.TempDir()
		p := writeFile(t, src, "a.md", plain)
		rep, err := enrich.Enrich(src, opts(src))
		if err != nil {
			t.Fatal(err)
		}
		return read(t, p), find(t, rep, "a.md"), rep
	}

	out1, fr1, rep1 := run()
	out2, _, _ := run()
	if out1 != out2 {
		t.Errorf("non-deterministic output:\n%q\n%q", out1, out2)
	}
	// A plain file gets no normalization signal and raises no advisory.
	if len(fr1.Normalized) != 0 {
		t.Errorf("plain file carried a normalized signal: %v", fr1.Normalized)
	}
	if len(rep1.Normalizations) != 0 {
		t.Errorf("plain file raised a normalization advisory: %v", rep1.Normalizations)
	}
	// Positive control: the plain file WAS enriched (generated added), so the run
	// is not vacuously "unchanged".
	if fr1.Status != enrich.StatusEnriched {
		t.Fatalf("control: plain file status = %q, want enriched", fr1.Status)
	}
}

// TestAC7_LoneCRInsideScalarValueIsTranslated is the fixture documenting design
// §9 AC7: a lone CR that is DATA inside a quoted scalar value (not a line
// separator) is also translated to LF and disclosed. This does not occur in a
// real classic-Mac file (there the CR is the separator) and extends existing
// residual bound #2; it is documented, not blocked.
func TestAC7_LoneCRInsideScalarValueIsTranslated(t *testing.T) {
	src := t.TempDir()
	// A double-quoted scalar whose value contains a literal lone CR byte.
	doc := "---\ntype: Guide\ntitle: \"a\rb\"\n---\nbody\n"
	p := writeFile(t, src, "a.md", doc)

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	out := []byte(read(t, p))
	if bytes.IndexByte(out, '\r') >= 0 {
		t.Errorf("lone CR inside a scalar value was not translated:\n%q", out)
	}
	fr := find(t, rep, "a.md")
	if len(fr.Normalized) != 1 || fr.Normalized[0] != "translated-lone-cr" {
		t.Errorf("normalized = %v, want [translated-lone-cr]", fr.Normalized)
	}
}

// TestNormalizationAdvisoryIsNotAFinding pins the issue-#154 semantics at the
// report level: a run whose ONLY observation is a boundary normalization
// discloses it (per-file signal + top-level advisory + prose line) yet produces
// ZERO findings, so --strict has nothing to gate on. The counterpart CLI-level
// exit-code assertion lives in cmd/normalize_boundary_test.go.
func TestNormalizationAdvisoryIsNotAFinding(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "a.md", bom+"---\ntype: Guide\ntitle: T\n---\nbody\n")

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	// Positive control: the normalization really happened and was disclosed.
	if len(rep.Normalizations) != 1 {
		t.Fatalf("normalizations = %v, want 1 advisory", rep.Normalizations)
	}
	if rep.NumSkipped != 0 {
		t.Fatalf("num_skipped = %d, want 0 (the file was enriched, not skipped)", rep.NumSkipped)
	}
	if rep.NumFindings() != 0 {
		t.Errorf("NumFindings = %d, want 0 (a normalization advisory must not gate)", rep.NumFindings())
	}
	// The advisory is additive JSON: present when raised, absent otherwise.
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\"normalizations\":[") {
		t.Errorf("JSON envelope missing the normalizations field: %s", b)
	}
}

// TestNormalizationsOmittedWhenEmpty proves the new field is additive: a run
// that normalized nothing serializes without it, so existing binder.report/v1
// consumers see an unchanged envelope.
func TestNormalizationsOmittedWhenEmpty(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "a.md", "---\ntype: Guide\ntitle: Plain\n---\nbody\n")

	rep, err := enrich.Enrich(src, opts(src))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "normalizations") {
		t.Errorf("clean run emitted the normalizations field: %s", b)
	}
}

func anyContains(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
