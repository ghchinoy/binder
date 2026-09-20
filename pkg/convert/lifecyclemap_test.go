package convert

import (
	"testing"
	"time"
)

// fixedNow is a deterministic clock for relative-date tests (2023-11-14 UTC,
// matching SOURCE_DATE_EPOCH=1700000000 used elsewhere).
var fixedNow = time.Unix(1700000000, 0).UTC()

func TestLookupPrefixMapLongestWins(t *testing.T) {
	m := map[string]string{"docs": "A", "docs/guides": "B"}
	if got := lookupPrefixMap(m, "docs/guides/intro.md"); got != "B" {
		t.Errorf("longest prefix = %q, want B", got)
	}
	if got := lookupPrefixMap(m, "docs/ref.md"); got != "A" {
		t.Errorf("shorter prefix = %q, want A", got)
	}
	if got := lookupPrefixMap(m, "other/x.md"); got != "" {
		t.Errorf("no match = %q, want empty", got)
	}
}

func TestLookupPrefixMapTrimsSlashesAndTieBreaks(t *testing.T) {
	// Keys trimmed of surrounding "/".
	m := map[string]string{"/docs/": "A"}
	if got := lookupPrefixMap(m, "docs/x.md"); got != "A" {
		t.Errorf("trimmed key match = %q, want A", got)
	}
	// Equal-length keys tie-break lexicographically (aaa < bbb).
	m2 := map[string]string{"bbb": "B", "aaa": "A"}
	if got := lookupPrefixMap(m2, "aaa/x.md"); got != "A" {
		t.Errorf("tie-break = %q, want A", got)
	}
}

func TestParseStatusMap(t *testing.T) {
	prefixes, def, err := ParseStatusMap("archive=deprecated,drafts=draft,default=active")
	if err != nil {
		t.Fatalf("ParseStatusMap: %v", err)
	}
	if def != "active" {
		t.Errorf("default = %q, want active", def)
	}
	if prefixes["archive"] != "deprecated" || prefixes["drafts"] != "draft" {
		t.Errorf("prefixes = %v", prefixes)
	}
	if _, ok := prefixes["default"]; ok {
		t.Error("default key must be extracted, not left in the prefix map")
	}
}

func TestParseStatusMapMalformed(t *testing.T) {
	if _, _, err := ParseStatusMap("archive"); err == nil {
		t.Error("want error for entry without '='")
	}
}

func TestApplyStatusMapSetsWhenAbsentAndDefault(t *testing.T) {
	opts := Options{StatusMap: map[string]string{"archive": "deprecated"}, StatusDefault: "active"}

	// Prefix match sets status.
	c := newConcept("")
	c.RelPath = "archive/old.md"
	applyLifecycleMaps(c, "archive/old.md", opts)
	if v, _ := c.Frontmatter.Get("status"); v != "deprecated" {
		t.Errorf("status = %v, want deprecated", v)
	}

	// No prefix match → default.
	c2 := newConcept("")
	applyLifecycleMaps(c2, "notes/x.md", opts)
	if v, _ := c2.Frontmatter.Get("status"); v != "active" {
		t.Errorf("status = %v, want active (default)", v)
	}

	// Authored status is never clobbered.
	c3 := newConcept("", "status", "stable")
	applyLifecycleMaps(c3, "archive/old.md", opts)
	if v, _ := c3.Frontmatter.Get("status"); v != "stable" {
		t.Errorf("status = %v, want stable (never clobber)", v)
	}
}

func TestParseStaleAfterMapGrammar(t *testing.T) {
	if _, err := ParseStaleAfterMap("a=+6m,b=+1y,c=+0d"); err != nil {
		t.Fatalf("valid grammar rejected: %v", err)
	}
	for _, bad := range []string{"a=6m", "a=+6w", "a=+m", "a=-1d", "a=+1.5d"} {
		if _, err := ParseStaleAfterMap(bad); err == nil {
			t.Errorf("malformed %q accepted", bad)
		}
	}
}

func TestRelativeDateDeterministic(t *testing.T) {
	// 2023-11-14 UTC anchor.
	cases := map[string]string{
		"+0d":  "2023-11-14",
		"+6m":  "2024-05-14",
		"+1y":  "2024-11-14",
		"+10d": "2023-11-24",
	}
	for spec, want := range cases {
		if got := relativeDate(spec, fixedNow); got != want {
			t.Errorf("relativeDate(%q) = %q, want %q", spec, got, want)
		}
	}
}

func TestApplyStaleAfterSetsWhenAbsent(t *testing.T) {
	opts := Options{StaleAfterMap: map[string]string{"bench": "+6m"}, Now: fixedNow}

	c := newConcept("")
	applyLifecycleMaps(c, "bench/run.md", opts)
	if v, _ := c.Frontmatter.Get("stale_after"); v != "2024-05-14" {
		t.Errorf("stale_after = %v, want 2024-05-14", v)
	}

	// Never clobber authored stale_after.
	c2 := newConcept("", "stale_after", "2030-01-01")
	applyLifecycleMaps(c2, "bench/run.md", opts)
	if v, _ := c2.Frontmatter.Get("stale_after"); v != "2030-01-01" {
		t.Errorf("stale_after = %v, want 2030-01-01 (never clobber)", v)
	}
}

func TestLifecycleMapsNoOpWhenUnset(t *testing.T) {
	c := newConcept("", "draft", "true")
	before := len(c.Frontmatter.Keys())
	applyLifecycleMaps(c, "any/x.md", Options{})
	if len(c.Frontmatter.Keys()) != before {
		t.Errorf("applyLifecycleMaps mutated frontmatter with no options set: %v", c.Frontmatter.Keys())
	}
}
