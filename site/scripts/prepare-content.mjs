// prepare-content.mjs is the build-time TEMPLATING step for the binder docs site.
// It takes the repository's already-authored, user-facing Markdown under ../docs
// (and ../README.md, ../CONTRIBUTING.md) and copies it into
// src/content/docs/_generated/, prepending only the Starlight frontmatter
// (title, slug, sidebar) each page needs to render.
//
// This is deliberately NOT a generator. It runs no binder binary and invents no
// fact — it copies existing prose verbatim (minus the leading H1, which Starlight
// re-renders from the frontmatter title, and with inter-doc links rewritten to
// the published URLs). docs/ stays the single source of truth: editing a source
// doc changes the published page, with no second copy of the prose to maintain.
// Internal files are simply never listed, so they can never leak into the site.
//
// Phase 2 extends the Phase 1 vertical slice (one doc) to the full §4.3 sitemap:
// README sections, the concept pages, guides, the user guide (ONE page), the LPG
// primer, and the project docs. The generated Command reference (docs/commands/)
// is intentionally OMITTED — it is produced by a separate phase and mapping a
// missing source would hard-fail the build.

import { readFile, writeFile, mkdir, rm, cp } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const siteRoot = join(here, "..");
const repoRoot = join(siteRoot, "..");
const docsRoot = join(repoRoot, "docs");
const outRoot = join(siteRoot, "src", "content", "docs");
const genRoot = join(outRoot, "_generated");
const publicRoot = join(siteRoot, "public");

// The Astro `base` (see astro.config.mjs). Inter-doc links and assets are
// rewritten to absolute, base-prefixed URLs so they resolve under project Pages
// (https://ghchinoy.github.io/binder/) regardless of a page's slug depth.
const BASE = "/binder";

// Each entry maps ONE source (a whole file, or one section of a file) to ONE
// published page.
//  - `src`     : path under the repo root to read (must exist; missing = hard error).
//  - `out`     : path under the private _generated/ staging dir to write (gitignored).
//  - `slug`    : the PUBLISHED, clean URL — decoupled from `out` so the _generated/
//                build-artifact prefix never reaches the public URL space.
//  - `title`   : the Starlight page title (rendered as the H1).
//  - `section` : OPTIONAL. When set, only the named Markdown section (that heading
//                up to the next heading of the same-or-higher level) is published.
//                Used to split README into its overview/install/agent-surface pages.
//                Section pages have no leading H1 to strip.
const pages = [
  // ── Start ───────────────────────────────────────────────────────────────
  { src: "README.md", out: "overview.md", slug: "overview", title: "What binder is", section: "What it does" },
  { src: "README.md", out: "install.md", slug: "install", title: "Installation", section: "Installation" },
  { src: "docs/tutorial.md", out: "tutorial.md", slug: "tutorial", title: "Tutorial" },

  // ── Concepts (NEW canonical docs/concepts/*.md — extracted, not invented) ──
  { src: "docs/concepts/byte-faithful.md", out: "concepts/byte-faithful.md", slug: "concepts/byte-faithfulness", title: "Byte-faithful round-trip" },
  { src: "docs/concepts/trust.md", out: "concepts/trust.md", slug: "concepts/trust", title: "Trust model & tiers" },
  { src: "docs/concepts/graph.md", out: "concepts/graph.md", slug: "concepts/graph", title: "Relationship extraction & the graph" },
  { src: "docs/concepts/okf-output.md", out: "concepts/okf-output.md", slug: "concepts/okf-output", title: "OKF v0.2 output structure" },
  { src: "docs/concepts/project.md", out: "concepts/project.md", slug: "concepts/project", title: "Graph projection (project)" },
  { src: "docs/lpg-inmemory-primer.md", out: "concepts/lpg-primer.md", slug: "concepts/lpg-primer", title: "In-memory LPG primer" },

  // ── Guides (extracted user_guide sections) ───────────────────────────────
  { src: "docs/user_guide.md", out: "guides/ci.md", slug: "guides/ci", title: "CI usage", section: "CI usage" },
  { src: "docs/user_guide.md", out: "guides/strict-mode.md", slug: "guides/strict-mode", title: "Strict mode", section: "Strict mode" },

  // ── Reference (user guide as ONE page; command reference omitted — Phase 3) ─
  { src: "docs/user_guide.md", out: "reference/user-guide.md", slug: "reference/user-guide", title: "User guide" },

  // ── Agent surface ─────────────────────────────────────────────────────────
  { src: "README.md", out: "agent/mcp.md", slug: "agent/mcp", title: "MCP server", section: "MCP server (`binder mcp`)" },
  { src: "README.md", out: "agent/plugin.md", slug: "agent/plugin", title: "okf-convert plugin & skill", section: "Agent Skill / Plugin" },

  // ── Project ───────────────────────────────────────────────────────────────
  { src: "docs/RELEASING.md", out: "project/releasing.md", slug: "project/releasing", title: "Releasing" },
  { src: "CONTRIBUTING.md", out: "project/contributing.md", slug: "project/contributing", title: "Contributing" },
];

