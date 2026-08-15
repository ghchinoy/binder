package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/lint"
)

// lintInput mirrors `binder lint` flags 1:1 (design §Tool surface).
type lintInput struct {
	Src    string `json:"src" jsonschema:"source markdown corpus directory to lint (read-only)"`
	Today  string `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
	Strict bool   `json:"strict,omitempty" jsonschema:"gate semantics only; does not change the payload (parity with the CLI flag)"`
}

// registerLint wires the lint tool: it runs the same convert.Analyze →
// lint.Lint pipeline as the CLI over a SOURCE corpus (writes nothing), returning
// the *lint.Report envelope byte-identical to `binder lint --json`. A
// missing/non-directory corpus path is a usage-class tool error, distinguishable
// from a mid-walk IO failure.
func registerLint(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "lint",
		Description: "Check a SOURCE markdown corpus (read-only) for broken links, missing titles, " +
			"orphans, stale, and schema issues. Returns the binder.report/v1 lint payload " +
			"(identical to `binder lint --json`).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in lintInput) (*mcp.CallToolResult, any, error) {
		if info, err := os.Stat(in.Src); err != nil || !info.IsDir() {
			return nil, nil, fmt.Errorf("corpus %q is not a readable directory", in.Src)
		}

		concepts, facts, _, err := convert.Analyze(in.Src, convert.Options{
			Codec:   d.codec,
			Version: d.version,
			Now:     resolveNow(),
		})
		if err != nil {
			return nil, nil, err
		}

		rep := lint.Lint(concepts, facts, todayOrNow(in.Today))
		rep.Src = in.Src
		return d.encode("lint", rep)
	})
}
