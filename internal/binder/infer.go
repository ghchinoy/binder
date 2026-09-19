package binder

import (
	"context"
	"fmt"
	"io"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/infer"
)

// inferCommand is the envelope `command` token for the infer capability.
const inferCommand = "infer"

// InferRequest carries the RESOLVED inputs an infer run needs. The adapter reads
// flags/config/env and passes values in — no flag-framework types cross the seam.
//
// The optional Gemini semantic tier is offered through the genai-free
// infer.GeminiClient interface only: the request carries either a pre-built
// GeminiClient (tests inject a fake) or a GeminiClientFactory the adapter supplies
// (cmd/infer.go injects internal/gemini.New). The concrete client and its
// GEMINI_API_KEY / GOOGLE_CLOUD_PROJECT env reads stay in that adapter package, so
// google.golang.org/genai never enters this package's (the future committed
// pkg/binder) API or dependency graph (design §6 Phase 4 / Residual Risk 7).
type InferRequest struct {
	// Src is the source markdown corpus directory (read-only; infer writes nothing).
	Src string
	// DefaultType is the fallback concept type reported when nothing is inferred.
	DefaultType string
	// UseGemini enables the optional Gemini semantic tier.
	UseGemini bool
	// GeminiModel / GeminiLocation / GeminiProject / GeminiBackend / GeminiAPIKey
	// are the resolved Gemini settings, threaded to the injected client factory.
	GeminiModel    string
	GeminiLocation string
	GeminiProject  string
	GeminiBackend  string
	GeminiAPIKey   string
	// GeminiRequired fails the run on a Gemini error instead of degrading to the
	// deterministic tiers.
	GeminiRequired bool
	// GeminiClient is an optional pre-built client (tests inject a fake). When set
	// it takes precedence over GeminiClientFactory.
	GeminiClient infer.GeminiClient
	// GeminiClientFactory constructs the concrete Gemini client at the adapter
	// edge. It references only genai-free infer types, so naming it here does not
	// pull the cloud SDK onto the seam.
	GeminiClientFactory func(ctx context.Context, opts infer.Options) (infer.GeminiClient, string, string, error)
	// Version is the binder version stamped into the JSON envelope's `binder`
	// field. Threaded until Version is relocated to the core (design Decision 3.6).
	Version string
}

// InferResult is the COMPLETE infer outcome: the fully-populated infer.Report plus
// the gating policy as methods. No adapter patches the report after the call.
type InferResult struct {
	// Report is the complete inference proposal.
	Report *infer.Report

	// version is captured so the Result renders its own envelope with the same
	// provenance the run used; adapters do not hand-build the envelope.
	version string
}

// Infer runs the inference capability once (the orchestration cmd/ implemented
// today), assembling infer.Options from the resolved request. The returned
// InferResult is complete; the caller renders and gates it without patching.
//
// ctx is threaded to infer.Infer, which already takes a context (design Non-Goal:
// context stays where it already exists).
func (s *Service) Infer(ctx context.Context, req InferRequest) (InferResult, error) {
	rep, err := infer.Infer(ctx, req.Src, s.codec, infer.Options{
		DefaultType:     req.DefaultType,
		UseGemini:       req.UseGemini,
		GeminiModel:     req.GeminiModel,
		GeminiLocation:  req.GeminiLocation,
		GeminiProject:   req.GeminiProject,
		GeminiBackend:   req.GeminiBackend,
		GeminiAPIKey:    req.GeminiAPIKey,
		GeminiRequired:  req.GeminiRequired,
		GeminiClient:    req.GeminiClient,
		NewGeminiClient: req.GeminiClientFactory,
	})
	if err != nil {
		return InferResult{}, err
	}
	return InferResult{Report: rep, version: req.Version}, nil
}

// Empty reports whether the run inferred no directory mappings. It is the one
// definition of infer's stdout-vs-stderr routing signal: with zero mappings the
// human diagnostic goes to STDERR so stdout stays empty and machine-consumable,
// preserving the documented `--type-map "$(binder infer SRC)"` idiom (survey §4.9).
// The adapter reads this to pick the stream; the service does not write to either.
func (r InferResult) Empty() bool {
	return len(r.Report.Mappings) == 0
}

// Warnings is the count of advisory warnings the run disclosed — the ONE
// definition of what --strict gates on for infer.
func (r InferResult) Warnings() int {
	return len(r.Report.Warnings)
}

// Gate returns a typed *clijson.FindingsError (exit 1) when --strict is set and
// any warning was disclosed, or nil otherwise. Bare infer never gates (exit 0),
// including the zero-mappings case, which is not a failure condition.
func (r InferResult) Gate(strict bool) error {
	n := r.Warnings()
	return clijson.Gate(strict, false, n > 0,
		fmt.Sprintf("infer encountered %d warning(s) (--strict)", n))
}

// EncodeJSON writes the binder.report/v1 envelope for this result to w — the same
// bytes `binder infer --json` produces — routing the envelope through the core
// clijson encoder so adapters never hand-build it.
func (r InferResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, inferCommand, r.Report)
}
