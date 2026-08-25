package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf"
)

// projectReport is the binder.report/v1 result payload for `binder project`. It
// carries the node-identity strategy (echoing the list_graphs vocabulary), the
// re-rooting stability signal (OQ-6), node/edge counts, the projected_as_of date
// the frozen tier/stale snapshot reflects (OQ-8), the target dialect, and a
// manifest of the emitted files with byte lengths.
type projectReport struct {
	Target            string                  `json:"target"`
	NodeKey           graph.NodeKey           `json:"node_key"`
	IdentityStability graph.IdentityStability `json:"identity_stability"`
	Counts            graph.Counts            `json:"counts"`
	ProjectedAsOf     string                  `json:"projected_as_of"`
	Artifacts         []projectArtifact       `json:"artifacts"`
}

// projectArtifact is one emitted file in the artifacts manifest.
type projectArtifact struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
}

func newProjectCmd(codec okf.Codec) *cobra.Command {
	var (
		out    string
		target string
		idKey  string
		today  string
	)
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
			b, err := bundle.Load(args[0], codec)
			if err != nil {
				return err
			}
			// Disclose unparseable files on stderr (#161): the recovered node now
			// exists so the emitted edges.csv no longer carries a dangling FK, but
			// the user must be told its frontmatter did not parse. stderr keeps the
			// report envelope on stdout clean.
			warnUnparsed(cmd.ErrOrStderr(), b)
			if today == "" {
				today = resolveNow().Format("2006-01-02")
			}
			proj := graph.Project(b, graph.ProjectOptions{
				Target: graph.Target(target),
				IDKey:  idKey,
				Today:  today,
			})

			if err := os.MkdirAll(out, 0o755); err != nil {
				return fmt.Errorf("creating output dir: %w", err)
			}

			// Emit each artifact and record it in the manifest with its byte length.
			// The emitted set is: schema.ddl (G1) + the loader row data (G2). Order
			// is fixed for determinism.
			var artifacts []projectArtifact
			emit := func(name string, data []byte) error {
				if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
					return fmt.Errorf("writing %s: %w", name, err)
				}
				artifacts = append(artifacts, projectArtifact{Name: name, Bytes: len(data)})
				return nil
			}
			// G2 artifact block (self-contained): the schema plus the loader row data
			// and the DML load statements that populate Nodes/Edges from those rows.
			for _, a := range []struct {
				name string
				data []byte
			}{
				{"schema.ddl", proj.DDL()},
				{"nodes.csv", proj.NodesCSV()},
				{"edges.csv", proj.EdgesCSV()},
				{"load.sql", proj.LoadSQL()},
			} {
				if err := emit(a.name, a.data); err != nil {
					return err
				}
			}

			// --- G3 provenance-completeness artifacts (OQ-8 items 3–4) ---
			// Self-contained append block: the NodeVerified rows and the tier/stale
			// derivation view. Kept together (not interleaved with the G2 row
			// emitters) so each phase's additions stay reviewable in isolation.
			for _, a := range []struct {
				name string
				data []byte
			}{
				{"node_verified.csv", proj.NodeVerifiedCSV()},
				{"derivation.sql", proj.DerivationView()},
			} {
				if err := emit(a.name, a.data); err != nil {
					return err
				}
			}
			// --- end G3 block ---

			rep := projectReport{
				Target:            string(proj.Target),
				NodeKey:           proj.NodeKey,
				IdentityStability: proj.Identity,
				Counts:            proj.Counts,
				ProjectedAsOf:     proj.AsOf,
				Artifacts:         artifacts,
			}
			if err := clijson.Encode(cmd.OutOrStdout(), Version, "project", rep); err != nil {
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
