package cmd

import (
	"fmt"
	"io"

	"github.com/ghchinoy/binder/internal/okf"
)

// warnUnparsed emits a stderr disclosure for every concept file the loader could
// not parse (#161). The read-side commands never drop such a file — an
// unparseable concept is recovered as body and kept in the node set — but the
// user must be told, so a "clean" exit is never a silent one. Warnings go to
// stderr so a command's primary output (a write manifest, a report, a graph)
// stays uncontaminated. It writes nothing when the bundle parsed cleanly.
func warnUnparsed(w io.Writer, b *okf.Bundle) {
	for _, u := range b.Unparsed {
		fmt.Fprintf(w, "warning: %s: frontmatter did not parse (%s); kept as body under never-reject and reported as unparsed\n", u.RelPath, u.Err)
	}
}
