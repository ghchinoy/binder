// no-fabrication.test.mjs — the machine expression of binder's ethos "never
// assert the unverified" for the site itself (design §2, §4.7, §9.5). The built
// site must not CLAIM a capability binder does not ship:
//   - no Search UI / nav (binder has no search; pagefind:false),
//   - no cosign / SBOM supply-chain CLAIM (binder ships checksums.txt only),
//   - no "author a bundle" / authoring-`node` lifecycle guide (binder has none),
//   - no migrate page/command (binder has no migrate).
//
// ── The allowlist boundary: CLAIM vs. honest DISCLAIMER ──────────────────────
// binder's own docs HONESTLY disclose what it does NOT do — e.g. "Cosign
// signatures and SBOMs are not published yet, so checksums.txt is the …". Those
// disclosures are byte-derived from docs/ and are REQUIRED honesty (§9.5), not
// fabrication. So this suite does NOT ban the mere words "cosign"/"SBOM". It
// targets:
//   (a) UI/nav PRESENCE for search (unambiguous — the widget is there or not), and
//   (b) affirmative CAPABILITY CLAIM phrasings, detected sentence-by-sentence
//       with negation/disclaimer awareness, so "are NOT published yet" is allowed
//       while "signed with cosign" is caught.
// The positive/negative controls below PIN that the detector actually draws that
// line — they are the load-bearing half of this test.

import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { join, relative } from "node:path";
import { dist, distHtmlFiles, read } from "./_helpers.mjs";

// Strip tags to visible text for sentence-level claim analysis.
function toText(html) {
  return html
    .replace(/<script[\s\S]*?<\/script>/gi, " ")
    .replace(/<style[\s\S]*?<\/style>/gi, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&#39;|&apos;/g, "'")
    .replace(/&quot;/g, '"')
    .replace(/&nbsp;/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

// A sentence carries a DISCLAIMER (an honest "we do NOT do this") if it holds a
// negation or scope marker. These flip a would-be claim into allowed honesty.
const DISCLAIMER = /\b(not|no|never|without|yet|deferred|intentionally|out of scope|out-of-scope|scope:|unverified|does not|do not|doesn't|don't|cannot|can't|no longer|absent|missing|unsupported|planned|future|would|only)\b/i;

// Affirmative verbs that, next to a capability keyword, assert the capability.
const CLAIM_VERB = /\b(sign|signed|signs|signing|publish|published|publishes|provide|provided|provides|include|included|includes|attach|attached|attest|attested|attests|generate|generated|generates|ship|ships|shipped|available|supported|supports|emit|emits|produce|produces)\b/i;

// Capability keywords for the supply-chain claim category.
const SUPPLY_CHAIN = /\b(cosign|sbom|sboms|supply[- ]chain|provenance attestation|signature attestation)\b/i;

// Split text into rough sentences for scoped analysis.
function sentences(text) {
  return text.split(/(?<=[.!?])\s+|\n+/).map((s) => s.trim()).filter(Boolean);
}

// Return the supply-chain CLAIM sentences in `text` (claim verb + keyword,
// WITHOUT a disclaimer marker). Honest "not published yet" sentences are NOT
// returned.
function supplyChainClaims(text) {
  const out = [];
  for (const s of sentences(text)) {
    if (SUPPLY_CHAIN.test(s) && CLAIM_VERB.test(s) && !DISCLAIMER.test(s)) {
      out.push(s);
    }
  }
  return out;
}

// Unambiguous forbidden phrasings/commands — binder has NO such surface, so any
// appearance is a fabrication regardless of sentence context.
const FORBIDDEN_PHRASES = [
  { re: /\bauthor(?:ing)?\s+(?:a\s+)?bundle\b/i, why: '"author(ing) a bundle" lifecycle (binder has none)' },
  { re: /\bbundle\s+authoring\b/i, why: '"bundle authoring" lifecycle (binder has none)' },
  { re: /\bbinder\s+node\b/i, why: '`binder node` (no such command)' },
  { re: /\bbinder\s+bundle\b/i, why: '`binder bundle` (no such command)' },
  { re: /\bbinder\s+log\b/i, why: '`binder log` (no such command)' },
  { re: /\bbinder\s+migrate\b/i, why: '`binder migrate` (no such command)' },
];

// Search UI/nav markers. `data-pagefind-body` (a plain content attribute) is NOT
// a search widget and is intentionally excluded — these target the actual UI.
const SEARCH_UI = [
  { re: /data-open-modal/i, why: "Starlight search-open trigger" },
  { re: /class=["'][^"']*\bsite-search\b/i, why: "site-search widget" },
  { re: /role=["']search["']/i, why: 'role="search"' },
  { re: /type=["']search["']/i, why: 'type="search" input' },
  { re: /<(?:aside|dialog)[^>]*\bpagefind/i, why: "pagefind search dialog" },
];

test("dist/ exists (build ran first)", () => {
  assert.ok(existsSync(dist), "dist/ not found — run `npm run build` first");
});

test("controls: the claim detector draws the claim/disclaimer line", () => {
  // Positive controls — real fabrications MUST be caught.
  assert.ok(
    supplyChainClaims("Every release is signed with cosign.").length > 0,
    "detector missed an affirmative cosign claim",
  );
  assert.ok(
    supplyChainClaims("An SBOM is published for each build.").length > 0,
    "detector missed an affirmative SBOM claim",
  );
  // Negative controls — honest disclaimers from binder's own docs MUST pass.
  assert.deepEqual(
    supplyChainClaims(
      "Cosign signatures and SBOMs are not published yet, so checksums.txt is the integrity anchor.",
    ),
    [],
    "detector wrongly flagged an honest 'not published yet' disclaimer",
  );
  assert.deepEqual(
    supplyChainClaims("cosign signatures and SBOMs are intentionally deferred."),
    [],
    "detector wrongly flagged an honest 'intentionally deferred' disclaimer",
  );
});

test("no search UI / widget in the built site", async () => {
  assert.ok(
    !existsSync(join(dist, "pagefind")),
    "dist/pagefind/ exists — a search index shipped (pagefind must stay false)",
  );
  const offenders = [];
  for (const f of await distHtmlFiles()) {
    const html = await read(f);
    for (const { re, why } of SEARCH_UI) {
      if (re.test(html)) offenders.push(`${relative(dist, f)}: ${why}`);
    }
  }
  assert.deepEqual(offenders, [], `search UI present:\n  ${offenders.join("\n  ")}`);
});

test("no forbidden capability CLAIM in built HTML (disclaimers allowed)", async () => {
  const claimOffenders = [];
  const phraseOffenders = [];
  for (const f of await distHtmlFiles()) {
    const html = await read(f);
    const text = toText(html);

    for (const s of supplyChainClaims(text)) {
      claimOffenders.push(`${relative(dist, f)}: "${s.slice(0, 120)}"`);
    }
    for (const { re, why } of FORBIDDEN_PHRASES) {
      if (re.test(text)) phraseOffenders.push(`${relative(dist, f)}: ${why}`);
    }
  }
  assert.deepEqual(
    claimOffenders,
    [],
    `supply-chain capability CLAIMS found (honest disclaimers are fine):\n  ${claimOffenders.join("\n  ")}`,
  );
  assert.deepEqual(
    phraseOffenders,
    [],
    `forbidden capability phrasings found:\n  ${phraseOffenders.join("\n  ")}`,
  );
});
