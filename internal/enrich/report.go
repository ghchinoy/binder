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
			fmt.Fprintf(&b, "  enriched %s%s\n", f.Path, changeSuffix("added", "overwritten", f))
		case StatusWouldEnrich:
			fmt.Fprintf(&b, "  would enrich %s%s\n", f.Path, changeSuffix("add", "overwrite", f))
		case StatusSkipped:
			fmt.Fprintf(&b, "  skipped %s (%s)\n", f.Path, f.Reason)
		}
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "  advisory: %s\n", w)
	}
	for _, n := range r.StatusNotes {
		fmt.Fprintf(&b, "  status: %s\n", n)
	}
	// Trust disclosure (Residual B): renders an identical block to convert via the
	// shared VerifiedStampReport.ProseSection, or nothing on the never-stamp default.
	b.WriteString(r.Verified.ProseSection())
	return b.String()
}

// changeSuffix renders the "(added: …)" / "(add: … ; overwrite: …)" tail of a
// per-file enriched/would-enrich line. The overwrite clause is present only when
// keys were refreshed in place (--overwrite-keys), so default output is
// unchanged. addVerb/owVerb differ between the applied and dry-run phrasings.
func changeSuffix(addVerb, owVerb string, f FileResult) string {
	var parts []string
	if len(f.Added) > 0 {
		parts = append(parts, fmt.Sprintf("%s: %s", addVerb, strings.Join(f.Added, ", ")))
	}
	if len(f.Overwritten) > 0 {
		parts = append(parts, fmt.Sprintf("%s: %s", owVerb, strings.Join(f.Overwritten, ", ")))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}
