import { defineCollection } from "astro:content";
import { okfLoader, okfSchema } from "astro-okf";

// The bundle is binder's own emitted fixture, read in place from testdata/ —
// nothing is vendored or copied, so this example renders exactly what binder
// produces. The path is relative to this Astro project's root (this directory's
// parent), which the loader resolves against `config.root`.
const BUNDLE = "../../../testdata/expected-rich";

// The clock is pinned so the staleness assertions in the Phase 1 acceptance
// tests mean the same thing on every machine and on any future day. A real site
// would leave `now` unset and get the actual build time.
const PINNED_NOW = new Date("2026-08-21T00:00:00Z");

export const collections = {
  kb: defineCollection({
    loader: okfLoader({ bundle: BUNDLE, now: () => PINNED_NOW }),
    schema: okfSchema(),
  }),
};
