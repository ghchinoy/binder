// A tiny machine-readable snapshot of the version version.ts resolved for THIS
// build. It exists for two honest reasons:
//   1. It gives agents / scripts a stable endpoint (/binder/version.json) to
//      read "which binder version does the site currently advertise".
//   2. It is how the no-hand-typed-version guardrail test ties the rendered
//      homepage badge back to version.ts: the endpoint and the badge both call
//      the SAME memoized fetchLatestVersion() within one build, so they can
//      never disagree — proving the displayed version is DERIVED, not typed.
//
// Emitted as a static file (dist/version.json) at build time; no runtime.

import type { APIRoute } from "astro";
import {
  fetchLatestVersion,
  displayVersion,
  FALLBACK_DISPLAY,
} from "../lib/version.ts";

export const GET: APIRoute = async () => {
  const v = await fetchLatestVersion();
  const body = {
    tag: v.tag,
    bare: v.bare,
    display: displayVersion(v),
    fallback: FALLBACK_DISPLAY,
    resolved: v.bare !== null,
    source: "https://api.github.com/repos/ghchinoy/binder/releases/latest",
  };
  return new Response(JSON.stringify(body, null, 2) + "\n", {
    headers: { "Content-Type": "application/json; charset=utf-8" },
  });
};

// Static build: no on-demand rendering needed.
export const prerender = true;
