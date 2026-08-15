package convert_test

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// TestTrustRoundTripByteFaithful proves that every v0.2 trust family survives a
// parse→serialize round-trip byte-for-byte (design-v2 §3.2), including flow-style
// mappings, bare-vs-list verified, usage_window, and the Attested Computation
// runtime — none of which binder understands well enough to safely re-encode.
func TestTrustRoundTripByteFaithful(t *testing.T) {
	fixtures := map[string]string{
		"sources+verified-list": `---
type: BigQuery Table
title: Orders
tags: [sales, orders]
generated: { by: human:alice, at: 2026-01-02T09:00:00Z }
status: stable
stale_after: 2027-01-01
usage_window: { from: 2026-01-01, to: 2026-12-31 }
verified:
  - by: human:bob
    at: 2026-02-01T10:00:00Z
  - by: binder/0.1.0
    at: 2026-02-02T10:00:00Z
sources:
  - id: schema-doc
    resource: https://example.com/orders-schema
    title: Orders schema
custom_key: preserved-verbatim
---

# Orders

Body text.
`,
		"verified-bare-mapping": `---
type: Note
title: Bare
generated: { by: binder/0.1.0, at: 2026-01-02T09:00:00Z }
verified: { by: human:carol, at: 2026-03-01T12:00:00Z }
---

# Bare

Single verified mapping (spec §5.2: treated as a one-element list).
`,
		"attested-computation": `---
type: Attested Computation
title: Revenue Calc
runtime: python:3.12
generated: { by: binder/0.1.0, at: 2026-01-02T09:00:00Z }
stale_after: 2020-01-01
---

# Revenue Calc

Attested body.
`,
	}

	codec := native.New()
	for name, raw := range fixtures {
		t.Run(name, func(t *testing.T) {
			c, err := codec.ParseConcept("x/doc.md", []byte(raw))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, err := codec.Serialize(c)
			if err != nil {
				t.Fatalf("serialize: %v", err)
			}
			if string(out) != raw {
				t.Errorf("round-trip not byte-faithful:\n--- want ---\n%s\n--- got ---\n%s", raw, out)
			}
		})
	}
}

// TestTrustDerivations checks the pure derivations (spec §5.3/§5.5): tier from
// verified events, staleness from stale_after, and Attested from concept type.
func TestTrustDerivations(t *testing.T) {
	codec := native.New()
	parse := func(raw string) *okf.Concept {
		c, err := codec.ParseConcept("x/doc.md", []byte(raw))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		c.Trust = okf.ProjectTrust(c.Frontmatter, c.Type)
		return c
	}

	// Human-reviewed: any verified event by a human: actor.
	human := parse("---\ntype: Note\nverified:\n  - by: binder/0.1.0\n    at: 2026-02-02T10:00:00Z\n  - by: human:bob\n    at: 2026-02-01T10:00:00Z\n---\n# h\n")
	if got := okf.TrustTier(human); got != okf.TierHumanReviewed {
		t.Errorf("tier = %q, want human-reviewed", got)
	}

	// Machine-confirmed: verified events, none human.
	machine := parse("---\ntype: Note\nverified: { by: binder/0.1.0, at: 2026-02-02T10:00:00Z }\n---\n# m\n")
	if got := okf.TrustTier(machine); got != okf.TierMachineConfirmed {
		t.Errorf("tier = %q, want machine-confirmed", got)
	}

	// Unverified: no verified events.
	unv := parse("---\ntype: Note\n---\n# u\n")
	if got := okf.TrustTier(unv); got != okf.TierUnverified {
		t.Errorf("tier = %q, want unverified", got)
	}

	// Staleness: today >= stale_after.
	stale := parse("---\ntype: Note\nstale_after: 2026-01-01\n---\n# s\n")
	if !okf.IsStale(stale, "2026-08-15") {
		t.Error("should be stale as of 2026-08-15")
	}
	if okf.IsStale(stale, "2025-12-31") {
		t.Error("should not be stale before stale_after")
	}
	if okf.IsStale(unv, "2026-08-15") {
		t.Error("no stale_after ⇒ never stale")
	}

	// Attested derives from type, never stored.
	att := parse("---\ntype: Attested Computation\nruntime: python:3.12\n---\n# a\n")
	if !att.Trust.Attested {
		t.Error("Attested Computation type should derive Attested=true")
	}
}

// TestValidateTrustNeverRejects confirms trust well-formedness problems are
// advisory only — a bundle is never rejected for them (spec §11).
func TestValidateTrustNeverRejects(t *testing.T) {
	codec := native.New()
	// Deliberately malformed trust: bad actor, bad dates, bad status, missing
	// runtime for an attested computation, source missing resource.
	raw := "---\n" +
		"type: Attested Computation\n" +
		"status: wobbly\n" +
		"stale_after: someday\n" +
		"generated: { by: \"no actor convention\", at: not-a-date }\n" +
		"verified:\n  - by: \"\"\n    at: 2026-13-99\n" +
		"usage_window: { from: nope, to: also-nope }\n" +
		"sources:\n  - title: no resource here\n" +
		"---\n\n# x\n"
	c, err := codec.ParseConcept("x/doc.md", []byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c.Trust = okf.ProjectTrust(c.Frontmatter, c.Type)
	findings := okf.ValidateTrust(c, okf.DefaultSpecVersion)
	if len(findings) == 0 {
		t.Fatal("expected advisory findings for malformed trust")
	}
	for _, f := range findings {
		if f.Severity == okf.SeverityError {
			t.Errorf("trust finding must never be an error (spec §11): %s", f)
		}
	}
	// And frontmatter still round-trips byte-faithfully despite the mess.
	out, err := codec.Serialize(c)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if !strings.Contains(string(out), "no actor convention") {
		t.Error("malformed trust values must be preserved, not dropped")
	}
}
