package binder

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/convert"
	"github.com/ghchinoy/binder/pkg/enrich"
)

// enrichCommand is the envelope `command` token for the enrich capability.
const enrichCommand = "enrich"

// EnrichRequest carries the RESOLVED inputs an enrich run needs — no ambient reads,
// no flag-framework types. It parallels ConvertRequest: DefaultType is
// config-resolved, VerifiedBy is already validated, TrustOrigin classifies its
// origin, and the raw flag grammars are parsed in the service. enrich has no MCP tool
// (it mutates the source in place), so it is CLI-fed only.
type EnrichRequest struct {
	// Src is the source markdown tree enriched in place (unless DryRun).
	Src string
	// DefaultType is the already-resolved concept type default.
	DefaultType string
	// Raw flag grammar, parsed in the service (malformed → clijson.Usage, exit 2).
	TypeMapRaw       string
	StatusMapRaw     string
	StaleAfterMapRaw string
	OverwriteKeysRaw string
	// CanonicalizeStatus opts into the fixed OKF §5.4 alias rewrite (issue #23).
	CanonicalizeStatus bool

	// VerifiedBy is the RESOLVED, already-validated actor ("" = none); TrustOrigin
	// classifies its origin for the never-fabricate-trust routing (see ResolveTrust).
	VerifiedBy  string
	TrustOrigin TrustOrigin

	// Version is stamped into generated.by; Now is the RESOLVED determinism instant.
	Version string
	Now     time.Time
	// DryRun computes + reports but writes nothing. Strict is gate-semantics only.
	DryRun bool
	Strict bool
}

// EnrichResult is the COMPLETE enrich outcome: the fully-populated enrich.Report —
// including report.Verified.Note — plus the gating policy as methods.
type EnrichResult struct {
	// Report is the complete capability report.
	Report *enrich.Report
	// version renders the JSON envelope with the run's provenance.
	version string
}

// Enrich assembles enrich.Options from the request (parsing the flag grammars,
// applying the status-vocabulary pre-write gate, routing the trust decision) and runs
// enrich.Enrich exactly once. The returned EnrichResult is complete — the trust Note
// lands in the report via enrich.Options.VerifiedByNote — so the caller renders and
// gates it without patching.
func (s *Service) Enrich(ctx context.Context, req EnrichRequest) (EnrichResult, error) {
	_ = ctx

	typeMap, err := convert.ParseTypeMap(req.TypeMapRaw)
	if err != nil {
		return EnrichResult{}, clijson.Usage(err)
	}
	// Non-conformant §5.4 status values warn on the default path and gate under
	// Strict, BEFORE any file is written (issue #23).
	statusMap, statusDefault, statusNotes, err := resolveStatusMap(req.StatusMapRaw, req.CanonicalizeStatus, req.Strict)
	if err != nil {
		return EnrichResult{}, err
	}
	staleAfterMap, err := convert.ParseStaleAfterMap(req.StaleAfterMapRaw)
	if err != nil {
		return EnrichResult{}, clijson.Usage(err)
	}
	// --overwrite-keys is the opt-in, scoped exception to additive-only (issue #22).
	// A malformed list, or naming a trust/attestation-carrying key, is a usage error
	// (exit 2) that names the offending key and modifies no file.
	overwriteKeys, err := enrich.ParseOverwriteKeys(req.OverwriteKeysRaw)
	if err != nil {
		return EnrichResult{}, clijson.Usage(err)
	}

	trust := ResolveTrust(req.VerifiedBy, req.TrustOrigin)

	opts := enrich.Options{
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
		OverwriteKeys:      overwriteKeys,
		Version:            req.Version,
		Now:                req.Now,
		DryRun:             req.DryRun,
	}

	rep, err := enrich.Enrich(req.Src, opts)
	if err != nil {
		return EnrichResult{}, err
	}
	return EnrichResult{Report: rep, version: req.Version}, nil
}

// Gate returns a typed *clijson.FindingsError (exit 1) when the run should gate, or
// nil otherwise. It covers only what NumFindings counts — skipped files (unparseable
// frontmatter) and preserve-or-advise warnings; bare enrich never gates. The
// non-conformant --status-map value is gated pre-write in resolveStatusMap and is not
// counted here (it is NARROWER than the user guide's "gating findings"). The message
// names each real quantity separately (issue #154). This is the ONE definition of
// enrich's post-write gate, moved off the CLI call site.
func (r EnrichResult) Gate(strict bool) error {
	return clijson.Gate(strict, false, r.Report.NumFindings() > 0,
		fmt.Sprintf("enrich skipped %d file(s) and raised %d warning(s) (--strict)",
			r.Report.NumSkipped, len(r.Report.Warnings)))
}

// EncodeJSON writes the binder.report/v1 envelope for this result to w — the same
// bytes `binder enrich --json` produces.
func (r EnrichResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, enrichCommand, r.Report)
}
