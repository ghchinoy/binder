// loader.test.mjs — drives okfLoader over binder's real, in-tree OKF bundle
// (testdata/expected-rich) through a stand-in LoaderContext whose parseData
// runs the real okfSchema. Nothing is vendored or hand-transcribed: if binder's
// emitted fixture changes, this test reads the change.

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { okfLoader, okfSchema } from "../dist/index.js";
import { createContext } from "./_context.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const BUNDLE = join(here, "..", "..", "..", "testdata", "expected-rich");

// Pinned so staleness means the same thing on every machine and on any day.
const NOW = new Date("2026-08-21T00:00:00Z");

async function loadBundle() {
  const { store, logs, context } = createContext({ schema: okfSchema(), root: here });
  const loader = okfLoader({ bundle: BUNDLE, now: () => NOW });
  await loader.load(context);
  return { store, logs };
}

test("the loader identifies itself with the package name", () => {
  assert.equal(okfLoader({ bundle: BUNDLE }).name, "astro-okf");
});

test("a missing bundle option is refused up front, not at build time", () => {
  assert.throws(() => okfLoader({}), /requires a `bundle` path/);
  assert.throws(() => okfLoader({ bundle: "  " }), /requires a `bundle` path/);
});

test("an unreadable bundle directory fails with the path in the message", async () => {
  const { context } = createContext({ schema: okfSchema(), root: here });
  const loader = okfLoader({ bundle: join(here, "no-such-bundle") });
  await assert.rejects(loader.load(context), /cannot read bundle directory/);
});

// All three malformed cases #164 covers. A try/catch wrapped around a
// fail-closed throw is the exact shape a fail-open regression takes, so this
// asserts the LOAD ACTUALLY REJECTS for each case — not merely that the message
// is prettier — and that the original codec wording survives the wrap. The
// wording check is what distinguishes a genuine parse-layer throw (the point of
// #164: Go and TS agreeing) from a silent collapse to `{}` that only Zod later
// rejects with "type Required": the harness runs the real okfSchema, so a
// fail-open regression would still make the load reject, but on Zod's message,
// not the codec's — and this test would then fail on the wording assertion.
const MALFORMED_CASES = [
  {
    name: "unterminated-fence",
    frontmatter: "---\ntype: Note\ntitle: Oops\n\n# No closing fence\n",
    wording: /unterminated '---' block/,
  },
  {
    name: "top-level-sequence",
    frontmatter: "---\n- one\n- two\n---\n\n# Body\n",
    wording: /expected a mapping at the top level/,
  },
  {
    name: "top-level-scalar",
    frontmatter: "---\njust a bare string\n---\n\n# Body\n",
    wording: /expected a mapping at the top level/,
  },
];

