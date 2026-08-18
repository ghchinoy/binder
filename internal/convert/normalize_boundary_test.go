package convert_test

// Boundary-normalization tests for issue #124 (design §4.4 Option A + §9) on the
// CONVERT surface — the sibling of the enrich-surface tests. They pin that a
// BOM-prefixed or lone-CR-fenced source is recognised (its human `verified`
// block preserved rather than demoted to body under a synthetic type), that a
// genuinely-broken fence still recovers-as-body via the existing never-reject
// path after the BOM is stripped, and that both cases disclose non-optionally
// (per-concept `normalized` signal + top-level advisory). A plain file is
// unchanged and deterministic (regression guard).
//
// Primary instrument (C11/C23): a RAW-byte check of the surviving attestation
// with a positive control proving the check can show a difference.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
	"github.com/ghchinoy/binder/internal/validate"
)

var bomBytes = []byte{0xEF, 0xBB, 0xBF}

const cvVerifiedBlockLF = "verified:\n  - by: human:ghchinoy\n    at: \"2026-01-01T00:00:00Z\"\n"

func conceptReportFor(t *testing.T, rep *convert.Report, rel string) convert.ConceptReport {
	t.Helper()
	for _, c := range rep.Concepts {
		if c.RelPath == rel {
			return c
		}
	}
	t.Fatalf("no ConceptReport for %q; have %+v", rel, rep.Concepts)
	return convert.ConceptReport{}
}

func tierOfBytes(t *testing.T, raw []byte) okf.Tier {
	t.Helper()
	c, err := native.New().ParseConcept("a.md", raw)
	if err != nil {
		t.Fatalf("re-parsing converted output failed: %v\n%q", err, raw)
	}
	c.Trust = okf.ProjectTrust(c.Frontmatter, "")
	return okf.TrustTier(c)
}

func convertOne(t *testing.T, src string) (*convert.Report, string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "bundle")
	rep, err := convert.Convert(src, out, convert.Options{Codec: native.New(), Version: "0.1.0", Now: fixedNow})
	if err != nil {
		t.Fatalf("convert must not abort: %v", err)
	}
	return rep, out
}

// TestConvertAC1_BOM_PreservesVerifiedAndDiscloses — design §9 AC1, convert surface.
func TestConvertAC1_BOM_PreservesVerifiedAndDiscloses(t *testing.T) {
	src := t.TempDir()
	doc := string(bomBytes) + "---\n" +
		"type: Guide\ntitle: Real Title\nowner: alice\n" +
		cvVerifiedBlockLF + "---\nbody\n"
	write(t, src, "a.md", doc)

	rep, out := convertOne(t, src)
	got, err := os.ReadFile(filepath.Join(out, "a.md"))
	if err != nil {
		t.Fatal(err)
	}

	// PRIMARY: attestation raw bytes survive; positive control shows the check bites.
	if !bytes.Contains(got, []byte(cvVerifiedBlockLF)) {
		t.Errorf("attestation NOT preserved byte-for-byte in converted output:\n%q", got)
	}
	mutated := strings.Replace(cvVerifiedBlockLF, "human:ghchinoy", "human:attacker", 1)
	if bytes.Contains(got, []byte(mutated)) {
		t.Fatalf("positive control impossible: a mutated actor should never appear")
	}
	if bytes.Contains(got, []byte("type: Note")) {
		t.Errorf("synthetic `type: Note` was invented (the #124 demotion):\n%q", got)
	}
	if !bytes.Contains(got, []byte("type: Guide")) {
		t.Errorf("authored `type: Guide` did not survive:\n%q", got)
	}
	if bytes.HasPrefix(got, bomBytes) {
		t.Errorf("leading UTF-8 BOM was not stripped:\n%q", got)
	}
	if got := tierOfBytes(t, got); got != okf.TierHumanReviewed {
		t.Errorf("tier = %q, want human-reviewed", got)
	}

	// Disclosure: per-concept signal + top-level advisory.
	cr := conceptReportFor(t, rep, "a.md")
	if len(cr.Normalized) != 1 || cr.Normalized[0] != "stripped-utf8-bom" {
		t.Errorf("concept normalized = %v, want [stripped-utf8-bom]", cr.Normalized)
	}
	if !anyContainsCV(rep.Warnings, "normalized before frontmatter recognition") ||
		!anyContainsCV(rep.Warnings, "stripped-utf8-bom") {
		t.Errorf("top-level advisory missing; warnings = %v", rep.Warnings)
	}
}