// Directories under docs/ copied verbatim into the site's public path so the
// existing diagrams (webp) and example bundles render as-is. Rebuilt every build.
const assetDirs = [
  { src: "docs/assets", out: "assets" },
  { src: "docs/examples", out: "examples" },
];

// Map a source doc's basename (after stripping ./ ../ and docs/ prefixes) to the
// published URL, so inter-doc Markdown links resolve on the site. Only files we
// actually publish are mapped; anything else is left untouched.
const SLUG_BY_BASENAME = {
  "README.md": `${BASE}/overview/`,
  "user_guide.md": `${BASE}/reference/user-guide/`,
  "tutorial.md": `${BASE}/tutorial/`,
  "lpg-inmemory-primer.md": `${BASE}/concepts/lpg-primer/`,
  "RELEASING.md": `${BASE}/project/releasing/`,
  "CONTRIBUTING.md": `${BASE}/project/contributing/`,
  "concepts/byte-faithful.md": `${BASE}/concepts/byte-faithfulness/`,
  "concepts/trust.md": `${BASE}/concepts/trust/`,
  "concepts/graph.md": `${BASE}/concepts/graph/`,
  "concepts/okf-output.md": `${BASE}/concepts/okf-output/`,
  "concepts/project.md": `${BASE}/concepts/project/`,
  // bare names, for sibling links inside docs/concepts/*.md
  "byte-faithful.md": `${BASE}/concepts/byte-faithfulness/`,
  "trust.md": `${BASE}/concepts/trust/`,
  "graph.md": `${BASE}/concepts/graph/`,
  "okf-output.md": `${BASE}/concepts/okf-output/`,
  "project.md": `${BASE}/concepts/project/`,
};

// YAML-escape a scalar we control (titles are ours, not user input) so the
// emitted frontmatter always parses even if a title contains a colon or quote.
function yamlString(s) {
  return JSON.stringify(String(s));
}

