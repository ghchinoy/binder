// clean-urls.test.mjs — the site ships its docs at clean, permanent URLs and
// NEVER leaks the private `_generated/` staging path that prepare-content.mjs
// writes to on disk (design §4.7, acceptance §9.6). It asserts on the BUILT
// output in dist/ — the bytes GitHub Pages actually serves.

import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { join, relative } from "node:path";
import { dist, walk, distHtmlFiles, read } from "./_helpers.mjs";

// A representative set of the clean routes the IA promises. Each must build to
// its own index.html under the clean slug — not under _generated/.
const CLEAN_ROUTES = [
  "overview",
  "install",
  "tutorial",
  "concepts/byte-faithfulness",
  "concepts/trust",
  "concepts/graph",
  "concepts/okf-output",
  "concepts/project",
  "concepts/lpg-primer",
  "guides/ci",
  "guides/strict-mode",
  "reference/user-guide",
  "reference/commands",
  "agent/mcp",
  "agent/plugin",
  "project/releasing",
  "project/contributing",
];

test("dist/ exists (build ran first)", () => {
  assert.ok(existsSync(dist), "dist/ not found — run `npm run build` first");
});

test("every doc is served at its clean URL", () => {
  for (const route of CLEAN_ROUTES) {
    const page = join(dist, route, "index.html");
    assert.ok(
      existsSync(page),
      `expected clean route /${route}/ to build to ${relative(dist, page)}`,
    );
  }
});

test("no built file path contains a _generated/ segment", async () => {
  const offenders = (await walk(dist))
    .map((f) => relative(dist, f))
    .filter((r) => r.split(/[\\/]/).includes("_generated"));
  assert.deepEqual(
    offenders,
    [],
    `these built paths leak the _generated/ prefix: ${offenders.join(", ")}`,
  );
});

test("no published HTML links or references a _generated path", async () => {
  const offenders = [];
  for (const f of await distHtmlFiles()) {
    const html = await read(f);
    if (/_generated\//.test(html)) offenders.push(relative(dist, f));
  }
  assert.deepEqual(
    offenders,
    [],
    `these pages reference a _generated path: ${offenders.join(", ")}`,
  );
});

test("no built URL string anywhere in dist contains _generated", async () => {
  // Catch text assets too (sitemap, json, txt) — not just rendered HTML.
  const offenders = [];
  for (const f of await walk(dist)) {
    if (/\.(html|xml|json|txt)$/.test(f)) {
      const body = await read(f);
      if (body.includes("/_generated/")) offenders.push(relative(dist, f));
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `these assets contain a /_generated/ URL: ${offenders.join(", ")}`,
  );
});

test("sitemap lists only clean URLs (no _generated)", async () => {
  const idx = join(dist, "sitemap-index.xml");
  const sm = join(dist, "sitemap-0.xml");
  assert.ok(existsSync(idx) || existsSync(sm), "no sitemap emitted in dist/");
  if (existsSync(sm)) {
    const xml = await read(sm);
    assert.ok(!xml.includes("_generated"), "sitemap-0.xml has a _generated URL");
  }
});
