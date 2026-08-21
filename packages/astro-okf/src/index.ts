/**
 * astro-okf — an Astro Content Layer loader for OKF v0.2 bundles.
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

export { okfLoader, type OkfLoaderOptions, type OkfDerived } from "./loader.js";
export { okfSchema, tierValues, type OkfEntryData } from "./schema.js";
export {
  deriveTier,
  isStale,
  isHumanActor,
  isValidActor,
  toISODay,
  type Actorstamp,
  type Tier,
} from "./trust.js";
export {
  splitFrontmatter,
  normalizeActorstamp,
  normalizeActorstamps,
  type SplitFile,
} from "./parse.js";
