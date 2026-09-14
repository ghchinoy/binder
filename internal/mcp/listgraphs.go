package mcp

import (
	"context"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/binder"
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

// registerListGraphs wires the read-only list_graphs tool: it drives the shared
// binder.Service.GraphDescribe — the same one code path the CLI's `binder graph
// schema-describe` uses — returning the binder.report/v1 list_graphs envelope. It
// adds NO business logic and NO second serialization path; it never writes to the
// bundle, mutates frontmatter, or mints an id (design §B.5). An unloadable bundle is
// an IO-class tool error.
func registerListGraphs(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_graphs",
		Description: "Introspect the property graph(s) binder can project from an OKF bundle: " +
			"graph name, node labels (the concept types present) and the single LINKS edge label, " +
			"each with property declarations and counts. Read-only; returns the binder.report/v1 " +
			"list_graphs payload derived from the same projection as `binder graph`.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listGraphsInput) (*mcp.CallToolResult, any, error) {
		// Read SOURCE_DATE_EPOCH at the adapter edge and apply the shared rule; a
		// malformed epoch falls back to the wall clock, matching the CLI.
		now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

		res, err := d.svc.GraphDescribe(ctx, binder.GraphDescribeRequest{
			Bundle:  in.Bundle,
			IDKey:   in.IDKey,
			Now:     now,
			Today:   in.Today,
			Version: d.version,
		})
		if err != nil {
			return nil, nil, err
		}
		return encodeResult(res)
	})
}
