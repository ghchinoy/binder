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

test("internal anchors resolve (FULLY STRICT — no baseline)", async () => {
  // Cross-page + same-page heading anchors must ALL resolve. The Phase-5
  // KNOWN_BROKEN_ANCHORS baseline has been deleted (Phase 6): every dangling
  // internal anchor — new or old — is now a hard failure with no exceptions.
  const broken = [];

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
        broken.push(`${sig} (missing #${frag})`);
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
      broken.push(`${sig} (missing #${frag} in target)`);
    }
  }

  assert.deepEqual(
    broken,
    [],
    `dangling internal anchors (fix these):\n  ${broken.join("\n  ")}`,
  );
});
