package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// hintCatalogFlags emits a best-effort hint when --include-backlinks or
// --include-graph is passed WITHOUT --group-by-type, where they are a silent
// no-op (the catalog they annotate is only produced under --group-by-type).
//
// The hint is written ONLY to stderr (never stdout), so it can never corrupt a
// --json payload, and it never changes the exit code. When --group-by-type is
// set it is silent. One line is emitted per offending flag, naming it.
func hintCatalogFlags(cmd *cobra.Command, groupByType, includeBacklinks, includeGraph bool) {
	if groupByType {
		return
	}
	w := cmd.ErrOrStderr()
	if includeBacklinks {
		fmt.Fprintln(w, "hint: --include-backlinks has no effect without --group-by-type")
	}
	if includeGraph {
		fmt.Fprintln(w, "hint: --include-graph has no effect without --group-by-type")
	}
}
