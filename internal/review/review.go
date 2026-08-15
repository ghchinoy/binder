// Package review summarizes an already-loaded OKF bundle for human inspection:
// concept counts, unresolved links, orphans, trust tiers, and stale concepts
// (design-v2 §4.5). It is a pure read over the binder-owned okf model and
// derives trust tiers/staleness with the owned okf functions — it never stores a
// credibility score (spec §5.1).
package review

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// wikilinkRE matches a residual wiki-style link left in a body: [[Target]] or
// [[Target|alias]]. `binder convert` rewrites RESOLVED wikilinks to standard
// markdown links, so any [[...]] surviving in a persisted body is by construction
// an UNRESOLVED reference the read side must report (design-v2 §4.2).
var wikilinkRE = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// Edge is one unresolved link, reported with its source concept (§4.2).
type Edge struct {
	From      string
	RawTarget string
	Text      string
}

// ConceptView is the per-concept derived summary.
type ConceptView struct {
	ID       string
	Type     string
	Tier     okf.Tier
	Stale    bool
	Attested bool
	Orphan   bool
}

// Report is the outcome of reviewing a bundle.
type Report struct {
	Root        string
	Today       string
	NumConcepts int
	ByType      map[string]int
	Tiers       map[okf.Tier]int
	Orphans     []string
	Stale       []string
	Attested    []string
	Unresolved  []Edge
	// UnparsedFrontmatter lists concepts carrying the binder recovery marker
	// (okf.RecoveryMarkerKey) — a file whose original frontmatter would not parse
	// and was preserved as body by `binder convert` (never-reject, design-v2 §4.6).
	// review reads the same persisted marker the converter stamped and warned
	// about, so the two surfaces can never disagree and no body-shape heuristic
	// (which cannot tell a recovered fence from a legit thematic break) is needed.
	UnparsedFrontmatter []string
	Concepts            []ConceptView
}

// Review computes the review report for a loaded bundle as of `today`
// (YYYY-MM-DD, used for staleness). An orphan is a concept that no other concept
// links to (no inbound resolved edge); it is reported for the user to wire up or
// accept, never removed.
func Review(b *okf.Bundle, today string) *Report {
	r := &Report{
		Root:        b.Root,
		Today:       today,
		NumConcepts: len(b.Concepts),
		ByType:      map[string]int{},
		Tiers:       map[okf.Tier]int{},
	}

	// The set of concept IDs that actually exist. A link the codec marks
	// "resolved" only means it is an in-bundle-shaped .md reference; it may still
	// name no concept. Cross-checking against this set is what distinguishes a
	// live edge from a broken concept reference (the same existence check the
	// converter does against its output set).
	exists := make(map[string]bool, len(b.Concepts))
	for _, c := range b.Concepts {
		exists[c.ID] = true
	}

	inbound := map[string]int{}
	for _, c := range b.Concepts {
		for _, l := range c.Links {
			if l.Resolved && exists[l.TargetID] && l.TargetID != c.ID {
				inbound[l.TargetID]++
			}
		}
	}

	concepts := append([]*okf.Concept(nil), b.Concepts...)
	sort.Slice(concepts, func(i, j int) bool { return concepts[i].ID < concepts[j].ID })

	for _, c := range concepts {
		typ := c.Type
		if typ == "" {
			typ = "(none)"
		}
		r.ByType[typ]++

		tier := okf.TrustTier(c)
		r.Tiers[tier]++

		stale := okf.IsStale(c, today)
		orphan := inbound[c.ID] == 0

		r.Concepts = append(r.Concepts, ConceptView{
			ID: c.ID, Type: c.Type, Tier: tier,
			Stale: stale, Attested: c.Trust.Attested, Orphan: orphan,
		})
		if orphan {
			r.Orphans = append(r.Orphans, c.ID)
		}
		if stale {
			r.Stale = append(r.Stale, c.ID)
		}
		if c.Trust.Attested {
			r.Attested = append(r.Attested, c.ID)
		}
		if okf.IsRecovered(c.Frontmatter) {
			r.UnparsedFrontmatter = append(r.UnparsedFrontmatter, c.ID)
		}
		for _, l := range c.Links {
			// A "broken" edge is a CONCEPT reference whose target concept does not
			// exist, matching what `binder convert` tracks. The codec optimistically
			// marks any in-bundle-shaped .md link resolved without checking the
			// target exists, so existence is cross-checked here. External URLs,
			// anchors, and links to non-concept files (assets, scripts) are not
			// concept references and are never reported — that would be noise and
			// inconsistent with the converter's own report.
			broken := false
			switch {
			case l.Resolved:
				broken = !exists[l.TargetID] // resolved shape, but no such concept
			default:
				broken = isBrokenConceptRef(l.RawTarget) // couldn't resolve an internal .md ref
			}
			if broken {
				r.Unresolved = append(r.Unresolved, Edge{From: c.ID, RawTarget: l.RawTarget, Text: l.Text})
			}
		}
		// Residual wikilinks: any [[...]] still in the body is an unresolved
		// reference (resolved ones were rewritten to markdown links at convert time),
		// but it is not a markdown link so the codec's LinkGraph never surfaces it.
		// Scan the persisted body directly so `review` reports it (design-v2 §4.2).
		for _, target := range residualWikilinks(c.Body) {
			r.Unresolved = append(r.Unresolved, Edge{From: c.ID, RawTarget: "[[" + target + "]]", Text: target})
		}
	}

	sort.Slice(r.Unresolved, func(i, j int) bool {
		if r.Unresolved[i].From != r.Unresolved[j].From {
			return r.Unresolved[i].From < r.Unresolved[j].From
		}
		return r.Unresolved[i].RawTarget < r.Unresolved[j].RawTarget
	})
	return r
}

