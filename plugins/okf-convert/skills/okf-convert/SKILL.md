---
name: okf-convert
description: Drive the binder CLI to ingest a plain-markdown corpus into a conformant Open Knowledge Format (OKF) v0.2 bundle, reasoning over binder's structured --json output rather than scraping prose. Use when asked to convert, ingest, or migrate a whole directory of markdown notes/docs into an OKF bundle, or to drive binder end-to-end (convert, validate, review). Teaches the ingestion-analysis judgment the deterministic converter cannot encode: detect binder and its JSON contract, dry-run and read the triage, convert for real, and validate for §11 conformance. Assumes the binder binary is installed — this is the binder-present, binder-driven surface. For authoring or validating a SINGLE bundle by hand with no binaries, use the tool-agnostic okf-author / okf-validate skills instead. Never fabricates trust: proposes trust and defers all stamping to binder; never invents verified/sources it cannot assert.
license: Apache-2.0
metadata:
  version: "1.0.0"
  sources:
    - Open Knowledge Format (OKF) SPEC.md v0.2
    - binder --json envelope (schema binder.report/v1)
---

# Ingest a markdown corpus into an OKF v0.2 bundle with binder

This skill teaches you to **drive the `binder` CLI** to turn a plain-markdown
corpus (a directory tree of `.md` files) into a **conformant OKF v0.2 bundle**,
making the ingestion-analysis judgment calls the deterministic converter cannot
make for you, and reasoning over binder's **structured `--json` output** — never
scraping its prose.

**This is the binder-present, binder-driven surface (Layer B).** It *assumes
binder is installed* and is the deliberate opposite of the tool-agnostic
`okf-author` / `okf-validate` skills, which author or validate a **single**
bundle by hand with zero binaries. For hand-authoring one concept, or for the
OKF format and trust vocabulary themselves, defer to those skills by name — do
not re-derive them here.

## The one guardrail that overrides everything: never fabricate trust

binder only ever stamps an honest `generated: binder/<version>` provenance mark.
It **never** auto-stamps `verified` and **never** invents `sources`. When you
drive binder you must hold the same line:

- Do **not** pass `--verified-by` unless a real, named actor actually attests to
  the content. A verification stamp is a claim; only make claims that are true.
- Do **not** invent `sources`/provenance the corpus does not contain.
- Do **not** write a credibility score or trust *tier* into any concept — tiers
  are *derived* on read, never stored.

Propose trust to the user; defer all stamping to deterministic binder. (Fuller
treatment lands in `references/trust-discipline.md`.)

## The binder JSON contract (what you parse)

Every binder command accepts `--json` and prints one deterministic envelope:

```json
{ "binder": "binder/0.1.0", "command": "convert",
  "schema": "binder.report/v1", "result": { } }
```

Reason over `.result` with `jq`; never parse the human prose. Exit codes are a
4-value contract: **0** ok · **1** findings/gate tripped · **2** usage error ·
**3** I/O error.

## Procedure (minimal end-to-end path)

### 1. Detect binder and confirm the contract

```bash
command -v binder || { echo "binder not installed"; exit 1; }
binder --version    # confirm you are on a binder that emits --json
```

If binder is **absent**, tell the user to install it — do **not** fake a
conversion. (For by-hand single-bundle authoring with no binary, hand off to the
`okf-author` skill.)

### 2. Dry-run and read the structured triage

Never write a bundle blind. Run a dry-run first and reason over the report:

```bash
binder convert <corpus> --dry-run --json > triage.json
jq '.result | {num_concepts, num_links, num_unresolved, num_recovered}' triage.json
jq '.result.unresolved' triage.json   # links that will not resolve
jq '.result.warnings'   triage.json   # recovered/unparseable frontmatter, etc.
```

Decide **remediate-source vs accept** *before* writing:

- **Unresolved links** — a real broken link in the source? Fix the source. A
  legal link to not-yet-written knowledge? Accept it (broken links are legal in
  OKF).
- **Recovered frontmatter** (`num_recovered` > 0) — files whose frontmatter did
  not parse were recovered as body. Inspect and fix the source frontmatter if
  the loss matters.
- **Type distribution** — every concept needs a non-empty `type`. If the corpus
  has none, choose `--default-type` and/or `--type-map` (per-directory types)
  now. (Full flag-choice triage: `references/ingestion-workflow.md`.)

### 3. Convert for real and validate for conformance

```bash
binder convert <corpus> -o <bundle> --json | jq '.result | {num_concepts, num_unresolved}'
binder validate <bundle> --json | jq '.result.findings'
```

`binder validate` checks the OKF §11 hard rule (every non-reserved `.md` has
parseable frontmatter with a **non-empty `type`**). A conformant bundle reports
`findings: []` and **exits 0**. If `findings` is non-empty (exit 1), read each
finding, fix the source or your flag choices, and re-run step 2→3 until validate
is clean.

### 4. Hand off

Report the bundle location, what you remediated vs accepted, and any residual
advisories (unresolved links, recovered files) so the user knows the bundle's
state. Do not add trust you could not honestly assert (see the guardrail above).

---

*A tiny plain-markdown corpus you can practice on ships at
[`assets/sample-corpus/`](assets/sample-corpus/): run steps 2–3 against it and
`binder validate` should report a conformant bundle (exit 0).*
