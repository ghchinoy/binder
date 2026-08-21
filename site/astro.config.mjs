// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// https://astro.build/config
//
// Hosting (LOCKED owner decision): the DEFAULT project GitHub Pages site at
// https://ghchinoy.github.io/binder/. Project Pages are served from a
// sub-path, so `site` is the origin and `base` is the "/binder" prefix.
// Starlight/Astro make every internal link and asset base-aware given these,
// so pages resolve under /binder/ with no hand-edited URLs. No custom domain.
export default defineConfig({
  site: "https://ghchinoy.github.io",
  base: "/binder",
  integrations: [
    starlight({
      title: "binder",
      // Pages are sourced from src/content/docs/. The user-facing docs under
      // ../docs are copied in at build time by scripts/prepare-content.mjs
      // (into _generated/) with Starlight frontmatter prepended — templating,
      // not generation. The sidebar references each page's PUBLISHED slug (the
      // clean URL), not its on-disk _generated/ path.
      sidebar: [
        { label: "Tutorial", slug: "tutorial" },
      ],
    }),
  ],
});