// String renders a deterministic, human-readable review.
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "binder review\n")
	fmt.Fprintf(&b, "  bundle: %s\n", r.Root)
	fmt.Fprintf(&b, "  concepts: %d\n", r.NumConcepts)

	b.WriteString("  by type:\n")
	for _, k := range sortedKeys(r.ByType) {
		fmt.Fprintf(&b, "    %s: %d\n", k, r.ByType[k])
	}

	b.WriteString("  trust tiers:\n")
	for _, tier := range []okf.Tier{okf.TierHumanReviewed, okf.TierMachineConfirmed, okf.TierUnverified} {
		fmt.Fprintf(&b, "    %s: %d\n", tier, r.Tiers[tier])
	}

	fmt.Fprintf(&b, "  stale (as of %s): %d\n", r.Today, len(r.Stale))
	for _, id := range r.Stale {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  attested computations: %d\n", len(r.Attested))
	for _, id := range r.Attested {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  unparsed frontmatter (recovered as body): %d\n", len(r.UnparsedFrontmatter))
	for _, id := range r.UnparsedFrontmatter {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  orphans (no inbound links): %d\n", len(r.Orphans))
	for _, id := range r.Orphans {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  unresolved links: %d\n", len(r.Unresolved))
	for _, e := range r.Unresolved {
		fmt.Fprintf(&b, "    %s -> %s\n", e.From, e.RawTarget)
	}
	return b.String()
}

// isBrokenConceptRef reports whether a raw, unresolved link target is an
// internal CONCEPT reference — i.e. a bundle-relative .md target that names no
// concept. External URLs, same-document anchors, and links to non-concept files
// (assets, scripts, directories) are not concept references and so are never
// "broken" edges, matching what `binder convert` tracks.
func isBrokenConceptRef(raw string) bool {
	t := strings.TrimSpace(raw)
	if t == "" || strings.HasPrefix(t, "#") {
		return false
	}
	if strings.Contains(t, "://") {
		return false
	}
	for _, p := range []string{"mailto:", "tel:", "ftp:"} {
		if strings.HasPrefix(t, p) {
			return false
		}
	}
	// Only a .md target (ignoring any #fragment) is a concept reference.
	if i := strings.IndexByte(t, '#'); i >= 0 {
		t = t[:i]
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(t)), ".md")
}

// residualWikilinks returns the targets of any [[...]] / [[...|alias]] wikilinks
// left in body, excluding those inside code spans/blocks (matching the converter's
// code-aware handling). By construction these are unresolved references.
func residualWikilinks(body string) []string {
	if !strings.Contains(body, "[[") {
		return nil
	}
	code := okf.CodeRegions(body)
	var out []string
	for _, idx := range wikilinkRE.FindAllStringSubmatchIndex(body, -1) {
		if okf.InCodeRegion(idx[0], code) {
			continue
		}
		inner := body[idx[2]:idx[3]]
		target := inner
		if i := strings.IndexByte(inner, '|'); i >= 0 {
			target = inner[:i]
		}
		if target = strings.TrimSpace(target); target != "" {
			out = append(out, target)
		}
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
