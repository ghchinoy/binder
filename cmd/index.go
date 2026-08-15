package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
)

func newIndexCmd(codec okf.Codec) *cobra.Command {
	var (
		dryRun           bool
		groupByType      bool
		includeBacklinks bool
		includeGraph     bool
	)
	cmd := &cobra.Command{
		Use:   "index <bundle>",
		Short: "(Re)generate the per-directory index.md nav tree (spec §8)",
		Long: "Index regenerates each directory's index.md as a navigation tree listing\n" +
			"that directory's concepts and immediate subdirectories (spec §8). The\n" +
			"bundle-root index.md declares okf_version (spec §12). log.md files are\n" +
			"never touched. Existing index.md files are regenerated; each write is\n" +
			"reported so nothing is overwritten silently.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// --include-backlinks/--include-graph only annotate the --group-by-type
			// catalog; warn (stderr only) if passed without it. Never gates.
			hintCatalogFlags(cmd, groupByType, includeBacklinks, includeGraph)
			root := args[0]
			b, err := bundle.Load(root, codec)
			if err != nil {
				return err
			}
			indexes := convert.GenerateIndexes(b.Concepts, b.OKFVersion, convert.IndexOptions{
				GroupByType:      groupByType,
				IncludeBacklinks: includeBacklinks,
				IncludeGraph:     includeGraph,
			})

			rels := make([]string, 0, len(indexes))
			for rel := range indexes {
				rels = append(rels, rel)
			}
			sort.Strings(rels)

			out := cmd.OutOrStdout()
			for _, rel := range rels {
				dst := filepath.Join(root, filepath.FromSlash(rel))
				action := "write"
				if _, err := os.Stat(dst); err == nil {
					action = "regenerate"
				}
				if dryRun {
					fmt.Fprintf(out, "would %s %s\n", action, rel)
					continue
				}
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dst, indexes[rel], 0o644); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s %s\n", action, rel)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report which index.md files would be written without writing")
	cmd.Flags().BoolVar(&groupByType, "group-by-type", false, "append an additive \"# Catalog\" of all concepts grouped by type to the root index.md")
	cmd.Flags().BoolVar(&includeBacklinks, "include-backlinks", false, "annotate catalog entries with inbound resolved edges (requires --group-by-type)")
	cmd.Flags().BoolVar(&includeGraph, "include-graph", false, "annotate catalog entries with outbound resolved edges (requires --group-by-type)")
	return cmd
}
