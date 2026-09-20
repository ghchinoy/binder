package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/graph"
	"github.com/ghchinoy/binder/pkg/okf"
)

// newGraphQueryCmd is the thin CLI adapter for the read-only graph query operation
// that was MCP-only (query_graph) before Phase 2. It maps flags onto a
// binder.GraphQueryRequest and drives the SAME binder.Service.GraphQuery the MCP tool
// uses; it carries NO capability logic (the service owns the semantic validation, the
// orchestration bundle.Load → graph.Build → graph.NewIndex → verb, and the envelope).
// It always emits the binder.report/v1 query_graph envelope — the payload is already
// machine-readable, so there is no prose form. A query that matches nothing is a
// result, not an error; malformed input is a usage error (exit 2).
func newGraphQueryCmd(codec okf.Codec) *cobra.Command {
	var (
		op        string
		id        string
		label     string
		direction string
		rel       string
		depth     int
		toLabel   string
		whereProp string
		whereEq   string
		from      string
		to        string
		maxDepth  int
		idKey     string
		today     string
	)
	// Construct the shared service ONCE with the composition root's codec (it is
	// stateless and safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
	cmd := &cobra.Command{
		Use:   "query <bundle> --op <verb>",
		Short: "Query the bundle's concept graph (lookup|neighbors|neighborhood|pattern|path)",
		Long: "Query runs one of five read-only verbs over the property graph binder\n" +
			"projects from an OKF bundle — the same graph as `binder graph`, `list_graphs`,\n" +
			"and `project`, so it stays in edge/identity parity by construction.\n\n" +
			"  --op lookup        list a concept by --id or all of a --label\n" +
			"  --op neighbors     one-hop from --id (--direction out|in|both, optional --rel)\n" +
			"  --op neighborhood  bounded k-hop BFS from --id (--depth 1..5)\n" +
			"  --op pattern       source nodes of --label linking to --to-label and/or a\n" +
			"                     --where-prop/--where-eq predicate (type|tier|stale)\n" +
			"  --op path          bounded shortest hop-path from --from to --to (--max-depth 1..5)\n\n" +
			"Every traversal is bounded; a query that matches nothing is a result, not an\n" +
			"error. Output is the binder.report/v1 query_graph payload. --id-key is accepted\n" +
			"for parity with schema-describe but does NOT re-key traversal identity in this\n" +
			"version (identity is always the path-derived concept id).",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --today up front as a usable date (exit 2), matching the other
			// commands; okfrules.IsStale is left untouched. All other semantic validation
			// (op, per-op required params, ranges) belongs to the service.
			if today != "" && !okf.IsValidISODate(today) {
				return clijson.Usage(fmt.Errorf("--today %q is not a valid date (expected YYYY-MM-DD)", today))
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the
			// core rule; a malformed epoch falls back to the wall clock.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			// Build the where clause only when a predicate flag is supplied, so the
			// service sees nil (no predicate) exactly as the MCP adapter passes it.
			var where *graph.WhereClause
			if cmd.Flags().Changed("where-prop") || cmd.Flags().Changed("where-eq") {
				where = &graph.WhereClause{Prop: whereProp, Eq: whereEq}
			}

			res, err := svc.GraphQuery(cmd.Context(), binder.GraphQueryRequest{
				Bundle:    args[0],
				Op:        binder.GraphQueryOp(op),
				Now:       now,
				Today:     today,
				IDKey:     idKey,
				ID:        id,
				Label:     label,
				Direction: direction,
				Rel:       rel,
				Depth:     depth,
				ToLabel:   toLabel,
				Where:     where,
				From:      from,
				To:        to,
				MaxDepth:  maxDepth,
				Version:   binder.Version,
			})
			if err != nil {
				return err
			}
			if err := res.EncodeJSON(cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("encoding json report: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&op, "op", "", "the query verb: lookup|neighbors|neighborhood|pattern|path (required)")
	cmd.Flags().StringVar(&id, "id", "", "lookup/neighbors/neighborhood: the subject node id (path-derived concept id)")
	cmd.Flags().StringVar(&label, "label", "", "lookup: the concept type to list; pattern: the source concept type (required for pattern)")
	cmd.Flags().StringVar(&direction, "direction", "", "neighbors/neighborhood/path: out|in|both (default out)")
	cmd.Flags().StringVar(&rel, "rel", "", "neighbors/neighborhood/pattern: optional exact-match filter on the edge relationship text")
	cmd.Flags().IntVar(&depth, "depth", 0, "neighborhood: BFS depth, required, 1..5")
	cmd.Flags().StringVar(&toLabel, "to-label", "", "pattern: optional target concept type")
	cmd.Flags().StringVar(&whereProp, "where-prop", "", "pattern: property predicate target: type|tier|stale")
	cmd.Flags().StringVar(&whereEq, "where-eq", "", "pattern: the exact value the property must equal")
	cmd.Flags().StringVar(&from, "from", "", "path: the source node id (required for path)")
	cmd.Flags().StringVar(&to, "to", "", "path: the target node id (required for path)")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 0, "path: maximum hop depth, required, 1..5")
	cmd.Flags().StringVar(&idKey, "id-key", "", "accepted for parity with schema-describe; does NOT re-key traversal identity in this version")
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	return cmd
}
