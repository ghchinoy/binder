// prepare-content.mjs is the build-time TEMPLATING step for the binder docs site.
// It takes the repository's already-authored, user-facing Markdown under ../docs
// and copies it into src/content/docs/_generated/, prepending only the Starlight
// frontmatter (title, slug, sidebar) each page needs to render.
//
// This is deliberately NOT a generator. It runs no binder binary and invents no
// fact — it copies existing prose verbatim (minus the leading H1, which Starlight
// re-renders from the frontmatter title). docs/ stays the single source of truth:
// editing docs/tutorial.md changes the published page, with no second copy of the
// prose to maintain. Internal files are simply never listed, so they can never
// leak into the published site.
//
// Phase 1 is a single end-to-end vertical slice: EXACTLY ONE doc
// (docs/tutorial.md -> /tutorial). The `pages` array is shaped so Phase 2 can
// extend the map by appending entries — no structural change needed.

import { readFile, writeFile, mkdir, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const siteRoot = join(here, "..");
const repoRoot = join(siteRoot, "..");
const docsRoot = join(repoRoot, "docs");
const outRoot = join(siteRoot, "src", "content", "docs");
const genRoot = join(outRoot, "_generated");

// Each entry maps ONE user-facing source doc to ONE published page.
//  - `src`   : path under ../docs to read (must exist; a missing source is a hard error).
//  - `out`   : path under the private _generated/ staging dir to write (gitignored).
//  - `slug`  : the PUBLISHED, clean URL — decoupled from `out` so the _generated/
//              build-artifact prefix never reaches the public URL space.
//  - `title` : the Starlight page title (rendered as the H1).
//  - `order` : sidebar order.
// Phase 2 extends the site by appending entries here.
const pages = [
  {
    src: "tutorial.md",
    out: "tutorial.md",
    slug: "tutorial",
    title: "Tutorial",
    order: 1,
  },
];

// YAML-escape a scalar we control (titles are ours, not user input) so the
// emitted frontmatter always parses even if a title contains a colon or quote.
function yamlString(s) {
  return JSON.stringify(String(s));
}

// Build a Starlight frontmatter block and append the body verbatim. No fact is
// added — only the header Starlight needs to render the page.
//
// The body's leading top-level "# Heading" is dropped: Starlight renders the
// frontmatter `title` as the page H1, so keeping the source H1 would duplicate it.
function withFrontmatter({ title, slug, order }, body) {
  const lines = ["---", `title: ${yamlString(title)}`];
  if (slug) lines.push(`slug: ${yamlString(slug)}`);
  if (order !== undefined) {
    lines.push("sidebar:");
    lines.push(`  order: ${order}`);
  }
  lines.push("---", "");
  const trimmed = body.replace(/^﻿?#[^\n]*\n+/, "");
  return lines.join("\n") + trimmed;
}

async function main() {
  // Start clean so a removed/renamed source cannot leave a stale page behind.
  await rm(genRoot, { recursive: true, force: true });
  await mkdir(genRoot, { recursive: true });

  for (const page of pages) {
    const srcPath = join(docsRoot, page.src);
    if (!existsSync(srcPath)) {
      throw new Error(
        `prepare-content: source doc not found: ${srcPath}. ` +
          `This step only copies existing docs; it never invents content. ` +
          `Fix the allowlist in scripts/prepare-content.mjs or restore the file.`,
      );
    }
    const body = await readFile(srcPath, "utf8");
    const outPath = join(genRoot, page.out);
    await mkdir(dirname(outPath), { recursive: true });
    await writeFile(outPath, withFrontmatter(page, body), "utf8");
    console.log(
      `prepared ${page.src} -> _generated/${page.out} (published at /${page.slug}/)`,
    );
  }

  console.log(`prepare-content: wrote ${pages.length} page(s) to ${genRoot}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
