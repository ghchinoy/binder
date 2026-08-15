package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/review"
)

func newReviewCmd(codec okf.Codec) *cobra.Command {
	var (
		today   string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "review <bundle>",
		Short: "Summarize a bundle: concepts, unresolved links, orphans, trust tiers, stale",
		Long: "Review reports the bundle's concepts by type, derived trust tiers, stale\n" +
			"concepts, Attested Computations, orphans (concepts nothing links to), and\n" +
			"unresolved links. Trust tiers and staleness are derived on demand, never\n" +
			"stored (spec §5.1/§5.3).",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bundle.Load(args[0], codec)
			if err != nil {
				return err
			}
			if today == "" {
				today = resolveNow().Format("2006-01-02")
			}
			rep := review.Review(b, today)
			if jsonOut {
				if err := clijson.Encode(cmd.OutOrStdout(), Version, "review", rep); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), rep.String())
			return nil
		},
	}
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the review report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	return cmd
}
