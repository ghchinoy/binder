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

// ── The claim/disclaimer boundary (narrowed after review round 1) ────────────
// Round 1 matched disclaimer words SENTENCE-WIDE and over-broadly (only / would
// / future / planned / bare "no"), so a present-tense fabricated CLAIM that
// merely co-occurred with one of those words slipped through as an "allowed
// disclaimer" (e.g. "…signed with cosign, and only the SBOM is omitted."). The
// fix is twofold: (1) analyze CLAUSE-by-clause, not sentence-wide, and (2) count
// only markers that ACTUALLY negate the capability — genuine negations, "no"
// BOUND to the capability noun, or explicit scope-outs — dropping the loose
// only/would/future/planned. A clause is a claim iff it names a supply-chain
// capability AND an affirmative verb AND is not itself a disclaimer clause.

// Genuine negations that actually negate a predicate ("is NOT signed", "does
// NOT ship", "without cosign", "no longer signs").
const NEGATION = /\b(not|never|without|cannot|can ?not|can't|isn't|aren't|wasn't|weren't|doesn't|don't|won't|no longer|neither|nor)\b/i;

// "no" BOUND directly to the capability, which negates it: "no cosign", "no
// SBOM", "no supply-chain attestation". (Bare "no" elsewhere is NOT a
// disclaimer — that was the round-1 hole.)
const BOUND_NO = /\bno\s+(cosign|sboms?|supply|signatures?|attestations?|provenance)\b/i;

// Explicit scope-out / not-configured disclosures (binder's honest idiom).
const SCOPE_OUT = /\b(out[- ]of[- ]scope|deferred|intentionally|not configured|unconfigured|unsupported|unimplemented|unavailable)\b|scope:/i;

// Affirmative verbs that, next to a capability keyword, assert the capability.
const CLAIM_VERB = /\b(sign|signed|signs|signing|publish|published|publishes|provide|provided|provides|include|included|includes|attach|attached|attest|attested|attests|generate|generated|generates|ship|ships|shipped|available|supported|supports|emit|emits|produce|produces)\b/i;

// Capability keywords for the supply-chain claim category.
const SUPPLY_CHAIN = /\b(cosign|sbom|sboms|supply[- ]chain|provenance attestation|signature attestation)\b/i;

// A clause is a DISCLAIMER (allowed) when it genuinely disavows the capability.
function isDisclaimerClause(clause) {
  return NEGATION.test(clause) || BOUND_NO.test(clause) || SCOPE_OUT.test(clause);
}

// Split text into rough sentences, then each sentence into clauses on
// punctuation and coordinating conjunctions — so a claim clause and a disclaimer
// clause in the same sentence are judged independently ("X is signed with
// cosign, and only the SBOM is omitted." → the cosign clause is judged a claim).
function sentences(text) {
  return text.split(/(?<=[.!?])\s+|\n+/).map((s) => s.trim()).filter(Boolean);
}
// NOTE: "yet" is deliberately NOT a splitter here. In binder's honest idiom it
// is almost always the adverb of "not configured YET" / "not published YET" —
// splitting on it would sever a disclaimer from its own tail clause (e.g. the
// parenthetical in "…intentionally not configured yet (no signs: / sboms:
// blocks)"). Leaving it un-split is also strictly SAFER for catching claims: a
// real claim joined by "yet" stays in one clause and is still flagged.
function clauses(sentence) {
  return sentence
    .split(/[,;]|\s+(?:and|but|so|while|whereas|however|although|though)\s+/i)
    .map((c) => c.trim())
    .filter(Boolean);
}

// Return the supply-chain CLAIM clauses in `text`: a clause that names a
// capability AND an affirmative verb AND is not itself a disclaimer clause.
function supplyChainClaims(text) {
  const out = [];
  for (const s of sentences(text)) {
    for (const c of clauses(s)) {
      if (SUPPLY_CHAIN.test(c) && CLAIM_VERB.test(c) && !isDisclaimerClause(c)) {
        out.push(c);
      }
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
  // Positive controls — real fabrications MUST be caught (non-empty).
  const POSITIVE = [
    "Every release is signed with cosign.",
    "An SBOM is published for each build.",
    // The three that SLIPPED the round-1 sentence-wide matcher (only/would/future):
    "Every binder release is signed with cosign, and only the SBOM is omitted.",
    "binder signs every release with cosign, as you would expect.",
    "The next future release is signed with cosign and ships an SBOM.",
  ];
  for (const s of POSITIVE) {
    assert.ok(
      supplyChainClaims(s).length > 0,
      `detector MISSED a fabricated capability claim: "${s}"`,
    );
  }

  // Negative controls — honest disclaimers MUST pass (empty). Includes binder's
  // real byte-derived idiom and the reviewer's suggested disclosures.
  const NEGATIVE = [
    "Cosign signatures and SBOMs are not published yet, so checksums.txt is the integrity anchor.",
    "cosign signatures and SBOMs are intentionally deferred.",
    "cosign signing is not configured for binder releases.",
    "SBOM generation is out of scope for now.",
    "binder does not sign releases with cosign and publishes no SBOM.",
    // binder's REAL byte-derived idiom (from docs/ → project/releasing): the
    // adverbial "yet" plus a parenthetical shorthand must stay ALLOWED.
    "cosign signatures and SBOMs are intentionally not configured yet (no signs: / sboms: blocks).",
  ];
  for (const s of NEGATIVE) {
    assert.deepEqual(
      supplyChainClaims(s),
      [],
      `detector WRONGLY flagged an honest disclaimer: "${s}"`,
    );
  }
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
