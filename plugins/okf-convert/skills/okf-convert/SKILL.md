---
name: okf-convert
description: "Drive the binder CLI to ingest a plain-markdown corpus into a conformant Open Knowledge Format (OKF) v0.2 bundle, reasoning over binder's structured --json output rather than scraping prose. Use when asked to convert, ingest, or migrate a whole directory of markdown notes/docs into an OKF bundle, or to drive binder end-to-end (convert, validate, review). Teaches the ingestion-analysis judgment the deterministic converter cannot encode: detect binder and its JSON contract, dry-run and read the triage, convert for real, and validate for §11 conformance. Assumes the binder binary is installed — this is the binder-present, binder-driven surface. For authoring or validating a SINGLE bundle by hand with no binaries, use the tool-agnostic okf-author / okf-validate skills instead. Never fabricates trust: proposes trust and defers all stamping to binder; never invents verified/sources it cannot assert."
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
OKF format and trust vocabulary themselves, defer to those skills — do not
re-derive them here. They ship in a **different repository and a different
plugin marketplace**:

> **Layer A — `okf-author` and `okf-validate`** live in the `okf-authoring`
> plugin at
> [`ghchinoy/agent-skills` → `plugins/okf-authoring`](https://github.com/ghchinoy/agent-skills/tree/main/plugins/okf-authoring).

The OKF v0.2 specification itself is
[`GoogleCloudPlatform/knowledge-catalog` → `okf/SPEC.md`](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).

## The one guardrail that overrides everything: never fabricate trust

binder stamps an honest `generated: binder/<version>` provenance mark, and it
never **invents** a `verified` actor or `sources` out of nothing. As of
`binder/0.3.1` it is also **safe by default**: with no `--verified-by` flag and
no attester *you* configured, `convert` and `enrich` write **no** `verified`
stamp at all. A stamp is written only when you decide it, and every stamp is
disclosed (`.result.verified` in `--json`). The remaining trap is a *default you
set* silently applying to a corpus you did not mean it for:

> A `verified` stamp is written only from an **explicit `--verified-by`**, or
> from a default **you set** in your **global** config
> (`~/.config/binder/config.yaml`). A global `verified_by:` counts as you having
> chosen a default and **will** stamp `convert` and `enrich` (which writes into
> your **source tree**) without the flag. **Neither `BINDER_VERIFIED_BY` (env)
> nor a repo-local `./.binder.yaml` authorizes stamping** — binder refuses both
> and discloses the refused value in `.result.verified.note`.

So the risk is no longer a stray repo-local file *or* an env export — both are
now refused. The remaining risk is a machine-wide **global** default applying
where you did not intend. Step 1 checks for such a configured attester before you
run anything.

- Do **not** pass `--verified-by` unless a real, named actor actually attests to
  the content. A verification stamp is a claim; only make claims that are true.
- Do **not** let a *default you set* stamp a corpus it should not. If step 1
  finds a live global-config attester you cannot vouch for here, pass
  `--verified-by ""` explicitly to suppress it.
- Do **not** invent `sources`/provenance the corpus does not contain.
- Do **not** write a credibility score or trust *tier* into any concept — tiers
  are *derived* on read, never stored.

binder will also **decline to co-sign**: when a concept already carries a
`verified` attestation from a *different* identity, a non-explicit (global-config)
default does **not** add a second stamp — it skips and discloses the skip under
`.result.verified.skipped`. Only an explicit `--verified-by` co-signs. Propose
trust to the user; defer all stamping to deterministic binder. The full
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
  never-fabricate-trust discipline, derive-tiers rule, the `--verified-by`
  guardrail (including the configured-actor trap), and a link to Layer A's
  `trust-vocabulary.md`. Load for step 6.

## The binder JSON contract (what you parse)

Eight commands accept `--json`: `convert`, `enrich`, `validate`, `review`,
`lint`, `infer`, `graph`, and `config`. (`index` and `mcp` do **not** — they
reject the flag.) Of those, **six** print the deterministic `binder.report/v1`
envelope — `convert`, `enrich`, `validate`, `review`, `lint`, and `infer`:

```json
{ "binder": "binder/0.3.1", "command": "convert",
  "schema": "binder.report/v1", "result": { } }
```

The `binder` field carries the running version, so treat it as informational and
never match on it.

Reason over `.result` with `jq`; never parse the human prose. Two exceptions:
`graph --json` emits the **raw** `{nodes,edges}` export (not the envelope), and
`config --json` uses schema `binder.config/v1`. Exit codes are a 4-value
contract: **0** ok · **1** findings/gate tripped · **2** usage error · **3** I/O
error. Full shapes: [`references/binder-json-contract.md`](references/binder-json-contract.md).

## Procedure (the 7-step ingestion loop)

### 1. Detect binder, confirm the version, check for a configured attester

```bash
command -v binder || { echo "binder not installed"; exit 1; }
binder --version    # need binder/0.3.1 or newer
```

**Minimum version is `binder/0.3.1`.** Safe-by-default stamping (no stamp
without an explicit flag or a default you set), the repo-local exclusion, and the
`.result.verified` disclosure this skill relies on all landed in `0.3.1`. Earlier
builds stamp `verified` from ambient config *without* the flag and predate parts
of the `--json` contract this skill parses.

If binder is **absent**, install it — do **not** fake a conversion:

```bash
brew install ghchinoy/tap/binder            # or:
go install github.com/ghchinoy/binder@latest
```

Prebuilt binaries: <https://github.com/ghchinoy/binder/releases>. Full
instructions: <https://github.com/ghchinoy/binder#installation>. If you cannot
install a binary at all, this skill does not apply — hand off to the
tool-agnostic **`okf-author`** skill in
[`ghchinoy/agent-skills` → `plugins/okf-authoring`](https://github.com/ghchinoy/agent-skills/tree/main/plugins/okf-authoring).

**Then check whether an attester is already configured** — per the guardrail
above, a *global*-config `verified_by` you never passed will still stamp (an env
`BINDER_VERIFIED_BY` and a repo-local one will not).

**Pin your working directory first, and run every later step from it.** binder
looks for `./.binder.yaml` relative to the *current* directory, so run this
pre-flight from the corpus directory to see the same config binder will. A
global default stamps regardless of cwd; `BINDER_VERIFIED_BY` and a repo-local
`./.binder.yaml` do **not** stamp, but you still want the pre-flight to surface
that they exist so you are not surprised by a repo-local `config_file` shadowing
the global one.

```bash
cd <the directory you will run every binder command from>
binder config --json | jq -c '.result | {config_file, verified_by: .values.verified_by}'
```

Match the reply against this table before running anything (binder itself keys
its stamping decision on the `config_file` discriminator below — a repo-local
path never authorizes a stamp):

| Reply | Meaning | Do |
|---|---|---|
| `{"config_file":"","verified_by":{"value":"","source":"default"}}` | nothing configured | proceed — nothing will stamp |
| `{"config_file":"","verified_by":{"value":"human:x","source":"env"}}` | inherited from `BINDER_VERIFIED_BY` | **does not stamp** — env is refused and disclosed in `.result.verified.note`; pass `--verified-by` only if a real actor attests |
| `{"config_file":".binder.yaml","verified_by":{"value":"human:x","source":"file"}}` | repo-local setting — **does not stamp** (Option A) | binder ignores it for stamping and discloses so; pass `--verified-by` only if a real actor attests |
| `{"config_file":"/home/u/.config/binder/config.yaml","verified_by":{"value":"human:x","source":"file"}}` | machine-wide default — **will stamp** | **stop** — a global default stamps without the flag; pass `--verified-by ""` to suppress it unless a real actor attests |

`source` **cannot** separate the last two rows — both report `file` — yet they
behave **oppositely**: the global one stamps, the repo-local one does not.
`config_file` is the discriminator: binder loads **exactly one** config file, a
repo-local `.binder.yaml` suppresses the global one entirely rather than merging
with it, and Option A honors *only* the global path for a no-flag stamp. (See the
binder user guide, *Configuration*.) `binder config`
itself does not accept `--verified-by`, so a `flag` source never appears here —
that case is one you typed yourself and already own.

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
  absent), `--canonicalize-status` (opt-in: rewrite `--status-map` aliases to the
  OKF §5.4 vocabulary).
- **Catalog/structure:** `--group-by-type` appends an additive `# Catalog` to the
  root; `--include-graph` / `--include-backlinks` annotate catalog entries with
  outbound / inbound edges (both require `--group-by-type`).
- **Scope & gating:** `--external-root` (repeatable) declares a known sibling
  workspace for `file://` resolution; `--strict` gates on unresolved links or
  recovery warnings (exit 1) instead of the default never-reject; `--report
  <file>` also writes the run report to disk.

If you cannot tell what the directories mean, ask binder rather than guessing:
`binder infer <corpus> --json` proposes a `dir=Type` mapping from folder
structure, filename patterns, and frontmatter hints. It is **proposal-only and
never writes** — review the proposal, then feed it to `--type-map`.

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
  enrich` — additive, frontmatter-only, byte-faithful, and atomic. It rewrites
  your **source tree in place**, so commit or stash first. Preview, then apply:

  ```bash
  binder enrich <corpus> --dry-run --json | jq -c '.result.files[] | select(.added|length>0)'
  binder enrich <corpus> --dry-run --json | jq -c '.result.verified'   # trust disclosure
  binder enrich <corpus> --json                      # apply (safe by default)
  ```

  **Check `.result.verified` in the dry-run before applying.** By default — no
  flag, no global default (step 1) — it stays empty and **no** `verified` is
  written. If a global default *you set* is live, the dry-run discloses exactly
  what it will stamp, and `verified` also appears in that file's `added`:

  ```json
  { "actor": "human:you", "source": "config", "num_stamped": 3,
    "skipped": [], "num_skipped": 0 }
  ```

  Add `--verified-by ""` to **suppress** a global default you cannot vouch
  for here; it is not needed when nothing is configured. Nothing unstamps a
  `verified` block after the fact (and there is no MCP `enrich` tool), so decide
  before applying. binder will not co-sign a concept a *different* identity
  already attested unless you pass an explicit `--verified-by` — it reports those
  under `.result.verified.skipped` and leaves the prior attestation untouched.
  Pass a real `--verified-by` only when a real actor genuinely attests.

  Enrich is idempotent **per actor within the same clock second only**. Stamps
  dedupe on `(by, at)`, so re-running under a wall clock appends a new stamp every
  time — three runs two seconds apart leave three identical-actor stamps. Pin
  `SOURCE_DATE_EPOCH` for a genuinely repeatable run. A different actor always
  appends.

- Structural/link issues: edit the source. Never invent a target to satisfy a
  link. Trust fields are *proposed*, never invented — see step 6.

### 5. Convert for real, then validate & review

Still in the directory you pinned in step 1:

```bash
binder convert <corpus> -o <bundle> --verified-by "" --json | jq '.result | {num_concepts, num_unresolved}'
binder validate <bundle> --json | jq '.result.findings'   # [] ⇒ conformant (exit 0)
binder review   <bundle> --json | jq '.result | {by_type, tiers, orphans, stale, unresolved}'
```

**`convert` applies the same stamping decision as `enrich`** — the CLI's
`--verified-by` resolution, not a quirk of either command. A **global**-config
default *you set* that you don't suppress writes `verified` into **every concept
in the bundle**, and `review` then reports them `human-reviewed` rather than
`unverified`. Neither `BINDER_VERIFIED_BY` nor a repo-local `./.binder.yaml`
stamps — both are refused and disclosed. Measured on the sample corpus
(5 concepts) with `binder/0.3.1`:

| step-5 `convert` | attester in scope | stamped | `review` tiers |
|---|---|---|---|
| no flag | global `~/.config/binder/config.yaml` → `verified_by: human:ghost` | 5 of 5 | `{"human-reviewed":5}` |
| `--verified-by ""` | same global default | 0 of 5 | `{"unverified":5}` |
| no flag | repo-local `./.binder.yaml` → `verified_by: human:ghost` | 0 of 5 (ignored) | `{"unverified":5}` |

The ignored repo-local case is disclosed in `.result.verified.note`
(`ignored repo-local .binder.yaml verified_by ...`). An inherited
`BINDER_VERIFIED_BY` behaves the same way — it is refused (0 stamped) and
disclosed with a parallel note (`ignored BINDER_VERIFIED_BY "...": an
environment default does not authorize stamping ...`), so only a **global**
config `verified_by` stamps without the flag. Pass a real `--verified-by` only
when a real actor genuinely attests. Confirm with the `tiers` line in the
`review` output below — it is the cheapest check that no claim was fabricated.

`binder validate` checks the OKF §11 hard rule that every non-reserved `.md` has
parseable frontmatter with a **non-empty `type`** — that is §11.1 and §11.2 only.
It reports `reserved_structure_checked: false`, meaning the §11.3 reserved-file
structure rules are **not** verified. So read `findings: []` as *"no §11.1/§11.2
violations"*, not *"fully §11-conformant"*; check that field rather than assuming:

```bash
binder validate <bundle> --json | jq -c '.result | {findings, reserved_structure_checked}'
```

A clean bundle reports `findings: []` and **exits 0**. If `findings` is non-empty
(exit 1), read each finding, fix the source or your flag choices, and iterate 3→5
until validate is clean and the `review` advisories are understood. Optionally
`binder lint <corpus> --json` for source health, `binder graph <bundle> --json`
for the raw edge export, and `binder infer <corpus> --json` to propose a
`--type-map` (proposal-only, never writes).

### 6. Trust-extraction review — the never-fabricate-trust crux

Load [`references/trust-discipline.md`](references/trust-discipline.md). binder
stamps an honest `generated: binder/<ver>`, never *invents* an actor, and by
default writes no `verified` at all — but a default **you set** in your global
config will stamp one, and it discloses every stamp under `.result.verified`.
Hold the line binder cannot hold for you:

- No `--verified-by` unless a **real, named actor actually attests** — never
  auto-pass it, never default it to yourself/the agent.
- No global default stamping where it should not. Re-check step 1 if your config
  changed since; pass `--verified-by ""` to suppress an attester you cannot vouch
  for here. (Neither `BINDER_VERIFIED_BY` nor a repo-local `./.binder.yaml` ever
  stamps — both are refused and disclosed.)
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
triage/enrich loop. Run steps 3–5 against it. It converts to a bundle that
`binder validate` already reports clean (`findings: []`, exit 0) **whether or not
you run `enrich` first** — `enrich` improves the source, it is not what makes the
bundle valid. Do not read a passing validate as proof that enrich worked.*
