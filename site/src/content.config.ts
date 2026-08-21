import { defineCollection } from "astro:content";
import { docsLoader } from "@astrojs/starlight/loaders";
import { docsSchema } from "@astrojs/starlight/schema";

// Starlight's Content Layer loader reads src/content/docs/. For Phase 1 that
// directory holds only the _generated/ copies that scripts/prepare-content.mjs
// produces from ../docs at build time (gitignored, rebuilt every build). This
// collection just renders them; it does not generate content.
export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
};
