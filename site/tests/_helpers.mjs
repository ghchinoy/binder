// Shared helpers for the site guardrail suites. NOT a test file — the Node test
// runner only picks up files matching *.test.mjs, so this is never run directly.

import { readdir, readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const here = dirname(fileURLToPath(import.meta.url));
export const siteRoot = join(here, "..");
export const dist = join(siteRoot, "dist");
export const srcRoot = join(siteRoot, "src");

// The Astro `base` prefix (see astro.config.mjs). Internal URLs on the built
// site are all prefixed with this.
export const BASE = "/binder";

// Recursively list every file under `dir`.
export async function walk(dir) {
  const out = [];
  for (const ent of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, ent.name);
    if (ent.isDirectory()) out.push(...(await walk(p)));
    else out.push(p);
  }
  return out;
}

// Every built HTML file under dist/, as absolute paths.
export async function distHtmlFiles() {
  return (await walk(dist)).filter((f) => f.endsWith(".html"));
}

// Read a file as UTF-8.
export function read(path) {
  return readFile(path, "utf8");
}
