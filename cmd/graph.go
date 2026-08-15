package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf"
)

func newGraphCmd(codec okf.Codec) *cobra.Command {
	var (
		format  string
		today   string
		output  string
		jsonOut bool
	)
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
			b, err := bundle.Load(args[0], codec)
			if err != nil {
				return err
			}
			if today == "" {
				today = resolveNow().Format("2006-01-02")
			}
			data, err := graph.Export(b, format, today)
			if err != nil {
				return err
			}
			if output != "" {
				if err := os.WriteFile(output, data, 0o644); err != nil {
					return fmt.Errorf("writing graph: %w", err)
				}
				return nil
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "dot", "output format: dot|json|graphml|html")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write graph to a file instead of stdout")
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "alias for --format json (the raw {nodes,edges} export, not the report envelope)")
	return cmd
}
