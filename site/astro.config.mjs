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
      // Brand seam: one stylesheet maps Starlight's --sl-* variables (palette,
      // type, accents) so the site is visibly binder's OWN — a clean, modern
      // tech feel, not stock Starlight. Restrained v1: no glassmorphism, no
      // heavy effects. See src/styles/tokens.css.
      customCss: ["./src/styles/tokens.css"],
      // O2 hygiene: binder has no search (pagefind:false), but Starlight's
      // default Search component still bundles a dead search script chunk.
      // Override it with a no-op so the build ships no search UI or dead chunk.
      components: {
        Search: "./src/components/EmptySearch.astro",
      },
      // No search UI. binder has no search capability, so shipping Starlight's
      // built-in Pagefind search box would assert an unshipped capability —
      // forbidden unconditionally by the design (§2 "site MUST NOT ship a search
      // box", §9.5 "no Search UI") and by binder's own ethos (never assert the
      // unverified). Disabling pagefind drops both the search markup and the
      // dist/pagefind/ index from the build.
      pagefind: false,
      // Pages are sourced from src/content/docs/. The user-facing docs under
      // ../docs (+ README.md / CONTRIBUTING.md) are copied in at build time by
      // scripts/prepare-content.mjs (into _generated/) with Starlight
      // frontmatter prepended — templating, not generation. The sidebar
      // references each page's PUBLISHED slug (the clean URL), not its on-disk
      // _generated/ path. The IA mirrors design §4.3. The generated Command
      // reference (Reference group) is wired in below — prepare-content.mjs
      // folds docs/commands/ (CI-drift-gated on main) into one page rendered at
      // /reference/commands.
      sidebar: [
        {
          label: "Start",
          items: [
            { label: "What binder is", slug: "overview" },
            { label: "Installation", slug: "install" },
            { label: "Tutorial", slug: "tutorial" },
          ],
        },
        {
          label: "Concepts",
          items: [
            { label: "Byte-faithful round-trip", slug: "concepts/byte-faithfulness" },
            { label: "Trust model & tiers", slug: "concepts/trust" },
            { label: "Relationship extraction & the graph", slug: "concepts/graph" },
            { label: "OKF v0.2 output structure", slug: "concepts/okf-output" },
            { label: "Graph projection (project)", slug: "concepts/project" },
            { label: "In-memory LPG primer", slug: "concepts/lpg-primer" },
          ],
        },
        {
          label: "Guides",
          items: [
            { label: "CI usage", slug: "guides/ci" },
            { label: "Strict mode", slug: "guides/strict-mode" },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "User guide", slug: "reference/user-guide" },
            // Generated Cobra command reference (docs/commands/, drift-gated on
            // main). prepare-content.mjs folds the index + every per-command
            // page into ONE self-contained page at /reference/commands with an
            // auto-generated on-page TOC. Source is CI-proven true to the
            // binary; the site renders it verbatim.
            { label: "Command reference", slug: "reference/commands" },
          ],
        },
        {
          label: "Agent surface",
          items: [
            { label: "MCP server", slug: "agent/mcp" },
            { label: "okf-convert plugin & skill", slug: "agent/plugin" },
          ],
        },
        {
          label: "Project",
          items: [
            { label: "Releasing", slug: "project/releasing" },
            { label: "Contributing", slug: "project/contributing" },
          ],
        },
      ],
    }),
  ],
});
