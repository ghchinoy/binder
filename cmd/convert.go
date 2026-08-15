package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
)

func newConvertCmd(codec okf.Codec) *cobra.Command {
	var (
		output      string
		defaultType string
		typeMapRaw  string
		dryRun      bool
		reportPath  string
	)

	cmd := &cobra.Command{
		Use:   "convert <src>",
		Short: "Convert a markdown corpus into an OKF v0.2 bundle",
		Long: "Convert walks a plain-markdown corpus and writes a conformant OKF v0.2\n" +
			"bundle: one concept per non-reserved .md, standard markdown links rewritten\n" +
			"to bundle-relative form, a root index.md declaring okf_version, and a\n" +
			"generated provenance stamp. It never mutates the source and is deterministic.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" && !dryRun {
				return fmt.Errorf("--output/-o is required (or use --dry-run)")
			}
			typeMap, err := convert.ParseTypeMap(typeMapRaw)
			if err != nil {
				return err
			}

			opts := convert.Options{
				Codec:       codec,
				DefaultType: defaultType,
				TypeMap:     typeMap,
				Version:     Version,
				Now:         resolveNow(),
				DryRun:      dryRun,
			}
			report, err := convert.Convert(args[0], output, opts)
			if err != nil {
				return err
			}

			fmt.Fprint(cmd.OutOrStdout(), report.String())
			if reportPath != "" {
				if err := os.WriteFile(reportPath, []byte(report.String()), 0o644); err != nil {
					return fmt.Errorf("writing report: %w", err)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output bundle directory")
	cmd.Flags().StringVar(&defaultType, "default-type", "Note", "type applied when none is present or mapped")
	cmd.Flags().StringVar(&typeMapRaw, "type-map", "", "per-directory type overrides, e.g. \"docs=Guide,adr=Decision\"")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be written without writing anything")
	cmd.Flags().StringVar(&reportPath, "report", "", "also write the run report to this file")
	return cmd
}

// resolveNow honors SOURCE_DATE_EPOCH for reproducible builds, falling back to
// the wall clock. generated.at is stamped from this instant.
func resolveNow() time.Time {
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(secs, 0).UTC()
		}
	}
	return time.Now()
}
