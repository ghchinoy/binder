// Package lint reports the health of a SOURCE markdown corpus BEFORE conversion,
// writing nothing (issue #8). It is the third command of the triad —
// `binder validate` checks an emitted bundle's spec §11 conformance, `binder
// review` summarizes an emitted bundle, and `binder lint` inspects the corpus as
// authored. lint is the only surface that sees pre-conversion source state: a
// missing title or a missing type: is invisible in a bundle because `binder
// convert` defaults them.
//
// lint performs NO walk/parse/resolve of its own. It runs over the resolved
// concepts and source facts that convert.Analyze already produced, so its
// "broken link" is by construction the converter's "unresolved link" (parity),
// and it shares linkcheck's broken-reference predicates with `binder review` so
// the two can never drift.
package lint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/linkcheck"
	"github.com/ghchinoy/binder/internal/okf"
)

// Finding is one link/schema problem attributed to a concept. Concept is the
// bundle concept id; Detail is the raw target or a short reason.
type Finding struct {
	Concept string `json:"concept"`
	Detail  string `json:"detail"`
}

// Report is the outcome of linting a corpus. Every slice is initialized so an
// empty run serializes to [] not null (#13). All buckets are sorted (concept id,
// then detail) for deterministic output.
type Report struct {
	Src              string    `json:"src"`               // the linted corpus path
	NumConcepts      int       `json:"num_concepts"`      // concepts discovered
	BrokenLinks      []Finding `json:"broken_links"`      // raw target (incl. #anchor) naming no concept/heading
	MissingTitles    []string  `json:"missing_titles"`    // concept ids with no authored title: and no first H1
	Orphans          []string  `json:"orphans"`           // 0 inbound AND 0 outbound resolved edges
	Stale            []string  `json:"stale"`             // okf.IsStale as of today
	SchemaViolations []Finding `json:"schema_violations"` // Detail: "missing type" | "invalid frontmatter: <err>"
}

// Lint computes the corpus health report from the resolved concepts and source
// facts convert.Analyze produced, as of today (YYYY-MM-DD, for staleness). It is
// read-only and deterministic. The caller sets Report.Src.
func Lint(concepts []*okf.Concept, facts []convert.SourceFacts, today string) *Report {
	r := &Report{
		NumConcepts:      len(concepts),
		BrokenLinks:      []Finding{},
		MissingTitles:    []string{},
		Orphans:          []string{},
		Stale:            []string{},
		SchemaViolations: []Finding{},
	}

	// The set of concept ids that actually exist. The codec optimistically marks
	// any in-bundle-shaped .md link "resolved" without checking the target names a
	// concept, so existence is cross-checked here — the same check convert does
	// against its output set, which is what makes lint's "broken" == convert's
	// "unresolved" (parity).
	exists := make(map[string]bool, len(concepts))
	for _, c := range concepts {
		exists[c.ID] = true
	}

	// Check 1 — broken links (shared linkcheck predicates, identical to review):
	// a resolved link whose target concept is absent, an unresolved internal .md
	// concept reference, or any residual [[...]] wikilink left in the body.
	for _, c := range concepts {
		for _, l := range c.Links {
			var broken bool
			switch {
			case l.Resolved:
				broken = !exists[l.TargetID]
			default:
				broken = linkcheck.IsBrokenConceptRef(l.RawTarget)
			}
			if broken {
				r.BrokenLinks = append(r.BrokenLinks, Finding{Concept: c.ID, Detail: l.RawTarget})
			}
		}
		for _, target := range linkcheck.ResidualWikilinks(c.Body) {
			r.BrokenLinks = append(r.BrokenLinks, Finding{Concept: c.ID, Detail: "[[" + target + "]]"})
		}
	}

	r.sortAll()
	return r
}

// sortAll orders every bucket deterministically: Finding slices by concept then
// detail, id slices lexically.
func (r *Report) sortAll() {
	sortFindings(r.BrokenLinks)
	sortFindings(r.SchemaViolations)
	sort.Strings(r.MissingTitles)
	sort.Strings(r.Orphans)
	sort.Strings(r.Stale)
}

func sortFindings(f []Finding) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].Concept != f[j].Concept {
			return f[i].Concept < f[j].Concept
		}
		return f[i].Detail < f[j].Detail
	})
}

// NumFindings is the total across all check buckets. Under --strict a non-zero
// count gates at exit 1 (option (a), the unified never-reject posture).
func (r *Report) NumFindings() int {
	return len(r.BrokenLinks) + len(r.MissingTitles) + len(r.Orphans) +
		len(r.Stale) + len(r.SchemaViolations)
}

// String renders a deterministic, human-readable report.
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "binder lint\n")
	fmt.Fprintf(&b, "  corpus: %s\n", r.Src)
	fmt.Fprintf(&b, "  concepts: %d\n", r.NumConcepts)

	fmt.Fprintf(&b, "  broken links: %d\n", len(r.BrokenLinks))
	for _, f := range r.BrokenLinks {
		fmt.Fprintf(&b, "    %s -> %s\n", f.Concept, f.Detail)
	}
	fmt.Fprintf(&b, "  missing titles: %d\n", len(r.MissingTitles))
	for _, id := range r.MissingTitles {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  schema violations: %d\n", len(r.SchemaViolations))
	for _, f := range r.SchemaViolations {
		fmt.Fprintf(&b, "    %s: %s\n", f.Concept, f.Detail)
	}
	fmt.Fprintf(&b, "  orphans: %d\n", len(r.Orphans))
	for _, id := range r.Orphans {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  stale: %d\n", len(r.Stale))
	for _, id := range r.Stale {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	return b.String()
}
