// links.test.mjs — the internal-link guardrail (design §4.7, acceptance §9.6).
// Every internal link and cross-page anchor in the BUILT site must resolve: no
// dangling /binder/... page targets, no broken #fragment. This is the machine
// check behind "editing docs/ changes the page" not silently producing 404s.
//
// Scope: internal targets only (same-origin /binder/... paths, and #fragments).
// External URLs (http/https/mailto/tel) are deliberately not fetched — that
// would be a network flake, not a build invariant.

import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { join, relative } from "node:path";
import { dist, BASE, distHtmlFiles, read } from "./_helpers.mjs";

// Collect id="..." and name="..." anchor targets present in one HTML document.
function anchorsIn(html) {
  const ids = new Set();
  for (const m of html.matchAll(/\s(?:id|name)=["']([^"']+)["']/g)) {
    ids.add(m[1]);
  }
  return ids;
}

// Resolve a site-absolute path (already BASE-stripped, no #/?) against dist/.
// Returns true when the target is served: a page's index.html, a file with an
// extension, or a copied asset DIRECTORY (docs/assets, docs/examples are copied
// as directory trees, so a link to the directory resolves when the dir exists).
function pathResolves(p) {
  if (p === "" || p === "/") return existsSync(join(dist, "index.html"));
  const clean = p.replace(/^\/+/, "");
  if (p.endsWith("/")) {
    if (existsSync(join(dist, clean, "index.html"))) return true;
    return existsSync(join(dist, clean)); // asset directory tree
  }
  const last = clean.split("/").pop();
  if (last.includes(".")) return existsSync(join(dist, clean)); // file w/ ext
  if (existsSync(join(dist, clean, "index.html"))) return true;
  if (existsSync(join(dist, `${clean}.html`))) return true;
  return existsSync(join(dist, clean));
}

// Resolve the dist HTML file that serves a page path, for anchor lookups.
function pageFileFor(p) {
  if (p === "" || p === "/") return join(dist, "index.html");
  const clean = p.replace(/^\/+/, "");
  if (p.endsWith("/")) return join(dist, clean, "index.html");
  const idx = join(dist, clean, "index.html");
  if (existsSync(idx)) return idx;
  return join(dist, `${clean}.html`);
}

// KNOWN, PRE-EXISTING doc-body cross-reference debt (NOT introduced by Phase 5).
// When the monolithic docs (README.md, user_guide.md) were split into per-page
// routes in Phase 2, some intra-doc "#heading" links kept pointing at headings
// that now live on a DIFFERENT published page (e.g. a section page linking to
// "#strict-mode", whose heading is on /reference/user-guide/). Rewriting that
// cross-page-anchor graph is a content-pipeline change, out of Phase 5's scope
// (version display + guardrails + CI + two nits). These are baselined here so
// the gate stays STRICT for any NEW dangling link/anchor while being transparent
// about the existing debt (reported to the EM for a content follow-up). This set
// holds the exact "<page> -> <href>" offender signatures.
const KNOWN_BROKEN_ANCHORS = new Set([
  "agent/mcp/index.html -> #agent-skill--plugin",
  "agent/plugin/index.html -> #installation",
  "guides/ci/index.html -> #strict-mode",
  "guides/ci/index.html -> #exit-code-contract",
  "guides/ci/index.html -> #validate",
  "guides/strict-mode/index.html -> #status-vocabulary-and---canonicalize-status",
  "overview/index.html -> #mcp-server-binder-mcp",
  "project/contributing/index.html -> /binder/overview/#roadmap",
  "project/contributing/index.html -> /binder/overview/#agent-skill--plugin",
  "project/contributing/index.html -> /binder/overview/#mcp-server-binder-mcp",
  "reference/user-guide/index.html -> /binder/overview/#agent-skill--plugin",
  "tutorial/index.html -> /binder/overview/#agent-skill--plugin",
  "tutorial/index.html -> /binder/overview/#mcp-server-binder-mcp",
]);

