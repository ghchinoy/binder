package binder

import (
	"fmt"

	"github.com/ghchinoy/binder/internal/okf"
)

// UnparsedWarnings returns the stderr disclosure lines for every file the bundle
// loader could not parse (#161/#163), as DATA — one message per unparsed concept
// (kept in the nav, recovered as body, never dropped) and one for an unparseable root
// index.md (okf_version not adopted). Each line is complete but carries no trailing
// newline; the caller frames it. This is the single home for the text that
// Service.Index and cmd.warnUnparsed both emit, so the two cannot drift. It returns
// no lines when the bundle parsed cleanly.
//
// It lives in this dedicated, capability-neutral file (not index.go) because it is a
// shared read-boundary advisory consumed by several surfaces — Service.Index, the
// read-side cmd/graph and cmd/project (via cmd.warnUnparsed), and Phase-2's migrated
// read handlers — none of which is "the index command"; a neutral home keeps that
// seam from looking index-owned.
func UnparsedWarnings(b *okf.Bundle) []string {
	var out []string
	for _, u := range b.Unparsed {
		out = append(out, fmt.Sprintf(
			"warning: %s: frontmatter did not parse (%s); kept as body under never-reject and reported as unparsed",
			u.RelPath, u.Err))
	}
	if b.RootVersionUnparsed != nil {
		out = append(out, fmt.Sprintf(
			"warning: %s: frontmatter did not parse (%s); okf_version not adopted, using default %s",
			b.RootVersionUnparsed.RelPath, b.RootVersionUnparsed.Err, b.OKFVersion))
	}
	return out
}
