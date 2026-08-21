// version.ts DERIVES the current released binder version at BUILD time. No
// version string is ever hand-typed into the site — the homepage version badge
// (and any future install snippet that wants to pin a version) read whatever
// this returns. The single source of truth is the GitHub Releases API for
// ghchinoy/binder.
//
// This runs during the SSG build (Node), never in the browser.
//
// binder's canonical version stamp has NO leading "v" (e.g. `binder/<version>`),
// while the git tag IS "v"-prefixed (e.g. `vX.Y.Z`). So this module exposes both
// forms and never conflates them:
//   - `tag`  — as published, "v"-prefixed (for release links / URLs).
//   - `bare` — the "v" stripped (for DISPLAY — binder's canonical stamp form).
//
// Determinism / graceful fallback (binder ethos — never assert the unverified):
// if the repo has no release yet, or the API is unreachable / rate-limited /
// offline during the build, this returns nulls rather than crashing. Callers
// render a sensible fallback (see `displayVersion`, `FALLBACK_DISPLAY`) so the
// site still builds from a clean checkout with no network. No fake current
// version is ever fabricated.

const RELEASES_API =
  "https://api.github.com/repos/ghchinoy/binder/releases/latest";

// What the UI shows when the version cannot be resolved at build time. It is a
// word, never a fabricated number — an honest "we could not verify a specific
// release, install the latest" rather than a made-up X.Y.Z.
export const FALLBACK_DISPLAY = "latest";

export interface BinderVersion {
  // The tag as published (e.g. `vX.Y.Z`), or `null` when unresolved.
  tag: string | null;
  // The tag with a leading "v" stripped (e.g. `X.Y.Z`) — binder's canonical
  // display stamp. `null` under the same conditions as `tag`.
  bare: string | null;
}

// Memoize the fetch so every caller in a single build (the homepage badge and
// the /version.json endpoint) shares ONE result — the badge the reader sees and
// the machine-readable JSON the guardrail test checks can never disagree within
// a build, regardless of network timing.
let cached: Promise<BinderVersion> | null = null;

/**
 * Resolve the latest published binder release at build time.
 *
 * Degrades gracefully: returns `{ tag: null, bare: null }` (never throws) when
 * there is no release, or the Releases API is unreachable / rate-limited /
 * offline. Callers must handle the null case (see `displayVersion`).
 */
export function fetchLatestVersion(): Promise<BinderVersion> {
  if (!cached) cached = doFetch();
  return cached;
}

async function doFetch(): Promise<BinderVersion> {
  const headers: Record<string, string> = {
    Accept: "application/vnd.github+json",
    "User-Agent": "binder-site-build",
    "X-GitHub-Api-Version": "2022-11-28",
  };
  // CI (docs.yml) already passes GITHUB_TOKEN; using it raises the rate limit
  // but is entirely optional — the unauthenticated path works too.
  const token = process.env.GITHUB_TOKEN;
  if (token) headers.Authorization = `Bearer ${token}`;

  try {
    const res = await fetch(RELEASES_API, { headers });
    if (!res.ok) {
      console.warn(
        `version: GitHub releases API returned ${res.status}; ` +
          `falling back to "${FALLBACK_DISPLAY}" for the version display.`,
      );
      return { tag: null, bare: null };
    }
    const data = (await res.json()) as { tag_name?: unknown };
    const tag = typeof data.tag_name === "string" ? data.tag_name : null;
    if (!tag) {
      console.warn(
        `version: releases API returned no tag_name; ` +
          `falling back to "${FALLBACK_DISPLAY}".`,
      );
      return { tag: null, bare: null };
    }
    return { tag, bare: tag.replace(/^v/, "") };
  } catch (err) {
    console.warn(
      `version: could not reach the GitHub releases API (${String(err)}); ` +
        `falling back to "${FALLBACK_DISPLAY}" for the version display.`,
    );
    return { tag: null, bare: null };
  }
}

/**
 * The BARE version to display, or the honest `FALLBACK_DISPLAY` word when the
 * version could not be verified. This is what the UI shows — always sourced
 * here, never hand-typed at a call site.
 */
export function displayVersion(v: BinderVersion): string {
  return v.bare ?? FALLBACK_DISPLAY;
}

/**
 * The canonical GitHub Releases URL for the resolved version: the exact tag's
 * release page when known, otherwise the releases index. Uses the "v"-prefixed
 * `tag` form (URLs use the tag, display uses `bare`).
 */
export function releaseUrl(v: BinderVersion): string {
  const base = "https://github.com/ghchinoy/binder/releases";
  return v.tag ? `${base}/tag/${v.tag}` : base;
}
