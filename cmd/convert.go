package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
)

func newConvertCmd(codec okf.Codec, cfg *config.Config) *cobra.Command {
	var (
		output        string
		defaultType   string
		typeMapRaw    string
		statusMapRaw  string
		staleAfterRaw string
		verifiedBy    string
		fmRefKeysRaw  string
		dryRun        bool
		reportPath    string
		mapCitations  bool
		sourceKeys    string
		mapDraft      bool
		jsonOut       bool
		strict        bool
		workspaceRoot string
		externalRoots []string

		groupByType      bool
		includeBacklinks bool
		includeGraph     bool
	)

	cmd := &cobra.Command{
		Use:   "convert <src>",
		Short: "Convert a markdown corpus into an OKF v0.2 bundle",
		Long: "Convert walks a plain-markdown corpus and writes a conformant OKF v0.2\n" +
			"bundle: one concept per non-reserved .md, standard markdown links rewritten\n" +
			"to bundle-relative form, a root index.md declaring okf_version, and a\n" +
			"generated provenance stamp. It never mutates the source and is deterministic.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" && !dryRun {
				return clijson.Usage(fmt.Errorf("--output/-o is required (or use --dry-run)"))
			}
			// --include-backlinks/--include-graph only annotate the --group-by-type
			// catalog; warn (stderr only) if passed without it. Never gates.
			hintCatalogFlags(cmd, groupByType, includeBacklinks, includeGraph)
			typeMap, err := convert.ParseTypeMap(typeMapRaw)
			if err != nil {
				return err
			}
			// Malformed map shapes/values are usage errors (exit 2).
			statusMap, statusDefault, err := convert.ParseStatusMap(statusMapRaw)
			if err != nil {
				return clijson.Usage(err)
			}
			staleAfterMap, err := convert.ParseStaleAfterMap(staleAfterRaw)
			if err != nil {
				return clijson.Usage(err)
			}
			// --external-root declares KNOWN sibling workspace roots so their
			// file:// links stay external without advising (issue #25). An empty
			// value is a usage error (exit 2); a well-formed path that does not
			// exist is accepted on purpose — a declared sibling may be absent from
			// this checkout (e.g. in CI), and requiring existence would defeat the
			// flag. No stat is performed.
			for _, er := range externalRoots {
				if strings.TrimSpace(er) == "" {
					return clijson.Usage(fmt.Errorf("--external-root value must not be empty"))
				}
			}

			// Resolve default_type through config precedence (flag > env > file >
			// default). Binding the flag lets an explicit --default-type win over a
			// configured value while a configured value still overrides the built-in.
			cfg.BindFlag(config.KeyDefaultType, cmd.Flags().Lookup("default-type"))
			defaultType = cfg.GetString(config.KeyDefaultType)

			// Resolve verified_by (flag > config default). Validate the effective
			// actor with okf.IsValidActor (option (a)): an invalid value is a usage
			// error (exit 2) listing the valid forms. The config default was already
			// validated fail-fast at config-load; this catches a bad flag value.
			cfg.BindFlag(config.KeyVerifiedBy, cmd.Flags().Lookup("verified-by"))
			verifiedBy = cfg.GetString(config.KeyVerifiedBy)
			if verifiedBy != "" && !okf.IsValidActor(verifiedBy) {
				return config.InvalidActorError(verifiedBy)
			}

			opts := convert.Options{
				Codec:         codec,
				DefaultType:   defaultType,
				TypeMap:       typeMap,
				StatusMap:     statusMap,
				StatusDefault: statusDefault,
				StaleAfterMap: staleAfterMap,
				VerifiedBy:    verifiedBy,
				FMRefKeys:     convert.ParseFMRefKeys(fmRefKeysRaw),
				Version:       Version,
				Now:           resolveNow(),
				DryRun:        dryRun,
				MapCitations:  mapCitations,
				SourceKeys:    convert.ParseFMRefKeys(sourceKeys),
				MapDraft:      mapDraft,
				WorkspaceRoot: workspaceRoot,
				ExternalRoots: externalRoots,

				GroupByType:      groupByType,
				IncludeBacklinks: includeBacklinks,
				IncludeGraph:     includeGraph,
			}
			report, err := convert.Convert(args[0], output, opts)
			if err != nil {
				return err
			}

			// --json and prose share the same report; --report writes whichever
			// format --json selects, so the file and stdout never disagree.
			out := report.String()
			if jsonOut {
				var buf bytes.Buffer
				if err := clijson.Encode(&buf, Version, "convert", report); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				out = buf.String()
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			if reportPath != "" {
				if err := os.WriteFile(reportPath, []byte(out), 0o644); err != nil {
					return fmt.Errorf("writing report: %w", err)
				}
			}

			// convert has no hard non-conformance; under --strict unresolved links
			// and recovery warnings (unparseable frontmatter preserved as body) gate
			// at exit 1. Without --strict it never gates (never-reject; exit 0). The
			// report is already emitted, so the signal never suppresses output.
			gatingPresent := report.NumUnresolved > 0 || report.NumRecovered > 0
			return clijson.Gate(strict, false, gatingPresent,
				fmt.Sprintf("convert produced %d unresolved link(s) and %d recovery warning(s) (--strict)",
					report.NumUnresolved, report.NumRecovered))
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output bundle directory")
	cmd.Flags().StringVar(&defaultType, "default-type", "Note", "type applied when none is present or mapped")
	cmd.Flags().StringVar(&typeMapRaw, "type-map", "", "per-directory type overrides, e.g. \"docs=Guide,adr=Decision\"")
	cmd.Flags().StringVar(&statusMapRaw, "status-map", "", "per-directory status, e.g. \"archive=deprecated,drafts=draft,default=active\" (set only when status absent)")
	cmd.Flags().StringVar(&staleAfterRaw, "stale-after-map", "", "per-directory stale_after relative to now, e.g. \"07-benchmarks=+6m,legacy=+0d\" (grammar +Nd/+Nm/+Ny; set only when absent)")
	cmd.Flags().StringVar(&verifiedBy, "verified-by", "", "actor to append as a verified stamp, e.g. \"human:ghchinoy\" or \"binder/0.1.0\" (defaults to config verified_by; "+config.ActorFormsHint+")")
	cmd.Flags().StringVar(&fmRefKeysRaw, "fm-ref-keys", "", "frontmatter keys treated as relationship edges, e.g. \"related,parent\"")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be written without writing anything")
	cmd.Flags().StringVar(&reportPath, "report", "", "also write the run report to this file")
	cmd.Flags().BoolVar(&mapCitations, "map-citations", false, "map a body \"# Citations\" list into sources entries")
	cmd.Flags().StringVar(&sourceKeys, "source-keys", "", "frontmatter keys to map into sources entries, e.g. \"source,author\"")
	cmd.Flags().BoolVar(&mapDraft, "map-draft", false, "map a draft:true marker to status:draft when status is absent")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the run report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) on unresolved links or recovery warnings; without it these never gate (never-reject)")
	cmd.Flags().StringVar(&workspaceRoot, "workspace-root", "", "boundary within which file:// links resolve to internal edges (default: the <src> root)")
	cmd.Flags().StringArrayVar(&externalRoots, "external-root", nil, "declare a KNOWN sibling-workspace root (repeatable); file:// links under it stay external but suppress the outside-root advisory")
	cmd.Flags().BoolVar(&groupByType, "group-by-type", false, "append an additive \"# Catalog\" of all concepts grouped by type to the root index.md")
	cmd.Flags().BoolVar(&includeBacklinks, "include-backlinks", false, "annotate catalog entries with inbound resolved edges (requires --group-by-type)")
	cmd.Flags().BoolVar(&includeGraph, "include-graph", false, "annotate catalog entries with outbound resolved edges (requires --group-by-type)")
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
