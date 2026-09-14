// Package render produces the canonical human/terminal prose for a binder Result
// (design §3.4: contract-bearing prose lives in the core so every consumer gets
// parity instead of re-deriving it). Even though lint's prose is light, Phase 1
// exercises this seam — the CLI renders through it — to prove the render pattern
// before the fan-out to prose-heavy capabilities like validate.
//
// It is presentation, not contract: it holds no capability logic and only formats a
// completed Result. JSON envelope output is a separate seam (LintResult.EncodeJSON).
package render

import "github.com/ghchinoy/binder/internal/binder"

// Lint returns the deterministic human-readable lint report for a LintResult,
// byte-identical to what the CLI printed before the collapse. The canonical prose is
// owned by lint.Report.String(); this is the core seam adapters call so the text has
// a single home.
func Lint(res binder.LintResult) string {
	return res.Report.String()
}
