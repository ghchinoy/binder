// workspace-dep.test.mjs — the site is a member of the repo's npm workspace and
// resolves the in-tree `astro-okf` package, not a published copy.
//
// Why this is a test and not a comment: `"astro-okf": "*"` in site/package.json
// only means something if `npm ci` at the workspace root actually links
// packages/astro-okf into site/node_modules AND the package has been built.
// Both are easy to break silently (a stale lockfile, a CI step reordered so the
// site builds before the loader does). This asserts the link is live, so the
// dogfooding wiring fails loudly the day it stops working.
//
// It deliberately does NOT assert anything about rendered pages: the site does
// not render an OKF bundle today (its docs are hand-authored prose, not OKF
// concepts), and pretending otherwise would be the kind of unearned claim the
// rest of this suite exists to catch. The demo collection is a later,
// additive change.

import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

test("the workspace link resolves to the in-tree packages/astro-okf", () => {
  const manifest = require.resolve("astro-okf/package.json");
  assert.match(
    manifest.replaceAll("\\", "/"),
    /\/packages\/astro-okf\/package\.json$/,
    `astro-okf resolved to ${manifest}, which is not the in-tree package`,
  );
});

test("the built loader is importable from the site", async () => {
  const mod = await import("astro-okf");
  assert.equal(typeof mod.okfLoader, "function", "okfLoader is not exported");
  assert.equal(typeof mod.okfSchema, "function", "okfSchema is not exported");
});

test("the loader's derived trust rules are the ones binder computes", async () => {
  const { deriveTier, isStale } = await import("astro-okf");

  // Spec 5.3, mirroring internal/okf/trust.go: a human: stamp promotes to
  // human-reviewed, a tool stamp only reaches machine-confirmed, and no stamps
  // at all is unverified.
  assert.equal(deriveTier(undefined), "unverified");
  assert.equal(deriveTier([{ by: "binder/0.1.0" }]), "machine-confirmed");
  assert.equal(deriveTier([{ by: "binder/0.1.0" }, { by: "human:bob" }]), "human-reviewed");

  // Spec 5.5: today >= stale_after, and no stale_after is never stale.
  assert.equal(isStale(undefined, "2026-08-21"), false);
  assert.equal(isStale("2027-01-01", "2026-08-21"), false);
  assert.equal(isStale("2026-08-21", "2026-08-21"), true);
});
