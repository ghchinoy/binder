/**
 * `okfLoader` — an Astro Content Layer loader for OKF v0.2 bundles.
 *
 * Scope note (Phase 1). This is the vertical slice: bundle directory in,
 * concept entries out, with the derived trust tier and staleness attached. It
 * deliberately does NOT yet do the things later phases own — reserved
 * `index.md` / `log.md` parsing (Phase 2), the link/backlink graph and
 * broken-link surfacing (Phase 3), footnote joins and advisories (Phase 2).
 * Those are absent rather than stubbed, so nothing here reports a result it did
 * not compute.
 *
 * The loader is a read-only consumer. It never writes to, moves, or rewrites
 * the bundle it is given.
 */

import { readFile, readdir } from "node:fs/promises";
import { join, relative, resolve, sep } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import type { Loader, LoaderContext } from "astro/loaders";

import { normalizeActorstamp, normalizeActorstamps, splitFrontmatter } from "./parse.js";
import { deriveTier, isHumanActor, isStale, toISODay, type Actorstamp, type Tier } from "./trust.js";

/** Options accepted by {@link okfLoader}. */
export interface OkfLoaderOptions {
  /**
   * Path to the OKF bundle root — the directory holding the bundle's
   * `index.md` and its concept files. A relative path is resolved against the
   * Astro project root (`config.root`), so it reads the same way as the other
   * paths in an `astro.config`.
   */
  bundle: string;
  /**
   * Clock used for the staleness comparison (spec §5.5). Injectable so a test
   * or a reproducible build can pin "now" instead of depending on the day the
   * build happened to run. Defaults to the real current time.
   */
  now?: () => Date;
}

/** Reserved OKF filenames (spec §3.1/§8/§9). Phase 1 does not emit these. */
const RESERVED_BASENAMES = new Set(["index.md", "log.md"]);

/**
 * The derived block attached to every emitted entry under the `_okf` key.
 * Everything in it was computed by this loader; none of it is stored in the
 * bundle.
 */
export interface OkfDerived {
  kind: "concept";
  tier: Tier;
  tierBasis: Actorstamp[];
  stale: boolean;
  staleAfter?: string;
  evaluatedOn: string;
}

/**
 * Creates the Content Layer loader.
 *
 * ```ts
 * import { defineCollection } from "astro:content";
 * import { okfLoader, okfSchema } from "astro-okf";
 *
 * export const collections = {
 *   kb: defineCollection({
 *     loader: okfLoader({ bundle: "../my-okf-bundle" }),
 *     schema: okfSchema(),
 *   }),
 * };
 * ```
 */
