# binder user guide

This is the deep reference for `binder`. The [README](../README.md) is the
concise landing page (what it is, install, quickstart); this guide documents
**every command and flag**, the OKF v0.2 output layout, the full trust
vocabulary, the relationship-extraction rules, malformed-input recovery, CI
usage, and worked end-to-end examples.

`binder` converts a plain-markdown corpus into a conformant
[Open Knowledge Format (OKF) v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle and reports on OKF bundles. It is **Phase 2 complete**.

> This guide grows alongside each Phase 2.x feature. Sections for planned work
> (`enrich`/`--in-place`, `lint`, declarative trust flags, `config`, `file://`
> edges, grouped/backlink indexes) are stubbed under
> [Roadmap & planned features](#roadmap--planned-features) and reference their
> tracking issues.

## Table of Contents

- [Invariants](#invariants)
- [Concepts and terminology](#concepts-and-terminology)
- [Commands](#commands)
  - [`convert`](#convert)
  - [`validate`](#validate)
  - [`index`](#index)
  - [`review`](#review)
  - [`graph`](#graph)
- [JSON output (`--json`) and the exit-code contract](#json-output---json-and-the-exit-code-contract)
- [Discovery surface (`--version` / `--help`)](#discovery-surface---version----help)
- [OKF v0.2 output structure](#okf-v02-output-structure)
- [Relationship extraction](#relationship-extraction)
- [The trust vocabulary](#the-trust-vocabulary)
- [Malformed-input recovery](#malformed-input-recovery)
- [Determinism and reproducible builds](#determinism-and-reproducible-builds)
- [CI usage](#ci-usage)
- [Worked end-to-end example](#worked-end-to-end-example)
- [Roadmap & planned features](#roadmap--planned-features)

## Invariants

Everything binder does is bounded by these guarantees. They hold across every
command and are the properties that make binder safe in a pipeline:

- **Native codec.** binder parses and serializes OKF with a single owned codec
  ([`goldmark`](https://github.com/yuin/goldmark) for markdown +
  [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3) `yaml.Node` for frontmatter).
  Every package above `internal/okf` depends only on binder-owned interfaces
  (`Codec`, `LinkGraph`) — the **dependency rule** — so the codec is swappable
  without touching the converter, CLI, or validators.
- **Byte-faithful round-trip.** Unmodified YAML frontmatter is passed through
  verbatim — including nested-map and list key order and scalar quoting style —
  so a round-trip changes nothing it did not have to change.
- **Deterministic output.** Given identical input and the same clock,
  `convert` produces byte-identical output; `review` and `graph` sort their
  output. `convert` honours `SOURCE_DATE_EPOCH` for any synthesised timestamp.
- **Never reject.** `convert` never aborts on a bad input file, and `validate`
  never rejects a bundle for missing optional fields, unknown keys, unknown type
  values, broken links, or absent trust families. Non-fatal issues are surfaced
  as **advisories**, never errors.
- **Never fabricate trust.** binder never invents a source, a credibility score,
  or provenance. Trust mapping is opt-in and additive; with no mapping flags,
  frontmatter round-trips byte-for-byte. Trust tiers and staleness are *derived*
  on demand from frontmatter, never stored.

## Concepts and terminology

| Term | Meaning |
|---|---|
| **Corpus** | A directory of ordinary `.md` files — the input to `convert`. |
| **Bundle** | A conformant OKF v0.2 directory tree — the output of `convert` and the input to `validate`/`index`/`review`/`graph`. |
| **Concept** | One non-reserved `.md` document. Its ID is its bundle-relative path minus `.md` (e.g. `metrics/revenue`). |
| **Reserved file** | `index.md` and `log.md`. These are structural, not concepts, and are not required to carry a `type`. |
| **Frontmatter** | The YAML block between `---` fences at the top of a concept. It is authoritative: `type` and the trust view are projections of it. |
| **Link / edge** | A directed relationship between concepts. Edges come from body markdown links (spec §6); resolved edges name a concept that exists in the bundle. |
| **Trust signals** | The v0.2 provenance/lifecycle vocabulary (`sources`, `generated`, `verified`, `status`, `stale_after`, …). |

## Commands

The binary exposes five commands. All bundle-reading commands
(`validate`/`index`/`review`/`graph`) load a bundle through the same codec, so
their views of concepts, links, and trust always agree.

```text
binder convert    Convert a markdown corpus into an OKF v0.2 bundle
binder validate   Check a bundle for OKF v0.2 conformance (spec §11)
binder index      (Re)generate the per-directory index.md nav tree (spec §8)
binder review     Summarize a bundle: concepts, links, orphans, trust tiers, stale
binder graph      Export the bundle's concept graph (dot|json|graphml|html)
```

Every command supports `-h`/`--help`. The root binary supports `-v`/`--version`.

### `convert`

```text
binder convert <src> [flags]
```

Walks a plain-markdown corpus and writes a conformant OKF v0.2 bundle. It **never
mutates the source**, is **deterministic**, and **never rejects** an input file.
For each non-reserved `.md` it emits one concept with:

- `type` ensured (precedence: existing frontmatter → `--type-map` per-directory →
  `--default-type`, default `Note`);
- `title` ensured (precedence: existing → first `# H1` → humanized filename);
- standard markdown links and `[[wikilinks]]` rewritten to bundle-relative form;
- `#hashtags` merged into frontmatter `tags`;
- frontmatter-ref edges materialized (when `--fm-ref-keys` is set);
- a `generated` provenance stamp added **only if absent**;
- a root `index.md` declaring `okf_version: "0.2"` and per-directory `index.md`
  navigation.

Output is required unless you pass `--dry-run`.

#### Flags

| Flag | Default | Purpose |
|---|---|---|
| `-o`, `--output` | — | Output bundle directory (required unless `--dry-run`). |
| `--dry-run` | `false` | Report what would be written without writing anything. |
| `--default-type` | `Note` | Concept type applied when none is present or mapped. |
| `--type-map` | — | Per-directory type overrides, e.g. `"docs=Guide,adr=Decision"`. The longest (most specific) matching directory key wins. |
| `--fm-ref-keys` | — | Frontmatter keys treated as relationship edges, e.g. `"related,parent"`. |
| `--map-citations` | `false` | Map a body `# Citations` list into `sources` entries. |
| `--source-keys` | — | Frontmatter keys to map into `sources` entries, e.g. `"source,author"`. |
| `--map-draft` | `false` | Map a `draft: true` marker to `status: draft` (only when `status` is absent). |
| `--report` | — | Also write the run report (the same text printed to stdout) to this file. |
| `--json` | `false` | Emit the run report as deterministic JSON (schema `binder.report/v1`) instead of prose. Composes with `--report` (the file gets whichever format `--json` selects). See [JSON output](#json-output---json-and-the-exit-code-contract). |

`--map-citations`, `--source-keys`, and `--map-draft` are the **trust-mapping**
flags — all off by default. See [The trust vocabulary](#the-trust-vocabulary).

#### Output report

`convert` (and `--dry-run`) prints a deterministic report: concept count, link
tally (resolved/unresolved), any recovered files, the per-concept list, an
unresolved-links list, and any warnings.

```text
binder convert
  source: testdata/corpus-clean
  output: /tmp/bundle
  concepts: 2
  links: 2 (resolved 2, unresolved 0)

Concepts:
  metrics/revenue.md  [type=Metric]
  overview.md  [type=Note]
```

### `validate`

```text
binder validate <bundle>
```

Checks the **hard conformance rules** (spec §11): every non-reserved `.md` has a
parseable frontmatter block with a non-empty `type`. Everything else — trust
well-formedness, actor conventions, date shapes — is reported as an **advisory**
and never rejects the bundle.

`validate` exits non-zero only when there is at least one hard violation
(unparseable frontmatter or a missing/empty `type`), which makes it a clean CI
gate. Reserved files (`index.md`/`log.md`) are counted but not required to carry
a `type`.

```text
bundle: /tmp/bundle
concepts: 2, reserved files: 2
RESULT: conformant (OKF 0.2)
```

A non-conformant bundle prints each finding and ends with
`RESULT: NOT conformant (N violation(s))` and a non-zero exit code.

| Flag | Default | Purpose |
|---|---|---|
| `--json` | `false` | Emit the validation result as deterministic JSON (schema `binder.report/v1`) instead of prose. The exit code is identical in both modes. See [JSON output](#json-output---json-and-the-exit-code-contract). |

### `index`

```text
binder index <bundle> [flags]
```

Regenerates each directory's `index.md` navigation tree (spec §8): the
directory's own concepts under `# Concepts` and its immediate subdirectories
under `# Subdirectories`. The **bundle-root `index.md`** is the only index that
carries frontmatter and the only place `okf_version` is declared (spec §12).
`log.md` files are never touched. Each write is reported (`write` for a new file,
`regenerate` for an existing one) so nothing is overwritten silently.

| Flag | Default | Purpose |
|---|---|---|
| `--dry-run` | `false` | Report which `index.md` files would be written, without writing. |

`convert` already generates these indexes; run `index` to refresh them after
hand-editing concepts in a bundle.

### `review`

```text
binder review <bundle> [flags]
```

Summarizes a loaded bundle: concept counts by type, **derived** trust tiers,
stale concepts, Attested Computations, files recovered from unparseable
frontmatter, orphans (concepts nothing links to), and unresolved links. Trust
tiers and staleness are derived on demand, never stored.

| Flag | Default | Purpose |
|---|---|---|
| `--today` | now | Date (`YYYY-MM-DD`) used for the staleness check; honours `SOURCE_DATE_EPOCH`. |
| `--json` | `false` | Emit the review report as deterministic JSON (schema `binder.report/v1`) instead of prose. See [JSON output](#json-output---json-and-the-exit-code-contract). |

```text
binder review
  bundle: /tmp/bundle
  concepts: 2
  by type:
    Metric: 1
    Note: 1
  trust tiers:
    human-reviewed: 1
    machine-confirmed: 0
    unverified: 1
  stale (as of 2026-08-15): 0
  attested computations: 0
  unparsed frontmatter (recovered as body): 0
  orphans (no inbound links): 0
  unresolved links: 0
```

An **unresolved link** in `review` is a concept reference (a bundle-relative
`.md` target, or a residual `[[wikilink]]`) that names no concept in the bundle.
External URLs, `mailto:`/`tel:`/`ftp:` targets, same-document `#anchors`, and
links to non-concept files (assets, scripts) are **not** concept references and
are never reported. An **orphan** is a concept with no inbound resolved edge — it
is reported for you to wire up or accept, never removed.

### `graph`

```text
binder graph <bundle> [flags]
```

Exports the concept graph. Edges are exactly the bundle's **resolved** links
(spec §6), so the graph matches what `validate` and `review` see. Output is
deterministic (nodes and edges sorted).

| Flag | Default | Purpose |
|---|---|---|
| `--format` | `dot` | Output format: `dot` \| `json` \| `graphml` \| `html`. |
| `--json` | `false` | Alias for `--format json` (the raw `{nodes,edges}` export, **not** the report envelope). Conflicting with an explicit `--format {dot,graphml,html}` is a usage error (exit 2). See [JSON output](#json-output---json-and-the-exit-code-contract). |
| `-o`, `--output` | stdout | Write the graph to a file instead of stdout. |
| `--today` | now | Date (`YYYY-MM-DD`) used for the per-node staleness flag. |

Each **node** carries `id`, `title`, `type`, derived `tier`, and `stale`. Each
**edge** carries `from`, `to`, and the link `text` (the relationship label by
convention). Format notes:

- **`dot`** — Graphviz `digraph` (`rankdir=LR`, box nodes), edge labels from link
  text. Pipe to `dot -Tsvg` to render.
- **`json`** — an indented `{ "nodes": [...], "edges": [...] }` object.
- **`graphml`** — GraphML XML with node keys `title`/`type`/`tier`/`stale` and an
  edge key `rel`.
- **`html`** — a self-contained, dependency-free page: a readable node/edge table
  plus the same JSON embedded as a `<script type="application/json">` island. It
  is a zero-extra-tool fallback, not an interactive viewer.

```text
digraph okf {
  rankdir=LR;
  node [shape=box];
  "metrics/revenue" [label="Revenue"];
  "overview" [label="Revenue Knowledge Base"];
  "metrics/revenue" -> "overview" [label="overview"];
  "overview" -> "metrics/revenue" [label="revenue metric"];
}
```

## JSON output (`--json`) and the exit-code contract

`convert`, `validate`, `review`, and `graph` accept `--json` for scripting,
agents, and CI. Prose is the default and is **byte-unchanged** when `--json` is
absent — `--json` is a presentation layer over the already-computed report, it
changes no behavior and fabricates no fields or trust data.

### The envelope (schema `binder.report/v1`)

`convert`, `validate`, and `review` wrap their existing report struct in a thin
envelope that carries the provenance and schema tag a consumer needs to parse it
safely:

```json
{
  "binder": "binder/0.1.0",
  "command": "convert",
  "schema": "binder.report/v1",
  "result": { }
}
```

| Envelope field | Meaning |
|---|---|
| `binder` | The producing binder version, `binder/<version>` (same string as `--version`). |
| `command` | `convert` \| `validate` \| `review`. |
| `schema` | The report contract identifier. Bumped **only** on a breaking change to a payload's shape or field names, so a consumer can branch on it. |
| `result` | The command's report object (see per-command fields below). |

`graph` is the deliberate exception — see [graph JSON](#graph-json-a-raw-export-not-the-envelope).

### Determinism

JSON output is deterministic: fixed 2-space indentation, HTML escaping **off**
(so `<`, `>`, `&` are literal), object keys sorted, and a trailing newline. All
timestamps derive from `SOURCE_DATE_EPOCH` (via `--today` for the read
commands), so two runs on the same input are byte-identical:

```bash
SOURCE_DATE_EPOCH=1700000000 binder review bundle --json > a.json
SOURCE_DATE_EPOCH=1700000000 binder review bundle --json > b.json
diff a.json b.json   # no output — identical
```

Empty lists always serialize as `[]` (never `null`) and counts/booleans are
always present, so a parser sees a stable shape regardless of the bundle.

### `convert --json` — `result` fields

| Field | Type | Meaning |
|---|---|---|
| `src` | string | Source corpus path. |
| `out` | string | Output bundle path (empty under `--dry-run`). |
| `concepts` | array | One object per converted concept (see below). |
| `warnings` | array of string | Non-fatal notices (e.g. reserved-file renames). |
| `unresolved` | array | Unresolved links, each `{ from, raw_target, text }`. |
| `num_concepts` | int | Concept count. |
| `num_links` | int | Total links seen. |
| `num_resolved` | int | Links resolved to a bundle concept. |
| `num_unresolved` | int | Links left unresolved (reported, not dropped). |
| `num_recovered` | int | Files whose unparseable frontmatter was preserved as body (§4.6). |
| `dry_run` | bool | Whether this was a `--dry-run`. |

Each `concepts[]` object: `rel_path`, `type`, `title`, `num_links`,
`num_unresolved`. Each `unresolved[]` object: `from` (source concept rel path),
`raw_target` (target exactly as written), `text` (link text / relationship label).

### `validate --json` — `result` fields

| Field | Type | Meaning |
|---|---|---|
| `root` | string | Bundle path. |
| `num_concepts` | int | Non-reserved concepts checked. |
| `num_reserved` | int | Reserved files (`index.md`/`log.md`) counted, not required to carry a `type`. |
| `findings` | array | Each `{ concept_id, severity, message }`. |

`severity` is `error` (a hard §11 violation — gates the exit code) or `advisory`
(trust/lifecycle well-formedness — reported, never gates). See the
[exit-code contract](#exit-code-contract).

### `review --json` — `result` fields

| Field | Type | Meaning |
|---|---|---|
| `root` | string | Bundle path. |
| `today` | string | The `YYYY-MM-DD` staleness date used. |
| `num_concepts` | int | Concept count. |
| `by_type` | object | `{ "<type>": count }` (types with no value show as `(none)`). |
| `tiers` | object | `{ "<tier>": count }` over `unverified` / `machine-confirmed` / `human-reviewed`. |
| `orphans` | array of string | Concept IDs with no inbound resolved edge. |
| `stale` | array of string | Concept IDs stale as of `today`. |
| `attested` | array of string | Attested-Computation concept IDs. |
| `unresolved` | array | Broken concept references, each `{ from, raw_target, text }`. |
| `unparsed_frontmatter` | array of string | Concept IDs recovered from unparseable frontmatter. |
| `concepts` | array | Per-concept view: `{ id, type, tier, stale, attested, orphan }`. |

`by_type` and `tiers` are JSON objects with sorted keys; all list fields are
`[]` when empty.

### graph JSON — a raw export, not the envelope

`graph` is already machine-readable, so `graph --json` is an **alias for
`--format json`**: it emits the raw graph export, **not** wrapped in the
`binder.report/v1` envelope. This asymmetry is deliberate — the graph payload is
a data export consumed directly (e.g. by a viewer), not a run report.

```json
{
  "nodes": [ { "id": "overview", "title": "Overview", "type": "Note", "tier": "unverified", "stale": false } ],
  "edges": [ { "from": "overview", "to": "metrics/revenue", "text": "revenue metric" } ]
}
```

`--json` composes with an explicit `--format json` (redundant but fine).
Combining `--json` with a conflicting `--format {dot,graphml,html}` is a **usage
error** (exit 2) — binder will not silently override your chosen format.

### Exit-code contract

Every command maps its outcome onto four stable codes. The code is about the
**run, not the output format** — it is identical in prose and `--json` mode.

| Code | Name | Meaning | Emitted by (today) |
|---|---|---|---|
| `0` | success | Completed with no gating findings. Advisories may be present — they are reported but never gate. | all commands, normal case |
| `1` | findings-present | A gating condition: `validate` spec §11 non-conformance (unparseable frontmatter or a missing/empty `type`). **Reserved:** advisories under a future `--strict` flag. | `validate` |
| `2` | usage-error | Bad flags/args — unknown flag, missing/extra argument, or a conflicting `--json`/`--format`. | any command |
| `3` | io-error | Cannot read the corpus/bundle, a write failure, or an internal error. | any command |

**Never-reject is preserved.** Broken links, orphans, staleness, recovered
frontmatter, and missing optional trust are all **advisories** — they are
reported (in prose and JSON) and exit `0`. The only default non-zero for a
well-formed run is `validate`'s spec §11 hard non-conformance, which is a genuine
violation, not an advisory. A `--strict` flag that flips advisories into gating
findings (exit `1`) is reserved by this contract and delivered separately
([#7](https://github.com/ghchinoy/binder/issues/7)); the `strict` row above is
stable from day one so consumers can rely on it.

> **Compatibility note.** This refines the previous behavior (where non-IO
> failures collapsed to exit `1`): `0` still means success, and failures are now
> more specific (`1`/`2`/`3`). No consumer that only checked "zero vs non-zero"
> is affected.

## Discovery surface (`--version` / `--help`)

An agent or script can enumerate binder's surface without parsing prose reports:

- **`binder --version`** prints `binder/<version>` (e.g. `binder/0.1.0`) — the
  exact string that appears in the JSON envelope's `binder` field. (`--version`
  is a root flag; passing it to a subcommand is a usage error.)
- **`binder --help`** lists every command; **`binder <cmd> --help`** prints that
  command's `Usage:` line and a `Flags:` section (name, shorthand, default,
  description) — a stable, documented shape sufficient to discover every flag,
  including `--json`.

A structured tool/flag catalog (a machine-readable command manifest) is not part
of this surface; it is delivered natively by the planned MCP server mode
(`binder mcp`).

## OKF v0.2 output structure

A converted bundle is an ordinary directory tree:

```text
bundle/
  index.md              # root index: declares okf_version, lists concepts + subdirs
  overview.md           # a concept
  metrics/
    index.md            # per-directory nav (no frontmatter)
    revenue.md          # a concept
```

Key rules binder enforces on emit:

- **One concept per non-reserved `.md`.** A concept's ID is its bundle-relative
  path minus `.md`.
- **`okf_version` lives only in the root `index.md`** (spec §12), and the root
  index is the only index carrying frontmatter (spec §8).

  ```yaml
  ---
  okf_version: "0.2"
  ---
  ```

- **Every concept has a non-empty `type`** — the one hard-required field
  (spec §11). All other frontmatter keys are optional and preserved as-is,
  including keys binder does not understand (spec §4).
- **A `generated` stamp** records the conversion, added only when the concept
  does not already carry one:

  ```yaml
  generated:
    at: "2023-11-14T22:13:20Z"
    by: binder/0.1.0
  ```

- **Reserved-name source files are never dropped.** A source `index.md`/`log.md`
  is renamed to `<stem>-note.md` (with a numeric suffix on collision) so binder
  can generate its own `index.md`, and the rename is reported.
- **Links are rewritten to bundle-relative-absolute form.** A body link
  `[t](../a/b.md#sec)` that resolves to a concept becomes `[t](/a/b.md#sec)`.

## Relationship extraction

binder extracts every relationship signal it can and rewrites resolved ones into
persisted body links, so the graph survives reload. Unresolved references are
**left in place and reported** — never dropped (spec §6/§11). Link-like text
inside fenced/indented code blocks or inline code spans is ignored throughout
(the same markdown-aware code detection the codec uses).

| Kind | Always on? | Behavior |
|---|---|---|
| **Standard markdown links** | yes | `[text](target.md#anchor)` pointing at a corpus concept is rewritten to `/bundle-relative.md#anchor`. External URLs, `#anchors`, and non-`.md` targets are left untouched. |
| **Wikilinks** | yes | `[[Target]]` and `[[Target\|alias]]` are resolved and rewritten to a standard markdown link `[display](/target.md)`. Unresolved wikilinks stay as `[[...]]` and are reported. |
| **Hashtags** | yes | `#hashtag` tokens in the body are merged (de-duplicated, order-preserving) into frontmatter `tags` (spec §4). A trailing hashtag in an H1 is stripped from the derived title but still tagged. |
| **Frontmatter refs** | opt-in (`--fm-ref-keys`) | Named frontmatter keys (e.g. `related`, `parent`) become edges. The original key/value is preserved, and each resolved target is materialized as a link in a trailing `## Related` section so the edge survives reload. |

### Wikilink and ref resolution

A wikilink or frontmatter-ref target is resolved against the corpus in this
precedence order (deterministic; ambiguous filename/title matches resolve to
nothing and are reported):

1. **Relative or bundle-absolute path** (interpreted against the linking
   concept's directory, then as a bare bundle path).
2. **Filename stem** (case-insensitive; the last path segment).
3. **Title slug** (the target lowercased with non-alphanumeric runs collapsed to
   single hyphens, matched against concept titles).

A frontmatter ref may be a single value, a YAML list, or a `[[Target]]`-style
scalar. Titles for the whole corpus are resolved before any body is rewritten, so
title-slug resolution sees every concept.

### Materialized `## Related` section

When `--fm-ref-keys` is set, each **resolved** ref target becomes a bullet in a
stable, de-duplicated `## Related` section appended to the concept body:

```markdown
## Related

- [architecture](/design/architecture.md)
```

This matters because the read side (`review`/`graph`) rebuilds edges only from
persisted body links — without materialization, a frontmatter-only edge would
vanish on reload and its target would be wrongly reported as an orphan. If no ref
resolves, the body is left byte-identical.

## The trust vocabulary

binder maps corpus-native provenance into the OKF v0.2 trust vocabulary,
**preserves** existing trust frontmatter byte-for-byte, and **derives** trust
tiers and staleness on demand. It never stores a credibility score and never
fabricates provenance (spec §5.1).

### Vocabulary

| Field | Shape | Notes |
|---|---|---|
| `generated` | `{ by, at }` | Provenance of the producing run. `by` follows the actor convention; `at` is an ISO 8601 datetime. binder stamps `{ by: "binder/<ver>", at }` only when absent. |
| `verified` | `{ by, at }` or a list of them | Verification events. A bare mapping is treated as a one-element list. Drives the trust tier. |
| `sources` | list of `{ id, resource, title, author, usage_count, last_modified }` | `resource` is required within an entry; `author` follows the actor convention; `last_modified` is a `YYYY-MM-DD` date. |
| `status` | `draft` \| `stable` \| `deprecated` | Absent ⇒ `stable` (spec §5.4). |
| `stale_after` | `YYYY-MM-DD` | Absolute date; drives staleness. |
| `usage_window` | `{ from, to }` | A date range; both bounds are `YYYY-MM-DD`. |
| Attested-Computation family | `runtime`, `parameters`, `computation`, `executor`, `attester` | Preserved and shape-checked. A concept of type `Attested Computation` requires `runtime` (advisory). binder does **not** execute attestations (no runtime). |

The **actor convention** (spec §7): `"<producer>/<version>"` for tools/agents
(e.g. `binder/0.1.0`, `reference_agent/gemini`), or one of the `human:`,
`process:`, `team:` prefixes for people, processes, and teams (e.g.
`human:alice`).

### Derived trust tiers

Tiers are computed from `verified`, never stored:

| Tier | Condition |
|---|---|
| `human-reviewed` | at least one `verified[].by` uses the `human:` prefix |
| `machine-confirmed` | one or more `verified` events, none by a `human:` actor |
| `unverified` | no `verified` events |

### Derived staleness

A concept is **stale** when `today >= stale_after` (using `--today`, else now;
`SOURCE_DATE_EPOCH` honoured). A concept without `stale_after` is never stale.

### Opt-in trust mapping (`convert`)

All mapping is off by default, deterministic, and additive (original keys are
preserved):

- **`--source-keys "source,url,author"`** — each named frontmatter key becomes a
  `sources` entry. The `author` key maps to a source `author`; every other key
  maps to a source `resource`.
- **`--map-citations`** — list items under a body `# Citations` heading (any
  level) become `sources` entries: a markdown link yields `{ title, resource }`,
  a bare URL yields `{ resource }`, other text yields `{ title }`.
- **`--map-draft`** — a `draft: true` marker sets `status: draft`, but only when
  `status` is absent (it never clobbers an existing status).

Mapped sources are de-duplicated against existing `sources` by
`(resource, title, author)`.

### Trust well-formedness (advisories)

`validate` reports — as advisories, never errors — any trust value that is
present but malformed: a missing required `resource`/`by`, an actor that does not
follow the convention, a non-ISO date, a `status` outside the enum, or an
Attested Computation missing `runtime`. A **missing** family is silent: absence
is not a violation.

## Malformed-input recovery

`convert` never rejects. A source file whose frontmatter will not parse (invalid
YAML in a closed fence, or an unterminated fence) is preserved losslessly: its
original text — fence and all — becomes the concept body, a default `type` is
stamped so the bundle stays conformant, and the file is reported.

Such a file carries a binder-namespaced marker in its emitted frontmatter:

```yaml
x_binder:
  recovered: true
  reason: unparseable-frontmatter
```

This marker is binder-owned (no OKF-vocabulary collision), round-trips as an
unknown key, and is the single authoritative signal both
`binder convert --report` and `binder review` read to report recovery — so the
two surfaces can never disagree, and no fragile body-shape heuristic is needed.
`review` lists these under `unparsed frontmatter (recovered as body)`.

Files with **no** frontmatter at all are the ordinary plain-markdown case: they
become concepts with the whole file as body and a defaulted `type`; they are not
"recovered" and carry no marker.

## Determinism and reproducible builds

Given identical input and the same clock, `convert` produces byte-identical
output, and re-converting an already-converted bundle is idempotent. To pin
synthesised timestamps (the `generated.at` stamp), set `SOURCE_DATE_EPOCH` to a
Unix timestamp:

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert corpus -o bundle
```

The same variable seeds the default `--today` used by `review` and `graph`
staleness, so a fully pinned pipeline is reproducible end to end.

## CI usage

`validate` is the CI gate: it exits non-zero only on a hard conformance
violation (see the [exit-code contract](#exit-code-contract)). A typical pipeline
converts, then validates:

```bash
set -euo pipefail

SOURCE_DATE_EPOCH=1700000000 binder convert docs/ -o build/bundle
binder validate build/bundle          # exit 1 on §11 non-conformance fails the job
binder review   build/bundle          # advisory summary (orphans, stale, unresolved)
binder graph    build/bundle --format json -o build/graph.json
```

For machine consumption, add `--json` and branch on both the exit code and the
parsed payload. The exit code is identical in prose and JSON mode, so a CI step
can gate on the code while archiving the JSON as an artifact:

```bash
set -euo pipefail

# Gate on the exit code (0 conformant, 1 non-conformant, 2 usage, 3 io);
# archive the deterministic JSON report either way.
if binder validate build/bundle --json > build/validate.json; then
  echo "conformant"
else
  code=$?
  echo "validate exited $code"        # 1 = non-conformant, 2/3 = usage/io
  jq '.result.findings[] | select(.severity=="error")' build/validate.json
  exit "$code"
fi

# review/graph are advisory: capture signal without failing the build.
binder review build/bundle --json | jq '{orphans: .result.orphans, stale: .result.stale}'
```

`review` and `graph` never fail the build (they always exit `0`); use them for
reporting and artifacts. To fail a build on unresolved links or orphans today,
parse the `review --json` output (a first-class `binder lint`, and a `--strict`
flag that gates advisories, are planned — see
[issue #8](https://github.com/ghchinoy/binder/issues/8) and
[issue #7](https://github.com/ghchinoy/binder/issues/7)).

The project's own exit gate additionally cross-checks binder's verdicts against
the external [`okfcli/okf`](https://github.com/okfcli/okf) validator in both
directions (`make gate`); see the README's Development section.

## Worked end-to-end example

Start with a tiny plain-markdown corpus:

```text
corpus/
  overview.md
  metrics/
    revenue.md
```

`overview.md` links to the metric and carries no frontmatter; `revenue.md` is
already OKF-shaped with trust signals:

```markdown
<!-- overview.md -->
# Revenue Knowledge Base

See the [revenue metric](metrics/revenue.md).
```

```markdown
---
type: Metric
title: Revenue
generated:
  by: reference_agent/gemini
  at: "2026-01-01T00:00:00Z"
verified:
  by: human:alice
  at: "2026-02-01T00:00:00Z"
status: stable
stale_after: "2027-01-01"
sources:
  - id: ledger
    title: General Ledger
    resource: https://example.com/ledger
    author: team:finance
---
# Revenue

Total recognised revenue for a reporting period. See the [overview](/overview.md).
```

**1. Convert** (pinned clock for reproducibility):

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert corpus -o bundle
```

```text
binder convert
  source: corpus
  output: bundle
  concepts: 2
  links: 2 (resolved 2, unresolved 0)

Concepts:
  metrics/revenue.md  [type=Metric]
  overview.md  [type=Note]
```

`overview.md` gets `type: Note`, its title derived from the H1, and a binder
`generated` stamp; `revenue.md`'s trust frontmatter round-trips untouched. A root
`index.md` (declaring `okf_version: "0.2"`) and `metrics/index.md` are generated.

**2. Validate:**

```bash
binder validate bundle
```

```text
bundle: bundle
concepts: 2, reserved files: 2
RESULT: conformant (OKF 0.2)
```

**3. Review** (revenue is human-reviewed via `verified.by: human:alice`;
overview is unverified):

```text
binder review
  bundle: bundle
  concepts: 2
  by type:
    Metric: 1
    Note: 1
  trust tiers:
    human-reviewed: 1
    machine-confirmed: 0
    unverified: 1
  stale (as of 2026-08-15): 0
  attested computations: 0
  unparsed frontmatter (recovered as body): 0
  orphans (no inbound links): 0
  unresolved links: 0
```

**4. Graph** (render with Graphviz):

```bash
binder graph bundle --format dot | dot -Tsvg -o graph.svg
```

## Roadmap & planned features

The following are **planned, not yet shipped**. This guide will grow a full
section for each as it lands; today each links to its tracking issue.

### Phase 2.x — `convert`/CLI enhancements

- **In-place enrichment** — a `binder enrich` / `binder convert --in-place` mode
  that injects missing required frontmatter into existing files without an
  out-of-place output directory.
  [#5](https://github.com/ghchinoy/binder/issues/5)
- **`file://` edge resolution** — resolve workspace-relative `file://` URIs that
  point inside the corpus as internal concept edges (today they are treated as
  external and skipped). [#6](https://github.com/ghchinoy/binder/issues/6)
- **Declarative trust & lifecycle flags** — `--stale-after-map`, `--verified-by`,
  `--status-map` for stamping provenance and freshness across directories.
  [#7](https://github.com/ghchinoy/binder/issues/7)
- **`binder lint`** — a standalone corpus linter / link-health checker with a
  non-zero exit on broken links for CI gates.
  [#8](https://github.com/ghchinoy/binder/issues/8)
- **Richer root `index.md`** — `--group-by-type` and `--include-backlinks` /
  `--include-graph` catalog synthesis.
  [#9](https://github.com/ghchinoy/binder/issues/9)
- **`binder config`** — a viper-backed config for actor identity and defaults, so
  common flags need not be passed every run.
  [#10](https://github.com/ghchinoy/binder/issues/10)

### Phases 3–6 — codec adapter and the reachability layer

- **Phase 3 — community-core codec adapter** (`--okf-impl=community`): a second
  `Codec` behind the existing interface, slotted in only after it is confirmed
  byte-complete against the golden bundles.
- **Phase 4 — MCP server mode** (`binder mcp`): binder's *additive*
  convert/author/emit tools over stdio (no read/search re-implementation).
- **Phase 5 — Agent Skill**: an `agentskills.io`-conformant skill driving the
  convert → validate → review workflow.
- **Phase 6 — Agent-Plugin bundle**: an isolated, optional `plugin/` (plugin
  manifest + skill + MCP manifest); deleting it leaves binder fully functional.

This guide is seeded per [issue #11](https://github.com/ghchinoy/binder/issues/11)
and grows alongside each feature above.
