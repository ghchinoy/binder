package binder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf"
)

// Envelope `command` tokens for the graph read-side operations. graph export emits
// RAW bytes (not an envelope); query and describe emit the binder.report/v1 envelope
// under the MCP tool names, which are the wire-contract command strings.
const (
	queryGraphCommand = "query_graph"
	listGraphsCommand = "list_graphs"
)

// --- graph export -----------------------------------------------------------

// GraphExportRequest carries the RESOLVED inputs a graph export needs.
type GraphExportRequest struct {
	// Bundle is the OKF bundle directory to export.
	Bundle string
	// Format is the export format: dot|json|graphml|html. graph.Export normalizes
	// it (empty == dot); the adapter validates it up front for a usage-class error.
	Format string
	// Now is the RESOLVED determinism instant (see ResolveNow); when Today is empty
	// the service defaults staleness to Now's date.
	Now time.Time
	// Today is the RESOLVED YYYY-MM-DD staleness date; empty defaults to Now's date.
	Today string
}

// GraphExportResult is the COMPLETE graph export outcome: the raw export bytes
// (byte-identical to `binder graph --format <fmt>` — NOT a report envelope) plus the
// loaded bundle so the adapter can disclose unparsed files on stderr (warnings as
// data, rendered by the adapter — design §3.3). This brings the export path onto the
// shared service/adapter framing while keeping the raw-bytes output exactly as before
// (the Phase-1 carry-over: mcp/graph.go used to hand-frame these bytes).
type GraphExportResult struct {
	// Data is the raw export bytes.
	Data []byte
	// Bundle is the loaded bundle, exposed so the adapter can render the unparsed
	// disclosure (it is not part of the export payload).
	Bundle *okf.Bundle
}

// GraphExport loads the bundle and runs graph.Export, owning the empty-Today
// default. It never writes the disclosure itself — that is the adapter's presentation
// concern — so the raw bytes stay uncontaminated for every consumer.
func (s *Service) GraphExport(ctx context.Context, req GraphExportRequest) (GraphExportResult, error) {
	_ = ctx

	b, err := bundle.Load(req.Bundle, s.codec)
	if err != nil {
		return GraphExportResult{}, err
	}

	today := req.Today
	if today == "" {
		today = req.Now.Format("2006-01-02")
	}

	data, err := graph.Export(b, req.Format, today)
	if err != nil {
		return GraphExportResult{}, err
	}
	return GraphExportResult{Data: data, Bundle: b}, nil
}

// --- graph schema-describe (list_graphs) ------------------------------------

// GraphDescribeRequest carries the RESOLVED inputs a schema-describe needs. This
// operation is reachable only through MCP today (list_graphs); the service makes it
// reachable through the public surface (design G4/AC4).
type GraphDescribeRequest struct {
	// Bundle is the OKF bundle directory to introspect.
	Bundle string
	// IDKey is the frontmatter key to prefer as the stable node key; empty falls
	// back to path-as-identity (spec §2).
	IDKey string
	// Now / Today: the RESOLVED staleness inputs (empty Today defaults to Now's date).
	Now   time.Time
	Today string
	// Version is stamped into the JSON envelope's `binder` field.
	Version string
}

// GraphDescribeResult carries the schema descriptor and renders the list_graphs
// envelope.
type GraphDescribeResult struct {
	Set     *graph.SchemaSet
	version string
}

// GraphDescribe loads the bundle and runs graph.Describe, owning the empty-Today
// default.
func (s *Service) GraphDescribe(ctx context.Context, req GraphDescribeRequest) (GraphDescribeResult, error) {
	_ = ctx

	b, err := bundle.Load(req.Bundle, s.codec)
	if err != nil {
		return GraphDescribeResult{}, err
	}

	today := req.Today
	if today == "" {
		today = req.Now.Format("2006-01-02")
	}

	set := graph.Describe(b, today, req.IDKey)
	return GraphDescribeResult{Set: set, version: req.Version}, nil
}

// EncodeJSON writes the binder.report/v1 list_graphs envelope to w.
func (r GraphDescribeResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, listGraphsCommand, r.Set)
}

// --- graph query (query_graph) ----------------------------------------------

// GraphQueryOp is one of the five read-only query verbs.
type GraphQueryOp string

const (
	OpLookup       GraphQueryOp = "lookup"
	OpNeighbors    GraphQueryOp = "neighbors"
	OpNeighborhood GraphQueryOp = "neighborhood"
	OpPattern      GraphQueryOp = "pattern"
	OpPath         GraphQueryOp = "path"
)

// GraphQueryRequest carries the RESOLVED inputs a graph query needs. bundle and op
// are always required; the remaining fields are per-op (validated by the service, so
// every consumer — not just the MCP adapter — gets the same semantic checks). This
// operation is reachable only through MCP today (query_graph); the service makes it
// reachable through the public surface (design G4/AC4). id_key is echoed but does NOT
// re-key traversal identity in this version (§14.1).
type GraphQueryRequest struct {
	Bundle    string
	Op        GraphQueryOp
	Now       time.Time
	Today     string
	IDKey     string
	ID        string
	Label     string
	Direction string
	Rel       string
	Depth     int
	ToLabel   string
	Where     *graph.WhereClause
	From      string
	To        string
	MaxDepth  int
	Version   string
}

