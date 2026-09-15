package cmd

import (
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

func newEnrichCmd(codec okf.Codec, cfg *config.Config) *cobra.Command {
	var (
		defaultType      string
		typeMapRaw       string
		statusMapRaw     string
		staleAfterRaw    string
		verifiedBy       string
		overwriteRaw     string
		canonicalizeStat bool
		dryRun           bool
		jsonOut          bool
		strict           bool
	)
	// Construct the shared service ONCE with the composition root's codec (stateless,
	// safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)

	cmd := &cobra.Command{
		Use:   "enrich <src>",
		Short: "Inject missing OKF frontmatter into a source markdown tree, in place",
		Long: "Enrich adds the missing required OKF frontmatter (type, title, generated)\n" +
			"to the markdown files under <src>, IN PLACE. It touches FRONTMATTER ONLY:\n" +
			"unlike `binder convert`, it does no link rewriting, no index generation, no\n" +
			"\"## Related\" section, and no tag merge — bodies are otherwise untouched.\n\n" +
			"It operates on the YAML only, so its writes stay reviewable on a\n" +
			"git-tracked tree: additive/never-clobber (it adds only ABSENT keys and\n" +
			"never overwrites an existing value; the sole exception is an authorized\n" +
			"`verified` stamp, which is APPENDED to any existing `verified` list, never\n" +
			"replacing a prior attestation), idempotent unless a `verified` stamp advances\n" +
			"(a rerun writes nothing when no verifier is set or the clock is pinned via\n" +
			"SOURCE_DATE_EPOCH; with a live verifier under a moving clock a rerun appends a\n" +
			"fresh stamp, since stamps dedup on (by, at)), and atomic (temp file + rename, so\n" +
			"an interrupted run leaves the source as it was rather than half-written).\n" +
			"Files needing no key are not written at all. Files whose frontmatter will not\n" +
			"parse, and reserved files (index.md/log.md), are skipped and never mutated.\n\n" +
			"Additive/never-clobber is the DEFAULT. --overwrite-keys <k1,k2,...> is an\n" +
			"opt-in exception that REFRESHES only the named keys in place even when they\n" +
			"already exist (e.g. --overwrite-keys status,stale_after after a new\n" +
			"benchmark release). Every other pre-existing key, custom frontmatter, and\n" +
			"key order are left in place; it respects --dry-run, the\n" +
			"atomic write, and skip-unchanged. Trust/attestation keys (verified,\n" +
			"verified_by, sources, generated, and the other provenance keys) are REFUSED\n" +
			"(exit 2) — overwriting them could destroy a human attestation.\n\n" +
			"Use --dry-run to preview. Skipped files, preserve-or-advise warnings, and a\n" +
			"non-conformant --status-map OKF §5.4 value are advisory: bare enrich exits 0;\n" +
			"--strict gates (exit 1) on them — the status-map value gates BEFORE anything is\n" +
			"written. The read-boundary normalization advisory (a stripped UTF-8 BOM or a\n" +
			"translated lone CR) is always reported and never gates.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]

			// A missing/non-directory source path is a usage error (exit 2),
			// checked up front so it is distinguishable from a mid-walk IO
			// failure (exit 3). This CLI-specific wording stays at the adapter edge.
			if info, err := os.Stat(src); err != nil || !info.IsDir() {
				return clijson.Usage(fmt.Errorf("source %q is not a readable directory", src))
			}

			// Resolve default_type through config precedence (flag > env > file >
			// default), mirroring convert (#10).
			cfg.BindFlag(config.KeyDefaultType, cmd.Flags().Lookup("default-type"))
			defaultType = cfg.GetString(config.KeyDefaultType)

			// Resolve+validate verified_by at the edge and classify its origin; the
			// never-fabricate-trust routing lives in the service. An invalid actor is a
			// usage error (exit 2) even when it will not be honored.
			cfg.BindFlag(config.KeyVerifiedBy, cmd.Flags().Lookup("verified-by"))
			verifiedBy, trustOrigin, err := resolveVerifiedBy(cfg)
			if err != nil {
				return err
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the core
			// rule; a malformed epoch falls back to the wall clock.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			// One code path: the service parses the flag grammars, applies the
			// status-vocabulary pre-write gate, routes the trust decision, assembles
			// enrich.Options, and returns a COMPLETE report (report.Verified.Note
			// included). The adapter only resolves inputs, renders, and gates.
			res, err := svc.Enrich(cmd.Context(), binder.EnrichRequest{
				Src:                src,
				DefaultType:        defaultType,
				TypeMapRaw:         typeMapRaw,
				StatusMapRaw:       statusMapRaw,
				StaleAfterMapRaw:   staleAfterRaw,
				OverwriteKeysRaw:   overwriteRaw,
				CanonicalizeStatus: canonicalizeStat,
				VerifiedBy:         verifiedBy,
				TrustOrigin:        trustOrigin,
				Version:            Version,
				Now:                now,
				DryRun:             dryRun,
				Strict:             strict,
			})
			if err != nil {
				// Path already validated above; a service failure is a parse usage error
				// (exit 2), the status-vocab pre-write gate (exit 1), or IO/internal (exit 3).
				return err
			}

			// The report is ALWAYS emitted before the gate signals, so the gate
			// never suppresses output.
			if jsonOut {
				if err := res.EncodeJSON(cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), render.Enrich(res))
			}

			// The gate decision (bare enrich never gates; --strict gates on the counted
			// findings — skipped files and preserve-or-advise warnings, NARROWER than
			// the user guide's "gating findings" which also covers the pre-write
			// status-vocab gate) is the Result's, defined once in the service.
			return res.Gate(strict)
		},
	}

	cmd.Flags().StringVar(&defaultType, "default-type", "Note", "type applied when none is present or mapped")
	cmd.Flags().StringVar(&typeMapRaw, "type-map", "", "per-directory type overrides, e.g. \"docs=Guide,adr=Decision\"")
	cmd.Flags().StringVar(&statusMapRaw, "status-map", "", "per-directory status, e.g. \"archive=deprecated,drafts=draft,default=active\" (set only when status absent)")
	cmd.Flags().StringVar(&staleAfterRaw, "stale-after-map", "", "per-directory stale_after relative to now, e.g. \"07-benchmarks=+6m,legacy=+0d\" (grammar +Nd/+Nm/+Ny; set only when absent)")
	cmd.Flags().StringVar(&verifiedBy, "verified-by", "", config.VerifiedByFlagUsage())
	cmd.Flags().StringVar(&overwriteRaw, "overwrite-keys", "", "opt-in: comma-separated keys to REFRESH in place even when present, e.g. \"status,stale_after\" (default is additive/never-clobber; trust keys "+strings.Join(okf.ProtectedTrustKeys(), ", ")+" are refused)")
	cmd.Flags().BoolVar(&canonicalizeStat, "canonicalize-status", false, "opt-in: rewrite known --status-map aliases to the OKF §5.4 vocabulary (active->stable, wip/in-progress->draft, archived/legacy->deprecated); off by default, each rewrite is reported")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be enriched without writing anything")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the run report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) on any of enrich's gating conditions, including a skipped file, a preserve-or-advise warning, and a non-conformant --status-map OKF §5.4 value; the read-boundary normalization advisory is reported but never gates; without it enrich never gates (never-reject)")
	return cmd
}
