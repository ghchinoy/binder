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
	r := Review(b, "2026-08-15", nil)

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

	// guide has an inbound edge from intro, so it is neither orphan nor entrypoint.
	// intro has outbound (to guide) but no inbound → ENTRYPOINT, not an orphan
	// (issue #24). orders and calc have no edges at all → true ORPHANS.
	orphans := strings.Join(r.Orphans, ",")
	if strings.Contains(orphans, "guide") {
		t.Errorf("guide should not be an orphan (intro links to it): %q", orphans)
	}
	if contains(r.Orphans, "intro") {
		t.Errorf("intro links out (to guide) so it is an entrypoint, not an orphan: %v", r.Orphans)
	}
	if !contains(r.Entrypoints, "intro") {
		t.Errorf("expected intro among entrypoints (no inbound links): %v", r.Entrypoints)
	}
	for _, id := range []string{"orders", "calc"} {
		if !contains(r.Orphans, id) {
			t.Errorf("expected %q among orphans %v", id, r.Orphans)
		}
		if contains(r.Entrypoints, id) {
			t.Errorf("%q has no edges; it is a true orphan, not an entrypoint: %v", id, r.Entrypoints)
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
	s1 := Review(b, "2026-08-15", nil).String()
	s2 := Review(b, "2026-08-15", nil).String()
	if s1 != s2 {
		t.Fatal("review output is not deterministic")
	}
	for _, want := range []string{"binder review", "concepts: 2", "trust tiers:", "orphans", "unresolved links: 0"} {
		if !strings.Contains(s1, want) {
			t.Errorf("review output missing %q:\n%s", want, s1)
		}
	}
}

func TestReviewOnlyReportsBrokenConceptRefs(t *testing.T) {
	c := concept("a", "Note", "A",
		okf.Link{RawTarget: "https://example.com/x", Resolved: false}, // external
		okf.Link{RawTarget: "#section", Resolved: false},              // anchor
		okf.Link{RawTarget: "mailto:x@example.com", Resolved: false},  // mailto
		okf.Link{RawTarget: "assets/logo.png", Resolved: false},       // non-concept file
		okf.Link{RawTarget: "scripts/run.sh", Resolved: false},        // non-concept file
		okf.Link{RawTarget: "missing.md", Resolved: false},            // the only broken concept ref
		okf.Link{RawTarget: "gone.md#h", Resolved: false},             // broken concept ref w/ fragment
	)
	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{c}}
	r := Review(b, "2026-08-15", nil)
	if len(r.Unresolved) != 2 {
		t.Fatalf("Unresolved = %+v, want 2 (missing.md, gone.md#h)", r.Unresolved)
	}
	if r.Unresolved[0].RawTarget != "gone.md#h" || r.Unresolved[1].RawTarget != "missing.md" {
		t.Errorf("Unresolved = %+v, want the two .md refs (sorted)", r.Unresolved)
	}
}

func TestReviewReportsResolvedButNonexistentTarget(t *testing.T) {
	// The codec optimistically marks an in-bundle .md link resolved even if the
	// target concept does not exist; review must catch it via existence check.
	c := concept("a", "Note", "A", okf.Link{TargetID: "ghost", RawTarget: "ghost.md", Resolved: true})
	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{c}}
	r := Review(b, "2026-08-15", nil)
	if len(r.Unresolved) != 1 || r.Unresolved[0].RawTarget != "ghost.md" {
		t.Errorf("Unresolved = %+v, want ghost.md (resolved shape, no such concept)", r.Unresolved)
	}
}

func TestReviewDetectsRecoveredFrontmatterBody(t *testing.T) {
	// NOTE (#161): this test exercises ONLY the persisted-marker source of the
	// unparsed_frontmatter disclosure. It is deliberately BLIND to the I-2 defect
	// it superficially resembles: it builds concepts in memory and stamps
	// okf.MarkRecovered by hand, so it never puts a genuinely-unparseable file on
	// disk and never goes through bundle.Load — the exact path where the defect
	// lived (Load dropped the file, so it never reached review at all). Keep this
	// test for what it does prove (marker-driven reporting is uniform and does not
	// false-positive on a thematic-break body), but the end-to-end regression is
	// covered by TestReviewDiscloseUnparsedFromBundleField below and by the
	// cmd-level TestReviewStrictGatesOnUnparseableConcept — do NOT trust this one
	// to guard the load-time drop.

	// Recovery is reported from the persisted marker `binder convert` stamps, NOT
	// from body shape — so it is uniform across recovery kinds and never fires on a
	// clean file whose body merely opens with a "---" rule and a colon-bearing line.

	// Closed fence with invalid YAML (recovered as body): body kept verbatim + marker.
	recovered := concept("bad", "Note", "Bad")
	recovered.Body = "---\ntitle: thing: bad colon\ngoal: x\n---\n\n# Real\nBody.\n"
	okf.MarkRecovered(recovered.Frontmatter, "unparseable-frontmatter")
	// UNTERMINATED fence (recovered as body) — same marker, surfaced uniformly.
	unterm := concept("unterm", "Note", "Unterm")
	unterm.Body = "---\ntitle: never closed\ntags: [a, b]\n\n# Heading After\n\nStill body.\n"
	okf.MarkRecovered(unterm.Frontmatter, "unparseable-frontmatter")

	clean := concept("ok", "Note", "OK")
	clean.Body = "# OK\n\nJust markdown.\n"
	// FALSE-POSITIVE GUARD: a cleanly-parsed file whose BODY opens with a "---"
	// thematic break followed by a "key:"-looking callout. No marker => must NOT be
	// reported as recovered. A body-shape heuristic would wrongly flag this.
	callout := concept("callout", "Guide", "Deploy")
	callout.Body = "---\n\nWarning: this API is deprecated.\n"

	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{recovered, unterm, clean, callout}}
	r := Review(b, "2026-08-15", nil)
	// Concepts are visited sorted by ID: "bad" then "unterm"; "callout"/"ok" excluded.
	if len(r.UnparsedFrontmatter) != 2 ||
		r.UnparsedFrontmatter[0] != "bad" || r.UnparsedFrontmatter[1] != "unterm" {
		t.Errorf("UnparsedFrontmatter = %v, want [bad unterm] (marker-driven, no false positive)", r.UnparsedFrontmatter)
	}
}

