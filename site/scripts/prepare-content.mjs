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
// primer, and the project docs.
//
// Phase 4 folds in the generated Command reference (docs/commands/, produced by
// `make docs`/cmd/gendocs on main and CI-drift-gated). It is not a simple
// one-file->one-page copy: the reference is a directory (an index README plus
// one file per command). We render it as ONE self-contained page at
// /reference/commands — the index intro, then every command in turn — with the
// inter-file `binder_*.md` links rewritten to in-page anchors so Starlight's
// auto-TOC gives a clean per-command outline. The site renders the CI-proven
// bytes verbatim; it never runs binder and never regenerates the reference. A
// missing docs/commands/ is a HARD ERROR, same contract as every other source.

import { readFile, writeFile, mkdir, rm, cp, readdir } from "node:fs/promises";
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
  { src: "docs/concepts/byte-faithful.md", out: "concepts/byte-faithful.md", slug: "concepts/byte-faithfulness", title: "Lossless frontmatter round-trip" },
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

// The generated command reference (docs/commands/) rendered as ONE page. `dir`
// must exist (hard error otherwise); `index` is the intro README; the remaining
// binder*.md files are appended in a stable order (root command first, then the
// rest cobra-sorted) with inter-file links rewritten to in-page anchors.
const commandsRef = {
  dir: "docs/commands",
  index: "README.md",
  out: "reference/commands.md",
  slug: "reference/commands",
  title: "Command reference",
};

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

// Rewrite the reference's inter-file links (e.g. `(binder_convert.md)`,
// `(binder_config_get.md)`) to in-page anchors, since every command lands on one
// page. Cobra's per-command heading ("## binder config get") slugifies to
// "binder-config-get" — identical to the filename with "_" -> "-", so the
// mapping is exact and deterministic.
function commandLinksToAnchors(body) {
  return body.replace(
    /\((binder(?:_[a-z]+)*\.md)(?:#[^)]*)?\)/g,
    (_whole, file) => `(#${file.replace(/\.md$/, "").replace(/_/g, "-")})`,
  );
}

// Fold docs/commands/ into ONE page: the index README first (its top-level H1
// stripped — Starlight renders the frontmatter title as the H1), then each
// per-command file (root command first, remaining cobra-sorted). Every command
// file keeps its "## binder <cmd>" H2 so Starlight's on-page TOC lists them.
async function buildCommandReference() {
  const dirPath = join(repoRoot, commandsRef.dir);
  if (!existsSync(dirPath)) {
    throw new Error(
      `prepare-content: command reference dir not found: ${dirPath}. ` +
        `It is generated by \`make docs\` (cmd/gendocs) and committed under ` +
        `${commandsRef.dir}/. This step only copies existing docs; restore it ` +
        `or fix scripts/prepare-content.mjs.`,
    );
  }

  const indexPath = join(dirPath, commandsRef.index);
  if (!existsSync(indexPath)) {
    throw new Error(
      `prepare-content: command reference index not found: ${indexPath}.`,
    );
  }

  // Command files: every binder*.md except the index README. Root "binder.md"
  // first (the overview), then the rest sorted for stable output.
  const entries = (await readdir(dirPath))
    .filter((f) => /^binder.*\.md$/.test(f) && f !== commandsRef.index)
    .sort((a, b) => (a === "binder.md" ? -1 : b === "binder.md" ? 1 : a.localeCompare(b)));

  if (entries.length === 0) {
    throw new Error(
      `prepare-content: no command pages found in ${dirPath} (expected binder*.md).`,
    );
  }

  const indexBody = await readFile(indexPath, "utf8");
  const parts = [commandLinksToAnchors(indexBody).trim()];
  for (const file of entries) {
    const cmdBody = await readFile(join(dirPath, file), "utf8");
    parts.push(commandLinksToAnchors(cmdBody).trim());
  }
  const combined = parts.join("\n\n") + "\n";

  const outPath = join(genRoot, commandsRef.out);
  await mkdir(dirname(outPath), { recursive: true });
  await writeFile(
    outPath,
    withFrontmatter(
      { title: commandsRef.title, slug: commandsRef.slug },
      combined,
    ),
    "utf8",
  );
  console.log(
    `prepared ${commandsRef.dir}/ (index + ${entries.length} command page(s)) ` +
      `-> _generated/${commandsRef.out} (published at /${commandsRef.slug}/)`,
  );
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

  // Fold the generated command reference (docs/commands/) into one page.
  await buildCommandReference();

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
