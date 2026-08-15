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
		strict  bool
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
			// Validate --today up front as a usable date (exit 2). A malformed value
			// would otherwise be silently accepted by okf.IsStale's string compare
			// and misreport staleness. Uses the same YYYY-MM-DD parse the rest of the
			// code uses (okf.IsValidISODate); okf.IsStale is left untouched.
			if today != "" && !okf.IsValidISODate(today) {
				return clijson.Usage(fmt.Errorf("--today %q is not a valid date (expected YYYY-MM-DD)", today))
			}
			b, err := bundle.Load(args[0], codec)
			if err != nil {
				return err
			}
			if today == "" {
				today = resolveNow().Format("2006-01-02")
			}
			rep := review.Review(b, today)
			// review has no hard non-conformance; under --strict any review finding
			// (orphans, stale, unresolved/broken edges, unparsed-frontmatter
			// recoveries) gates at exit 1. Without --strict it never gates (exit 0).
			findings := len(rep.Orphans) + len(rep.Stale) + len(rep.Unresolved) + len(rep.UnparsedFrontmatter)
			gate := clijson.Gate(strict, false, findings > 0,
				fmt.Sprintf("review found %d gating finding(s) (--strict)", findings))

			if jsonOut {
				if err := clijson.Encode(cmd.OutOrStdout(), Version, "review", rep); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				return gate
			}
			fmt.Fprint(cmd.OutOrStdout(), rep.String())
			return gate
		},
	}
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the review report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) when any review finding is present (orphans, stale, unresolved, unparsed)")
	return cmd
}