// GraphQueryResult carries the verb's typed payload and renders the query_graph
// envelope. The payload is one of graph's *Result types, kept as `any` so the same
// clijson encoder path serialises it (no second serialization path).
type GraphQueryResult struct {
	payload any
	version string
}

// GraphQuery validates the per-op params, loads the bundle, builds the deterministic
// graph.Model, indexes it, dispatches the verb, and returns the typed result. It owns
// the orchestration (bundle.Load → graph.Build → graph.NewIndex → verb) and the
// semantic validation that used to live in the MCP adapter, so an external caller
// gets both. A well-formed query that matches nothing is a result, not an error
// (never-reject); only malformed input is a usage error.
func (s *Service) GraphQuery(ctx context.Context, req GraphQueryRequest) (GraphQueryResult, error) {
	_ = ctx

	if err := validateGraphQuery(req); err != nil {
		return GraphQueryResult{}, err // usage-class typed error
	}

	b, err := bundle.Load(req.Bundle, s.codec)
	if err != nil {
		return GraphQueryResult{}, err
	}

	today := req.Today
	if today == "" {
		today = req.Now.Format("2006-01-02")
	}

	idx := graph.NewIndex(graph.Build(b, today))
	return GraphQueryResult{payload: dispatchGraphQuery(idx, req), version: req.Version}, nil
}

// EncodeJSON writes the binder.report/v1 query_graph envelope to w.
func (r GraphQueryResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, queryGraphCommand, r.payload)
}

// dispatchGraphQuery runs the validated verb against the index and returns its typed
// result payload. direction defaults to "out" when empty, consistent with the
// per-op defaults.
func dispatchGraphQuery(idx *graph.Index, req GraphQueryRequest) any {
	dir := directionOrDefault(req.Direction)
	switch req.Op {
	case OpLookup:
		return idx.Lookup(req.IDKey, req.ID, req.Label)
	case OpNeighbors:
		return idx.Neighbors(req.IDKey, req.ID, dir, req.Rel)
	case OpNeighborhood:
		return idx.Neighborhood(req.IDKey, req.ID, req.Depth, dir, req.Rel)
	case OpPattern:
		return idx.Pattern(req.IDKey, req.Label, req.ToLabel, req.Rel, req.Where)
	case OpPath:
		return idx.Path(req.IDKey, req.From, req.To, dir, req.MaxDepth)
	}
	return nil // unreachable: validateGraphQuery rejects unknown ops
}

// validateGraphQuery enforces the semantic constraints the transport cannot: a known
// op, the per-op required params, the depth range, a valid direction, a valid
// where.prop, and the lookup one-of. Each maps to a typed usage error (exit 2 in the
// CLI contract; an IsError tool result over MCP). A referenced node id that does not
// exist is NOT validated here — that is a finding produced by the verb (never-reject).
func validateGraphQuery(req GraphQueryRequest) error {
	switch req.Op {
	case OpLookup:
		if (req.ID == "") == (req.Label == "") {
			return usageErr("lookup requires exactly one of id or label")
		}
	case OpNeighbors:
		if req.ID == "" {
			return usageErr("neighbors requires id")
		}
		if err := validDirection(req.Direction); err != nil {
			return err
		}
	case OpNeighborhood:
		if req.ID == "" {
			return usageErr("neighborhood requires id")
		}
		if err := validDepth("depth", req.Depth); err != nil {
			return err
		}
		if err := validDirection(req.Direction); err != nil {
			return err
		}
	case OpPattern:
		if req.Label == "" {
			return usageErr("pattern requires label")
		}
		if req.ToLabel == "" && req.Where == nil {
			return usageErr("pattern requires at least one of to_label or where")
		}
		if req.Where != nil {
			if err := validProp(req.Where.Prop); err != nil {
				return err
			}
		}
	case OpPath:
		if req.From == "" || req.To == "" {
			return usageErr("path requires from and to")
		}
		if err := validDepth("max_depth", req.MaxDepth); err != nil {
			return err
		}
		if err := validDirection(req.Direction); err != nil {
			return err
		}
	case "":
		return usageErr("op is required (want lookup|neighbors|neighborhood|pattern|path)")
	default:
		return usageErr(fmt.Sprintf("unknown op %q (want lookup|neighbors|neighborhood|pattern|path)", req.Op))
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
	return usageErr(fmt.Sprintf("invalid direction %q (want out|in|both)", dir))
}

// validDepth enforces the mandatory hard bound 1..MaxDepth on a required depth param.
func validDepth(name string, v int) error {
	if v < 1 || v > graph.MaxDepth {
		return usageErr(fmt.Sprintf("%s must be in 1..%d", name, graph.MaxDepth))
	}
	return nil
}

// validProp enforces where.prop ∈ {type, tier, stale}.
func validProp(prop string) error {
	switch prop {
	case "type", "tier", "stale":
		return nil
	}
	return usageErr(fmt.Sprintf("invalid where.prop %q (want type|tier|stale)", prop))
}

// usageErr wraps msg as a clijson usage error (exit 2 in the CLI contract; an IsError
// tool result over MCP). The typed error travels from the core so every adapter maps
// it identically.
func usageErr(msg string) error {
	return clijson.Usage(errors.New(msg))
}
