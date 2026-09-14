package mcp

import (
	"context"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/binder"
)

// graphInput mirrors `binder graph` flags (design §Tool surface). Unlike the
// CLI, the tool default format is json (the CLI defaults to dot).
type graphInput struct {
	Bundle string `json:"bundle" jsonschema:"path to the OKF bundle directory to export"`
	Format string `json:"format,omitempty" jsonschema:"output format: dot|json|graphml|html (default json)"`
	Today  string `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
}

// registerGraph wires the graph tool: it drives the shared binder.Service.GraphExport
// and returns the RAW export bytes — NOT the binder.report/v1 envelope. For
// format:json that is the raw {nodes,edges} object, matching `binder graph --format
// json` (a documented exception to the envelope). The raw bytes are framed by the ONE
// shared textResult helper (Phase 1 left this handler hand-framing them); byte output
// is unchanged. An unknown format is a usage-class tool error (graph.Export rejects it).
func registerGraph(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "graph",
		Description: "Export the bundle's concept graph in dot|json|graphml|html (default json). " +
			"Returns the RAW export bytes, NOT the report envelope: format:json is the raw " +
			"{nodes,edges} object, identical to `binder graph --format json`.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in graphInput) (*mcp.CallToolResult, any, error) {
		format := in.Format
		if format == "" {
			format = "json"
		}
		// Read SOURCE_DATE_EPOCH at the adapter edge and apply the shared rule; a
		// malformed epoch falls back to the wall clock, matching the CLI.
		now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

		res, err := d.svc.GraphExport(ctx, binder.GraphExportRequest{
			Bundle: in.Bundle,
			Format: format,
			Now:    now,
			Today:  in.Today,
		})
		if err != nil {
			// Unknown format / export failure — usage/IO-class tool error.
			return nil, nil, err
		}
		// Raw export bytes, framed by the one shared helper (not the envelope path).
		return textResult(string(res.Data))
	})
}
