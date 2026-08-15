package convert

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// Report summarizes a conversion run. It is returned by Convert and is the
// payload of --report and --dry-run output.
type Report struct {
	Src           string           `json:"src"`
	Out           string           `json:"out"`
	Concepts      []ConceptReport  `json:"concepts"`
	Warnings      []string         `json:"warnings"`
	Unresolved    []UnresolvedLink `json:"unresolved"`
	NumConcepts   int              `json:"num_concepts"`
	NumLinks      int              `json:"num_links"`
	NumResolved   int              `json:"num_resolved"`
	NumUnresolved int              `json:"num_unresolved"`
	NumRecovered  int              `json:"num_recovered"` // files whose unparseable frontmatter was preserved as body (§4.6)
	DryRun        bool             `json:"dry_run"`
}

// UnresolvedLink is one link whose target is not a concept in the bundle. It is
// left in place (spec §6) and reported so the user can fix or accept it (§4.2).
type UnresolvedLink struct {
	From      string `json:"from"`       // source concept rel path
	RawTarget string `json:"raw_target"` // target exactly as written
	Text      string `json:"text"`       // link text / relationship label
}

// ConceptReport describes one converted concept.
type ConceptReport struct {
	RelPath       string `json:"rel_path"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	NumLinks      int    `json:"num_links"`
	NumUnresolved int    `json:"num_unresolved"`
}

func (r *Report) addWarning(format string, args ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// addUnresolved records every unresolved edge of a concept for the report.
func (r *Report) addUnresolved(c *okf.Concept) {
	for _, l := range c.Links {
		if !l.Resolved {
			r.Unresolved = append(r.Unresolved, UnresolvedLink{
				From: c.RelPath, RawTarget: l.RawTarget, Text: l.Text,
			})
		}
	}
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
	if r.NumRecovered > 0 {
		fmt.Fprintf(&b, "  recovered as body (unparseable frontmatter): %d\n", r.NumRecovered)
	}

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
	if len(r.Unresolved) > 0 {
		unresolved := append([]UnresolvedLink(nil), r.Unresolved...)
		sort.Slice(unresolved, func(i, j int) bool {
			if unresolved[i].From != unresolved[j].From {
				return unresolved[i].From < unresolved[j].From
			}
			return unresolved[i].RawTarget < unresolved[j].RawTarget
		})
		b.WriteString("\nUnresolved links:\n")
		for _, u := range unresolved {
			fmt.Fprintf(&b, "  %s -> %s\n", u.From, u.RawTarget)
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
