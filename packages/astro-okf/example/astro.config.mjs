// @ts-check
import { defineConfig } from "astro/config";

// The smallest host that can prove the loader works: no integrations, no
// theme, no assets. Its only job is to run a real `astro build` over a real
// binder-emitted OKF bundle so the Content Layer contract is exercised for
// real, and to emit HTML the Phase 1 acceptance tests can assert against.
export default defineConfig({
  outDir: "./dist",
  // The example ships no images, so skipping sharp keeps the build free of an
  // optional native dependency.
  image: { service: { entrypoint: "astro/assets/services/noop" } },
});
