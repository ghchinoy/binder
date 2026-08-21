/**
 * Trust derivation — a direct port of binder's `internal/okf/trust.go`.
 *
 * The OKF spec fixes these rules (§5.3 tiers, §5.5 staleness, §7 actors), and
 * binder already implements them. This module re-implements the SAME rules in
 * pure TypeScript so that a bundle rendered through `astro-okf` shows exactly
 * the tier binder computes for it. Divergence here would mean the site and the
 * producer disagree about a bundle, which is precisely the kind of unearned
 * claim this package exists to prevent.
 *
 * Nothing in this module is stored in a bundle. Everything it returns is
 * DERIVED, and callers are expected to keep it in the namespaced `_okf` block
 * so a template can never confuse it with frontmatter the author wrote.
 */

/** A derived trust level (spec §5.3). Never stored, only computed. */
export type Tier = "unverified" | "machine-confirmed" | "human-reviewed";

/** A `{ by, at }` provenance stamp (spec §5.2). */
export interface Actorstamp {
  by: string;
  at?: string;
}

/** Actor prefixes that identify a non-tool actor (spec §7). */
const ACTOR_PREFIXES = ["human:", "process:", "team:"] as const;

/**
 * Reports whether `actor` carries the `human:` prefix that promotes a verified
 * stamp to the human-reviewed tier (spec §5.3).
 *
 * Port of `okf.IsHumanActor`. This is the SINGLE predicate `deriveTier` uses,
 * so the tier and any downstream projection can never disagree about what
 * counts as human review.
 */
export function isHumanActor(actor: string): boolean {
  return typeof actor === "string" && actor.startsWith("human:");
}

/**
 * Reports whether `actor` follows the actor convention (spec §7):
 * `<producer>/<version>` for tools and agents, or one of the `human:`,
 * `process:`, `team:` prefixes for people, processes and teams.
 *
 * Port of `okf.IsValidActor`. A false result is ADVISORY only — it never
 * rejects a concept (spec §11).
 */
export function isValidActor(actor: string): boolean {
  if (typeof actor !== "string") return false;
  const a = actor.trim();
  if (a === "") return false;
  for (const p of ACTOR_PREFIXES) {
    if (a.startsWith(p)) return a.length > p.length;
  }
  // "<producer>/<version>": a single leading slash with non-empty sides and no
  // whitespace anywhere (matches Go's IndexByte + ContainsAny check).
  const i = a.indexOf("/");
  if (i > 0 && i < a.length - 1) return !/[ \t]/.test(a);
  return false;
}

/**
 * Derives the trust tier from a concept's `verified[]` events (spec §5.3).
 *
 * Port of `okf.TrustTier`: no `verified` entries means `unverified`; any
 * `verified[].by` with the `human:` prefix means `human-reviewed`; otherwise
 * `machine-confirmed`. The tier can never exceed what the stamps support.
 */
export function deriveTier(verified: readonly Actorstamp[] | undefined): Tier {
  if (!verified || verified.length === 0) return "unverified";
  for (const v of verified) {
    if (isHumanActor(v?.by ?? "")) return "human-reviewed";
  }
  return "machine-confirmed";
}

/**
 * Reports whether a concept is stale as of `today` (a `YYYY-MM-DD` string),
 * i.e. `today >= stale_after` (spec §5.5).
 *
 * Port of `okf.IsStale`, including its lexicographic string comparison — for
 * zero-padded ISO dates that ordering is the same as chronological ordering,
 * and matching it exactly is what keeps binder and this loader in agreement.
 * A concept without `stale_after` is never stale.
 */
export function isStale(staleAfter: string | undefined, today: string): boolean {
  if (!staleAfter) return false;
  return today >= staleAfter;
}

/**
 * Formats a `Date` as the `YYYY-MM-DD` UTC day string that {@link isStale}
 * compares against. UTC (not local time) keeps a build reproducible wherever
 * it runs.
 */
export function toISODay(now: Date): string {
  return now.toISOString().slice(0, 10);
}
