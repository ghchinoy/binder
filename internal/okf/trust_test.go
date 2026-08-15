package okf

import "testing"

func fmFrom(pairs ...any) *OrderedMap {
	m := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1])
	}
	return m
}

func TestProjectTrustBareVerifiedMappingBecomesList(t *testing.T) {
	// Spec §5.2 / §11: a bare { by, at } mapping MUST be treated as a one-element list.
	fm := fmFrom("verified", map[string]any{"by": "human:alice", "at": "2026-01-01T00:00:00Z"})
	ts := ProjectTrust(fm, "Metric")
	if len(ts.Verified) != 1 {
		t.Fatalf("bare verified mapping should normalize to 1-element list, got %d", len(ts.Verified))
	}
	if ts.Verified[0].By != "human:alice" {
		t.Fatalf("unexpected verifier: %+v", ts.Verified[0])
	}
}

func TestProjectTrustVerifiedList(t *testing.T) {
	fm := fmFrom("verified", []any{
		map[string]any{"by": "human:alice", "at": "2026-01-01T00:00:00Z"},
		map[string]any{"by": "process:nightly", "at": "2026-01-02T00:00:00Z"},
	})
	ts := ProjectTrust(fm, "Metric")
	if len(ts.Verified) != 2 {
		t.Fatalf("want 2 verifiers, got %d", len(ts.Verified))
	}
}

func TestProjectTrustGeneratedAndSources(t *testing.T) {
	fm := fmFrom(
		"generated", map[string]any{"by": "binder/0.1.0", "at": "2026-01-01T00:00:00Z"},
		"status", "draft",
		"stale_after", "2026-09-23",
		"sources", []any{map[string]any{"id": "s1", "resource": "https://x", "title": "T", "author": "team:a"}},
	)
	ts := ProjectTrust(fm, "Note")
	if ts.Generated == nil || ts.Generated.By != "binder/0.1.0" {
		t.Fatalf("generated not projected: %+v", ts.Generated)
	}
	if ts.Status != "draft" || ts.StaleAfter != "2026-09-23" {
		t.Fatalf("status/stale_after not projected: %q %q", ts.Status, ts.StaleAfter)
	}
	if len(ts.Sources) != 1 || ts.Sources[0].ID != "s1" || ts.Sources[0].Author != "team:a" {
		t.Fatalf("sources not projected: %+v", ts.Sources)
	}
}

