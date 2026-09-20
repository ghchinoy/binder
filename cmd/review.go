package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/binder/render"
	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/okf"
)

func newReviewCmd(codec okf.Codec) *cobra.Command {
	var (
		today       string
		jsonOut     bool
		strict      bool
		entrypoints []string
	)
	// Construct the shared service ONCE with the composition root's codec (it is
	// stateless and safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
	cmd := &cobra.Command{
		Use:   "review <bundle>",
		Short: "Summarize a bundle: concepts, unresolved links, orphans, trust tiers, stale",
		Long: "Review reports the bundle's concepts by type, derived trust tiers, stale\n" +
			"concepts, Attested Computations, entrypoints, orphans, and unresolved\n" +
			"links. A concept with no inbound links is an ENTRYPOINT when it links out\n" +
			"(or is a recognized root README.md, or is named via --entrypoint)\n" +
			"and a true ORPHAN only when it has no inbound AND no outbound links. Trust\n" +
			"tiers and staleness are derived on demand, never stored (spec §5.1/§5.3).",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --today up front as a usable date (exit 2). A malformed value
			// would otherwise be silently accepted by okfrules.IsStale's string compare
			// and misreport staleness. Uses the same YYYY-MM-DD parse the rest of the
			// code uses (okf.IsValidISODate); okfrules.IsStale is left untouched.
			if today != "" && !okf.IsValidISODate(today) {
				return clijson.Usage(fmt.Errorf("--today %q is not a valid date (expected YYYY-MM-DD)", today))
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the
			// core rule; a malformed epoch falls back to the wall clock exactly as
			// cmd.resolveNow did, so the historical silent-fallback contract holds.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			// One code path: the service owns Load → Review, the Today default, and
			// the gating-finding definition. The adapter only resolves inputs,
			// renders, and maps the gate error to an exit code.
			res, err := svc.Review(cmd.Context(), binder.ReviewRequest{
				Bundle:      args[0],
				Entrypoints: entrypoints,
				Now:         now,
				Today:       today,
				Version:     binder.Version,
			})
			if err != nil {
				return err
			}

			// Report is ALWAYS emitted before the gate signals, so the gate never
			// suppresses output.
			if jsonOut {
				if err := res.EncodeJSON(cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				return res.Gate(strict)
			}
			fmt.Fprint(cmd.OutOrStdout(), render.Review(res))
			return res.Gate(strict)
		},
	}
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the review report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) when any review finding is present (orphans, stale, unresolved, unparsed)")
	cmd.Flags().StringSliceVar(&entrypoints, "entrypoint", nil, "concept id or path to treat as an entrypoint, not an orphan (repeatable); root README.md is recognized automatically")
	return cmd
}
