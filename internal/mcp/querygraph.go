package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/graph"
)

// queryGraphInput is the typed param set for the query_graph tool. bundle and op
// are always required; the remaining params are per-op (validated semantically
// below, since the SDK only enforces presence of the Go-required fields). It
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

// registerQueryGraph wires the additive, read-only query_graph tool: it validates
// the per-op params (semantic usage errors), loads the bundle, builds the
// deterministic graph.Model, indexes it, dispatches the verb, and encodes the
// result through the shared clijson encoder — the SAME binder.report/v1 envelope
// every other tool uses (no second serialization path). It adds no build path and
// no new dependency: bundle.Load → graph.Build → graph.NewIndex → verb →
// d.encode. It never writes to the bundle, mutates frontmatter, or mints an id.
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
	}, func(_ context.Context, _ *mcp.CallToolRequest, in queryGraphInput) (*mcp.CallToolResult, any, error) {
		if err := validateQuery(in); err != nil {
			return nil, nil, err // usage-class tool error
		}
		b, err := bundle.Load(in.Bundle, d.codec)
		if err != nil {
			return nil, nil, err // IO-class tool error
		}
		model := graph.Build(b, todayOrNow(in.Today))
		idx := graph.NewIndex(model)
		result := dispatchQuery(idx, in)
		return d.encode("query_graph", result)
	})
}

// dispatchQuery runs the validated verb against the index and returns its typed
// result payload. direction defaults to "out" when empty (mirrored into the echoed
// query), consistent with the design's per-op defaults.
func dispatchQuery(idx *graph.Index, in queryGraphInput) any {
	dir := directionOrDefault(in.Direction)
	switch in.Op {
	case "lookup":
		return idx.Lookup(in.IDKey, in.ID, in.Label)
	case "neighbors":
		return idx.Neighbors(in.IDKey, in.ID, dir, in.Rel)
	case "neighborhood":
		return idx.Neighborhood(in.IDKey, in.ID, in.Depth, dir, in.Rel)
	case "pattern":
		var w *graph.WhereClause
		if in.Where != nil {
			w = &graph.WhereClause{Prop: in.Where.Prop, Eq: in.Where.Eq}
		}
		return idx.Pattern(in.IDKey, in.Label, in.ToLabel, in.Rel, w)
	case "path":
		return idx.Path(in.IDKey, in.From, in.To, dir, in.MaxDepth)
	}
	return nil // unreachable: validateQuery rejects unknown ops
}

// validateQuery enforces the semantic constraints the SDK cannot: a known op,
// the per-op required params, the depth range, a valid direction, a valid
// where.prop, and the lookup one-of. Each maps to a usage error (exit 2 / tool
// IsError). A referenced node id that does not exist is NOT validated here — that
// is a finding produced by the verb (never-reject, design §6).
func validateQuery(in queryGraphInput) error {
	switch in.Op {
	case "lookup":
		if (in.ID == "") == (in.Label == "") {
			return usage("lookup requires exactly one of id or label")
		}
	case "neighbors":
		if in.ID == "" {
			return usage("neighbors requires id")
		}
		if err := validDirection(in.Direction); err != nil {
			return err
		}
	case "neighborhood":
		if in.ID == "" {
			return usage("neighborhood requires id")
		}
		if err := validDepth("depth", in.Depth); err != nil {
			return err
		}
		if err := validDirection(in.Direction); err != nil {
			return err
		}
	case "pattern":
		if in.Label == "" {
			return usage("pattern requires label")
		}
		if in.ToLabel == "" && in.Where == nil {
			return usage("pattern requires at least one of to_label or where")
		}
		if in.Where != nil {
			if err := validProp(in.Where.Prop); err != nil {
				return err
			}
		}
	case "path":
		if in.From == "" || in.To == "" {
			return usage("path requires from and to")
		}
		if err := validDepth("max_depth", in.MaxDepth); err != nil {
			return err
		}
		if err := validDirection(in.Direction); err != nil {
			return err
		}
	case "":
		return usage("op is required (want lookup|neighbors|neighborhood|pattern|path)")
	default:
		return usage(fmt.Sprintf("unknown op %q (want lookup|neighbors|neighborhood|pattern|path)", in.Op))
	}
	return nil
}

// directionOrDefault normalizes an empty direction to the "out" default.
func directionOrDefault(dir string) string {
	if dir == "" {
		return "out"
	}
	return dir
}

// validDirection accepts an empty (defaulted) direction or out|in|both.
func validDirection(dir string) error {
	switch dir {
	case "", "out", "in", "both":
		return nil
	}
	return usage(fmt.Sprintf("invalid direction %q (want out|in|both)", dir))
}

// validDepth enforces the mandatory hard bound 1..MaxDepth on a required depth
// param (name identifies the offending param in the message).
func validDepth(name string, v int) error {
	if v < 1 || v > graph.MaxDepth {
		return usage(fmt.Sprintf("%s must be in 1..%d", name, graph.MaxDepth))
	}
	return nil
}

// validProp enforces where.prop ∈ {type, tier, stale}.
func validProp(prop string) error {
	switch prop {
	case "type", "tier", "stale":
		return nil
	}
	return usage(fmt.Sprintf("invalid where.prop %q (want type|tier|stale)", prop))
}

// usage wraps msg as a clijson usage error (exit 2 in the CLI contract; an
// IsError tool result over MCP).
func usage(msg string) error {
	return clijson.Usage(errors.New(msg))
}
