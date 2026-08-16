package convert

import (
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

// verifiedList reads the verified value as a []any for assertions.
func verifiedList(t *testing.T, c *okf.Concept) []any {
	t.Helper()
	v, ok := c.Frontmatter.Get("verified")
	if !ok {
		return nil
	}
	list, _ := v.([]any)
	return list
}

func TestVerifiedByNoStampWhenEmpty(t *testing.T) {
	c := newConcept("")
	applyVerifiedBy(c, Options{Now: fixedNow}) // no actor
	if _, ok := c.Frontmatter.Get("verified"); ok {
		t.Error("verified stamp written with no actor configured (never auto-stamp)")
	}
}

func TestVerifiedByAppends(t *testing.T) {
	c := newConcept("")
	applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow})
	list := verifiedList(t, c)
	if len(list) != 1 {
		t.Fatalf("verified len = %d, want 1", len(list))
	}
	m := list[0].(map[string]any)
	if okf.AsString(m["by"]) != "human:ghchinoy" {
		t.Errorf("by = %v, want human:ghchinoy", m["by"])
	}
	if okf.AsString(m["at"]) != "2023-11-14T22:13:20Z" {
		t.Errorf("at = %v, want deterministic RFC3339 UTC", m["at"])
	}
}

func TestVerifiedByAppendsToExisting(t *testing.T) {
	c := newConcept("")
	c.Frontmatter.Set("verified", []any{
		map[string]any{"by": "process:old-bot", "at": "2020-01-01T00:00:00Z"},
	})
	// Explicit --verified-by is the co-sign path (Residual A permits it); the
	// mechanic under test is that appending preserves the prior stamp.
	applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow, VerifiedByExplicit: true})
	list := verifiedList(t, c)
	if len(list) != 2 {
		t.Fatalf("verified len = %d, want 2 (append preserves prior)", len(list))
	}
}

func TestVerifiedByDedupIdempotent(t *testing.T) {
	c := newConcept("")
	opts := Options{VerifiedBy: "human:ghchinoy", Now: fixedNow}
	applyVerifiedBy(c, opts)
	applyVerifiedBy(c, opts) // re-run with same clock → idempotent
	if list := verifiedList(t, c); len(list) != 1 {
		t.Errorf("verified len = %d, want 1 (dedup by by,at)", len(list))
	}
}

func TestVerifiedByBareMappingNormalized(t *testing.T) {
	// spec §5.2: a bare {by,at} mapping is a one-element list; appending must
	// normalize to a list and preserve the original.
	c := newConcept("")
	c.Frontmatter.Set("verified", map[string]any{"by": "process:bot", "at": "2020-01-01T00:00:00Z"})
	// Explicit co-sign path (a bare mapping is a different identity here).
	applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow, VerifiedByExplicit: true})
	list := verifiedList(t, c)
	if len(list) != 2 {
		t.Fatalf("verified len = %d, want 2", len(list))
	}
}

func TestVerifiedByPreservesSpecInvalidScalar(t *testing.T) {
	// A spec-invalid SCALAR verified value must be PRESERVED unchanged (never
	// silently dropped) and reported via an advisory; no stamp is appended.
	c := newConcept("")
	c.Frontmatter.Set("verified", "reviewed by bob")
	res := applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow})
	if res.Advisory == "" {
		t.Fatal("expected an advisory for a spec-invalid scalar verified value")
	}
	if res.Stamped {
		t.Error("must not stamp over a spec-invalid scalar value")
	}
	v, _ := c.Frontmatter.Get("verified")
	if s, _ := v.(string); s != "reviewed by bob" {
		t.Errorf("authored scalar was not preserved: got %#v", v)
	}
}

