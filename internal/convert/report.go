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
	// StatusNotes carries the OKF §5.4 status-vocabulary notes for a run (issue
	// #23): non-conformant --status-map values (a warning naming the value, its
	// key and the legal set) and any opt-in canonicalization rewrites. It is
	// additive and always emitted, initialised to [] on a conformant run so the
	// binder.report/v1 envelope keeps a stable shape (matching PR #56's
	// empty-array fix for infer.warnings); a nil slice would marshal to null.
	StatusNotes []string `json:"status_notes"`
	// Verified discloses the never-fabricate-trust decision for this run: which
	// actor (if any) was stamped, from where, every concept it stamped, and every
	// concept where a stamp was DECLINED because a different identity had already
	// attested (Residual A). It is additive to binder.report/v1 and always emitted
	// so the decision is observable even when nothing was stamped — an opt-in the
	// user cannot observe taking effect is indistinguishable from auto-stamping.
	Verified VerifiedStampReport `json:"verified"`
}

// VerifiedStampReport is the run-level trust disclosure (Residual B). Slices are
// always initialized so --json serializes to [] rather than null, keeping the
// binder.report/v1 envelope shape stable.
type VerifiedStampReport struct {
	// Actor is the verifier that was stamped (or would be, under --dry-run), or ""
	// when no verifier was determined.
	Actor string `json:"actor"`
	// Source is where Actor came from: "flag" | "env" | "config" | "none".
	Source string `json:"source"`
	// Stamped lists the out-relative concept paths that received the stamp, sorted.
	Stamped    []string `json:"stamped"`
	NumStamped int      `json:"num_stamped"`
	// Skipped lists concepts where a config/env stamp was declined because a
	// different identity had already attested them (Residual A), sorted by path.
	Skipped    []VerifiedSkip `json:"skipped"`
	NumSkipped int            `json:"num_skipped"`
	// Note is a human-readable disclosure of a resolved-but-unhonored verifier —
	// currently a repo-local .binder.yaml verified_by that Option A does not honor.
	// Empty when there is nothing to report.
	Note string `json:"note,omitempty"`
}

// VerifiedSkip is one concept where a config/env stamp was declined to avoid
// co-signing another identity's attestation (Residual A).
type VerifiedSkip struct {
	Path          string `json:"path"`           // out-relative concept path
	ExistingActor string `json:"existing_actor"` // the different identity already attesting
}

// NewVerifiedStampReport returns a disclosure with initialized (non-nil) slices,
// so --json serializes an empty run to [] rather than null. It is exported so the
// sibling enrich package builds an identically-shaped disclosure.
func NewVerifiedStampReport() VerifiedStampReport {
	return VerifiedStampReport{Source: "none", Stamped: []string{}, Skipped: []VerifiedSkip{}}
}

// ProseSection renders the human-readable trust-disclosure block (Residual B) for
// a run, or "" when there is nothing to disclose (no verifier determined, nothing
// skipped, no note — the never-stamp default). It is a method on the disclosure so
// convert and enrich render an identical block. The block always names the actor
// and its source when a stamp was written, lists every stamped and every skipped
// path, and surfaces any resolved-but-unhonored verifier note, so an opt-in the
// user cannot observe taking effect never looks like auto-stamping.
func (v VerifiedStampReport) ProseSection() string {
	if v.Actor == "" && v.NumSkipped == 0 && v.Note == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nTrust (verified stamps):\n")
	if v.Actor != "" {
		fmt.Fprintf(&b, "  actor: %s (source: %s)\n", v.Actor, v.Source)
		fmt.Fprintf(&b, "  stamped: %d file(s)\n", v.NumStamped)
		for _, p := range v.Stamped {
			fmt.Fprintf(&b, "    - %s\n", p)
		}
	}
	if v.NumSkipped > 0 {
		fmt.Fprintf(&b, "  skipped: %d file(s) (a different identity already attested; pass --verified-by to co-sign)\n", v.NumSkipped)
		for _, s := range v.Skipped {
			fmt.Fprintf(&b, "    - %s (already attested by %s)\n", s.Path, s.ExistingActor)
		}
	}
	if v.Note != "" {
		fmt.Fprintf(&b, "  note: %s\n", v.Note)
	}
	return b.String()
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
	if len(r.StatusNotes) > 0 {
		b.WriteString("\nStatus vocabulary (OKF §5.4):\n")
		for _, n := range r.StatusNotes {
			fmt.Fprintf(&b, "  - %s\n", n)
		}
	}
	b.WriteString(r.Verified.ProseSection())
	return b.String()
}