for (const c of MALFORMED_CASES) {
  test(`#164: a ${c.name} file fails the load loudly, naming the file and keeping the codec wording`, async () => {
    const dir = await mkdtemp(join(tmpdir(), "astro-okf-loader-"));
    try {
      await writeFile(join(dir, "ok.md"), "---\ntype: Note\ntitle: Fine\n---\n\n# Fine\n");
      await writeFile(join(dir, `${c.name}.md`), c.frontmatter);

      const { context } = createContext({ schema: okfSchema(), root: here });
      const loader = okfLoader({ bundle: dir, now: () => NOW });
      await assert.rejects(
        loader.load(context),
        (err) => {
          assert.match(
            err.message,
            new RegExp(`${c.name}\\.md`),
            `${c.name}: error must name the offending file`,
          );
          assert.match(
            err.message,
            c.wording,
            `${c.name}: error must carry the codec wording, not a downstream Zod message`,
          );
          return true;
        },
        `${c.name}: the load must reject (fail closed), not fail open`,
      );
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
}

test("every concept in the bundle becomes an entry, keyed by bundle-relative path", async () => {
  const { store } = await loadBundle();
  assert.deepEqual(store.keys().sort(), [
    "attested/calc",
    "guides/index-note",
    "guides/setup",
    "intro",
    "tables/orders",
  ]);
});

test("spec 11: reserved index.md files are skipped, never validated as concepts", async () => {
  const { store, logs } = await loadBundle();
  // The bundle has four index.md files (root plus three directories); none of
  // them carries a `type`, so validating them as concepts would fail the build.
  for (const key of store.keys()) {
    assert.doesNotMatch(key, /(^|\/)index$/, `reserved file ${key} was emitted as a concept`);
  }
  assert.match(logs.info.join("\n"), /4 reserved files skipped/);
});

test("acceptance: tables/orders derives human-reviewed, and the basis is the human stamp", async () => {
  const { store } = await loadBundle();
  const orders = store.get("tables/orders");
  assert.ok(orders, "tables/orders was not loaded");

  // The fixture carries verified: [human:bob, binder/0.1.0] (spec 5.3).
  assert.equal(orders.data._okf.tier, "human-reviewed");
  assert.deepEqual(orders.data._okf.tierBasis, [
    { by: "human:bob", at: "2026-02-01T10:00:00Z" },
  ]);

  // The derived block is namespaced and the raw families are untouched, so a
  // template renders evidence and conclusion from different places.
  assert.deepEqual(orders.data.verified, [
    { by: "human:bob", at: "2026-02-01T10:00:00Z" },
    { by: "binder/0.1.0", at: "2026-02-02T10:00:00Z" },
  ]);
  assert.equal(orders.data.type, "BigQuery Table");
  assert.equal(orders.data.title, "Orders Table");
});

test("acceptance: a concept with no verified entries derives unverified with no basis", async () => {
  const { store } = await loadBundle();
  for (const id of ["intro", "guides/setup", "guides/index-note"]) {
    const entry = store.get(id);
    assert.equal(entry.data.verified, undefined, `${id} unexpectedly has verified entries`);
    assert.equal(entry.data._okf.tier, "unverified", `${id} should be unverified`);
    assert.deepEqual(entry.data._okf.tierBasis, [], `${id} should have no tier basis`);
  }
});

test("spec 5.2: attested/calc's bare verified mapping is read as a one-element list", async () => {
  const { store } = await loadBundle();
  const calc = store.get("attested/calc");
  // The fixture spells it `verified: { by: human:carol, at: ... }`.
  assert.deepEqual(calc.data.verified, [{ by: "human:carol", at: "2026-03-01T12:00:00Z" }]);
  assert.equal(calc.data._okf.tier, "human-reviewed");
});

test("acceptance: staleness is computed against the injected clock, with the date kept", async () => {
  const { store } = await loadBundle();

  const calc = store.get("attested/calc");
  assert.equal(calc.data.stale_after, "2020-01-01");
  assert.equal(calc.data._okf.stale, true);
  assert.equal(calc.data._okf.staleAfter, "2020-01-01");
  assert.equal(calc.data._okf.evaluatedOn, "2026-08-21");

  const orders = store.get("tables/orders");
  assert.equal(orders.data._okf.stale, false, "stale_after 2027-01-01 is not yet reached in 2026");
  assert.equal(orders.data._okf.staleAfter, "2027-01-01");

  const intro = store.get("intro");
  assert.equal(intro.data._okf.stale, false, "no stale_after means never stale");
  assert.equal(intro.data._okf.staleAfter, undefined);
});

test("the same bundle read on a later day goes stale, and only then", async () => {
  const { store, context } = (() => {
    const c = createContext({ schema: okfSchema(), root: here });
    return { store: c.store, context: c.context };
  })();
  await okfLoader({ bundle: BUNDLE, now: () => new Date("2027-01-01T00:00:00Z") }).load(context);
  // Boundary equality: 2027-01-01 >= 2027-01-01 (spec 5.5).
  assert.equal(store.get("tables/orders").data._okf.stale, true);
  assert.equal(store.get("tables/orders").data._okf.evaluatedOn, "2027-01-01");
});

test("spec 4.1: unknown producer keys survive into the entry data", async () => {
  const { store } = await loadBundle();
  const setup = store.get("guides/setup");
  // `related` and `draft` are not OKF vocabulary; the fixture carries them and
  // the schema must not drop them.
  assert.equal(setup.data.related, "[[Orders Table]]");
  assert.equal(setup.data.draft, true);
  // `status: draft` is off the derived path entirely — stored, not interpreted.
  assert.equal(setup.data.status, "draft");
});

test("the body is stored and rendered, and the frontmatter block is not in it", async () => {
  const { store } = await loadBundle();
  const orders = store.get("tables/orders");
  assert.match(orders.body, /# Schema/);
  assert.doesNotMatch(orders.body, /stale_after/);
  assert.equal(orders.rendered.html, orders.body);
  assert.ok(orders.digest, "no digest recorded");
  // Astro rejects an absolute filePath: it must be relative to the project
  // root, and an OKF bundle normally lives outside it.
  assert.ok(!orders.filePath.startsWith("/"), `filePath is absolute: ${orders.filePath}`);
  assert.match(orders.filePath, /expected-rich\/tables\/orders\.md$/);
});

test("a relative bundle path resolves against the Astro project root", async () => {
  const { store, context } = (() => {
    const c = createContext({ schema: okfSchema(), root: here });
    return { store: c.store, context: c.context };
  })();
  // `here` is packages/astro-okf/test, so this is the same bundle by another route.
  await okfLoader({ bundle: "../../../testdata/expected-rich", now: () => NOW }).load(context);
  assert.ok(store.has("tables/orders"));
});

test("a re-load replaces the store rather than accumulating stale entries", async () => {
  const { store, context } = (() => {
    const c = createContext({ schema: okfSchema(), root: here });
    return { store: c.store, context: c.context };
  })();
  const loader = okfLoader({ bundle: BUNDLE, now: () => NOW });
  await loader.load(context);
  const first = store.keys().length;
  await loader.load(context);
  assert.equal(store.keys().length, first);
});
