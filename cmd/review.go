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
		today       string
		jsonOut     bool
		strict      bool
		entrypoints []string
	)
	cmd := &cobra.Command{
		Use:   "review <bundle>",
		Short: "Summarize a bundle: concepts, unresolved links, orphans, trust tiers, stale",
		Long: "Review reports the bundle's concepts by type, derived trust tiers, stale\n" +
			"concepts, Attested Computations, entrypoints, orphans, and unresolved\n" +
			"links. A concept with no inbound links is an ENTRYPOINT when it links out\n" +
			"(or is a recognized root README.md/index.md, or is named via --entrypoint)\n" +
			"and a true ORPHAN only when it has no inbound AND no outbound links. Trust\n" +
			"tiers and staleness are derived on demand, never stored (spec §5.1/§5.3).",
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
			rep := review.Review(b, today, entrypoints)
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
	cmd.Flags().StringSliceVar(&entrypoints, "entrypoint", nil, "concept id or path to treat as an entrypoint, not an orphan (repeatable); root README.md and index.md are recognized automatically")
	return cmd
}
