package convert

import (
	"fmt"
	"sort"
	"strings"
)

// Report summarizes a conversion run. It is returned by Convert and is the
// payload of --report and --dry-run output.
type Report struct {
	Src           string
	Out           string
	Concepts      []ConceptReport
	Warnings      []string
	NumConcepts   int
	NumLinks      int
	NumResolved   int
	NumUnresolved int
	DryRun        bool
}

// ConceptReport describes one converted concept.
type ConceptReport struct {
	RelPath       string
	Type          string
	Title         string
	NumLinks      int
	NumUnresolved int
}

func (r *Report) addWarning(format string, args ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// String renders a human-readable, deterministic report.
func (r *Report) String() string {
	var b strings.Builder
	mode := "convert"
	if r.DryRun {
		mode = "convert --dry-run (no files written)"
	}
	fmt.Fprintf(&b, "binder %s\n", mode)
	fmt.Fprintf(&b, "  source: %s\n", r.Src)
	fmt.Fprintf(&b, "  output: %s\n", r.Out)
	fmt.Fprintf(&b, "  concepts: %d\n", r.NumConcepts)
	fmt.Fprintf(&b, "  links: %d (resolved %d, unresolved %d)\n", r.NumLinks, r.NumResolved, r.NumUnresolved)

	concepts := append([]ConceptReport(nil), r.Concepts...)
	sort.Slice(concepts, func(i, j int) bool { return concepts[i].RelPath < concepts[j].RelPath })
	if len(concepts) > 0 {
		b.WriteString("\nConcepts:\n")
		for _, c := range concepts {
			fmt.Fprintf(&b, "  %s  [type=%s]", c.RelPath, c.Type)
			if c.NumUnresolved > 0 {
				fmt.Fprintf(&b, "  (%d unresolved links)", c.NumUnresolved)
			}
			b.WriteString("\n")
		}
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "  - %s\n", w)
		}
	}
	return b.String()
}
