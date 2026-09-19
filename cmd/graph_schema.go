package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
)

// newGraphSchemaCmd is the thin CLI adapter for the read-only schema-describe
// operation that was MCP-only (list_graphs) before Phase 2. It maps flags onto a
// binder.GraphDescribeRequest and drives the SAME binder.Service.GraphDescribe the MCP
// tool uses; it carries NO capability logic. It always emits the binder.report/v1
// list_graphs envelope — the payload is already machine-readable, so there is no prose
// form. It is read-only: it never writes to the bundle, mutates frontmatter, or mints
// an id.
func newGraphSchemaCmd(codec okf.Codec) *cobra.Command {
	var (
		idKey string
		today string
	)
	// Construct the shared service ONCE with the composition root's codec (it is
	// stateless and safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
	cmd := &cobra.Command{
		Use:   "schema-describe <bundle>",
		Short: "Introspect the property graph(s) binder can project from a bundle",
		Long: "Schema-describe reports the property graph(s) binder can project from an OKF\n" +
			"bundle: graph name, node labels (the concept types present) and the single\n" +
			"LINKS edge label, each with property declarations and counts. It is derived\n" +
			"from the same projection as `binder graph`, so it stays in parity by\n" +
			"construction.\n\n" +
			"Read-only; output is the binder.report/v1 list_graphs payload. Node identity\n" +
			"(node_key) is the concept's authored frontmatter value under --id-key when\n" +
			"present and non-empty, otherwise the path-derived concept id (spec §2).",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate --today up front as a usable date (exit 2), matching the other
			// commands; okf.IsStale is left untouched.
			if today != "" && !okf.IsValidISODate(today) {
				return clijson.Usage(fmt.Errorf("--today %q is not a valid date (expected YYYY-MM-DD)", today))
			}

			// Read the ambient determinism state HERE (adapter edge) and apply the
			// core rule; a malformed epoch falls back to the wall clock.
			now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

			res, err := svc.GraphDescribe(cmd.Context(), binder.GraphDescribeRequest{
				Bundle:  args[0],
				IDKey:   idKey,
				Now:     now,
				Today:   today,
				Version: Version,
			})
			if err != nil {
				return err
			}
			if err := res.EncodeJSON(cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("encoding json report: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&idKey, "id-key", "", "frontmatter key to prefer as the stable node key; empty falls back to path-as-identity (spec §2)")
	cmd.Flags().StringVar(&today, "today", "", "date (YYYY-MM-DD) used for staleness; defaults to now")
	return cmd
}
