package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/enrich"
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
			// failure (exit 3).
			if info, err := os.Stat(src); err != nil || !info.IsDir() {
				return clijson.Usage(fmt.Errorf("source %q is not a readable directory", src))
			}

			typeMap, err := convert.ParseTypeMap(typeMapRaw)
			if err != nil {
				return clijson.Usage(err)
			}
			// Malformed map shapes/values are usage errors (exit 2). Non-conformant
			// §5.4 status values warn on the default path and gate under --strict,
			// BEFORE any file is written (issue #23); --canonicalize-status opts into
			// the fixed alias rewrite.
			statusMap, statusDefault, statusNotes, err := resolveStatusMap(statusMapRaw, canonicalizeStat, strict)
			if err != nil {
				return err
			}
			staleAfterMap, err := convert.ParseStaleAfterMap(staleAfterRaw)
			if err != nil {
				return clijson.Usage(err)
			}
			// --overwrite-keys is the opt-in, scoped exception to additive-only
			// (issue #22). A malformed list, or naming a trust/attestation-carrying
			// key, is a usage error (exit 2) that names the offending key and
			// modifies no file.
			overwriteKeys, err := enrich.ParseOverwriteKeys(overwriteRaw)
			if err != nil {
				return clijson.Usage(err)
			}

			// Resolve default_type through config precedence (flag > env > file >
			// default), mirroring convert (#10).
			cfg.BindFlag(config.KeyDefaultType, cmd.Flags().Lookup("default-type"))
			defaultType = cfg.GetString(config.KeyDefaultType)

			// Resolve verified_by under the never-fabricate-trust ruling, mirroring
			// convert exactly: explicit --verified-by always stamps; otherwise a stamp
			// is written only when the resolved origin satisfies the user-set exception
			// (config.PermitsStampWithoutFlag). An invalid actor is a usage error (exit 2).
			cfg.BindFlag(config.KeyVerifiedBy, cmd.Flags().Lookup("verified-by"))
			vb, err := resolveVerifiedBy(cfg)
			if err != nil {
				return err
			}
			verifiedBy = vb.Actor

			opts := enrich.Options{
				Codec:              codec,
				DefaultType:        defaultType,
				TypeMap:            typeMap,
				StatusMap:          statusMap,
				StatusDefault:      statusDefault,
				StatusNotes:        statusNotes,
				StaleAfterMap:      staleAfterMap,
				VerifiedBy:         verifiedBy,
				VerifiedByExplicit: vb.Explicit,
				VerifiedBySource:   vb.Source,
				OverwriteKeys:      overwriteKeys,
				Version:            Version,
				Now:                resolveNow(),
				DryRun:             dryRun,
			}
			rep, err := enrich.Enrich(src, opts)
			if err != nil {
				// Path already validated above; any failure here is IO/internal (exit 3).
				return err
			}
			// Disclose a resolved-but-unhonored verifier (a BINDER_VERIFIED_BY env
			// default or a repo-local config, neither of which authorizes stamping).
			rep.Verified.Note = vb.Note

			// The report is ALWAYS emitted before the gate signals, so the gate
			// never suppresses output.
			if jsonOut {
				if err := clijson.Encode(cmd.OutOrStdout(), Version, "enrich", rep); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), rep.String())
			}

			// This is the post-run gate, and it covers only what NumFindings
			// counts: skipped files (unparseable frontmatter) and preserve-or-advise
			// warnings. It is NARROWER than "gating findings" in the user guide,
			// which also covers the non-conformant --status-map OKF §5.4 value gated
			// pre-write in resolveStatusMap (cmd/statusmap.go) and never counted
			// here. Bare enrich never gates (exit 0); --strict gates on the counted
			// findings (exit 1). The boundary-normalization advisory (#124) is
			// reported but excluded from NumFindings, so it never gates. The message
			// names each real quantity separately: printing the findings total as
			// "skipped N file(s)" claimed skips that had not happened (issue #154).
			return clijson.Gate(strict, false, rep.NumFindings() > 0,
				fmt.Sprintf("enrich skipped %d file(s) and raised %d warning(s) (--strict)",
					rep.NumSkipped, len(rep.Warnings)))
		},
	}

	cmd.Flags().StringVar(&defaultType, "default-type", "Note", "type applied when none is present or mapped")
	cmd.Flags().StringVar(&typeMapRaw, "type-map", "", "per-directory type overrides, e.g. \"docs=Guide,adr=Decision\"")
	cmd.Flags().StringVar(&statusMapRaw, "status-map", "", "per-directory status, e.g. \"archive=deprecated,drafts=draft,default=active\" (set only when status absent)")
	cmd.Flags().StringVar(&staleAfterRaw, "stale-after-map", "", "per-directory stale_after relative to now, e.g. \"07-benchmarks=+6m,legacy=+0d\" (grammar +Nd/+Nm/+Ny; set only when absent)")
	cmd.Flags().StringVar(&verifiedBy, "verified-by", "", "actor to append as a verified stamp, e.g. \"human:ghchinoy\" or \"binder/0.3.0\"; a stamp is written ONLY when passed here, or when verified_by is set in your GLOBAL config (neither BINDER_VERIFIED_BY nor a repo-local .binder.yaml authorizes stamping; "+config.ActorFormsHint+")")
	cmd.Flags().StringVar(&overwriteRaw, "overwrite-keys", "", "opt-in: comma-separated keys to REFRESH in place even when present, e.g. \"status,stale_after\" (default is additive/never-clobber; trust keys "+strings.Join(okf.ProtectedTrustKeys(), ", ")+" are refused)")
	cmd.Flags().BoolVar(&canonicalizeStat, "canonicalize-status", false, "opt-in: rewrite known --status-map aliases to the OKF §5.4 vocabulary (active->stable, wip/in-progress->draft, archived/legacy->deprecated); off by default, each rewrite is reported")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would be enriched without writing anything")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the run report as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) on any of enrich's gating conditions, including a skipped file, a preserve-or-advise warning, and a non-conformant --status-map OKF §5.4 value; the read-boundary normalization advisory is reported but never gates; without it enrich never gates (never-reject)")
	return cmd
}