// TestConvertAC2_LoneCR_PreservesVerifiedAndDiscloses — design §9 AC2, convert surface.
func TestConvertAC2_LoneCR_PreservesVerifiedAndDiscloses(t *testing.T) {
	src := t.TempDir()
	cr := "\r"
	doc := "---" + cr + "type: Guide" + cr + "title: Real Title" + cr + "owner: alice" + cr +
		"verified:" + cr + "  - by: human:ghchinoy" + cr + "    at: \"2026-01-01T00:00:00Z\"" + cr +
		"---" + cr + "body" + cr
	write(t, src, "a.md", doc)

	rep, out := convertOne(t, src)
	got, err := os.ReadFile(filepath.Join(out, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(cvVerifiedBlockLF)) {
		t.Errorf("attestation content not preserved after lone-CR translation:\n%q", got)
	}
	if bytes.IndexByte(got, '\r') >= 0 {
		t.Errorf("a lone CR survived into converted output:\n%q", got)
	}
	if bytes.Contains(got, []byte("type: Note")) {
		t.Errorf("synthetic `type: Note` was invented:\n%q", got)
	}
	if got := tierOfBytes(t, got); got != okf.TierHumanReviewed {
		t.Errorf("tier = %q, want human-reviewed", got)
	}
	cr2 := conceptReportFor(t, rep, "a.md")
	if len(cr2.Normalized) != 1 || cr2.Normalized[0] != "translated-lone-cr" {
		t.Errorf("concept normalized = %v, want [translated-lone-cr]", cr2.Normalized)
	}
	if !anyContainsCV(rep.Warnings, "translated-lone-cr") {
		t.Errorf("top-level advisory missing; warnings = %v", rep.Warnings)
	}
}

// TestConvertAC3_RecoverAsBodyFiresAfterNormalization — design §9 AC3, convert
// surface: after the BOM is stripped the fence opens, the YAML fails to parse,
// and the EXISTING never-reject recover-as-body path fires (NumRecovered, a
// recovery marker) — a path bypassed for these inputs before the fix. The
// recovered output carries no BOM, and the normalization is disclosed.
func TestConvertAC3_RecoverAsBodyFiresAfterNormalization(t *testing.T) {
	cases := []struct {
		name, doc string
	}{
		{"bom_invalid_yaml", string(bomBytes) + "---\ntitle: thing: with an unquoted colon\ngoal: another: bad\n---\n\n# Heading\n\nBody.\n"},
		{"bom_unterminated_fence", string(bomBytes) + "---\ntitle: never closed\ntags: [a, b]\n\n# Heading After\n\nStill body.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := t.TempDir()
			write(t, src, "bad.md", tc.doc)

			rep, out := convertOne(t, src)
			got, err := os.ReadFile(filepath.Join(out, "bad.md"))
			if err != nil {
				t.Fatal(err)
			}
			if rep.NumRecovered != 1 {
				t.Errorf("NumRecovered = %d, want 1 (recover-as-body must fire)", rep.NumRecovered)
			}
			c, err := native.New().ParseConcept("bad.md", got)
			if err != nil {
				t.Fatalf("recovered concept must re-parse cleanly: %v", err)
			}
			if !okf.IsRecovered(c.Frontmatter) {
				t.Errorf("recovered concept must carry the recovery marker:\n%q", got)
			}
			if bytes.HasPrefix(got, bomBytes) {
				t.Errorf("BOM survived into recovered output:\n%q", got)
			}
			// The original (now-inert) fence text is preserved verbatim in the body.
			if !bytes.Contains(got, []byte("Heading")) {
				t.Errorf("original body text did not survive recovery:\n%q", got)
			}
			cr := conceptReportFor(t, rep, "bad.md")
			if len(cr.Normalized) != 1 || cr.Normalized[0] != "stripped-utf8-bom" {
				t.Errorf("concept normalized = %v, want [stripped-utf8-bom]", cr.Normalized)
			}
			if !anyContainsCV(rep.Warnings, "normalized before frontmatter recognition") {
				t.Errorf("top-level advisory missing; warnings = %v", rep.Warnings)
			}
			// Bundle stays conformant.
			res, err := validate.Bundle(out, native.New(), okf.SpecV02)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Conformant() {
				t.Errorf("bundle should be conformant after recovery: %v", res.Errors())
			}
		})
	}
}

// TestConvertAC6_PlainUnchangedAndDeterministic — design §9 AC6 regression guard,
// convert surface: a plain (no BOM/lone-CR) file gets no `normalized` signal and
// no advisory, and the output is byte-identical across runs.
func TestConvertAC6_PlainUnchangedAndDeterministic(t *testing.T) {
	src := t.TempDir()
	write(t, src, "a.md", "---\ntype: Guide\ntitle: Plain\n---\nbody\n")

	rep, out := convertOne(t, src)
	got, err := os.ReadFile(filepath.Join(out, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	cr := conceptReportFor(t, rep, "a.md")
	if len(cr.Normalized) != 0 {
		t.Errorf("plain file carried a normalized signal: %v", cr.Normalized)
	}
	if anyContainsCV(rep.Warnings, "normalized before frontmatter recognition") {
		t.Errorf("plain file raised a normalization advisory: %v", rep.Warnings)
	}
	_, out2 := convertOne(t, src)
	got2, err := os.ReadFile(filepath.Join(out2, "a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, got2) {
		t.Errorf("non-deterministic convert output:\n%q\n%q", got, got2)
	}
}

func anyContainsCV(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