// TestVerifiedByResidualASkipsNonExplicitCoSign proves the Residual A guard: a
// non-explicit (global config) actor must NOT co-sign a document a DIFFERENT identity
// has already attested. It is a SKIP — the result reports Skipped with the existing
// actor, and the pre-existing attestation is left byte-identical (no append, no
// reshape).
func TestVerifiedByResidualASkipsNonExplicitCoSign(t *testing.T) {
	c := newConcept("")
	c.Frontmatter.Set("verified", []any{
		map[string]any{"by": "human:ahormati", "at": "2020-01-01T00:00:00Z"},
	})
	res := applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow, VerifiedByExplicit: false})

	if !res.Skipped {
		t.Fatal("non-explicit actor co-signed a different identity (Residual A violated)")
	}
	if res.Stamped {
		t.Error("Skipped result must not also be Stamped")
	}
	if res.ExistingActor != "human:ahormati" {
		t.Errorf("ExistingActor = %q, want human:ahormati", res.ExistingActor)
	}
	// Anti-vacuity + no-reshape: the list is unchanged (still exactly the one prior
	// stamp, same actor, same timestamp).
	list := verifiedList(t, c)
	if len(list) != 1 {
		t.Fatalf("verified len = %d, want 1 (skip must not append)", len(list))
	}
	m := list[0].(map[string]any)
	if okf.AsString(m["by"]) != "human:ahormati" || okf.AsString(m["at"]) != "2020-01-01T00:00:00Z" {
		t.Errorf("skip reshaped the prior attestation: got %#v", m)
	}
}

// TestVerifiedByExplicitMayCoSign is the anti-vacuity control for the skip above:
// the SAME different-identity document, stamped with an EXPLICIT actor, DOES
// co-sign (two stamps), proving the guard keys on explicitness, not on presence of
// a prior stamp.
func TestVerifiedByExplicitMayCoSign(t *testing.T) {
	c := newConcept("")
	c.Frontmatter.Set("verified", []any{
		map[string]any{"by": "human:ahormati", "at": "2020-01-01T00:00:00Z"},
	})
	res := applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow, VerifiedByExplicit: true})

	if !res.Stamped {
		t.Fatal("explicit actor failed to co-sign (should be permitted)")
	}
	if res.Skipped {
		t.Error("explicit co-sign must not be reported as Skipped")
	}
	if list := verifiedList(t, c); len(list) != 2 {
		t.Fatalf("verified len = %d, want 2 (explicit co-sign appends)", len(list))
	}
}

// TestVerifiedByNonExplicitSameIdentityAppends confirms the guard is scoped to a
// DIFFERENT identity: a non-explicit actor may still add its own timestamp to a
// document it alone has attested (no other identity present).
func TestVerifiedByNonExplicitSameIdentityAppends(t *testing.T) {
	c := newConcept("")
	c.Frontmatter.Set("verified", []any{
		map[string]any{"by": "human:ghchinoy", "at": "2020-01-01T00:00:00Z"},
	})
	res := applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow, VerifiedByExplicit: false})
	if res.Skipped {
		t.Fatal("skip triggered for the SAME identity (guard must key on a DIFFERENT identity)")
	}
	if !res.Stamped {
		t.Error("non-explicit same-identity re-attestation should append a new timestamp")
	}
}

func TestVerifiedByTierDerived(t *testing.T) {
	// A human: actor derives human-reviewed; a tool/process actor derives
	// machine-confirmed. The tier is never stored, only computed.
	human := newConcept("")
	applyVerifiedBy(human, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow})
	human.Trust = okf.ProjectTrust(human.Frontmatter, human.Type)
	if tier := okf.TrustTier(human); tier != okf.TierHumanReviewed {
		t.Errorf("human tier = %q, want human-reviewed", tier)
	}
	if human.Frontmatter.Has("tier") {
		t.Error("tier must not be stored in frontmatter (stays derived)")
	}

	bot := newConcept("")
	applyVerifiedBy(bot, Options{VerifiedBy: "binder/0.1.0", Now: fixedNow})
	bot.Trust = okf.ProjectTrust(bot.Frontmatter, bot.Type)
	if tier := okf.TrustTier(bot); tier != okf.TierMachineConfirmed {
		t.Errorf("bot tier = %q, want machine-confirmed", tier)
	}
}
