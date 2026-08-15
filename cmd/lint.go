package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/lint"
	"github.com/ghchinoy/binder/internal/okf"
)

func newLintCmd(codec okf.Codec) *cobra.Command {
	var (
		today   string
		jsonOut bool
		strict  bool
	)
	cmd := &cobra.Command{
		Use:   "lint <corpus>",
		Short: "Check a source markdown corpus for broken links, missing titles, orphans, stale, schema issues",
		Long: "Lint performs a read-only pass over a SOURCE markdown corpus (it writes\n" +
			"nothing) and reports five health checks: broken links (incl. #anchors),\n" +
			"missing titles, orphan concepts, stale concepts, and schema violations\n" +
			"(missing type:, invalid frontmatter). Unlike `binder review`/`binder\n" +
			"validate`, which read an emitted bundle, lint sees the corpus as authored —\n" +
			"a missing title or type: is masked once convert defaults it.\n\n" +
			"Findings are advisory: bare lint always exits 0. Use --strict to gate\n" +
			"(exit 1) when any finding is present, e.g. in CI.",
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
			if today == "" {
				today = resolveNow().Format("2006-01-02")
			}

			// A missing/non-directory corpus path is a usage error (exit 2), checked
			// up front so it is distinguishable from a mid-walk IO failure (exit 3).
			if info, err := os.Stat(src); err != nil || !info.IsDir() {
				return clijson.Usage(fmt.Errorf("corpus %q is not a readable directory", src))
			}

			concepts, facts, _, err := convert.Analyze(src, convert.Options{
				Codec:   codec,
				Version: Version,
				Now:     resolveNow(),
			})
			if err != nil {
				// Path already validated above; any analysis failure here is
				// IO/internal (exit 3).
				return err
			}

			rep := lint.Lint(concepts, facts, today)
			rep.Src = src

			// Report is ALWAYS emitted before the gate signals, so the gate never
			// suppresses output (option (a), unified never-reject).
			if jsonOut {
				if err := clijson.Encode(cmd.OutOrStdout(), Version, "lint", rep); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), rep.String())
			}

			// lint produces only spec-tolerated advisories (invalid YAML is recovered
			// under never-reject, missing type is defaulted), so hardNonConformance is
			// always false — §11 hard conformance stays `binder validate`'s job. Bare
			// lint never gates (exit 0); --strict gates on any finding (exit 1).
			return clijson.Gate(strict, false, rep.NumFindings() > 0,
				fmt.Sprintf("lint found %d finding(s) (--strict)", rep.NumFindings()))
		},
	}
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the lint report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) when any lint finding is present; without it lint never gates (never-reject)")
	return cmd
}
