package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/okf"
)

// validateInput mirrors `binder validate` flags 1:1 (design §Tool surface).
type validateInput struct {
	Bundle string `json:"bundle" jsonschema:"path to the OKF bundle directory to validate"`
	Strict bool   `json:"strict,omitempty" jsonschema:"gate semantics only; does not change the payload (parity with the CLI flag)"`
}

// registerValidate wires the validate tool: it drives the shared
// binder.Service.Validate — the same one code path the CLI uses — and returns the
// binder.report/v1 validate payload byte-identical to `binder validate --json`.
// Findings are returned IN the payload (never-reject); only an unreadable/unloadable
// bundle path is a tool error (IO class).
func registerValidate(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "validate",
		Description: "Check an OKF v0.2 bundle for hard conformance (spec §11). Returns the " +
			"binder.report/v1 validate payload (identical to `binder validate --json`); " +
			"non-conformance is reported IN the payload, not as a tool error.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in validateInput) (*mcp.CallToolResult, any, error) {
		res, err := d.svc.Validate(ctx, binder.ValidateRequest{
			Bundle:  in.Bundle,
			Spec:    okf.DefaultSpecVersion,
			Version: d.version,
		})
		if err != nil {
			// Unreadable/unloadable path — IO-class tool error, not a crash.
			return nil, nil, err
		}
		return encodeResult(res)
	})
}
