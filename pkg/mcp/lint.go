package mcp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/pkg/binder"
)

// lintInput mirrors `binder lint` flags 1:1 (design §Tool surface).
type lintInput struct {
	Src         string   `json:"src" jsonschema:"source markdown corpus directory to lint (read-only)"`
	Today       string   `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
	Strict      bool     `json:"strict,omitempty" jsonschema:"gate semantics only; does not change the payload (parity with the CLI flag)"`
	Entrypoints []string `json:"entrypoints,omitempty" jsonschema:"concept ids or paths to treat as entrypoints, not orphans (parity with --entrypoint); root README.md is recognized automatically"`
}

// registerLint wires the lint tool: it drives the shared binder.Service.Lint over
// a SOURCE corpus (writes nothing) — the same one code path the CLI uses — and
// returns the binder.report/v1 envelope byte-identical to `binder lint --json`. A
// missing/non-directory corpus path is a usage-class tool error, distinguishable
// from a mid-walk IO failure.
func registerLint(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "lint",
		Description: "Check a SOURCE markdown corpus (read-only) for broken links, missing titles, " +
			"orphans, stale, and schema issues. Returns the binder.report/v1 lint payload " +
			"(identical to `binder lint --json`).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in lintInput) (*mcp.CallToolResult, any, error) {
		if info, err := os.Stat(in.Src); err != nil || !info.IsDir() {
			return nil, nil, fmt.Errorf("corpus %q is not a readable directory", in.Src)
		}

		// Read SOURCE_DATE_EPOCH at the adapter edge and apply the shared rule (the
		// one that used to be duplicated as this package's resolveNow); a malformed
		// epoch falls back to the wall clock, matching the CLI. The empty-Today
		// default is now the service's, so this handler no longer computes it.
		now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

		res, err := d.svc.Lint(ctx, binder.LintRequest{
			Src:         in.Src,
			Entrypoints: in.Entrypoints,
			Now:         now,
			Today:       in.Today,
			Version:     d.version,
		})
		if err != nil {
			return nil, nil, err
		}

		// The envelope is produced by the core Result and framed by the one shared
		// helper — byte-identical to `binder lint --json` and to the CLI's output.
		return encodeResult(res)
	})
}
