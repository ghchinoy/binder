package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/binder/render"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/okf"
)

func newConvertCmd(codec okf.Codec, cfg *config.Config) *cobra.Command {
	var (
		output           string
		defaultType      string
		typeMapRaw       string
		statusMapRaw     string
		staleAfterRaw    string
		verifiedBy       string
		fmRefKeysRaw     string
		dryRun           bool
		reportPath       string
		mapCitations     bool
		sourceKeys       string
		mapDraft         bool
		canonicalizeStat bool
		jsonOut          bool
		strict           bool
		workspaceRoot    string
		externalRoots    []string

		groupByType      bool
		includeBacklinks bool
		includeGraph     bool
	)
	// Construct the shared service ONCE with the composition root's codec (stateless,
	// safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)

	cmd := &cobra.Command{
		Use:   "convert <src>",
		Short: "Convert a markdown corpus into an OKF v0.2 bundle",
		Long: "Convert walks a plain-markdown corpus and writes a conformant OKF v0.2\n" +
			"bundle: one concept per non-reserved .md, standard markdown links rewritten\n" +
			"to bundle-relative form, a root index.md declaring okf_version, and a\n" +
			"generated provenance stamp. Output is\n" +
			"deterministic for identical inputs, with a single time-varying field: the\n" +
			"generated provenance timestamp (generated.at), which records when the run\n" +
			"actually happened. Pin SOURCE_DATE_EPOCH to make output byte-identical across runs.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" && !dryRun {
				return clijson.Usage(fmt.Errorf("--output/-o is required (or use --dry-run)"))
			}
			// --include-backlinks/--include-graph only annotate the --group-by-type
			// catalog; warn (stderr only) if passed without it. Never gates.
			hintCatalogFlags(cmd, groupByType, includeBacklinks, includeGraph)
			// --external-root declares KNOWN sibling workspace roots so their
			// file:// links stay external without advising (issue #25). An empty
			// value is a usage error (exit 2); a well-formed path that does not
			// exist is accepted on purpose — a declared sibling may be absent from
			// this checkout (e.g. in CI), and requiring existence would defeat the
			// flag. No stat is performed. This wording is CLI-specific, so it stays
			// at the adapter edge (the MCP tool phrases it as external_root).
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

			// Resolve+validate verified_by at the edge and classify its origin; the
			// never-fabricate-trust routing (Source vocabulary + refused-verifier Note)
			// lives in the service. An invalid actor is a usage error (exit 2) even when
			// it will not be honored; the config default was validated fail-fast at load.
			cfg.BindFlag(config.KeyVerifiedBy, cmd.Flags().Lookup("verified-by"))
			verifiedBy, trustOrigin, err := resolveVerifiedBy(cfg)
			if err != nil {
				return err
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the core
			// rule; a malformed epoch falls back to the wall clock, preserving the
			// historical silent-fallback contract.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			// One code path: the service parses the flag grammars, applies the
			// status-vocabulary pre-write gate, routes the trust decision, assembles
			// convert.Options, and returns a COMPLETE report (report.Verified.Note
			// included). The adapter only resolves inputs, renders, and gates.
			res, err := svc.Convert(cmd.Context(), binder.ConvertRequest{
				Src:                args[0],
				Out:                output,
				DryRun:             dryRun,
				DefaultType:        defaultType,
				TypeMapRaw:         typeMapRaw,
				StatusMapRaw:       statusMapRaw,
				StaleAfterMapRaw:   staleAfterRaw,
				FMRefKeysRaw:       fmRefKeysRaw,
				SourceKeysRaw:      sourceKeys,
				CanonicalizeStatus: canonicalizeStat,
				MapCitations:       mapCitations,
				MapDraft:           mapDraft,
				VerifiedBy:         verifiedBy,
				TrustOrigin:        trustOrigin,
				WorkspaceRoot:      workspaceRoot,
				ExternalRoots:      externalRoots,
				GroupByType:        groupByType,
				IncludeBacklinks:   includeBacklinks,
				IncludeGraph:       includeGraph,
				Version:            binder.Version,
				Now:                now,
				Strict:             strict,
			})
			if err != nil {
				return err
			}

			// --json and prose share the same report; --report writes whichever
			// format --json selects, so the file and stdout never disagree.
			out := render.Convert(res)
			if jsonOut {
				var buf bytes.Buffer
				if err := res.EncodeJSON(&buf); err != nil {
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

			// The gate decision (bare convert never gates; --strict gates on unresolved
			// links or recovery warnings) is the Result's, defined once in the service.
			return res.Gate(strict)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output bundle directory")
	cmd.Flags().StringVar(&defaultType, "default-type", "Note", "type applied when none is present or mapped")
	cmd.Flags().StringVar(&typeMapRaw, "type-map", "", "per-directory type overrides, e.g. \"docs=Guide,adr=Decision\"")
	cmd.Flags().StringVar(&statusMapRaw, "status-map", "", "per-directory status, e.g. \"archive=deprecated,drafts=draft,default=active\" (set only when status absent)")
	cmd.Flags().StringVar(&staleAfterRaw, "stale-after-map", "", "per-directory stale_after relative to now, e.g. \"07-benchmarks=+6m,legacy=+0d\" (grammar +Nd/+Nm/+Ny; set only when absent)")
	cmd.Flags().StringVar(&verifiedBy, "verified-by", "", config.VerifiedByFlagUsage())
	cmd.Flags().StringVar(&fmRefKeysRaw, "fm-ref-keys", "", "frontmatter keys treated as relationship edges, e.g. \"related,parent\"")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be written without writing anything")
	cmd.Flags().StringVar(&reportPath, "report", "", "also write the run report to this file")
	cmd.Flags().BoolVar(&mapCitations, "map-citations", false, "map a body \"# Citations\" list into sources entries")
	cmd.Flags().StringVar(&sourceKeys, "source-keys", "", "frontmatter keys to map into sources entries, e.g. \"source,author\"")
	cmd.Flags().BoolVar(&mapDraft, "map-draft", false, "map a draft:true marker to status:draft when status is absent")
	cmd.Flags().BoolVar(&canonicalizeStat, "canonicalize-status", false, "opt-in: rewrite known --status-map aliases to the OKF §5.4 vocabulary (active->stable, wip/in-progress->draft, archived/legacy->deprecated); off by default, each rewrite is reported")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the run report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) on unresolved links or recovery warnings; without it these never gate (never-reject)")
	cmd.Flags().StringVar(&workspaceRoot, "workspace-root", "", "boundary within which file:// links resolve to internal edges (default: the <src> root)")
	cmd.Flags().StringArrayVar(&externalRoots, "external-root", nil, "declare a KNOWN sibling-workspace root (repeatable); file:// links under it stay external but suppress the outside-root advisory")
	cmd.Flags().BoolVar(&groupByType, "group-by-type", false, "append an additive \"# Catalog\" of all concepts grouped by type to the root index.md")
	cmd.Flags().BoolVar(&includeBacklinks, "include-backlinks", false, "annotate catalog entries with inbound resolved edges (requires --group-by-type)")
	cmd.Flags().BoolVar(&includeGraph, "include-graph", false, "annotate catalog entries with outbound resolved edges (requires --group-by-type)")
	return cmd
}
