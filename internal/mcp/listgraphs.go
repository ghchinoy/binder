package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/graph"
)

// listGraphsInput mirrors the other tools' typed-params style (design §B.3).
// bundle is required; today and id_key are optional. id_key is a config point,
// not a hard-coded assumption: empty means path-as-identity (spec §2), a set
// key prefers an authored stable-id frontmatter value when present (§A.3).
type listGraphsInput struct {
	Bundle string `json:"bundle" jsonschema:"path to the OKF bundle directory to introspect"`
	Today  string `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
	IDKey  string `json:"id_key,omitempty" jsonschema:"frontmatter key to prefer as the stable node key; empty falls back to path-as-identity (spec §2)"`
}

// registerListGraphs wires the read-only list_graphs tool: it loads the bundle
// and calls the existing graph.Describe, returning the binder.report/v1
// list_graphs envelope via the shared clijson encoder. Like the other tools it
// adds NO business logic and NO second serialization path — Load + Build +
// aggregate only; it never writes to the bundle, mutates frontmatter, or mints
// an id (design §B.5). An unloadable bundle is an IO-class tool error.
func registerListGraphs(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_graphs",
		Description: "Introspect the property graph(s) binder can project from an OKF bundle: " +
			"graph name, node labels (the concept types present) and the single LINKS edge label, " +
			"each with property declarations and counts. Read-only; returns the binder.report/v1 " +
			"list_graphs payload derived from the same projection as `binder graph`.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in listGraphsInput) (*mcp.CallToolResult, any, error) {
		b, err := bundle.Load(in.Bundle, d.codec)
		if err != nil {
			return nil, nil, err
		}
		desc := graph.Describe(b, todayOrNow(in.Today), in.IDKey)
		return d.encode("list_graphs", desc)
	})
}
