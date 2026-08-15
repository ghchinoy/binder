package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/graph"
)

// graphInput mirrors `binder graph` flags (design §Tool surface). Unlike the
// CLI, the tool default format is json (the CLI defaults to dot).
type graphInput struct {
	Bundle string `json:"bundle" jsonschema:"path to the OKF bundle directory to export"`
	Format string `json:"format,omitempty" jsonschema:"output format: dot|json|graphml|html (default json)"`
	Today  string `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
}

// registerGraph wires the graph tool: it loads the bundle and calls the existing
// graph.Export, returning the RAW export bytes — NOT the binder.report/v1
// envelope. For format:json that is the raw {nodes,edges} object, matching
// `binder graph --format json` (a documented exception to the envelope). An
// unknown format is a usage-class tool error (graph.Export rejects it).
func registerGraph(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "graph",
		Description: "Export the bundle's concept graph in dot|json|graphml|html (default json). " +
			"Returns the RAW export bytes, NOT the report envelope: format:json is the raw " +
			"{nodes,edges} object, identical to `binder graph --format json`.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in graphInput) (*mcp.CallToolResult, any, error) {
		format := in.Format
		if format == "" {
			format = "json"
		}
		b, err := bundle.Load(in.Bundle, d.codec)
		if err != nil {
			return nil, nil, err
		}
		data, err := graph.Export(b, format, todayOrNow(in.Today))
		if err != nil {
			// Unknown format / export failure — usage-class tool error.
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
