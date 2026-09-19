package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf"
)

func newProjectCmd(codec okf.Codec) *cobra.Command {
	var (
		out    string
		target string
		idKey  string
		today  string
	)
	// Construct the shared service ONCE with the composition root's codec (it is
	// stateless and safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
	cmd := &cobra.Command{
		Use:   "project <bundle> --out <dir>",
		Short: "Project a bundle into offline property-graph DDL (Spanner SQL/PGQ)",
		Long: "Project emits a deterministic, credential-free property-graph schema for a\n" +
			"loaded OKF bundle. It writes schema.ddl (CREATE TABLE Nodes, Edges and the\n" +
			"NodeVerified attestation table plus a CREATE PROPERTY GRAPH wrapper with a\n" +
			"single LINKS edge label) to --out and prints a binder.report/v1 summary to\n" +
			"stdout.\n\n" +
			"The projection reuses the same node/edge model as `binder graph`,\n" +
			"`list_graphs`, and `query_graph`, so it stays in edge/identity parity by\n" +
			"construction. Node identity (node_key) is the concept's authored frontmatter\n" +
			"value under --id-key when present and non-empty, otherwise the path-derived\n" +
			"concept id; binder NEVER mints a key. The tier/stale columns are the frozen\n" +
			"projection-time snapshot as of --today (SOURCE_DATE_EPOCH-honoring); stale_after\n" +
			"carries the raw authored input so stale stays re-derivable.\n\n" +
			"Alongside schema.ddl it emits the loader row data (nodes.csv, edges.csv,\n" +
			"load.sql) and the provenance artifacts node_verified.csv (the verified[]\n" +
			"attestations, copied losslessly: order preserved, by/at verbatim as authored,\n" +
			"is_human = the human: prefix) and derivation.sql (a CREATE VIEW that\n" +
			"recomputes tier/stale from stale_after and NodeVerified for any date).\n" +
			"--target defaults to spanner and is the only accepted value in this release.\n" +
			"No cloud credentials are used or needed.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// --target: spanner is the only accepted value in v0.4.0 (usage/exit 2).
			if target != string(graph.TargetSpanner) {
				return clijson.Usage(fmt.Errorf("--target %q is not supported (only %q in v0.4.0)", target, graph.TargetSpanner))
			}
			// --out is required (usage/exit 2): the command's purpose is to emit files.
			if out == "" {
				return clijson.Usage(fmt.Errorf("--out <dir> is required"))
			}
			// Validate --today up front as a usable date (exit 2), matching graph/review.
			if today != "" && !okf.IsValidISODate(today) {
				return clijson.Usage(fmt.Errorf("--today %q is not a valid date (expected YYYY-MM-DD)", today))
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the
			// core rule; a malformed epoch falls back to the wall clock.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			// One code path: the service owns Load → Project, the emission order, the
			// byte accounting, and the report manifest.
			res, err := svc.Project(cmd.Context(), binder.ProjectRequest{
				Bundle:  args[0],
				Out:     out,
				Target:  target,
				IDKey:   idKey,
				Now:     now,
				Today:   today,
				Version: Version,
			})
			if err != nil {
				return err
			}
			// Disclose unparseable files on stderr (#161): the recovered node now
			// exists so the emitted edges.csv no longer carries a dangling FK, but the
			// user must be told its frontmatter did not parse. stderr keeps the report
			// envelope on stdout clean.
			warnUnparsed(cmd.ErrOrStderr(), res.Bundle)

			if err := res.EncodeJSON(cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("encoding json report: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output directory for emitted artifacts (required)")
	cmd.Flags().StringVar(&target, "target", string(graph.TargetSpanner), "projection target dialect (only \"spanner\" in v0.4.0)")
	cmd.Flags().StringVar(&idKey, "id-key", "", "authored frontmatter key to use as node identity; falls back to path identity per concept")
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for the frozen tier/stale snapshot; defaults to now")
	return cmd
}