export function okfLoader(options: OkfLoaderOptions): Loader {
  if (!options || typeof options.bundle !== "string" || options.bundle.trim() === "") {
    throw new Error("astro-okf: okfLoader requires a `bundle` path to an OKF bundle directory.");
  }
  const clock = options.now ?? (() => new Date());

  return {
    name: "astro-okf",
    async load(context: LoaderContext): Promise<void> {
      const { store, parseData, generateDigest, renderMarkdown, logger, config } = context;
      const projectRoot = config?.root ? fileURLToPath(config.root) : process.cwd();
      const bundleRoot = resolve(projectRoot, options.bundle);
      const evaluatedOn = toISODay(clock());

      const files = (await collectMarkdown(bundleRoot)).sort();
      if (files.length === 0) {
        logger.warn(`No markdown files found under bundle ${bundleRoot} — collection is empty.`);
      }

      // A full reload every time. Trust derivation is per-concept today, but
      // Phase 3's backlinks are a whole-bundle property (editing A changes B's
      // backlinks), so a partial store is the wrong default to establish here.
      // Digests are still recorded below: they are what Astro uses to notice a
      // change, and they cost nothing to compute.
      store.clear();

      let emitted = 0;
      let reserved = 0;

      for (const file of files) {
        const id = bundleRelativeId(bundleRoot, file);
        if (RESERVED_BASENAMES.has(basenameOf(file))) {
          // Reserved files carry no `type` and MUST NOT be validated as
          // concepts (spec §11). Phase 2 gives them their own entry kinds; for
          // now they are skipped rather than mis-rendered.
          reserved += 1;
          logger.debug(`Skipping reserved file ${id}.md (reserved-file support lands in Phase 2)`);
          continue;
        }

        const raw = await readFile(file, "utf8");
        let frontmatter: Record<string, unknown>;
        let body: string;
        try {
          ({ data: frontmatter, body } = splitFrontmatter(raw));
        } catch (err) {
          // splitFrontmatter throws on malformed frontmatter (issue #164). It
          // is called in this loop with no id of its own, so name the offending
          // file here — a bundle load that aborts mid-way without saying WHICH
          // file is as unhelpful as the "type Required" diagnostic N-1 set out
          // to replace.
          throw new Error(
            `astro-okf: ${id}.md: ${(err as Error).message}`,
            { cause: err },
          );
        }

        const verified = normalizeActorstamps(frontmatter.verified);
        const generated = normalizeActorstamp(frontmatter.generated);
        const staleAfter = typeof frontmatter.stale_after === "string" ? frontmatter.stale_after : undefined;
        const tier = deriveTier(verified);

        const derived: OkfDerived = {
          kind: "concept",
          tier,
          tierBasis: tierBasisFor(tier, verified),
          stale: isStale(staleAfter, evaluatedOn),
          evaluatedOn,
        };
        if (staleAfter !== undefined) derived.staleAfter = staleAfter;

        const merged: Record<string, unknown> = { ...frontmatter, _okf: derived };
        // Write the normalized §5.2 shapes back so the schema and every
        // template see one spelling, whichever the bundle used.
        if (verified !== undefined) merged.verified = verified;
        if (generated !== undefined) merged.generated = generated;

        // Astro requires filePath to be relative to the project root, and a
        // bundle usually sits outside it, so this is commonly a `../` path.
        const projectRelativePath = toPosix(relative(projectRoot, file));

        const data = await parseData({ id, data: merged, filePath: projectRelativePath });
        const rendered = await renderMarkdown(body, { fileURL: pathToFileURL(file) });

        store.set({
          id,
          data,
          body,
          filePath: projectRelativePath,
          digest: generateDigest(raw),
          rendered,
        });
        emitted += 1;
      }

      logger.info(
        `Loaded ${emitted} concept${emitted === 1 ? "" : "s"} from ${bundleRoot}` +
          (reserved > 0 ? ` (${reserved} reserved file${reserved === 1 ? "" : "s"} skipped)` : ""),
      );
    },
  };
}

/**
 * The stamps that actually justify a tier — the evidence a renderer shows so a
 * reader can check the derivation instead of trusting it.
 *
 * `unverified` has none by definition. `human-reviewed` is carried by the
 * `human:` stamps alone, so listing the tool stamps alongside them would blur
 * which one earned the promotion. `machine-confirmed` rests on all of them.
 */
function tierBasisFor(tier: Tier, verified: Actorstamp[] | undefined): Actorstamp[] {
  if (tier === "unverified" || !verified) return [];
  if (tier === "human-reviewed") return verified.filter((v) => isHumanActor(v.by));
  return verified;
}

/** The collection id for a file: its bundle-relative path without `.md`. */
function bundleRelativeId(bundleRoot: string, file: string): string {
  return toPosix(relative(bundleRoot, file)).replace(/\.md$/i, "");
}

/** Normalizes a platform path to forward slashes, so ids are stable on Windows. */
function toPosix(p: string): string {
  return p.split(sep).join("/");
}

function basenameOf(file: string): string {
  return file.split(sep).pop()?.toLowerCase() ?? "";
}

/** Recursively collects every `.md` file under `dir`, absolute paths. */
async function collectMarkdown(dir: string): Promise<string[]> {
  const out: string[] = [];
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch (err) {
    throw new Error(
      `astro-okf: cannot read bundle directory ${dir}: ${(err as Error).message}`,
      { cause: err },
    );
  }
  for (const entry of entries) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules" || entry.name.startsWith(".")) continue;
      out.push(...(await collectMarkdown(full)));
    } else if (entry.isFile() && entry.name.toLowerCase().endsWith(".md")) {
      out.push(full);
    }
  }
  return out;
}