func TestTrustTier(t *testing.T) {
	cases := []struct {
		name string
		fm   *OrderedMap
		want Tier
	}{
		{"no verified", NewOrderedMap(), TierUnverified},
		{"machine only", fmFrom("verified", []any{map[string]any{"by": "process:nightly"}}), TierMachineConfirmed},
		{"agent only", fmFrom("verified", []any{map[string]any{"by": "reference_agent/gemini"}}), TierMachineConfirmed},
		{"has human", fmFrom("verified", []any{
			map[string]any{"by": "process:nightly"},
			map[string]any{"by": "human:alice"},
		}), TierHumanReviewed},
		{"bare human mapping", fmFrom("verified", map[string]any{"by": "human:alice"}), TierHumanReviewed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Concept{Trust: ProjectTrust(tc.fm, "Metric")}
			if got := TrustTier(c); got != tc.want {
				t.Fatalf("TrustTier = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsStale(t *testing.T) {
	cases := []struct {
		staleAfter string
		today      string
		want       bool
	}{
		{"", "2026-12-31", false},           // no stale_after ⇒ never stale
		{"2026-09-23", "2026-09-22", false}, // before
		{"2026-09-23", "2026-09-23", true},  // on the day (today >= stale_after)
		{"2026-09-23", "2026-09-24", true},  // after
	}
	for _, tc := range cases {
		c := &Concept{Trust: TrustSignals{StaleAfter: tc.staleAfter}}
		if got := IsStale(c, tc.today); got != tc.want {
			t.Fatalf("IsStale(%q, today=%q) = %v, want %v", tc.staleAfter, tc.today, got, tc.want)
		}
	}
}

func TestValidateTrustIsAdvisoryOnly(t *testing.T) {
	fm := fmFrom(
		"type", AttestedComputationType,
		"generated", map[string]any{"at": "2026-01-01T00:00:00Z"}, // missing by
		"verified", []any{map[string]any{"at": "2026-01-01T00:00:00Z"}}, // missing by
		// no runtime on an Attested Computation
	)
	c := &Concept{ID: "c", Type: AttestedComputationType, Frontmatter: fm, Trust: ProjectTrust(fm, AttestedComputationType)}
	findings := ValidateTrust(c, SpecV02)
	if len(findings) == 0 {
		t.Fatal("expected advisory findings")
	}
	for _, f := range findings {
		if f.Severity != SeverityAdvisory {
			t.Fatalf("ValidateTrust must only emit advisories, got %q: %s", f.Severity, f.Message)
		}
	}
}

func TestValidateTrustCleanConcept(t *testing.T) {
	fm := fmFrom(
		"type", "Note",
		"generated", map[string]any{"by": "binder/0.1.0", "at": "2026-01-01T00:00:00Z"},
		"verified", []any{map[string]any{"by": "human:alice", "at": "2026-02-01T09:00:00Z"}},
		"status", "stable",
		"stale_after", "2027-01-01",
		"sources", []any{map[string]any{
			"id": "s1", "resource": "https://x", "author": "team:finance", "last_modified": "2026-06-15",
		}},
		"usage_window", map[string]any{"from": "2026-06-01", "to": "2026-06-30"},
	)
	c := &Concept{ID: "c", Type: "Note", Frontmatter: fm, Trust: ProjectTrust(fm, "Note")}
	if findings := ValidateTrust(c, SpecV02); len(findings) != 0 {
		t.Fatalf("expected no findings on a well-formed concept, got %v", findings)
	}
}

func TestIsValidActor(t *testing.T) {
	valid := []string{"human:alice", "process:nightly", "team:finance", "reference_agent/gemini-2.5-pro", "binder/0.1.0"}
	for _, a := range valid {
		if !IsValidActor(a) {
			t.Errorf("IsValidActor(%q) = false, want true", a)
		}
	}
	invalid := []string{"", "human:", "alice", "just text", "no space/ok but has space", "/leading", "trailing/"}
	for _, a := range invalid {
		if IsValidActor(a) {
			t.Errorf("IsValidActor(%q) = true, want false", a)
		}
	}
}

func TestIsValidISODates(t *testing.T) {
	if !IsValidISODate("2026-12-31") {
		t.Error("2026-12-31 should be a valid date")
	}
	if IsValidISODate("2026-13-40") || IsValidISODate("not-a-date") {
		t.Error("bad dates should be invalid")
	}
	if !IsValidISODateTime("2026-06-30T14:00:00Z") {
		t.Error("RFC3339 datetime should be valid")
	}
	if !IsValidISODateTime("2026-06-30") {
		t.Error("date-only content stamp should be tolerated as a datetime")
	}
	if IsValidISODateTime("yesterday") {
		t.Error("garbage datetime should be invalid")
	}
}

func TestValidateTrustShapeAdvisories(t *testing.T) {
	fm := fmFrom(
		"type", "Metric",
		"generated", map[string]any{"by": "not-an-actor", "at": "whenever"},
		"verified", []any{map[string]any{"by": "alice", "at": "2026-01-01T00:00:00Z"}},
		"status", "wip",
		"stale_after", "31-12-2026",
		"sources", []any{map[string]any{"id": "s1", "author": "somebody", "last_modified": "nope"}}, // missing resource
		"usage_window", map[string]any{"from": "2026-06-01", "to": "later"},
	)
	c := &Concept{ID: "c", Type: "Metric", Frontmatter: fm, Trust: ProjectTrust(fm, "Metric")}
	findings := ValidateTrust(c, SpecV02)
	wantSubstrings := []string{
		"generated.by", "generated.at", "verified[0].by", "status", "stale_after",
		"sources[0] is missing required 'resource'", "sources[0].author", "sources[0].last_modified",
		"usage_window.to",
	}
	joined := ""
	for _, f := range findings {
		if f.Severity != SeverityAdvisory {
			t.Fatalf("all findings must be advisory, got %q", f.Severity)
		}
		joined += f.Message + "\n"
	}
	for _, want := range wantSubstrings {
		if !contains(joined, want) {
			t.Errorf("missing advisory containing %q; got:\n%s", want, joined)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
