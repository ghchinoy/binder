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

Propose trust to the user; defer all stamping to deterministic binder. The full
treatment — including the `--verified-by` guardrail — is in
[`references/trust-discipline.md`](references/trust-discipline.md), loaded at
step 6.

## Load these when you need them (progressive disclosure)

Don't pull these into context up front — open each when the step calls for it:

- [`references/binder-json-contract.md`](references/binder-json-contract.md) — the
  exact `--json` envelope, exit codes, and every command's `result` shape with
  `jq` examples. Load whenever you parse binder output.
- [`references/ingestion-workflow.md`](references/ingestion-workflow.md) — the
  pre-convert flag-choice triage, the `enrich`/`lint` remediation loop, and the
  accept-vs-remediate decision table. Load for steps 2–5.
- [`references/trust-discipline.md`](references/trust-discipline.md) — the
  never-fabricate-trust discipline, derive-tiers rule, and the `--verified-by`
  guardrail, with a by-name pointer to Layer A's `trust-vocabulary.md`. Load for
  step 6.

## The binder JSON contract (what you parse)

Every binder command accepts `--json`. `convert`, `enrich`, `validate`, `review`,
and `lint` print one deterministic envelope:

```json
{ "binder": "binder/0.1.0", "command": "convert",
  "schema": "binder.report/v1", "result": { } }
```

Reason over `.result` with `jq`; never parse the human prose. Two exceptions:
`graph --json` emits the **raw** `{nodes,edges}` export (not the envelope), and
`config --json` uses schema `binder.config/v1`. Exit codes are a 4-value
contract: **0** ok · **1** findings/gate tripped · **2** usage error · **3** I/O
error. Full shapes: [`references/binder-json-contract.md`](references/binder-json-contract.md).

## Procedure (the 7-step ingestion loop)

### 1. Detect binder and confirm the contract

```bash
command -v binder || { echo "binder not installed"; exit 1; }
binder --version    # confirm you are on a binder that emits --json
```

If binder is **absent**, tell the user to install it — do **not** fake a
conversion. (For by-hand single-bundle authoring with no binary, hand off to the
`okf-author` skill.)

### 2. Pre-convert triage — choose the flags (judgment)

The converter cannot infer your corpus's conventions; you must decide. Inspect
the tree and pick the flags — all real (`binder convert --help`):

- **Types:** `--default-type` (fallback) and `--type-map "docs=Guide,adr=Decision"`
  (per-directory) so every concept gets a non-empty `type`.
- **Edges:** `--fm-ref-keys "related,parent"` for frontmatter keys that are edges;
  `--workspace-root` for the `file://` resolution boundary.
- **Provenance:** `--source-keys`, `--map-citations` — map only *real* provenance
  (see step 6 first).
- **Lifecycle:** `--map-draft`, `--status-map`, `--stale-after-map` (set only when
  absent).

Full flag-choice triage: [`references/ingestion-workflow.md`](references/ingestion-workflow.md) §A.

### 3. Dry-run and read the structured triage

Never write a bundle blind. Dry-run writes nothing; reason over the report:

```bash
binder convert <corpus> --dry-run --json > triage.json
jq '.result | {num_concepts, num_links, num_unresolved, num_recovered}' triage.json
jq '.result.unresolved' triage.json   # links that will not resolve
jq '.result.warnings'   triage.json   # recovered/unparseable frontmatter, etc.
binder lint <corpus> --json | jq '.result'   # corpus-as-authored: missing type/title, orphans
```

Decide **remediate-source vs accept** *before* writing. Broken links and missing
optional fields are **legal** — fix only what is genuinely wrong. Decision table:
[`references/ingestion-workflow.md`](references/ingestion-workflow.md) §B.

### 4. Remediate at the source — never fabricate

- Missing required frontmatter (`type`/`title`/`generated`): prefer `binder
  enrich` — additive, idempotent, frontmatter-only, byte-faithful. Preview first:

  ```bash
  binder enrich <corpus> --dry-run --json | jq '.result.files'
  binder enrich <corpus> --json           # apply
  ```

- Structural/link issues: edit the source. Never invent a target to satisfy a
  link. Trust fields are *proposed*, never invented — see step 6.

### 5. Convert for real, then validate & review

```bash
binder convert <corpus> -o <bundle> --json | jq '.result | {num_concepts, num_unresolved}'
binder validate <bundle> --json | jq '.result.findings'   # [] ⇒ conformant (exit 0)
binder review   <bundle> --json | jq '.result | {by_type, tiers, orphans, stale, unresolved}'
```

`binder validate` checks the OKF §11 hard rule (every non-reserved `.md` has
parseable frontmatter with a **non-empty `type`**). A conformant bundle reports
`findings: []` and **exits 0**. If `findings` is non-empty (exit 1), read each
finding, fix the source or your flag choices, and iterate 3→5 until validate is
clean and the `review` advisories are understood. Optionally `binder lint
<corpus> --json` for source health and `binder graph <bundle> --json` for the raw
edge export.

### 6. Trust-extraction review — the never-fabricate-trust crux

Load [`references/trust-discipline.md`](references/trust-discipline.md). binder
only stamps an honest `generated: binder/<ver>`; it **never** stamps `verified`
and **never** invents `sources`. Hold the same line:

- No `--verified-by` unless a **real, named actor actually attests** — never
  auto-pass it, never default it to yourself/the agent.
- No invented provenance; enable `--source-keys`/`--map-citations` only for keys
  that carry *real* provenance.
- Never store a credibility score/tier — tiers are **derived** (`binder review`
  computes them on read).

Propose trust to the user; defer all stamping to deterministic binder.

### 7. Hand off / summarize

Report the bundle location, what you **remediated vs accepted**, and any residual
advisories (unresolved links, orphans, recovered files) so the user knows the
bundle's state. Do not add trust you could not honestly assert.

---

*A plain-markdown corpus you can practice on ships at
[`assets/sample-corpus/`](assets/sample-corpus/) — it deliberately contains an
unresolved link, a missing-title file, and a no-frontmatter file to exercise the
triage/enrich loop. Run steps 3–5 against it and, after enrich, `binder validate`
reports a conformant bundle (exit 0).*
