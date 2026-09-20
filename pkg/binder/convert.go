package binder

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/convert"
)

// convertCommand is the envelope `command` token for the convert capability — the
// one literal, owned here rather than re-supplied at each adapter call site.
const convertCommand = "convert"

// ConvertRequest carries the RESOLVED inputs a convert run needs — no ambient reads,
// no flag-framework types. The adapter reads flags/config/env at its edge and passes
// values in: DefaultType is already config-resolved, VerifiedBy is already validated,
// and TrustOrigin classifies where VerifiedBy came from (the adapter maps its own
// origin onto binder.TrustOrigin). The raw map/list grammar strings ("k=v,k=v" /
// "a,b") are parsed HERE (in the service) so the CLI and MCP do not each re-assemble
// convert.Options — the duplication Phase 3 removes.
type ConvertRequest struct {
	// Src is the source markdown corpus directory; Out is the output bundle dir.
	Src string
	Out string
	// DryRun reports what would be written without writing (the analysis preview).
	DryRun bool

	// DefaultType is the already-resolved concept type default (flag>env>file>"Note").
	DefaultType string
	// Raw flag grammar, parsed in the service (malformed → clijson.Usage, exit 2).
	TypeMapRaw       string
	StatusMapRaw     string
	StaleAfterMapRaw string
	FMRefKeysRaw     string
	SourceKeysRaw    string
	// CanonicalizeStatus opts into the fixed OKF §5.4 alias rewrite (issue #23).
	CanonicalizeStatus bool
	MapCitations       bool
	MapDraft           bool

	// VerifiedBy is the RESOLVED, already-validated actor ("" = none). TrustOrigin
	// classifies its origin for the never-fabricate-trust routing (see ResolveTrust).
	VerifiedBy  string
	TrustOrigin TrustOrigin

	// WorkspaceRoot bounds file:// resolution; ExternalRoots declares sibling roots.
	WorkspaceRoot string
	ExternalRoots []string

	// Index-catalog options (issue #9), off by default → byte-identical output.
	GroupByType      bool
	IncludeBacklinks bool
	IncludeGraph     bool

	// Version is stamped into generated.by and the JSON envelope. Now is the RESOLVED
	// determinism instant (see ResolveNow), read from SOURCE_DATE_EPOCH at the edge.
	Version string
	Now     time.Time
	// Strict is gate-semantics only: it drives the status-vocabulary pre-write gate
	// (below) and the post-write gate (ConvertResult.Gate). The MCP surface passes
	// false so it never gates; the payload is identical either way.
	Strict bool
}

// ConvertResult is the COMPLETE convert outcome (design §3.3): the fully-populated
// convert.Report — including report.Verified.Note, so no adapter patches the report
// after the call — plus the gating policy as methods.
type ConvertResult struct {
	// Report is the complete capability report.
	Report *convert.Report
	// version is captured so the Result renders its own JSON envelope with the same
	// provenance the run used; adapters do not hand-build the envelope.
	version string
}

// Convert assembles convert.Options from the request (parsing the flag grammars,
// applying the status-vocabulary pre-write gate, and routing the trust decision) and
// runs convert.Convert exactly once. The returned ConvertResult is complete — the
// trust Note lands in the report via convert.Options.VerifiedByNote, not a post-call
// patch — so the caller renders and gates it without touching the report.
//
// ctx is accepted for a uniform service signature; the underlying convert pipeline
// takes no context yet (design Residual Risk 6), so it is not threaded further.
func (s *Service) Convert(ctx context.Context, req ConvertRequest) (ConvertResult, error) {
	_ = ctx

	// Malformed map shapes/values are usage errors (exit 2).
	typeMap, err := convert.ParseTypeMap(req.TypeMapRaw)
	if err != nil {
		return ConvertResult{}, clijson.Usage(err)
	}
	// Non-conformant §5.4 status values warn on the default path and gate under
	// Strict, BEFORE any file is written (issue #23); resolveStatusMap wraps a
	// malformed argument in clijson.Usage internally, so a bare return preserves the
	// exit-2 contract and the strict gate returns a FindingsError (exit 1).
	statusMap, statusDefault, statusNotes, err := resolveStatusMap(req.StatusMapRaw, req.CanonicalizeStatus, req.Strict)
	if err != nil {
		return ConvertResult{}, err
	}
	staleAfterMap, err := convert.ParseStaleAfterMap(req.StaleAfterMapRaw)
	if err != nil {
		return ConvertResult{}, clijson.Usage(err)
	}

	// Never-fabricate-trust: the ONE routing (Source vocabulary + refused-verifier
	// Note) lives in ResolveTrust; the resolved Note is threaded into the report via
	// VerifiedByNote so convert.Analyze completes it — no adapter patch.
	trust := ResolveTrust(req.VerifiedBy, req.TrustOrigin)

	opts := convert.Options{
		Codec:              s.codec,
		DefaultType:        req.DefaultType,
		TypeMap:            typeMap,
		StatusMap:          statusMap,
		StatusDefault:      statusDefault,
		StatusNotes:        statusNotes,
		StaleAfterMap:      staleAfterMap,
		VerifiedBy:         trust.Actor,
		VerifiedByExplicit: trust.Explicit,
		VerifiedBySource:   trust.Source,
		VerifiedByNote:     trust.Note,
		FMRefKeys:          convert.ParseFMRefKeys(req.FMRefKeysRaw),
		Version:            req.Version,
		Now:                req.Now,
		DryRun:             req.DryRun,
		MapCitations:       req.MapCitations,
		SourceKeys:         convert.ParseFMRefKeys(req.SourceKeysRaw),
		MapDraft:           req.MapDraft,
		WorkspaceRoot:      req.WorkspaceRoot,
		ExternalRoots:      req.ExternalRoots,
		GroupByType:        req.GroupByType,
		IncludeBacklinks:   req.IncludeBacklinks,
		IncludeGraph:       req.IncludeGraph,
	}

	report, err := convert.Convert(req.Src, req.Out, opts)
	if err != nil {
		return ConvertResult{}, err
	}
	return ConvertResult{Report: report, version: req.Version}, nil
}

// Gate returns a typed *clijson.FindingsError (exit 1) when the run should gate, or
// nil otherwise. convert has no hard non-conformance; under Strict unresolved links
// and recovery warnings gate, and without Strict it never gates (never-reject). This
// is the ONE definition of convert's post-write gate, moved off the CLI call site.
func (r ConvertResult) Gate(strict bool) error {
	gatingPresent := r.Report.NumUnresolved > 0 || r.Report.NumRecovered > 0
	return clijson.Gate(strict, false, gatingPresent,
		fmt.Sprintf("convert produced %d unresolved link(s) and %d recovery warning(s) (--strict)",
			r.Report.NumUnresolved, r.Report.NumRecovered))
}

// EncodeJSON writes the binder.report/v1 envelope for this result to w via the core
// clijson encoder — the same bytes `binder convert --json` produces. Routing the
// envelope through the Result keeps adapters from hand-building it.
func (r ConvertResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, convertCommand, r.Report)
}
