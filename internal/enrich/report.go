package enrich

import (
	"fmt"
	"strings"
)

// String renders a human-readable, deterministic report. Files are already
// sorted by path; the same input yields the same output.
func (r *Report) String() string {
	var b strings.Builder

	verb := "enriched"
	if r.DryRun {
		verb = "would enrich"
	}
	fmt.Fprintf(&b, "enrich %s\n", r.Src)
	if r.DryRun {
		b.WriteString("(dry run — no files written)\n")
	}
	fmt.Fprintf(&b, "%d file(s): %d %s, %d unchanged, %d skipped\n",
		r.NumFiles, r.NumEnriched, verb, r.NumUnchanged, r.NumSkipped)

	// List actionable outcomes only (enriched/would-enrich/skipped); the
	// unchanged majority is summarized in the counts above and enumerated in
	// --json. Files are pre-sorted by path so the order is deterministic.
	for _, f := range r.Files {
		switch f.Status {
		case StatusEnriched:
			fmt.Fprintf(&b, "  enriched %s (added: %s)\n", f.Path, strings.Join(f.Added, ", "))
		case StatusWouldEnrich:
			fmt.Fprintf(&b, "  would enrich %s (add: %s)\n", f.Path, strings.Join(f.Added, ", "))
		case StatusSkipped:
			fmt.Fprintf(&b, "  skipped %s (%s)\n", f.Path, f.Reason)
		}
	}
	return b.String()
}
