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

	"github.com/ghchinoy/binder/internal/okf"
)

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
	Concepts    []ConceptView
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

	inbound := map[string]int{}
	for _, c := range b.Concepts {
		for _, l := range c.Links {
			if l.Resolved && l.TargetID != c.ID {
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
		for _, l := range c.Links {
			if !l.Resolved {
				r.Unresolved = append(r.Unresolved, Edge{From: c.ID, RawTarget: l.RawTarget, Text: l.Text})
			}
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

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
