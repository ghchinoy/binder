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
		{"", "2026-12-31", false},          // no stale_after ⇒ never stale
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
	fm := fmFrom("type", "Note", "generated", map[string]any{"by": "binder/0.1.0", "at": "2026-01-01T00:00:00Z"})
	c := &Concept{ID: "c", Type: "Note", Frontmatter: fm, Trust: ProjectTrust(fm, "Note")}
	if findings := ValidateTrust(c, SpecV02); len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}
