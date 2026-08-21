// trust.test.mjs — the derived trust rules must agree with binder's
// internal/okf/trust.go bit for bit. If they drift, the same bundle renders one
// tier here and reports another from the producer, and one of the two is a
// false statement about how much review the content has had.
//
// Each case below names the spec rule it pins.

import { test } from "node:test";
import assert from "node:assert/strict";

import { deriveTier, isHumanActor, isStale, isValidActor, toISODay } from "../dist/index.js";

test("spec 5.3: no verified entries means unverified", () => {
  assert.equal(deriveTier(undefined), "unverified");
  assert.equal(deriveTier([]), "unverified");
});

test("spec 5.3: only tool/process stamps reach machine-confirmed, never higher", () => {
  assert.equal(deriveTier([{ by: "binder/0.1.0" }]), "machine-confirmed");
  assert.equal(deriveTier([{ by: "process:nightly" }]), "machine-confirmed");
  assert.equal(
    deriveTier([{ by: "reference_agent/gemini-2.5-pro" }, { by: "binder/0.4.0" }]),
    "machine-confirmed",
  );
  // A team is not a human under spec 7's promotion rule.
  assert.equal(deriveTier([{ by: "team:data-platform" }]), "machine-confirmed");
});

test("spec 5.3: a single human: stamp promotes to human-reviewed", () => {
  assert.equal(deriveTier([{ by: "human:bob" }]), "human-reviewed");
  assert.equal(deriveTier([{ by: "binder/0.1.0" }, { by: "human:bob" }]), "human-reviewed");
  assert.equal(deriveTier([{ by: "human:bob" }, { by: "binder/0.1.0" }]), "human-reviewed");
});

test("spec 5.3: a near-miss prefix does not count as human review", () => {
  // These are the fabrication risk: anything that merely LOOKS human must not
  // be promoted.
  assert.equal(deriveTier([{ by: "human" }]), "machine-confirmed");
  assert.equal(deriveTier([{ by: "Human:bob" }]), "machine-confirmed");
  assert.equal(deriveTier([{ by: "not-human:bob" }]), "machine-confirmed");
  assert.equal(deriveTier([{ by: "humans:bob" }]), "machine-confirmed");
  assert.equal(deriveTier([{ by: "" }]), "machine-confirmed");
});

test("isHumanActor matches okf.IsHumanActor exactly", () => {
  assert.equal(isHumanActor("human:alice"), true);
  // "human:" with an empty id still carries the prefix, which is what Go's
  // IsHumanActor reports (a bare length-and-prefix check). The empty id is an
  // advisory concern, not a tier question. Pinned here so a later "tidy-up"
  // cannot quietly change the tier this repo and binder agree on.
  assert.equal(isHumanActor("human:"), true);
  assert.equal(isHumanActor("process:etl"), false);
  assert.equal(isHumanActor(""), false);
});

test("spec 7: isValidActor matches okf.IsValidActor", () => {
  assert.equal(isValidActor("human:alice"), true);
  assert.equal(isValidActor("process:nightly"), true);
  assert.equal(isValidActor("team:data"), true);
  assert.equal(isValidActor("binder/0.1.0"), true);
  assert.equal(isValidActor("reference_agent/gemini-2.5-pro"), true);
  // Prefix with nothing after it.
  assert.equal(isValidActor("human:"), false);
  // Slash at either end, or absent.
  assert.equal(isValidActor("/0.1.0"), false);
  assert.equal(isValidActor("binder/"), false);
  assert.equal(isValidActor("binder"), false);
  // Whitespace anywhere disqualifies the producer/version form.
  assert.equal(isValidActor("binder 1/0.1.0"), false);
  assert.equal(isValidActor(""), false);
  assert.equal(isValidActor("   "), false);
});

test("spec 5.5: staleness is today >= stale_after, boundary included", () => {
  assert.equal(isStale(undefined, "2026-08-21"), false, "no stale_after is never stale");
  assert.equal(isStale("", "2026-08-21"), false, "empty stale_after is never stale");
  assert.equal(isStale("2026-08-22", "2026-08-21"), false, "before the date");
  assert.equal(isStale("2026-08-21", "2026-08-21"), true, "ON the date is stale (>=)");
  assert.equal(isStale("2026-08-20", "2026-08-21"), true, "after the date");
  // Ordering across year/month boundaries, the case a naive comparison breaks.
  assert.equal(isStale("2020-01-01", "2026-08-21"), true);
  assert.equal(isStale("2027-01-01", "2026-12-31"), false);
});

test("toISODay reads the UTC day, not the local one", () => {
  assert.equal(toISODay(new Date("2026-08-21T00:00:00Z")), "2026-08-21");
  // 23:30 UTC is already the next day in some local zones; the UTC day is what
  // makes the build reproducible wherever it runs.
  assert.equal(toISODay(new Date("2026-08-21T23:30:00Z")), "2026-08-21");
});
