// version.test.mjs — the "no hand-typed version" guardrail (design §4.7,
// acceptance §9.6). binder's ethos: never assert the unverified; determinism.
// The version a reader sees MUST be DERIVED at build time from src/lib/version.ts
// (which reads the GitHub Releases API), never typed into a page by hand.
//
// This proves that two ways:
//   1. No authored site source (index.astro, components, lib) hard-codes a
//      current-version literal (an X.Y.Z / vX.Y.Z). The byte-derived docs pages
//      are NOT authored site source — they are prose copied from docs/ — so this
//      check is scoped to the files the site itself writes.
//   2. The RENDERED badge in dist/ matches what version.ts resolved for this
//      build, read back from the /version.json endpoint (same memoized fetch, so
//      badge and JSON can never disagree). The displayed form is the BARE
//      version (no leading "v"), or the honest "latest" fallback.

import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { dist, srcRoot, read, distHtmlFiles, walk } from "./_helpers.mjs";

// A three-component version literal, optionally "v"-prefixed. This is the shape
// a hand-typed "current version" would take (e.g. `0.4.0` or `v0.4.0`). Note a
// spec reference like "OKF v0.2" is only TWO components and does not match.
const VERSION_LITERAL = /v?\d+\.\d+\.\d+/;

// The authored site source that must never hard-code a version: EVERY page and
// component source under src/pages and src/components (globbed, not a fixed
// list — so a NEW page that hardcodes a version is caught in future). This
// deliberately EXCLUDES src/content/docs/_generated (byte-derived docs prose,
// which legitimately carries version numbers from the source docs) — that dir
// lives under src/content, not under pages/components, so it is never scanned.
// version.ts (under src/lib) has its own dedicated check below.
const SOURCE_EXT = /\.(astro|ts|tsx|js|jsx|mjs|cjs|md|mdx)$/;
async function authoredSources() {
  const roots = [join(srcRoot, "pages"), join(srcRoot, "components")];
  const files = [];
  for (const r of roots) {
    if (!existsSync(r)) continue;
    for (const f of await walk(r)) {
      if (SOURCE_EXT.test(f) && !f.split(/[\\/]/).includes("_generated")) {
        files.push(f);
      }
    }
  }
  return files;
}

test("dist/ exists (build ran first)", () => {
  assert.ok(
    existsSync(dist),
    "dist/ not found — run `npm run build` before the test suite",
  );
});

test("no authored page/component source hard-codes a current-version literal", async () => {
  const files = await authoredSources();
  assert.ok(files.length > 0, "no authored page/component sources found to scan");
  const offenders = [];
  for (const file of files) {
    const body = await read(file);
    const hit = VERSION_LITERAL.exec(body);
    if (hit) offenders.push(`${file}: "${hit[0]}"`);
  }
  assert.deepEqual(
    offenders,
    [],
    `hand-typed version literal(s) found — the version must come from ` +
      `src/lib/version.ts, not page source:\n  ${offenders.join("\n  ")}`,
  );
});

test("version.ts stays the single source (no X.Y.Z literal baked into it)", async () => {
  const body = await read(join(srcRoot, "lib", "version.ts"));
  const hit = VERSION_LITERAL.exec(body);
  assert.equal(
    hit,
    null,
    `src/lib/version.ts contains a version literal "${hit && hit[0]}" — the ` +
      `current version must be fetched, never hard-coded (fallback is a word)`,
  );
});

test("/version.json is emitted and well-formed (tag/bare split correct)", async () => {
  const vjson = join(dist, "version.json");
  assert.ok(existsSync(vjson), "dist/version.json not emitted by the build");
  const data = JSON.parse(await read(vjson));

  assert.ok("tag" in data && "bare" in data && "display" in data);

  if (data.resolved) {
    // A real release was resolved at build time.
    assert.match(
      data.bare,
      /^\d+\.\d+\.\d+/,
      `bare version "${data.bare}" is not a semver`,
    );
    assert.ok(
      !data.bare.startsWith("v"),
      `bare version "${data.bare}" must NOT carry a leading "v"`,
    );
    assert.equal(
      data.tag,
      `v${data.bare}`,
      `tag "${data.tag}" must be the "v"-prefixed form of bare "${data.bare}"`,
    );
    assert.equal(data.display, data.bare, "display must be the bare form");
  } else {
    // Graceful fallback: no fabricated number, just the honest word.
    assert.equal(data.tag, null);
    assert.equal(data.bare, null);
    assert.equal(data.display, data.fallback);
    assert.ok(
      !VERSION_LITERAL.test(data.display),
      `fallback display "${data.display}" must not be a fabricated version`,
    );
  }
});

test("the rendered homepage badge matches version.ts (derived, not typed)", async () => {
  const data = JSON.parse(await read(join(dist, "version.json")));
  const home = join(dist, "index.html");
  assert.ok(existsSync(home), "dist/index.html not found");
  const html = await read(home);

  // The badge carries data-version so the check is precise, not a substring
  // guess. There must be at least one, and every one must equal version.ts's
  // resolved display value.
  const found = [...html.matchAll(/data-version=["']([^"']+)["']/g)].map(
    (m) => m[1],
  );
  assert.ok(
    found.length > 0,
    "no version badge (data-version) found in the built homepage",
  );
  for (const v of found) {
    assert.equal(
      v,
      data.display,
      `homepage badge shows "${v}" but version.ts resolved "${data.display}"`,
    );
  }

  // The value must also appear in the visible badge text, not just an attribute.
  assert.ok(
    html.includes(`<span class="vb-value">${data.display}</span>`),
    `visible badge value "${data.display}" not rendered in the homepage`,
  );
});

test("no OTHER built page hard-codes a version the badge does not source", async () => {
  // Belt-and-braces: the homepage is the only page that DISPLAYS the release
  // version as a badge. Assert no page invents a version badge with a value that
  // disagrees with version.ts (a stray hand-typed data-version elsewhere).
  const data = JSON.parse(await read(join(dist, "version.json")));
  for (const f of await distHtmlFiles()) {
    const html = await read(f);
    for (const m of html.matchAll(/data-version=["']([^"']+)["']/g)) {
      assert.equal(
        m[1],
        data.display,
        `${f} carries data-version="${m[1]}" — not sourced from version.ts`,
      );
    }
  }
});
