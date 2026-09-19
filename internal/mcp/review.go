package mcp

import (
	"context"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/binder"
)

// reviewInput mirrors `binder review` flags 1:1 (design §Tool surface).
type reviewInput struct {
	Bundle      string   `json:"bundle" jsonschema:"path to the OKF bundle directory to review"`
	Today       string   `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
	Strict      bool     `json:"strict,omitempty" jsonschema:"gate semantics only; does not change the payload (parity with the CLI flag)"`
	Entrypoints []string `json:"entrypoints,omitempty" jsonschema:"concept ids or paths to treat as entrypoints, not orphans (parity with --entrypoint); root README.md is recognized automatically"`
}

// registerReview wires the review tool: it drives the shared binder.Service.Review —
// the same one code path the CLI uses — and returns the binder.report/v1 review
// payload byte-identical to `binder review --json`. Findings are returned IN the
// payload (never-reject); only an unloadable bundle is a tool error (IO class).
func registerReview(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "review",
		Description: "Summarize an OKF bundle: concepts by type, trust tiers, stale, orphans, " +
			"unresolved links. Returns the binder.report/v1 review payload (identical to " +
			"`binder review --json`).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in reviewInput) (*mcp.CallToolResult, any, error) {
		// Read SOURCE_DATE_EPOCH at the adapter edge and apply the shared rule; a
		// malformed epoch falls back to the wall clock, matching the CLI. The
		// empty-Today default is the service's.
		now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

		res, err := d.svc.Review(ctx, binder.ReviewRequest{
			Bundle:      in.Bundle,
			Entrypoints: in.Entrypoints,
			Now:         now,
			Today:       in.Today,
			Version:     d.version,
		})
		if err != nil {
			return nil, nil, err
		}
		return encodeResult(res)
	})
}
