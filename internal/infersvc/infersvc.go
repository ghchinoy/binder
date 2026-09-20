// Package infersvc is the internal infer orchestration + policy + render seam for
// the binder CLI adapter (cmd/infer.go). It mirrors the shape of the pkg/binder
// service facades for the other capabilities — Request → Result → orchestration →
// gate/render/encode — but is kept INTERNAL on purpose.
//
// The infer capability is DEFERRED from the first published pkg/ slice (owner
// ruling 2026-09-20): nothing infer-related is exported from pkg/ until an
// example/consumer demonstrates the need (narrow-now, examples-as-discovery). The
// feature itself is unchanged and fully functional through this internal seam; the
// concrete Gemini client (which imports google.golang.org/genai) stays behind the
// genai-free infer.GeminiClient interface, injected at the adapter edge, so the
// cloud SDK never reaches this package's dependency graph. See
// docs/api-stability.md.
package infersvc

import (
	"context"
	"fmt"
	"io"

	"github.com/ghchinoy/binder/internal/infer"
	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/okf"
)

// inferCommand is the envelope `command` token for the infer capability.
const inferCommand = "infer"

// Service is the infer orchestration + policy layer. It is constructed once with
// the composition root's codec choice (cmd/root.go stays the one place a concrete
// codec is selected) and is safe for concurrent use — it holds only the injected
// codec and no mutable state.
type Service struct {
	codec okf.Codec
}

// New builds a Service over the given codec, injected by the adapter's composition
// root (native.New() for the CLI today), preserving the okf.Codec pluggability seam.
func New(codec okf.Codec) *Service {
	return &Service{codec: codec}
}

// InferRequest carries the RESOLVED inputs an infer run needs. The adapter reads
// flags/config/env and passes values in — no flag-framework types cross the seam.
//
// The optional Gemini semantic tier is offered through the genai-free
// infer.GeminiClient interface only: the request carries either a pre-built
// GeminiClient (tests inject a fake) or a GeminiClientFactory the adapter supplies
// (cmd/infer.go injects internal/gemini.New). The concrete client and its
// GEMINI_API_KEY / GOOGLE_CLOUD_PROJECT env reads stay in that adapter package, so
// google.golang.org/genai never enters this package's dependency graph.
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
	// Version is the binder version stamped into the JSON envelope's `binder` field.
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
// ctx is threaded to infer.Infer, which already takes a context.
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

// Render returns the canonical human-readable infer prose for an InferResult,
// byte-identical to what the CLI printed before the collapse. The text is owned by
// infer.Report.String(); this is the seam the adapter calls so the prose has a
// single home. Which STREAM it goes to (stdout when mappings exist, stderr when
// empty) is the adapter's call, driven by InferResult.Empty().
func Render(res InferResult) string {
	return res.Report.String()
}
