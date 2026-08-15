package review

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// concept is a tiny helper to build a concept with a title and links.
func concept(id, typ, title string, links ...okf.Link) *okf.Concept {
	fm := okf.NewOrderedMap()
	if title != "" {
		fm.Set("title", title)
	}
	return &okf.Concept{ID: id, RelPath: id + ".md", Type: typ, Frontmatter: fm, Links: links}
}

func edge(to string) okf.Link {
	return okf.Link{TargetID: to, RawTarget: to, Resolved: true}
}

func TestReviewCountsTypesTiersOrphansUnresolved(t *testing.T) {
	intro := concept("intro", "Note", "Intro", edge("guide"), okf.Link{RawTarget: "missing.md", Resolved: false})
	guide := concept("guide", "Guide", "Guide")
	// human-reviewed, and stale as of the review date.
	guide.Trust = okf.TrustSignals{
		Verified:   []okf.Actorstamp{{By: "human:alice", At: "2026-01-01T00:00:00Z"}},
		StaleAfter: "2026-06-01",
	}
	// machine-confirmed.
	table := concept("orders", "BigQuery Table", "Orders")
	table.Trust = okf.TrustSignals{Verified: []okf.Actorstamp{{By: "binder/0.1.0", At: "2026-01-01T00:00:00Z"}}}
	// attested computation.
	calc := concept("calc", okf.AttestedComputationType, "Calc")
	calc.Trust = okf.TrustSignals{Attested: true}

	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{intro, guide, table, calc}}
	r := Review(b, "2026-08-15")

	if r.NumConcepts != 4 {
		t.Fatalf("NumConcepts = %d, want 4", r.NumConcepts)
	}
	if r.ByType["Note"] != 1 || r.ByType["Guide"] != 1 || r.ByType["BigQuery Table"] != 1 {
		t.Errorf("ByType = %v", r.ByType)
	}
	if r.Tiers[okf.TierHumanReviewed] != 1 {
		t.Errorf("human-reviewed = %d, want 1", r.Tiers[okf.TierHumanReviewed])
	}
	if r.Tiers[okf.TierMachineConfirmed] != 1 {
		t.Errorf("machine-confirmed = %d, want 1", r.Tiers[okf.TierMachineConfirmed])
	}
	if r.Tiers[okf.TierUnverified] != 2 {
		t.Errorf("unverified = %d, want 2 (intro, calc)", r.Tiers[okf.TierUnverified])
	}

	// guide has an inbound edge from intro; everything else is an orphan.
	orphans := strings.Join(r.Orphans, ",")
	if strings.Contains(orphans, "guide") {
		t.Errorf("guide should not be an orphan (intro links to it): %q", orphans)
	}
	for _, id := range []string{"intro", "orders", "calc"} {
		if !contains(r.Orphans, id) {
			t.Errorf("expected %q among orphans %v", id, r.Orphans)
		}
	}

	if !contains(r.Stale, "guide") {
		t.Errorf("guide should be stale as of 2026-08-15: %v", r.Stale)
	}
	if !contains(r.Attested, "calc") {
		t.Errorf("calc should be reported attested: %v", r.Attested)
	}
	if len(r.Unresolved) != 1 || r.Unresolved[0].From != "intro" || r.Unresolved[0].RawTarget != "missing.md" {
		t.Errorf("Unresolved = %+v, want one intro->missing.md", r.Unresolved)
	}
}

func TestReviewStringIsDeterministic(t *testing.T) {
	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{
		concept("b", "Note", "B"),
		concept("a", "Note", "A", edge("b")),
	}}
	s1 := Review(b, "2026-08-15").String()
	s2 := Review(b, "2026-08-15").String()
	if s1 != s2 {
		t.Fatal("review output is not deterministic")
	}
	for _, want := range []string{"binder review", "concepts: 2", "trust tiers:", "orphans", "unresolved links: 0"} {
		if !strings.Contains(s1, want) {
			t.Errorf("review output missing %q:\n%s", want, s1)
		}
	}
}

func TestReviewIgnoresExternalAndAnchorTargets(t *testing.T) {
	c := concept("a", "Note", "A",
		okf.Link{RawTarget: "https://example.com/x", Resolved: false},
		okf.Link{RawTarget: "#section", Resolved: false},
		okf.Link{RawTarget: "mailto:x@example.com", Resolved: false},
		okf.Link{RawTarget: "missing.md", Resolved: false}, // the only real broken edge
	)
	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{c}}
	r := Review(b, "2026-08-15")
	if len(r.Unresolved) != 1 || r.Unresolved[0].RawTarget != "missing.md" {
		t.Errorf("Unresolved = %+v, want only missing.md (externals/anchors filtered)", r.Unresolved)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
