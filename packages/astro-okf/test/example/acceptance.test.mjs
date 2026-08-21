// acceptance.test.mjs — the Phase 1 acceptance gate, asserted against the HTML
// a real `astro build` produced from a real binder-emitted OKF bundle.
//
// The unit tests in ../ prove the derivation in isolation. This file proves the
// thing that actually matters: that the derivation survives the whole Astro
// pipeline and reaches the page correctly LABELLED. A tier that is right in
// memory and unmarked in the HTML is still a page presenting a computed value
// as if it were a stored one.
//
// Every expectation is re-derived from the source bundle text where it can be,
// rather than transcribed, so this suite cannot drift into agreeing with a
// wrong implementation.

import { test, before } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { readFile, readdir } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const pkgRoot = join(here, "..", "..");
const dist = join(pkgRoot, "example", "dist");
const bundle = join(pkgRoot, "..", "..", "testdata", "expected-rich");

const pages = new Map(); // concept id -> built HTML

async function walk(dir) {
  const out = [];
  for (const ent of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, ent.name);
    if (ent.isDirectory()) out.push(...(await walk(p)));
    else out.push(p);
  }
  return out;
}

before(async () => {
  assert.ok(
    existsSync(dist),
    "example/dist not found — run `npm run build:example --workspace astro-okf` first",
  );
  for (const file of await walk(join(dist, "kb"))) {
    if (!file.endsWith("index.html")) continue;
    const id = file
      .slice(join(dist, "kb").length + 1, -"/index.html".length)
      .replaceAll("\\", "/");
    pages.set(id, await readFile(file, "utf8"));
  }
});

test("acceptance: the bundle's concepts each built to a page", () => {
  assert.deepEqual(
    [...pages.keys()].sort(),
    ["attested/calc", "guides/index-note", "guides/setup", "intro", "tables/orders"],
  );
});

test("acceptance: tables/orders renders its body content, not just its metadata", () => {
  const html = pages.get("tables/orders");
  assert.match(html, /<h1[^>]*>\s*Schema\s*<\/h1>/, "the concept body did not render");
  assert.match(html, /One row per completed customer order\./, "the description is missing");
  assert.match(html, /BigQuery docs/, "the body's citation list is missing");
});

test("acceptance: tables/orders shows human-reviewed, explicitly marked as derived", () => {
  const html = pages.get("tables/orders");
  assert.match(html, /data-okf-tier="human-reviewed"/);
  assert.match(html, /data-okf-tier-derived="true"/);
  // The marker is not only machine-readable: a human reading the page is told.
  assert.match(html, /derived by astro-okf/i);
  // And the evidence is on the page, so the claim is checkable.
  assert.match(html, /human:bob/);
});

test("acceptance: a concept with no verified entries renders unverified", () => {
  for (const id of ["intro", "guides/setup", "guides/index-note"]) {
    assert.match(pages.get(id), /data-okf-tier="unverified"/, `${id} is not unverified`);
    assert.match(
      pages.get(id),
      /no evidence for any tier above unverified/i,
      `${id} does not say why it is unverified`,
    );
  }
});

test("acceptance: staleness renders with the date it was decided against", () => {
  // attested/calc has stale_after 2020-01-01 and the example pins now to
  // 2026-08-21, so it is stale.
  const calc = pages.get("attested/calc");
  assert.match(calc, /data-okf-stale="true"/);
  assert.match(calc, /2020-01-01/, "the stale_after date is not shown");
  assert.match(calc, /2026-08-21/, "the date staleness was evaluated on is not shown");

  // tables/orders has stale_after 2027-01-01, which the pinned clock has not
  // reached.
  assert.match(pages.get("tables/orders"), /data-okf-stale="false"/);

  // intro has no stale_after at all, which is neither stale nor fresh.
  assert.match(pages.get("intro"), /data-okf-stale="unset"/);
});

test("no page presents a trust tier without the derived marker", () => {
  for (const [id, html] of pages) {
    const tiers = [...html.matchAll(/data-okf-tier="([^"]+)"/g)];
    assert.ok(tiers.length > 0, `${id} renders no tier at all`);
    for (const m of tiers) {
      const around = html.slice(Math.max(0, m.index - 400), m.index + 400);
      assert.match(
        around,
        /data-okf-tier-derived="true"/,
        `${id} renders tier ${m[1]} without the derived marker`,
      );
    }
  }
});

test("no page claims a tier its source frontmatter cannot support", async () => {
  // Re-derived from the bundle TEXT, independently of the loader's own code
  // path, so this check cannot pass merely because the implementation agrees
  // with itself.
  for (const [id, html] of pages) {
    const source = await readFile(join(bundle, `${id}.md`), "utf8");
    const fm = source.startsWith("---") ? source.slice(3, source.indexOf("\n---", 3)) : "";
    const hasVerified = /^verified:/m.test(fm);
    const hasHumanVerifier = hasVerified && /human:/.test(fm.slice(fm.indexOf("verified:")));
    const rendered = /data-okf-tier="([^"]+)"/.exec(html)?.[1];

    if (!hasVerified) {
      assert.equal(rendered, "unverified", `${id} has no verified entries but renders ${rendered}`);
    } else if (!hasHumanVerifier) {
      assert.notEqual(
        rendered,
        "human-reviewed",
        `${id} has no human: verifier but renders human-reviewed`,
      );
    }
  }
});

test("no page renders a verifier or source the bundle does not contain", async () => {
  for (const [id, html] of pages) {
    const source = await readFile(join(bundle, `${id}.md`), "utf8");
    for (const m of html.matchAll(/\b(human|process|team):[A-Za-z0-9_.-]+/g)) {
      assert.ok(
        source.includes(m[0]),
        `${id} renders actor ${m[0]}, which is not in its frontmatter`,
      );
    }
  }
});

test("reserved index.md files did not become concept pages", () => {
  for (const id of pages.keys()) {
    assert.doesNotMatch(id, /(^|\/)index$/, `reserved file ${id} was rendered as a concept`);
  }
  assert.ok(!existsSync(join(dist, "kb", "index")), "the bundle root index.md became a page");
});

test("the listing page links every concept and labels each tier as derived", async () => {
  const html = await readFile(join(dist, "index.html"), "utf8");
  for (const id of pages.keys()) {
    assert.ok(html.includes(`/kb/${id}/`), `the listing page does not link ${id}`);
  }
  assert.match(html, /data-okf-tier-derived="true"/);
});
