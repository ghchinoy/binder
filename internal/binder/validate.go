package binder

import (
	"context"
	"fmt"
	"io"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/validate"
)

// validateCommand is the envelope `command` token for the validate capability.
const validateCommand = "validate"

// ValidateRequest carries the RESOLVED inputs a validate run needs. Validate reads
// an on-disk OKF bundle and never writes; there is no determinism instant to
// resolve (staleness is not part of conformance).
type ValidateRequest struct {
	// Bundle is the OKF bundle directory to validate.
	Bundle string
	// Spec is the spec version to validate against. When empty the service defaults
	// it to okf.DefaultSpecVersion — the one place that default lives.
	Spec okf.SpecVersion
	// Version is the binder version stamped into the JSON envelope's `binder` field.
	Version string
}

// ValidateResult is the COMPLETE validate outcome. It carries the fully-populated
// validate.Result, the spec it was checked against (so the canonical prose can name
// it without re-deriving a default), and owns the gating policy as methods — the
// only capability whose Gate encodes a non-false hardNonConformance (design §4.9:
// hard §11 non-conformance always gates; trust advisories gate only under --strict).
type ValidateResult struct {
	// Result is the complete capability report.
	Result *validate.Result
	// Spec is the spec version the bundle was validated against, echoed into the
	// canonical prose ("RESULT: conformant (OKF <spec>)").
	Spec okf.SpecVersion

	// version is captured so the Result renders its own JSON envelope with the same
	// provenance the run used; adapters do not hand-build the envelope.
	version string
}

// Validate runs validate.Bundle and returns the complete result. The empty-Spec
// default is owned here, so no adapter supplies okf.DefaultSpecVersion at the call
// site.
//
// ctx is accepted for a uniform service signature and future cancellation; the
// underlying filesystem capability takes no context yet (design Non-Goal / Residual
// Risk 6).
func (s *Service) Validate(ctx context.Context, req ValidateRequest) (ValidateResult, error) {
	_ = ctx

	spec := req.Spec
	if spec == "" {
		spec = okf.DefaultSpecVersion
	}

	result, err := validate.Bundle(req.Bundle, s.codec, spec)
	if err != nil {
		return ValidateResult{}, err
	}
	return ValidateResult{Result: result, Spec: spec, version: req.Version}, nil
}

// GatingFindings reports how many findings would gate. Hard §11 violations always
// gate; trust advisories gate only under --strict, so the count that *always* gates
// is the error count. This is the one definition of validate's gating arithmetic.
func (r ValidateResult) GatingFindings() int {
	return len(r.Result.Errors())
}

// Gate returns a typed *clijson.FindingsError (exit 1) when the run should gate, or
// nil otherwise. Validate is the ONLY capability passing a non-false
// hardNonConformance to clijson.Gate: a hard §11 non-conformance always gates
// (regardless of --strict), while trust well-formedness advisories gate only under
// --strict (#7). The message mirrors the historical cmd/validate.go wording exactly.
func (r ValidateResult) Gate(strict bool) error {
	hard := !r.Result.Conformant()
	adv := len(r.Result.Advisories()) > 0
	errs := r.Result.Errors()
	msg := fmt.Sprintf("bundle is not conformant (%d violation(s))", len(errs))
	if !hard && strict && adv {
		msg = fmt.Sprintf("bundle has %d advisory finding(s) (--strict)", len(r.Result.Advisories()))
	}
	return clijson.Gate(strict, hard, adv, msg)
}

// EncodeJSON writes the binder.report/v1 envelope for this result to w via the core
// clijson encoder — the same bytes `binder validate --json` produces.
func (r ValidateResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, validateCommand, r.Result)
}