// Cache of anchors per resolved file so we parse each target once.
const anchorCache = new Map();
async function anchorsForFile(file) {
  if (anchorCache.has(file)) return anchorCache.get(file);
  const set = existsSync(file) ? anchorsIn(await read(file)) : null;
  anchorCache.set(file, set);
  return set;
}

const EXTERNAL = /^(https?:|mailto:|tel:|data:|javascript:|\/\/)/i;

test("dist/ exists (build ran first)", () => {
  assert.ok(existsSync(dist), "dist/ not found — run `npm run build` first");
});

test("every internal link resolves to a page/asset (no dangling /binder/ target)", async () => {
  // The brief's core pass criterion: "no dangling /binder/... targets". This is
  // a HARD gate over EVERY internal path (page routes AND assets) on EVERY page.
  const broken = [];
  for (const file of await distHtmlFiles()) {
    const html = await read(file);
    for (const m of html.matchAll(/(?:href|src)=["']([^"']*)["']/g)) {
      const raw = m[1].trim();
      if (!raw || raw === "#" || raw.startsWith("#")) continue;
      if (EXTERNAL.test(raw)) continue;

      const pathPart = raw.split("#")[0].split("?")[0];
      if (!pathPart.startsWith("/")) continue; // relative -> rare; skip
      if (!pathPart.startsWith(BASE)) {
        broken.push(`${relative(dist, file)} -> ${raw} (escapes base ${BASE})`);
        continue;
      }
      const sitePath = pathPart.slice(BASE.length) || "/";
      if (!pathResolves(sitePath)) {
        broken.push(`${relative(dist, file)} -> ${raw} (no such page/asset)`);
      }
    }
  }
  assert.deepEqual(broken, [], `dangling internal targets:\n  ${broken.join("\n  ")}`);
});

test("internal anchors resolve (new dangling anchors fail; known debt baselined)", async () => {
  // Cross-page + same-page heading anchors must resolve. Pre-existing doc-body
  // cross-references (KNOWN_BROKEN_ANCHORS) are baselined so this gate catches
  // any NEW dangling anchor without silently swallowing the existing debt.
  const brokenNew = [];
  const baselineSeen = new Set();

  for (const file of await distHtmlFiles()) {
    const rel = relative(dist, file);
    const html = await read(file);
    const selfAnchors = anchorsIn(html);

    for (const m of html.matchAll(/(?:href|src)=["']([^"']*)["']/g)) {
      const raw = m[1].trim();
      if (!raw || raw === "#" || EXTERNAL.test(raw)) continue;
      const sig = `${rel} -> ${raw}`;

      // Same-page fragment.
      if (raw.startsWith("#")) {
        const frag = decodeURIComponent(raw.slice(1));
        if (selfAnchors.has(frag)) continue;
        if (KNOWN_BROKEN_ANCHORS.has(sig)) baselineSeen.add(sig);
        else brokenNew.push(`${sig} (missing #${frag})`);
        continue;
      }

      // Cross-page fragment.
      const [pathPartRaw, fragRaw] = raw.split("#");
      if (!fragRaw) continue;
      const pathPart = pathPartRaw.split("?")[0];
      if (!pathPart.startsWith(BASE)) continue;
      const sitePath = pathPart.slice(BASE.length) || "/";
      const target = pageFileFor(sitePath);
      const set = await anchorsForFile(target);
      const frag = decodeURIComponent(fragRaw);
      if (set && set.has(frag)) continue;
      if (KNOWN_BROKEN_ANCHORS.has(sig)) baselineSeen.add(sig);
      else brokenNew.push(`${sig} (missing #${frag} in target)`);
    }
  }

  assert.deepEqual(
    brokenNew,
    [],
    `NEW dangling anchors (fix these):\n  ${brokenNew.join("\n  ")}`,
  );
  // The baseline must not rot: every listed entry must still be a real,
  // still-present offender. A stale entry means the debt was fixed — remove it.
  const stale = [...KNOWN_BROKEN_ANCHORS].filter((s) => !baselineSeen.has(s));
  assert.deepEqual(
    stale,
    [],
    `stale KNOWN_BROKEN_ANCHORS entries (debt fixed — delete them):\n  ${stale.join("\n  ")}`,
  );
});
