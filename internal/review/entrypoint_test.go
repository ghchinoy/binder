package review

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// conceptAt is like concept() but lets the test set an explicit RelPath so root
// entrypoint recognition (README.md) can be exercised.
func conceptAt(id, relPath, typ string, links ...okf.Link) *okf.Concept {
	fm := okf.NewOrderedMap()
	fm.Set("title", id)
	return &okf.Concept{ID: id, RelPath: relPath, Type: typ, Frontmatter: fm, Links: links}
}

// TestReviewRootReadmeIsEntrypointNotOrphan proves the issue #24 false positive is
// fixed generally: a root README that links out (no inbound) is an ENTRYPOINT, a
// non-README node with outbound-but-no-inbound is ALSO an entrypoint (not a README
// carve-out), and a node with no edges at all is still a true ORPHAN.
func TestReviewRootReadmeIsEntrypointNotOrphan(t *testing.T) {
	readme := conceptAt("README", "README.md", "Note", edge("guide"))
	start := conceptAt("start", "start.md", "Note", edge("guide")) // non-README entrypoint
	guide := conceptAt("guide", "guide.md", "Guide")               // linked-to → normal
	lonely := conceptAt("lonely", "lonely.md", "Note")             // no edges → true orphan

	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{readme, start, guide, lonely}}
	r := Review(b, "2026-08-15", nil)

	if !contains(r.Entrypoints, "README") {
		t.Errorf("root README must be an entrypoint, not an orphan: entrypoints=%v orphans=%v", r.Entrypoints, r.Orphans)
	}
	if !contains(r.Entrypoints, "start") {
		t.Errorf("a non-README node with outbound/no-inbound must be an entrypoint (general rule): %v", r.Entrypoints)
	}
	if contains(r.Orphans, "README") || contains(r.Orphans, "start") {
		t.Errorf("entrypoints must not appear among orphans: %v", r.Orphans)
	}
	if contains(r.Entrypoints, "guide") || contains(r.Orphans, "guide") {
		t.Errorf("guide has inbound links; it is neither entrypoint nor orphan: entrypoints=%v orphans=%v", r.Entrypoints, r.Orphans)
	}
	if !contains(r.Orphans, "lonely") {
		t.Errorf("a node with no inbound AND no outbound edges is a true orphan: %v", r.Orphans)
	}
	if contains(r.Entrypoints, "lonely") {
		t.Errorf("a true orphan must not be reclassified as an entrypoint: %v", r.Entrypoints)
	}

	// The per-concept view carries the flags too.
	for _, cv := range r.Concepts {
		switch cv.ID {
		case "README", "start":
			if !cv.Entrypoint || cv.Orphan {
				t.Errorf("%s: Entrypoint/Orphan = %v/%v, want true/false", cv.ID, cv.Entrypoint, cv.Orphan)
			}
		case "lonely":
			if cv.Entrypoint || !cv.Orphan {
				t.Errorf("lonely: Entrypoint/Orphan = %v/%v, want false/true", cv.Entrypoint, cv.Orphan)
			}
		}
	}

	// Prose surfaces both buckets with the disambiguating labels.
	s := r.String()
	if !strings.Contains(s, "entrypoints (no inbound links): 2") {
		t.Errorf("prose missing entrypoint count:\n%s", s)
	}
	if !strings.Contains(s, "orphans (no inbound or outbound links): 1") {
		t.Errorf("prose missing orphan count:\n%s", s)
	}
}

// TestReviewDesignatedEntrypoint: a true orphan (no edges) named via the
// entrypoints designation is reclassified as an entrypoint; matching works by
// concept id and by path form.
func TestReviewDesignatedEntrypoint(t *testing.T) {
	hub := conceptAt("hub", "hub.md", "Note")       // no edges → would be an orphan
	other := conceptAt("other", "other.md", "Note") // no edges → true orphan
	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{hub, other}}

	r := Review(b, "2026-08-15", []string{"hub.md"}) // path form
	if !contains(r.Entrypoints, "hub") {
		t.Errorf("designated 'hub.md' must be an entrypoint: %v", r.Entrypoints)
	}
	if contains(r.Orphans, "hub") {
		t.Errorf("designated entrypoint must not be an orphan: %v", r.Orphans)
	}
	if !contains(r.Orphans, "other") {
		t.Errorf("undesignated node with no edges is still a true orphan: %v", r.Orphans)
	}
}

// TestReviewRootReadmeWithoutOutboundStillEntrypoint: a root README with NO
// resolved outbound edges (its links go only to external URLs) is still an
// entrypoint by recognition, never a false-positive orphan.
func TestReviewRootReadmeWithoutOutboundStillEntrypoint(t *testing.T) {
	readme := conceptAt("README", "README.md", "Note",
		okf.Link{RawTarget: "https://example.com", Resolved: false})
	b := &okf.Bundle{Root: "/b", Concepts: []*okf.Concept{readme}}
	r := Review(b, "2026-08-15", nil)
	if !contains(r.Entrypoints, "README") || contains(r.Orphans, "README") {
		t.Errorf("recognized root README must be an entrypoint even with no resolved outbound edges: entrypoints=%v orphans=%v", r.Entrypoints, r.Orphans)
	}
}
