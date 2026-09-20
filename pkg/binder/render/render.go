// Package render produces the canonical human/terminal prose for a binder Result
// (design §3.4: contract-bearing prose lives in the core so every consumer gets
// parity instead of re-deriving it). Even though lint's prose is light, Phase 1
// exercises this seam — the CLI renders through it — to prove the render pattern
// before the fan-out to prose-heavy capabilities like validate.
//
// It is presentation, not contract: it holds no capability logic and only formats a
// completed Result. JSON envelope output is a separate seam (LintResult.EncodeJSON).
package render

import (
	"fmt"
	"strings"

	"github.com/ghchinoy/binder/pkg/binder"
)

// Lint returns the deterministic human-readable lint report for a LintResult,
// byte-identical to what the CLI printed before the collapse. The canonical prose is
// owned by lint.Report.String(); this is the core seam adapters call so the text has
// a single home.
func Lint(res binder.LintResult) string {
	return res.Report.String()
}

// Infer returns the canonical human-readable infer prose for an InferResult,
// byte-identical to what the CLI printed before the collapse. The text is owned by
// infer.Report.String(); this is the core seam adapters call so the prose has a
// single home. Which STREAM it goes to (stdout when mappings exist, stderr when
// empty) is the adapter's call, driven by InferResult.Empty().
func Infer(res binder.InferResult) string {
	return res.Report.String()
}

// Convert returns the deterministic human-readable convert report for a
// ConvertResult, byte-identical to what the CLI printed before the collapse. The
// canonical prose (including the trust-disclosure block) is owned by
// convert.Report.String(); this is the core seam adapters call.
func Convert(res binder.ConvertResult) string {
	return res.Report.String()
}

// Enrich returns the deterministic human-readable enrich report for an EnrichResult,
// byte-identical to what the CLI printed before the collapse. The canonical prose is
// owned by enrich.Report.String(); this is the core seam adapters call.
func Enrich(res binder.EnrichResult) string {
	return res.Report.String()
}

// Review returns the deterministic human-readable review report for a ReviewResult.
// The canonical prose is owned by review.Report.String(); this is the core seam.
func Review(res binder.ReviewResult) string {
	return res.Report.String()
}

// Validate returns the canonical human-readable verdict for a ValidateResult. This
// is the prose that lived 100% in cmd/validate.go (design §4.9: validate.Result has
// no String()); it now has a single home in the core so every consumer gets the same
// scope-disclosure line and RESULT: verdict instead of re-deriving it. The text is
// byte-identical to what the CLI printed before the collapse.
func Validate(res binder.ValidateResult) string {
	r := res.Result
	errs := r.Errors()

	var b strings.Builder
	fmt.Fprintf(&b, "bundle: %s\n", r.Root)
	fmt.Fprintf(&b, "concepts: %d, reserved files: %d\n", r.NumConcepts, r.NumReserved)
	// Make the unchecked scope explicit so `conformant` is not read as covering the
	// reserved files, which are counted but not structurally examined (spec §8/§9
	// deferred, #77). Never fabricate trust: the verdict must not silently claim a
	// surface it never inspected.
	if !r.ReservedStructureChecked && r.NumReserved > 0 {
		fmt.Fprintf(&b, "scope: reserved-file structure (index.md, log.md) not validated; verdict covers concept files only\n")
	}
	for _, f := range r.Advisories() {
		fmt.Fprintf(&b, "%s\n", f)
	}
	for _, f := range errs {
		fmt.Fprintf(&b, "%s\n", f)
	}
	if r.Conformant() {
		fmt.Fprintf(&b, "RESULT: conformant (OKF %s)\n", res.Spec)
	} else {
		fmt.Fprintf(&b, "RESULT: NOT conformant (%d violation(s))\n", len(errs))
	}
	return b.String()
}
