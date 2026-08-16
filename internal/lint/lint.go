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
	"github.com/ghchinoy/binder/internal/graph"
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
	Orphans          []string  `json:"orphans"`           // 0 inbound AND 0 outbound resolved edges (true orphans)
	Entrypoints      []string  `json:"entrypoints"`       // 0 inbound but outbound/recognized-root/designated (issue #24)
	Stale            []string  `json:"stale"`             // okf.IsStale as of today
	SchemaViolations []Finding `json:"schema_violations"` // Detail: "missing type" | "invalid frontmatter: <err>"
}

// Lint computes the corpus health report from the resolved concepts and source
// facts convert.Analyze produced, as of today (YYYY-MM-DD, for staleness). It is
// read-only and deterministic. The caller sets Report.Src.
func Lint(concepts []*okf.Concept, facts []convert.SourceFacts, today string, entrypoints []string) *Report {
	r := &Report{
		NumConcepts:      len(concepts),
		BrokenLinks:      []Finding{},
		MissingTitles:    []string{},
		Orphans:          []string{},
		Entrypoints:      []string{},
		Stale:            []string{},
		SchemaViolations: []Finding{},
	}

	// The set of concept ids that actually exist. The codec optimistically marks
	// any in-bundle-shaped .md link "resolved" without checking the target names a
	// concept, so existence is cross-checked here — the same check convert does
	// against its output set, which is what makes lint's "broken" == convert's
	// "unresolved" (parity).
	exists := make(map[string]bool, len(concepts))
	// slugSets holds each concept's heading-slug set for anchor resolution, from
	// the single pinned convention (okf.HeadingSlugs). Precomputed once so a
	// concept that is the target of many #anchor links is slugged only once.
	slugSets := make(map[string]map[string]bool, len(concepts))
	for _, c := range concepts {
		exists[c.ID] = true
		set := make(map[string]bool)
		for _, s := range okf.HeadingSlugs(c.Body) {
			set[s] = true
		}
		slugSets[c.ID] = set
	}

	// Check 1 — broken links (shared linkcheck predicates, identical to review):
	// a resolved link whose target concept is absent, an unresolved internal .md
	// concept reference, or any residual [[...]] wikilink left in the body. Plus
	// anchors: a resolved cross-doc "foo.md#bar" whose target concept lacks the
	// heading slug "bar", and a same-doc "#bar" whose own concept lacks it.
	for _, c := range concepts {
		for _, l := range c.Links {
			var broken bool
			switch {
			case l.Resolved:
				switch {
				case !exists[l.TargetID]:
					broken = true // resolved shape, but no such concept
				default:
					// The .md resolved; if it carries a #fragment, the target concept
					// must have that heading slug.
					if frag := fragmentOf(l.RawTarget); frag != "" && !slugSets[l.TargetID][frag] {
						broken = true
					}
				}
			default:
				broken = linkcheck.IsBrokenConceptRef(l.RawTarget)
			}
			if broken {
				r.BrokenLinks = append(r.BrokenLinks, Finding{Concept: c.ID, Detail: l.RawTarget})
			}
		}
		// Same-document anchors ("#bar") are left in place by convert and are not
		// recorded as edges, so read them straight from the body via the shared
		// markdown-link extractor (code-region-aware). A "#bar" whose own concept
		// has no such heading slug is broken.
		for _, ml := range okf.ExtractMarkdownLinks(c.Body) {
			dest := strings.TrimSpace(ml.Dest)
			if !strings.HasPrefix(dest, "#") {
				continue
			}
			if frag := dest[1:]; frag != "" && !slugSets[c.ID][frag] {
				r.BrokenLinks = append(r.BrokenLinks, Finding{Concept: c.ID, Detail: dest})
			}
		}
		for _, target := range linkcheck.ResidualWikilinks(c.Body) {
			r.BrokenLinks = append(r.BrokenLinks, Finding{Concept: c.ID, Detail: "[[" + target + "]]"})
		}
	}

	// Checks 2 & 5 — source-fact checks over the pre-default authored state. These
	// are exactly what convert masks: a missing title is defaulted to a humanized
	// filename, a missing type: is defaulted, and invalid frontmatter is recovered
	// as body under never-reject. lint is the only surface that sees them.
	for _, f := range facts {
		if !f.TitlePresent {
			r.MissingTitles = append(r.MissingTitles, f.ConceptID)
		}
		switch {
		case f.Recovered:
			// The frontmatter parse failure is the root schema problem; the missing
			// type it necessarily also causes is subsumed by it, so report only the
			// invalid-frontmatter finding rather than double-listing the same file.
			detail := "invalid frontmatter"
			if f.RecoverErr != "" {
				// The codec's parse error already begins with "invalid frontmatter:";
				// use it as-is so the required prefix appears exactly once, otherwise
				// prepend it.
				if strings.HasPrefix(f.RecoverErr, "invalid frontmatter") {
					detail = f.RecoverErr
				} else {
					detail = "invalid frontmatter: " + f.RecoverErr
				}
			}
			r.SchemaViolations = append(r.SchemaViolations, Finding{Concept: f.ConceptID, Detail: detail})
		case !f.TypePresent:
			r.SchemaViolations = append(r.SchemaViolations, Finding{Concept: f.ConceptID, Detail: "missing type"})
		}
	}

	// Check 3 — node roles (issue #24). Edges come from the SINGLE resolved-edge
	// definition graph.EdgesFromConcepts — the same edges `binder graph` and the #9
	// catalog see — so lint never forks resolution. A concept with no inbound edge
	// is either a true ORPHAN (also no outbound) or an ENTRYPOINT (has outbound, or
	// is a recognized root README.md, or was designated via --entrypoint).
	// review applies the same rule over different inputs: lint reads the corpus as
	// authored, review the emitted bundle, with conversion (renames, defaulting) in
	// between — so the two usually agree but are not guaranteed to. Both are advisory.
	edges := graph.EdgesFromConcepts(concepts)
	inbound := make(map[string]int, len(concepts))
	outbound := make(map[string]int, len(concepts))
	for _, e := range edges {
		outbound[e.From]++
		inbound[e.To]++
	}
	designated := linkcheck.EntrypointSet(entrypoints)

	// Check 4 — stale: okf.IsStale as of today. today is deterministic (the
	// command derives it from --today or SOURCE_DATE_EPOCH). Inherently advisory.
	for _, c := range concepts {
		if inbound[c.ID] == 0 {
			if outbound[c.ID] > 0 || designated[c.ID] || linkcheck.IsRootEntrypoint(c.RelPath) {
				r.Entrypoints = append(r.Entrypoints, c.ID)
			} else {
				r.Orphans = append(r.Orphans, c.ID)
			}
		}
		if okf.IsStale(c, today) {
			r.Stale = append(r.Stale, c.ID)
		}
	}

	r.sortAll()
	return r
}

// fragmentOf returns the #fragment of a raw link target (the part after the
// first '#'), or "" if it has none.
func fragmentOf(target string) string {
	if i := strings.IndexByte(target, '#'); i >= 0 {
		return target[i+1:]
	}
	return ""
}

// sortAll orders every bucket deterministically: Finding slices by concept then
// detail, id slices lexically.
func (r *Report) sortAll() {
	sortFindings(r.BrokenLinks)
	sortFindings(r.SchemaViolations)
	sort.Strings(r.MissingTitles)
	sort.Strings(r.Orphans)
	sort.Strings(r.Entrypoints)
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
	fmt.Fprintf(&b, "  entrypoints (no inbound links): %d\n", len(r.Entrypoints))
	for _, id := range r.Entrypoints {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  orphans (no inbound or outbound links): %d\n", len(r.Orphans))
	for _, id := range r.Orphans {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  stale: %d\n", len(r.Stale))
	for _, id := range r.Stale {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	return b.String()
}
