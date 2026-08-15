package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/review"
)

// reviewInput mirrors `binder review` flags 1:1 (design §Tool surface).
type reviewInput struct {
	Bundle string `json:"bundle" jsonschema:"path to the OKF bundle directory to review"`
	Today  string `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
	Strict bool   `json:"strict,omitempty" jsonschema:"gate semantics only; does not change the payload (parity with the CLI flag)"`
}

// registerReview wires the review tool: it loads the bundle and calls the
// existing review.Review, returning the *review.Report envelope byte-identical to
// `binder review --json`. Findings are returned IN the payload (never-reject);
// only an unloadable bundle is a tool error (IO class).
func registerReview(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "review",
		Description: "Summarize an OKF bundle: concepts by type, trust tiers, stale, orphans, " +
			"unresolved links. Returns the binder.report/v1 review payload (identical to " +
			"`binder review --json`).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in reviewInput) (*mcp.CallToolResult, any, error) {
		b, err := bundle.Load(in.Bundle, d.codec)
		if err != nil {
			return nil, nil, err
		}
		rep := review.Review(b, todayOrNow(in.Today))
		return d.encode("review", rep)
	})
}
