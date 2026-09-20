package binder

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ghchinoy/binder/pkg/bundle"
	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/review"
)

// reviewCommand is the envelope `command` token for the review capability.
const reviewCommand = "review"

// ReviewRequest carries the RESOLVED inputs a review run needs — no ambient reads.
type ReviewRequest struct {
	// Bundle is the OKF bundle directory to review.
	Bundle string
	// Entrypoints are concept ids/paths to treat as entrypoints, not orphans.
	Entrypoints []string
	// Now is the RESOLVED determinism instant (see ResolveNow); when Today is empty
	// the service defaults staleness to Now's date.
	Now time.Time
	// Today is the RESOLVED YYYY-MM-DD staleness date; empty defaults to Now's date.
	Today string
	// Version is the binder version stamped into the JSON envelope's `binder` field.
	Version string
}

// ReviewResult is the COMPLETE review outcome. It carries the review.Report and owns
// the gating-finding arithmetic that used to live at the cmd/review.go call site
// (design §4.9: a public review.Report alone would not tell a caller which fields
// gate).
type ReviewResult struct {
	// Report is the complete capability report.
	Report *review.Report

	// version is captured so the Result renders its own JSON envelope with the same
	// provenance the run used; adapters do not hand-build the envelope.
	version string
}

// Review loads the bundle and runs review.Review, owning the empty-Today default.
//
// ctx is accepted for a uniform service signature and future cancellation; the
// underlying filesystem capability takes no context yet (design Non-Goal / Residual
// Risk 6).
func (s *Service) Review(ctx context.Context, req ReviewRequest) (ReviewResult, error) {
	_ = ctx

	b, err := bundle.Load(req.Bundle, s.codec)
	if err != nil {
		return ReviewResult{}, err
	}

	today := req.Today
	if today == "" {
		today = req.Now.Format("2006-01-02")
	}

	rep := review.Review(b, today, req.Entrypoints)
	return ReviewResult{Report: rep, version: req.Version}, nil
}

// GatingFindings is the ONE definition of how many review findings gate — the
// arithmetic that used to sit inline in cmd/review.go: orphans, stale, unresolved
// edges, and unparsed-frontmatter recoveries.
func (r ReviewResult) GatingFindings() int {
	rep := r.Report
	return len(rep.Orphans) + len(rep.Stale) + len(rep.Unresolved) + len(rep.UnparsedFrontmatter)
}

// Gate returns a typed *clijson.FindingsError (exit 1) when the run should gate, or
// nil otherwise. Review has no hard non-conformance, so bare review never gates and
// --strict gates only when a gating finding is present.
func (r ReviewResult) Gate(strict bool) error {
	n := r.GatingFindings()
	return clijson.Gate(strict, false, n > 0,
		fmt.Sprintf("review found %d gating finding(s) (--strict)", n))
}

// EncodeJSON writes the binder.report/v1 envelope for this result to w via the core
// clijson encoder — the same bytes `binder review --json` produces.
func (r ReviewResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, reviewCommand, r.Report)
}
