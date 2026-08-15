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
	applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow})
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
	applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow})
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
	adv := applyVerifiedBy(c, Options{VerifiedBy: "human:ghchinoy", Now: fixedNow})
	if adv == "" {
		t.Fatal("expected an advisory for a spec-invalid scalar verified value")
	}
	v, _ := c.Frontmatter.Get("verified")
	if s, _ := v.(string); s != "reviewed by bob" {
		t.Errorf("authored scalar was not preserved: got %#v", v)
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
