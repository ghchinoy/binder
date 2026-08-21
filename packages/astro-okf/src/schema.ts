/**
 * The collection schema.
 *
 * The schema has two regions and the split between them is the whole point:
 *
 *  - the OKF frontmatter families, carrying EXACTLY what the bundle said, and
 *  - a namespaced `_okf` block, carrying only what the loader COMPUTED.
 *
 * They are never merged. A template renders `verified[]` from the raw field and
 * the trust tier from `_okf.tier` (labelled derived), so a template physically
 * cannot present a derived tier as if the author had stored it.
 *
 * The object is `.passthrough()`d because spec §4.1 says a consumer MUST NOT
 * reject documents with unrecognized fields — an unknown producer key survives
 * into `entry.data` untouched rather than being dropped or reinterpreted.
 */

import { z } from "astro/zod";

/**
 * An actor string (spec §7). Deliberately NOT shape-validated here: spec §11
 * says trust well-formedness is advisory and MUST NOT reject a bundle. Phase 2
 * surfaces a malformed actor as an `_okf.advisories[]` entry instead.
 */
const Actor = z.string();

const Actorstamp = z.object({
  by: Actor,
  at: z.string().optional(),
});

/**
 * A `sources[]` entry (spec §5.1). `resource` is required by the spec *within*
 * an entry, but it is left optional here so a sloppy bundle still renders;
 * Phase 2 reports the omission as an advisory rather than failing the build.
 */
const Source = z
  .object({
    id: z.string().optional(),
    resource: z.string().optional(),
    title: z.string().optional(),
    author: Actor.optional(),
    usage_count: z.union([z.number(), z.string()]).optional(),
    last_modified: z.string().optional(),
  })
  .passthrough();

/** The derived trust tiers (spec §5.3). */
export const tierValues = ["unverified", "machine-confirmed", "human-reviewed"] as const;

/**
 * The loader-derived block.
 *
 * Phase 1 computes only the tier and staleness. The graph, footnote-join and
 * advisory fields described in the design arrive in Phases 2 and 3 and are
 * deliberately ABSENT here rather than present-and-empty: an empty
 * `brokenLinks: []` would read as "this concept has no broken links", which
 * Phase 1 has not checked and therefore must not say.
 */
const Derived = z.object({
  /** Which OKF file role this entry came from. Phase 1 emits concepts only. */
  kind: z.enum(["concept"]),
  /** Derived trust tier (spec §5.3). Never a stored claim. */
  tier: z.enum(tierValues),
  /**
   * The exact `verified[]` stamps that justify `tier` — empty for `unverified`.
   * Renderers show this as the evidence behind the badge, so a reader can check
   * the derivation instead of trusting it.
   */
  tierBasis: z.array(Actorstamp),
  /** `today >= stale_after` evaluated at build time (spec §5.5). */
  stale: z.boolean(),
  /** The `stale_after` value the staleness decision was made against, if any. */
  staleAfter: z.string().optional(),
  /** The `YYYY-MM-DD` UTC day `stale` was evaluated for. Makes builds auditable. */
  evaluatedOn: z.string(),
});

/**
 * Returns the Zod schema for an OKF concept collection.
 *
 * Usage:
 * ```ts
 * defineCollection({ loader: okfLoader({ bundle: "./my-bundle" }), schema: okfSchema() });
 * ```
 */
export const okfSchema = () =>
  z
    .object({
      // `type` is the one always-required concept key (spec §4.1/§11). Unknown
      // type VALUES are fine — the vocabulary is open, so this is a string, not
      // an enum.
      type: z.string(),
      title: z.string().optional(),
      description: z.string().optional(),
      resource: z.string().optional(),
      source: z.string().optional(),
      tags: z.array(z.string()).optional(),
      generated: Actorstamp.optional(),
      // Normalized upstream in parse.ts: a bare `{ by, at }` mapping has already
      // become a one-element list by the time it reaches this schema (spec §5.2).
      verified: z.array(Actorstamp).optional(),
      sources: z.array(Source).optional(),
      usage_window: z
        .object({ from: z.string().optional(), to: z.string().optional() })
        .passthrough()
        .optional(),
      // Not an enum: spec §5.4 fixes the vocabulary but §11 forbids rejecting a
      // document over it. Phase 2 turns an off-vocabulary value into an advisory.
      status: z.string().optional(),
      stale_after: z.string().optional(),
      _okf: Derived,
    })
    .passthrough();

/** The validated shape of an entry's `data`, for consumers typing their pages. */
export type OkfEntryData = z.infer<ReturnType<typeof okfSchema>>;
