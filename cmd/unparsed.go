package cmd

import (
	"fmt"
	"io"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/okf"
)

// warnUnparsed emits a stderr disclosure for every file the loader could not
// parse (#161/#163). The read-side commands never drop such a file — an
// unparseable concept is recovered as body and kept in the node set, and an
// unparseable root index.md simply does not contribute an okf_version — but the
// user must be told, so a "clean" exit is never a silent one. Warnings go to
// stderr so a command's primary output (a write manifest, a report, a graph)
// stays uncontaminated. It writes nothing when the bundle parsed cleanly.
//
// The disclosure TEXT now lives in exactly one place — binder.UnparsedWarnings —
// so the read-side commands (graph/project) and the service-backed `index` write
// path emit byte-identical wording; this helper only frames each line to w. The
// service-backed commands (index) surface the same lines as data on their Result.
func warnUnparsed(w io.Writer, b *okf.Bundle) {
	for _, line := range binder.UnparsedWarnings(b) {
		fmt.Fprintln(w, line)
	}
}
