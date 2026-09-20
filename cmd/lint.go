package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/binder/render"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
)

func newLintCmd(codec okf.Codec) *cobra.Command {
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
		Use:   "lint <corpus>",
		Short: "Check a source markdown corpus for broken links, missing titles, orphans, stale, schema issues",
		Long: "Lint performs a read-only pass over a SOURCE markdown corpus (it writes\n" +
			"nothing) and reports broken links (incl. #anchors), missing titles, orphan\n" +
			"concepts, entrypoints, stale concepts, and schema violations (missing\n" +
			"type:, invalid frontmatter). A concept with no inbound links is an\n" +
			"ENTRYPOINT when it links out (or is a recognized root README.md,\n" +
			"or is named via --entrypoint) and a true ORPHAN only when it has no inbound\n" +
			"AND no outbound links. Unlike `binder review`/`binder validate`, which read\n" +
			"an emitted bundle, lint sees the corpus as authored — a missing title or\n" +
			"type: is masked once convert defaults it.\n\n" +
			"It also reports unquoted colon-space scalars: any frontmatter key whose\n" +
			"unquoted plain-scalar value contains \": \" (e.g. title: Multi-View: Tabs),\n" +
			"which YAML reads as a nested mapping rather than the intended string.\n" +
			"A colon-tab counts the same, and a flow mapping is scanned too (one level\n" +
			"in when written on a single line). Quoting the value is the fix. This one is advisory-only and\n" +
			"never gates, even under --strict: it is derived only for a file whose\n" +
			"frontmatter did not parse, and names the key to quote in a file already\n" +
			"reported as an invalid-frontmatter schema violation.\n\n" +
			"Findings are advisory: bare lint always exits 0 (entrypoints never gate).\n" +
			"Use --strict to gate (exit 1) when any finding is present, e.g. in CI.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			// Validate --today up front as a usable date (exit 2). A malformed value
			// would otherwise be silently accepted by okf.IsStale's string compare
			// and misreport staleness. Uses the same YYYY-MM-DD parse the rest of the
			// code uses (okf.IsValidISODate); okf.IsStale is left untouched.
			if today != "" && !okf.IsValidISODate(today) {
				return clijson.Usage(fmt.Errorf("--today %q is not a valid date (expected YYYY-MM-DD)", today))
			}

			// A missing/non-directory corpus path is a usage error (exit 2), checked
			// up front so it is distinguishable from a mid-walk IO failure (exit 3).
			if info, err := os.Stat(src); err != nil || !info.IsDir() {
				return clijson.Usage(fmt.Errorf("corpus %q is not a readable directory", src))
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the
			// core rule; a malformed epoch falls back to the wall clock exactly as
			// cmd.resolveNow did, so the historical silent-fallback contract holds.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			// One code path: the service owns Analyze → Lint, the rep.Src fill, the
			// Today default, and the gating-finding definition. The adapter only
			// resolves inputs, renders, and maps the gate error to an exit code.
			res, err := svc.Lint(cmd.Context(), binder.LintRequest{
				Src:         src,
				Entrypoints: entrypoints,
				Now:         now,
				Today:       today,
				Version:     binder.Version,
			})
			if err != nil {
				// Path already validated above; any analysis failure here is
				// IO/internal (exit 3).
				return err
			}

			// Report is ALWAYS emitted before the gate signals, so the gate never
			// suppresses output (option (a), unified never-reject).
			if jsonOut {
				if err := res.EncodeJSON(cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), render.Lint(res))
			}

			// The gate decision (bare lint never gates; --strict gates on any gating
			// finding) is the Result's, defined once in the service.
			return res.Gate(strict)
		},
	}
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the lint report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) when any lint finding is present; without it lint never gates (never-reject)")
	cmd.Flags().StringSliceVar(&entrypoints, "entrypoint", nil, "concept id or path to treat as an entrypoint, not an orphan (repeatable); root README.md is recognized automatically")
	return cmd
}
