package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/okf"
)

func newIndexCmd(codec okf.Codec) *cobra.Command {
	var (
		dryRun           bool
		groupByType      bool
		includeBacklinks bool
		includeGraph     bool
	)
	// Construct the shared service ONCE with the composition root's codec (stateless,
	// safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
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

			// One code path: the service loads the bundle, collects the unparsed-file
			// advisories, and performs the writes (sorted order, write-vs-regenerate
			// classification, dry-run short-circuit). It returns the write manifest and
			// the advisories as DATA; the adapter renders the manifest to stdout and the
			// warnings to stderr. On a mid-run IO failure the service returns the partial
			// manifest (files already written) plus the error, so the adapter renders
			// what happened before surfacing the failure — matching the old loop.
			res, err := svc.Index(cmd.Context(), binder.IndexRequest{
				Root:             args[0],
				DryRun:           dryRun,
				GroupByType:      groupByType,
				IncludeBacklinks: includeBacklinks,
				IncludeGraph:     includeGraph,
			})

			// Disclose files the loader could not parse (#161/#163) on stderr so the
			// write manifest on stdout stays clean.
			for _, w := range res.Warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), w)
			}
			out := cmd.OutOrStdout()
			for _, e := range res.Entries {
				if res.DryRun {
					fmt.Fprintf(out, "would %s %s\n", e.Action, e.Rel)
				} else {
					fmt.Fprintf(out, "%s %s\n", e.Action, e.Rel)
				}
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report which index.md files would be written without writing")
	cmd.Flags().BoolVar(&groupByType, "group-by-type", false, "append an additive \"# Catalog\" of all concepts grouped by type to the root index.md")
	cmd.Flags().BoolVar(&includeBacklinks, "include-backlinks", false, "annotate catalog entries with inbound resolved edges (requires --group-by-type)")
	cmd.Flags().BoolVar(&includeGraph, "include-graph", false, "annotate catalog entries with outbound resolved edges (requires --group-by-type)")
	return cmd
}
