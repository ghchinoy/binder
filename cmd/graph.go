package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf"
)

func newGraphCmd(codec okf.Codec) *cobra.Command {
	var (
		format string
		today  string
		output string
	)
	cmd := &cobra.Command{
		Use:   "graph <bundle>",
		Short: "Export the bundle's concept graph (dot|json|graphml|html)",
		Long: "Graph exports the bundle's concept graph. Edges are exactly the bundle's\n" +
			"resolved links (spec §6), so the graph matches validate and review. Output\n" +
			"is deterministic.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
	return cmd
}
