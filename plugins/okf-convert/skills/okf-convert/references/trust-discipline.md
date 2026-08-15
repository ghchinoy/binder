# Trust discipline — never fabricate trust

This is the single overriding guardrail of the `okf-convert` skill. When you
drive binder you hold the same line binder holds: **propose trust, never
fabricate it; defer all stamping to the deterministic tool.**

## What binder does (and does not) stamp

- binder stamps an **honest** `generated: binder/<version>` provenance mark on
  what it produces. That is a true statement about how the file was generated.
- binder **never** auto-stamps `verified`, and **never** invents `sources`. Trust
  mapping is off by default and byte-faithful — it only maps signals you
  explicitly point it at (`--source-keys`, `--map-citations`), and only from real
  corpus content.

## The three invariants (from OKF v0.2 — do not violate)

1. **Never store a credibility score or trust tier.** Record objective signals
   (`generated`, `verified` events, `sources`); the *tier* (`unverified` /
   `machine-confirmed` / `human-reviewed`) is **derived on read** — `binder
   review` computes it, it is never written to frontmatter. No
   `score`/`credibility`/`tier` keys.
2. **Never reject or drop unknown keys/types.** Preserve producer-defined keys
   verbatim; unknown `type` values are legal.
3. **Broken links and missing optional fields are legal.** Never invent a target,
   a source, or a verifier to make a report look cleaner.

## The binder-driving rules (the crux)

- **Do NOT pass `--verified-by` unless a real, named actor actually attests.** The
  flag appends a `verified` stamp; a stamp is a *claim of verification*. Only pass
  it when a genuine actor (`human:<id>`, `process:<id>`, `team:<id>`, or
  `<producer>/<version>`) has actually reviewed the content. Do not auto-pass it,
  do not default it to yourself, do not stamp the agent as a verifier of content
  it merely generated.
- **Do NOT enable `--source-keys` / `--map-citations` for noise.** Map only keys
  and citation lists that carry *real* provenance. Review which corpus-native
  provenance is genuine before mapping it.
- **Do NOT add `verified`/`sources` the agent cannot assert.** If provenance is
  unknown, leave it absent — absent is legal and honest; invented is a lie.
- **Propose, then defer.** When trust *should* be added, surface it to the user
  and let them (or a real attester) decide; let deterministic binder do the
  stamping. You are the judgment, not the notary.

## The vocabulary lives in Layer A — refer to it by name

This reference deliberately does **not** re-teach the full provenance / trust /
lifecycle vocabulary. For the complete §5 (`sources`, `generated`, `verified`),
§7 (actor convention), and §10 (Attested Computation) definitions, see the
tool-agnostic **`okf-authoring`** plugin's reference:

> `okf-authoring` → `references/trust-vocabulary.md`
> (in `ghchinoy/agent-skills`)

That is a **by-name pointer, not a runtime dependency** — Agent Plugins are
self-contained and this skill resolves nothing across repos at run time. The
short restatement above is sufficient to drive binder correctly; load the Layer A
vocabulary when you need the full field-by-field definitions.
