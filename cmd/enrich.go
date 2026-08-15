package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/enrich"
	"github.com/ghchinoy/binder/internal/okf"
)

func newEnrichCmd(codec okf.Codec, cfg *config.Config) *cobra.Command {
	var (
		defaultType string
		typeMapRaw  string
		dryRun      bool
		jsonOut     bool
		strict      bool
	)

	cmd := &cobra.Command{
		Use:   "enrich <src>",
		Short: "Inject missing OKF frontmatter into a source markdown tree, in place",
		Long: "Enrich adds the missing required OKF frontmatter (type, title, generated)\n" +
			"to the markdown files under <src>, IN PLACE. It touches FRONTMATTER ONLY:\n" +
			"unlike `binder convert`, it does no link rewriting, no index generation, no\n" +
			"\"## Related\" section, and no tag merge — bodies are otherwise untouched.\n\n" +
			"It is safe on a git-tracked tree: additive/never-clobber (only ABSENT keys\n" +
			"are added), idempotent (a second run writes nothing), byte-faithful (body and\n" +
			"pre-existing keys are preserved exactly), and atomic (temp file + rename, so\n" +
			"an interrupt never corrupts a source file). Files needing no key are not\n" +
			"written at all. Files whose frontmatter will not parse, and reserved files\n" +
			"(index.md/log.md), are skipped and never mutated.\n\n" +
			"Use --dry-run to preview. Skipped files are advisory: bare enrich exits 0;\n" +
			"--strict gates (exit 1) when any file is skipped.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]

			// A missing/non-directory source path is a usage error (exit 2),
			// checked up front so it is distinguishable from a mid-walk IO
			// failure (exit 3).
			if info, err := os.Stat(src); err != nil || !info.IsDir() {
				return clijson.Usage(fmt.Errorf("source %q is not a readable directory", src))
			}

			typeMap, err := convert.ParseTypeMap(typeMapRaw)
			if err != nil {
				return clijson.Usage(err)
			}

			// Resolve default_type through config precedence (flag > env > file >
			// default), mirroring convert (#10).
			cfg.BindFlag(config.KeyDefaultType, cmd.Flags().Lookup("default-type"))
			defaultType = cfg.GetString(config.KeyDefaultType)

			opts := enrich.Options{
				Codec:       codec,
				DefaultType: defaultType,
				TypeMap:     typeMap,
				Version:     Version,
				Now:         resolveNow(),
				DryRun:      dryRun,
			}
			rep, err := enrich.Enrich(src, opts)
			if err != nil {
				// Path already validated above; any failure here is IO/internal (exit 3).
				return err
			}

			// The report is ALWAYS emitted before the gate signals, so the gate
			// never suppresses output.
			if jsonOut {
				if err := clijson.Encode(cmd.OutOrStdout(), Version, "enrich", rep); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), rep.String())
			}

			// enrich produces one advisory: skipped files (unparseable frontmatter).
			// Bare enrich never gates (exit 0); --strict gates on any skip (exit 1).
			return clijson.Gate(strict, false, rep.NumFindings() > 0,
				fmt.Sprintf("enrich skipped %d file(s) (--strict)", rep.NumFindings()))
		},
	}

	cmd.Flags().StringVar(&defaultType, "default-type", "Note", "type applied when none is present or mapped")
	cmd.Flags().StringVar(&typeMapRaw, "type-map", "", "per-directory type overrides, e.g. \"docs=Guide,adr=Decision\"")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be enriched without writing anything")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the run report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) when any file is skipped; without it enrich never gates (never-reject)")
	return cmd
}