// True for a Markdown heading line ("# ", "## ", …). `inFence` tracks fenced
// code blocks so a "# comment" inside a ``` block is never mistaken for a heading.
function headingLevel(line) {
  const m = /^(#{1,6})\s+\S/.exec(line);
  return m ? m[1].length : 0;
}

// Extract exactly one section: the heading whose text matches `wanted`, up to
// (but excluding) the next heading of the same-or-higher level. Fence-aware. The
// matched heading line itself is dropped — Starlight renders our frontmatter
// title as the page H1.
function extractSection(body, wanted) {
  const lines = body.split("\n");
  let inFence = false;
  let start = -1;
  let level = 0;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (/^\s*(```|~~~)/.test(line)) inFence = !inFence;
    if (inFence) continue;
    if (start === -1) {
      const lvl = headingLevel(line);
      if (lvl > 0 && line.replace(/^#{1,6}\s+/, "").trim() === wanted) {
        start = i + 1;
        level = lvl;
      }
    } else {
      const lvl = headingLevel(line);
      if (lvl > 0 && lvl <= level) {
        return lines.slice(start, i).join("\n").trim() + "\n";
      }
    }
  }
  if (start === -1) {
    throw new Error(
      `prepare-content: section "${wanted}" not found in source. ` +
        `Fix the allowlist in scripts/prepare-content.mjs or the source heading.`,
    );
  }
  return lines.slice(start).join("\n").trim() + "\n";
}

// Rewrite one Markdown link/image target to its published URL. External links,
// mailto:, and bare in-page #anchors are left untouched.
function rewriteTarget(target) {
  if (/^(https?:|mailto:|#|\/)/.test(target)) return target;
  const hash = target.indexOf("#");
  const pathPart = hash === -1 ? target : target.slice(0, hash);
  const anchor = hash === -1 ? "" : target.slice(hash);
  const base = pathPart.replace(/^(\.\.?\/)+/, "").replace(/^docs\//, "");
  if (SLUG_BY_BASENAME[base]) return SLUG_BY_BASENAME[base] + anchor;
  if (base.startsWith("assets/") || base.startsWith("examples/")) {
    return `${BASE}/${base}${anchor}`;
  }
  return target;
}

// Rewrite every Markdown inline link and image target in the body.
function rewriteLinks(body) {
  return body.replace(
    /(!?\[[^\]]*\])\(\s*(<[^>]+>|[^)\s]+)((?:\s+"[^"]*")?)\s*\)/g,
    (whole, label, target, title) => {
      const bare = target.replace(/^<|>$/g, "");
      const rewritten = rewriteTarget(bare);
      return `${label}(${rewritten}${title})`;
    },
  );
}

// Build a Starlight frontmatter block and append the (link-rewritten) body. No
// fact is added — only the header Starlight needs to render the page.
//
// For whole-file pages the body's leading top-level "# Heading" is dropped
// (Starlight renders the frontmatter `title` as the H1, so keeping it would
// duplicate). Section pages have no leading H1, so nothing is stripped there.
function withFrontmatter({ title, slug, order, section }, body) {
  const lines = ["---", `title: ${yamlString(title)}`];
  if (slug) lines.push(`slug: ${yamlString(slug)}`);
  if (order !== undefined) {
    lines.push("sidebar:");
    lines.push(`  order: ${order}`);
  }
  lines.push("---", "");
  const stripped = section ? body : body.replace(/^﻿?#[^\n]*\n+/, "");
  return lines.join("\n") + rewriteLinks(stripped);
}

async function main() {
  // Start clean so a removed/renamed source cannot leave a stale page behind.
  await rm(genRoot, { recursive: true, force: true });
  await mkdir(genRoot, { recursive: true });

  for (const page of pages) {
    const srcPath = join(repoRoot, page.src);
    if (!existsSync(srcPath)) {
      throw new Error(
        `prepare-content: source doc not found: ${srcPath}. ` +
          `This step only copies existing docs; it never invents content. ` +
          `Fix the allowlist in scripts/prepare-content.mjs or restore the file.`,
      );
    }
    let body = await readFile(srcPath, "utf8");
    if (page.section) body = extractSection(body, page.section);
    const outPath = join(genRoot, page.out);
    await mkdir(dirname(outPath), { recursive: true });
    await writeFile(outPath, withFrontmatter(page, body), "utf8");
    console.log(
      `prepared ${page.src}${page.section ? ` [§ ${page.section}]` : ""} ` +
        `-> _generated/${page.out} (published at /${page.slug}/)`,
    );
  }

  // Copy the diagram/example directories into the site's public path. Rebuilt
  // every build; kept out of the tracked site/ tree (see site/.gitignore).
  for (const dir of assetDirs) {
    const from = join(repoRoot, dir.src);
    if (!existsSync(from)) {
      throw new Error(`prepare-content: asset dir not found: ${from}.`);
    }
    const to = join(publicRoot, dir.out);
    await rm(to, { recursive: true, force: true });
    await cp(from, to, { recursive: true });
    console.log(`copied ${dir.src}/ -> public/${dir.out}/`);
  }

  console.log(`prepare-content: wrote ${pages.length} page(s) to ${genRoot}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
