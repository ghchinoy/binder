package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
)

func newGraphCmd(codec okf.Codec) *cobra.Command {
	var (
		format  string
		today   string
		output  string
		jsonOut bool
	)
	// Construct the shared service ONCE with the composition root's codec (it is
	// stateless and safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
	cmd := &cobra.Command{
		Use:   "graph <bundle>",
		Short: "Export the bundle's concept graph (dot|json|graphml|html)",
		Long: "Graph exports the bundle's concept graph. Edges are exactly the bundle's\n" +
			"resolved links (spec §6), so the graph matches validate and review. Output\n" +
			"is deterministic.\n\n" +
			"graph is already machine-readable, so --json is an alias for --format json\n" +
			"(the raw {nodes,edges} export, NOT the report envelope used by the other\n" +
			"commands). Combining --json with a conflicting --format is a usage error.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// --json aliases --format json (§4.4). It composes with an explicit
			// --format json (redundant-but-fine) but conflicts with any other
			// format — a usage error (exit 2), not a silent override.
			if jsonOut {
				if cmd.Flags().Changed("format") && format != "json" {
					return clijson.Usage(fmt.Errorf("--json conflicts with --format %s; --json selects --format json", format))
				}
				format = "json"
			}
			// An invalid --format value is a usage error (exit 2), validated up
			// front so it fails the same way whether or not the bundle is
			// readable. graph.Export applies the identical normalization and treats
			// "" as "dot", so "" is permitted here too (exit 0); a genuine
			// IO/marshal failure from Export still surfaces as exit 3.
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "dot", "json", "graphml", "html":
			default:
				return clijson.Usage(fmt.Errorf("unknown graph format %q (want dot|json|graphml|html)", format))
			}
			// Validate --today up front as a usable date (exit 2). A malformed value
			// would otherwise be silently accepted by okf.IsStale's string compare
			// and misreport staleness. Uses the same YYYY-MM-DD parse the rest of the
			// code uses (okf.IsValidISODate); okf.IsStale is left untouched.
			if today != "" && !okf.IsValidISODate(today) {
				return clijson.Usage(fmt.Errorf("--today %q is not a valid date (expected YYYY-MM-DD)", today))
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the
			// core rule; a malformed epoch falls back to the wall clock.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			// One code path: the service owns Load → Export and the Today default.
			res, err := svc.GraphExport(cmd.Context(), binder.GraphExportRequest{
				Bundle: args[0],
				Format: format,
				Now:    now,
				Today:  today,
			})
			if err != nil {
				return err
			}
			// Disclose unparseable files on stderr (#161): the recovered node now
			// exists so edges no longer dangle, but the user must be told its
			// frontmatter did not parse. stderr keeps the exported graph clean.
			warnUnparsed(cmd.ErrOrStderr(), res.Bundle)

			if output != "" {
				if err := os.WriteFile(output, res.Data, 0o644); err != nil {
					return fmt.Errorf("writing graph: %w", err)
				}
				return nil
			}
			_, err = cmd.OutOrStdout().Write(res.Data)
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "dot", "output format: dot|json|graphml|html")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write graph to a file instead of stdout")
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "alias for --format json (the raw {nodes,edges} export, not the report envelope)")
	// Attach the read-only introspection subcommands (query, schema-describe) that
	// were MCP-only before Phase 2; they are thin adapters over the same service.
	cmd.AddCommand(newGraphQueryCmd(codec))
	cmd.AddCommand(newGraphSchemaCmd(codec))
	return cmd
}
