// Package binder is binder's shared service / orchestration + policy layer — the
// "one implementation" the CLI, the MCP server, and external Go callers all drive
// (design §3, G1/G3). For each capability it owns request assembly, multi-step
// orchestration, the policy that used to be stranded in cmd/ (the SOURCE_DATE_EPOCH
// determinism rule, the gating-finding definition), and the complete wire-contract
// result — so no adapter patches a report after the call.
//
// Phase 1 delivers the vertical slice for one capability: lint. It exercises the
// full Request → Result → orchestration → gate → render pattern (design §6 Phase
// 1) so the seam is proven by two real consumers before any fan-out.
//
// This package lives under internal/ during phases 1–4 and is promoted to pkg/ at
// the final phase (design §3.6, Decision 1); everything here is reversible until
// then. No cobra/pflag/viper type appears in any signature, and there is no
// os.Exit / log.Fatal / fmt.Print on the service path (design AC5) — os.Exit stays
// at main.go, driven by clijson.ExitCode.
package binder

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/convert"
	"github.com/ghchinoy/binder/pkg/lint"
	"github.com/ghchinoy/binder/pkg/okf"
)

// lintCommand is the envelope `command` token for the lint capability — the one
// literal, owned here rather than re-supplied at each adapter call site.
const lintCommand = "lint"

// Service is the shared orchestration + policy layer. It is constructed once with
// the composition root's codec choice (design §3.5: cmd/root.go stays the one place
// a concrete codec is selected) and is safe for concurrent use — it holds only the
// injected codec and no mutable state (design Residual Risk 5; see TestLintConcurrent).
type Service struct {
	codec okf.Codec
}

// New builds a Service over the given codec. The codec is injected by the adapter's
// composition root (native.New() for the CLI/MCP today) rather than hardcoded here,
// preserving the okf.Codec pluggability seam.
func New(codec okf.Codec) *Service {
	return &Service{codec: codec}
}

// LintRequest carries the RESOLVED inputs a lint run needs — no ambient reads, no
// flag-framework types. The adapter reads flags/config/env and passes values in.
type LintRequest struct {
	// Src is the source markdown corpus directory (read-only; lint writes nothing).
	Src string
	// Entrypoints are concept ids/paths to treat as entrypoints, not orphans.
	Entrypoints []string
	// Now is the RESOLVED determinism instant (see ResolveNow). It drives
	// convert.Analyze's generated-stamp times and, when Today is empty, the default
	// staleness date. Adapters resolve it from SOURCE_DATE_EPOCH; the service never
	// reads the environment.
	Now time.Time
	// Today is the RESOLVED YYYY-MM-DD date used for staleness. When empty the
	// service defaults it to Now's date — the one place that rule lives (it used to
	// be duplicated in cmd.resolveNow's caller and pkg/mcp.todayOrNow).
	Today string
	// Version is the binder version stamped into the analyze step and the JSON
	// envelope's `binder` field. Threaded through the request until Version is
	// relocated to the core (design Decision 3.6, a later phase).
	Version string
}

// LintResult is the COMPLETE lint outcome (design §3.3, Phase 1 point 1). It carries
// the fully-populated lint.Report — Src is filled inside the service, so no adapter
// patches the report after the call — and owns the gating policy as methods.
type LintResult struct {
	// Report is the complete capability report, including Src.
	Report *lint.Report

	// version is captured so the Result renders its own JSON envelope with the same
	// provenance the run used; adapters do not hand-build the envelope.
	version string
}

// Lint runs the orchestration cmd/ and pkg/mcp/ each implemented independently
// today (convert.Analyze → lint.Lint) exactly once, and owns the rep.Src fill. The
// returned LintResult is complete; the caller renders and gates it without patching.
//
// ctx is accepted for a uniform service signature and future cancellation; the
// underlying filesystem capabilities take no context yet (design Non-Goal / Residual
// Risk 6), so it is not threaded further in Phase 1.
func (s *Service) Lint(ctx context.Context, req LintRequest) (LintResult, error) {
	_ = ctx

	concepts, facts, _, err := convert.Analyze(req.Src, convert.Options{
		Codec:   s.codec,
		Version: req.Version,
		Now:     req.Now,
	})
	if err != nil {
		return LintResult{}, err
	}

	today := req.Today
	if today == "" {
		today = req.Now.Format("2006-01-02")
	}

	rep := lint.Lint(concepts, facts, today, req.Entrypoints)
	rep.Src = req.Src // owned here now — the field cmd/ and mcp/ used to patch.

	return LintResult{Report: rep, version: req.Version}, nil
}

// GatingFindings is the ONE definition of how many lint findings gate. It excludes
// the issue-#93 colon-space advisory by construction (lint.Report.NumFindings), so
// that advisory can never move an exit code. This replaces the per-adapter arithmetic
// that used to sit at each cmd/mcp call site.
func (r LintResult) GatingFindings() int {
	return r.Report.NumFindings()
}

// Gate returns a typed *clijson.FindingsError (exit 1) when the run should gate, or
// nil otherwise — reusing the existing typed-error classification, not a new one.
// lint has no hard non-conformance (that stays `binder validate`'s job), so bare
// lint never gates and --strict gates only when a gating finding is present.
func (r LintResult) Gate(strict bool) error {
	n := r.GatingFindings()
	return clijson.Gate(strict, false, n > 0,
		fmt.Sprintf("lint found %d finding(s) (--strict)", n))
}

// EncodeJSON writes the binder.report/v1 envelope for this result to w via the core
// clijson encoder — the same bytes `binder lint --json` produces. Routing the
// envelope through the Result keeps adapters from hand-building it (design §6 Phase 1
// point 2); external callers get machine output identical to the CLI.
func (r LintResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, lintCommand, r.Report)
}

// ResolveNow is the pure SOURCE_DATE_EPOCH determinism RULE (design §3.3): the one
// definition replacing cmd.resolveNow and its mirror in pkg/mcp/server.go for
// the lint path. It reads no environment — the adapter passes the resolved epoch
// string and a fallback (typically time.Now()). An empty epoch yields the fallback;
// a valid epoch yields that instant in UTC; a malformed epoch yields the fallback
// plus an error so a caller that wants strictness can observe it. The CLI/MCP
// adapters preserve their historical silent-fallback behavior by using the returned
// time and ignoring the error.
func ResolveNow(sourceDateEpoch string, fallback time.Time) (time.Time, error) {
	if sourceDateEpoch == "" {
		return fallback, nil
	}
	secs, err := strconv.ParseInt(sourceDateEpoch, 10, 64)
	if err != nil {
		return fallback, fmt.Errorf("invalid SOURCE_DATE_EPOCH %q: %w", sourceDateEpoch, err)
	}
	return time.Unix(secs, 0).UTC(), nil
}

// ResolveNowFromEnv is a convenience wrapper for callers who want binder's ambient
// SOURCE_DATE_EPOCH behavior: it reads the environment and applies the ResolveNow
// rule with time.Now() as the fallback. CLI/MCP adapters do NOT use this — they read
// the env themselves and call ResolveNow, keeping ambient reads at the edges (design
// §3.3). It exists for external callers who want the same env contract.
func ResolveNowFromEnv() (time.Time, error) {
	return ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())
}
