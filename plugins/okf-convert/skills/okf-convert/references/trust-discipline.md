# Trust discipline — never fabricate trust

This is the single overriding guardrail of the `okf-convert` skill. When you
drive binder you hold the same line binder holds: **propose trust, never
fabricate it; defer all stamping to the deterministic tool.**

## What binder does (and does not) stamp

- binder stamps an **honest** `generated: binder/<version>` provenance mark on
  what it produces. That is a true statement about how the file was generated.
- binder **never invents** a `verified` actor or `sources`. Trust mapping is off
  by default and byte-faithful — it only maps signals you explicitly point it at
  (`--source-keys`, `--map-citations`), and only from real corpus content.
- **But binder does stamp `verified` from a configured actor.** `--verified-by`
  resolves through the precedence chain **flag > env > config file > default**, so
  an inherited `BINDER_VERIFIED_BY` or a `verified_by:` key in `.binder.yaml`
  makes `convert` *and* `enrich` write an attestation nobody typed. "I did not
  pass the flag" is therefore **not** a guarantee that no claim was made. Run the
  step 1 pre-flight (`binder config --json`) and pass `--verified-by ""` to
  suppress an actor you cannot vouch for.

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
- **Do NOT let a configured actor stamp on your behalf.** Check
  `binder config --json | jq -c '.result | {config_file, verified_by: .values.verified_by}'`
  first. A `source` of `env`, or of `file` where `config_file` is a machine-wide
  path rather than the corpus's own `.binder.yaml`, is inherited state and not an
  attestation — pass `--verified-by ""` to suppress it. Note `source` alone cannot
  distinguish a repo-local config from a global one; `config_file` is what tells
  you which file is actually in effect.
- **Suppress on `convert` as well as `enrich`.** Both resolve `--verified-by`
  through the same chain, so a configured actor stamps the whole bundle on
  `convert` even if you never touched `enrich`. Check the result with the `tiers`
  line from `binder review`: `{"unverified": N}` means nothing was claimed,
  `{"human-reviewed": N}` means something was.
- **Run the pre-flight from the directory you will run everything else from.**
  binder resolves `./.binder.yaml` against the *current* directory, so a check run
  from a parent reports `default` while the real run, one directory down, stamps.
  The detection and the risk must share a cwd or the check is theatre.
- **Stamps are not idempotent under a wall clock.** Dedup is on `(by, at)`, so
  re-running `enrich` seconds apart appends a fresh stamp for the same actor each
  time. Pin `SOURCE_DATE_EPOCH` when you need a repeatable run.
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

> [`ghchinoy/agent-skills` → `plugins/okf-authoring` →
> `references/trust-vocabulary.md`](https://github.com/ghchinoy/agent-skills/blob/main/plugins/okf-authoring/references/trust-vocabulary.md)

The OKF v0.2 spec those definitions derive from is
[`GoogleCloudPlatform/knowledge-catalog` → `okf/SPEC.md`](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).

That is a **documentation pointer, not a runtime dependency** — Agent Plugins are
self-contained and this skill resolves nothing across repos at run time. Note
`okf-authoring` ships from a *different* plugin marketplace than this one. The
short restatement above is sufficient to drive binder correctly; load the Layer A
vocabulary when you need the full field-by-field definitions.
