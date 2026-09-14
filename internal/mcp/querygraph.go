package mcp

import (
	"context"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/graph"
)

// queryGraphInput is the typed param set for the query_graph tool. bundle and op
// are always required; the remaining params are per-op (validated semantically by
// the service, since the SDK only enforces presence of the Go-required fields). It
// mirrors the list_graphs input style (design §3). id_key is accepted for parity
// with list_graphs and is echoed back, but it does NOT re-key traversal identity
// in this version (§14.1).
type queryGraphInput struct {
	Bundle    string      `json:"bundle" jsonschema:"path to the OKF bundle directory to query"`
	Op        string      `json:"op" jsonschema:"the query verb: lookup|neighbors|neighborhood|pattern|path"`
	Today     string      `json:"today,omitempty" jsonschema:"date (YYYY-MM-DD) used for staleness; defaults to now (honors SOURCE_DATE_EPOCH)"`
	IDKey     string      `json:"id_key,omitempty" jsonschema:"accepted for parity with list_graphs; does NOT re-key traversal identity in this version (identity is always the path-derived Concept.ID) and is never minted — the response echoes node_key.honored:false when a non-empty value is supplied"`
	ID        string      `json:"id,omitempty" jsonschema:"lookup/neighbors/neighborhood: the subject node id (path-derived Concept.ID)"`
	Label     string      `json:"label,omitempty" jsonschema:"lookup: the concept type to list; pattern: the source concept type (required for pattern)"`
	Direction string      `json:"direction,omitempty" jsonschema:"neighbors/neighborhood/path: out|in|both (default out)"`
	Rel       string      `json:"rel,omitempty" jsonschema:"neighbors/neighborhood/pattern: optional exact-match filter on the edge relationship text (Edge.Text)"`
	Depth     int         `json:"depth,omitempty" jsonschema:"neighborhood: BFS depth, required, 1..5"`
	ToLabel   string      `json:"to_label,omitempty" jsonschema:"pattern: optional target concept type"`
	Where     *whereInput `json:"where,omitempty" jsonschema:"pattern: optional property predicate over type/tier/stale"`
	From      string      `json:"from,omitempty" jsonschema:"path: the source node id (required for path)"`
	To        string      `json:"to,omitempty" jsonschema:"path: the target node id (required for path)"`
	MaxDepth  int         `json:"max_depth,omitempty" jsonschema:"path: maximum hop depth, required, 1..5"`
}

// whereInput is the typed pattern property predicate: prop ∈ {type, tier, stale}
// matched exactly against eq (stale compares against "true"/"false").
type whereInput struct {
	Prop string `json:"prop" jsonschema:"the target property to filter on: type|tier|stale"`
	Eq   string `json:"eq" jsonschema:"the exact value the property must equal"`
}

// registerQueryGraph wires the additive, read-only query_graph tool: it maps the
// typed params onto a binder.GraphQueryRequest and drives the shared
// binder.Service.GraphQuery — the same one code path the CLI's `binder graph query`
// uses. The service owns the semantic validation, the orchestration (bundle.Load →
// graph.Build → graph.NewIndex → verb), and the envelope; this handler only maps
// input and frames the result. A query that matches nothing is a result, not an
// error; malformed input is a usage-class tool error; an unloadable bundle is an
// IO-class tool error.
func registerQueryGraph(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "query_graph",
		Description: "Query the property graph binder projects from an OKF bundle (read-only). " +
			"Required op selects one of five verbs: lookup (by id or label), neighbors (one-hop, " +
			"direction out|in|both, optional rel filter on edge text), neighborhood (bounded k-hop " +
			"BFS, depth 1..5), pattern (source nodes of a label linking to a node matching to_label " +
			"and/or a type/tier/stale predicate), path (bounded shortest hop-path existence). " +
			"Returns the binder.report/v1 query_graph payload. Every traversal is bounded; a query " +
			"that matches nothing is a result, not an error. id_key is accepted for parity with " +
			"list_graphs but does NOT re-key traversal identity in this version.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in queryGraphInput) (*mcp.CallToolResult, any, error) {
		// Read SOURCE_DATE_EPOCH at the adapter edge and apply the shared rule; a
		// malformed epoch falls back to the wall clock, matching the CLI.
		now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

		res, err := d.svc.GraphQuery(ctx, queryRequestFrom(in, now, d.version))
		if err != nil {
			return nil, nil, err // usage- or IO-class tool error
		}
		return encodeResult(res)
	})
}

// queryRequestFrom maps the MCP tool input onto the service request. It is the one
// adapter-mapping seam; the semantic validation and dispatch live in the service.
func queryRequestFrom(in queryGraphInput, now time.Time, version string) binder.GraphQueryRequest {
	var where *graph.WhereClause
	if in.Where != nil {
		where = &graph.WhereClause{Prop: in.Where.Prop, Eq: in.Where.Eq}
	}
	return binder.GraphQueryRequest{
		Bundle:    in.Bundle,
		Op:        binder.GraphQueryOp(in.Op),
		Now:       now,
		Today:     in.Today,
		IDKey:     in.IDKey,
		ID:        in.ID,
		Label:     in.Label,
		Direction: in.Direction,
		Rel:       in.Rel,
		Depth:     in.Depth,
		ToLabel:   in.ToLabel,
		Where:     where,
		From:      in.From,
		To:        in.To,
		MaxDepth:  in.MaxDepth,
		Version:   version,
	}
}