// TestReviewDiscloseUnparsedFromBundleField is the review-side #161 regression
// the blind test above misses: it proves review reports a file the LOADER
// recorded as unparsed (Bundle.Unparsed) even when the concept carries NO
// persisted recovery marker. This is the genuinely-unparseable-file case — the
// file that used to be dropped before it could ever reach review. Before the fix,
// UnparsedFrontmatter was populated ONLY from the marker, so a bundle with an
// Unparsed entry but no marker reported an empty list; this test fails then and
// passes now.
func TestReviewDiscloseUnparsedFromBundleField(t *testing.T) {
	// A recovered concept as the loader produces it: no marker set here, to prove
	// the disclosure does not depend on the marker being present.
	rec := concept("bad", "", "")
	rec.Frontmatter = okf.NewOrderedMap() // deliberately unmarked
	b := &okf.Bundle{
		Root:     "/b",
		Concepts: []*okf.Concept{rec},
		Unparsed: []okf.UnparsedConcept{{ID: "bad", RelPath: "bad.md", Err: "invalid frontmatter"}},
	}
	r := Review(b, "2026-08-15", nil)
	if len(r.UnparsedFrontmatter) != 1 || r.UnparsedFrontmatter[0] != "bad" {
		t.Fatalf("UnparsedFrontmatter = %v, want [bad] (from Bundle.Unparsed, marker-independent)", r.UnparsedFrontmatter)
	}
}

// TestReviewUnparsedDedupesMarkerAndField proves a load-time recovery — which is
// BOTH recorded in Bundle.Unparsed AND marker-stamped by the loader — is counted
// exactly once, so --strict's finding count is not inflated by double-reporting.
func TestReviewUnparsedDedupesMarkerAndField(t *testing.T) {
	rec := concept("bad", "", "")
	okf.MarkRecovered(rec.Frontmatter, "unparseable-frontmatter")
	b := &okf.Bundle{
		Root:     "/b",
		Concepts: []*okf.Concept{rec},
		Unparsed: []okf.UnparsedConcept{{ID: "bad", RelPath: "bad.md", Err: "invalid frontmatter"}},
	}
	r := Review(b, "2026-08-15", nil)
	if len(r.UnparsedFrontmatter) != 1 || r.UnparsedFrontmatter[0] != "bad" {
		t.Fatalf("UnparsedFrontmatter = %v, want [bad] counted once (marker+field dedup)", r.UnparsedFrontmatter)
	}
}

func TestReviewReportsResidualWikilinkAsUnresolved(t *testing.T) {
	// A [[...]] left in a persisted body is by construction unresolved: convert
	// rewrites resolved wikilinks to markdown links and leaves only broken ones.
	c := concept("a", "Note", "A")
	c.Body = "# A\n\nSee [[Nonexistent Topic]] and [[Other|alias]].\n\n" +
		"In code it is ignored: `[[Not A Link]]`.\n"
	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{c}}
	r := Review(b, "2026-08-15", nil)
	if len(r.Unresolved) != 2 {
		t.Fatalf("Unresolved = %+v, want 2 residual wikilinks", r.Unresolved)
	}
	// Sorted by (from, raw target): [[Nonexistent Topic]] < [[Other]].
	if r.Unresolved[0].RawTarget != "[[Nonexistent Topic]]" || r.Unresolved[1].RawTarget != "[[Other]]" {
		t.Errorf("Unresolved = %+v, want the two residual wikilinks", r.Unresolved)
	}
	if !strings.Contains(r.String(), "unresolved links: 2") {
		t.Errorf("review output should report 2 unresolved links:\n%s", r.String())
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
