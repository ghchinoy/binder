// Package review summarizes an already-loaded OKF bundle for human inspection:
// concept counts, unresolved links, orphans, trust tiers, and stale concepts
// (design-v2 §4.5). It is a pure read over the binder-owned okf model and
// derives trust tiers/staleness with the owned okf functions — it never stores a
// credibility score (spec §5.1).
package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/linkcheck"
	"github.com/ghchinoy/binder/internal/okf"
)

// Edge is one unresolved link, reported with its source concept (§4.2).
type Edge struct {
	From      string `json:"from"`
	RawTarget string `json:"raw_target"`
	Text      string `json:"text"`
}

// ConceptView is the per-concept derived summary.
type ConceptView struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Tier     okf.Tier `json:"tier"`
	Stale    bool     `json:"stale"`
	Attested bool     `json:"attested"`
	Orphan   bool     `json:"orphan"`
	// Entrypoint is true when the concept has no inbound resolved edge but is a
	// legitimate root rather than a true orphan: it has outbound resolved edges, or
	// it is a recognized root entrypoint (README.md/index.md), or it was designated
	// via --entrypoint. Orphan and Entrypoint are mutually exclusive; both require
	// zero inbound edges (issue #24).
	Entrypoint bool `json:"entrypoint"`
}

// Report is the outcome of reviewing a bundle.
type Report struct {
	Root        string           `json:"root"`
	Today       string           `json:"today"`
	NumConcepts int              `json:"num_concepts"`
	ByType      map[string]int   `json:"by_type"`
	Tiers       map[okf.Tier]int `json:"tiers"`
	Orphans     []string         `json:"orphans"`
	// Entrypoints are concepts with no inbound resolved edge that are NOT true
	// orphans (issue #24): they have outbound resolved edges, or are a recognized
	// root entrypoint (README.md/index.md), or were designated via --entrypoint. A
	// root README that indexes the corpus lands here, not in Orphans. Advisory only.
	Entrypoints []string `json:"entrypoints"`
	Stale       []string `json:"stale"`
	Attested    []string `json:"attested"`
	Unresolved  []Edge   `json:"unresolved"`
	// UnparsedFrontmatter lists concepts carrying the binder recovery marker
	// (okf.RecoveryMarkerKey) — a file whose original frontmatter would not parse
	// and was preserved as body by `binder convert` (never-reject, design-v2 §4.6).
	// review reads the same persisted marker the converter stamped and warned
	// about, so the two surfaces can never disagree and no body-shape heuristic
	// (which cannot tell a recovered fence from a legit thematic break) is needed.
	UnparsedFrontmatter []string      `json:"unparsed_frontmatter"`
	Concepts            []ConceptView `json:"concepts"`
}

// Review computes the review report for a loaded bundle as of `today`
// (YYYY-MM-DD, used for staleness). Node roles are classified by inbound/outbound
// resolved edges (issue #24): a concept with no inbound AND no outbound edge is a
// true ORPHAN; a concept with no inbound but with outbound edges — or one that is
// a recognized root entrypoint (README.md/index.md) or was designated via
// `entrypoints` — is an ENTRYPOINT, not an orphan. Both are reported for the user
// to wire up or accept, never removed; the classification is advisory only and
// never gates. `entrypoints` designates additional concepts (by id or path) as
// entrypoints, over and above the general rule and the recognized roots.
func Review(b *okf.Bundle, today string, entrypoints []string) *Report {
	// Slices are initialized so an empty review serializes to [] not null (#13).
	r := &Report{
		Root:                b.Root,
		Today:               today,
		NumConcepts:         len(b.Concepts),
		ByType:              map[string]int{},
		Tiers:               map[okf.Tier]int{},
		Orphans:             []string{},
		Entrypoints:         []string{},
		Stale:               []string{},
		Attested:            []string{},
		Unresolved:          []Edge{},
		UnparsedFrontmatter: []string{},
		Concepts:            []ConceptView{},
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

	// Inbound and outbound resolved-edge counts drive the orphan/entrypoint split.
	// An edge counts only when it is resolved, names a concept that exists, and is
	// not a self-loop — the same live-edge test used for orphans before #24.
	inbound := map[string]int{}
	outbound := map[string]int{}
	for _, c := range b.Concepts {
		for _, l := range c.Links {
			if l.Resolved && exists[l.TargetID] && l.TargetID != c.ID {
				inbound[l.TargetID]++
				outbound[c.ID]++
			}
		}
	}
	// Concepts the user designated as entrypoints (by id or path), in addition to
	// the general rule and the recognized root entrypoints.
	designated := linkcheck.EntrypointSet(entrypoints)

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
		// Node role (issue #24): only a concept with no inbound edge is ever an
		// orphan or an entrypoint. Among those, it is an ENTRYPOINT when it links
		// outward, is a recognized root (README.md/index.md), or was designated;
		// otherwise it is a TRUE ORPHAN (no inbound AND no outbound). The two are
		// mutually exclusive.
		noInbound := inbound[c.ID] == 0
		entrypoint := noInbound && (outbound[c.ID] > 0 || designated[c.ID] || linkcheck.IsRootEntrypoint(c.RelPath))
		orphan := noInbound && !entrypoint

		r.Concepts = append(r.Concepts, ConceptView{
			ID: c.ID, Type: c.Type, Tier: tier,
			Stale: stale, Attested: c.Trust.Attested, Orphan: orphan, Entrypoint: entrypoint,
		})
		if orphan {
			r.Orphans = append(r.Orphans, c.ID)
		}
		if entrypoint {
			r.Entrypoints = append(r.Entrypoints, c.ID)
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
				broken = linkcheck.IsBrokenConceptRef(l.RawTarget) // couldn't resolve an internal .md ref
			}
			if broken {
				r.Unresolved = append(r.Unresolved, Edge{From: c.ID, RawTarget: l.RawTarget, Text: l.Text})
			}
		}
		// Residual wikilinks: any [[...]] still in the body is an unresolved
		// reference (resolved ones were rewritten to markdown links at convert time),
		// but it is not a markdown link so the codec's LinkGraph never surfaces it.
		// Scan the persisted body directly so `review` reports it (design-v2 §4.2).
		for _, target := range linkcheck.ResidualWikilinks(c.Body) {
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
	fmt.Fprintf(&b, "  entrypoints (outbound, no inbound): %d\n", len(r.Entrypoints))
	for _, id := range r.Entrypoints {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  orphans (no inbound or outbound links): %d\n", len(r.Orphans))
	for _, id := range r.Orphans {
		fmt.Fprintf(&b, "    %s\n", id)
	}
	fmt.Fprintf(&b, "  unresolved links: %d\n", len(r.Unresolved))
	for _, e := range r.Unresolved {
		fmt.Fprintf(&b, "    %s -> %s\n", e.From, e.RawTarget)
	}
	return b.String()
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
