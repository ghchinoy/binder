# binder

A Go command-line tool that converts a plain-markdown corpus into a conformant
[Open Knowledge Format (OKF) v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle and validates OKF bundles against the spec's conformance rules.

![license](https://img.shields.io/badge/license-Apache--2.0-blue)
![go](https://img.shields.io/badge/go-1.26-00ADD8)

**Status: Phase 2 complete** — a Go CLI that converts a non-OKF markdown corpus
into a conformant [OKF v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle, extracts every relationship signal (wikilinks, anchor links, frontmatter
refs, hashtags), maps corpus-native provenance into the trust vocabulary,
generates per-directory index navigation, validates bundles against the spec's
§11 conformance rules, and preserves trust frontmatter byte-for-byte on
round-trip. It also reports and visualizes a bundle (`review`, `graph`), lints a
source corpus before conversion (`lint`), declaratively stamps trust/lifecycle
metadata (`--status-map`, `--stale-after-map`, `--verified-by`), is configurable
via `binder config`, and supports `--strict` CI gating.

## Table of Contents

- [What it does](#what-it-does)
- [Installation](#installation)
- [Usage](#usage)
- [Agent Skill / Plugin](#agent-skill--plugin)
- [MCP server (`binder mcp`)](#mcp-server-binder-mcp)
- [How it works](#how-it-works)
- [Development](#development)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

## What it does

`binder` has these commands:

- **`convert`** walks a directory of ordinary markdown files and writes an OKF
  v0.2 bundle: one concept per non-reserved `.md`, standard markdown links and
  `[[wikilinks]]` rewritten to bundle-relative form, frontmatter-ref edges,
  `#hashtags` merged into `tags`, per-directory `index.md` navigation, and a
  generated provenance stamp. It never mutates the source.
- **`enrich`** adds the missing required frontmatter (`type`, `title`,
  `generated`) to a source markdown tree **in place** — frontmatter only, no body
  changes. It is additive/never-clobber, idempotent, atomic, and safe on a
  git-tracked repo (see [Enriching a source corpus](#enriching-a-source-corpus-in-place-binder-enrich)).
- **`validate`** checks a bundle against the OKF v0.2 §11 conformance rules and
  reports trust/lifecycle well-formedness as advisories.
- **`index`** (re)generates per-directory `index.md` navigation for a bundle, and
  can add a type-grouped `# Catalog` to the root index (`--group-by-type`).
- **`review`** summarizes a bundle: concepts by type, trust tiers,
  stale/attested, orphans, and unresolved links.
- **`lint`** reports source-corpus health *before* conversion (writes nothing):
  broken links (incl. `#anchors`), missing titles, orphans, stale concepts, and
  schema violations. `--strict` gives a non-zero CI gate.
- **`graph`** exports the concept graph (edges = resolved links) as
  dot/json/graphml/html.
- **`config`** shows the resolved effective configuration (viper-backed) and
  where each value came from (flag/env/file/default).
- **`mcp`** runs binder as a stdio MCP server, exposing the additive verbs
  (`convert`/`validate`/`review`/`lint`/`graph`) plus the read-only `list_graphs`
  graph-introspection tool as MCP tools that return the same `binder.report/v1`
  payloads as `--json` (see [MCP server](#mcp-server-binder-mcp)).

`convert` can also declaratively stamp trust and lifecycle metadata across
directory sections — `--status-map`, `--stale-after-map`, and `--verified-by`
(#7) — and every command supports `--strict` to gate advisories in CI (see
[Declarative trust & lifecycle flags](#declarative-trust--lifecycle-flags-convert)
and [Strict mode](#strict-mode)).

Two properties make it trustworthy for pipelines:

- **Deterministic output.** `convert` honours `SOURCE_DATE_EPOCH` for any
  synthesised timestamps, so identical input yields byte-identical output.
- **Byte-faithful frontmatter round-trip.** Unmodified YAML frontmatter is passed
  through verbatim — including nested-map and list key order — so a round-trip
  changes nothing it did not have to change.

Verdicts are differential-validated against the vendor-neutral
[`okfcli/okf`](https://github.com/okfcli/okf) validator (v0.3.0) in both
directions as part of the exit gate.

## Installation

Install the latest release with the Go toolchain (requires Go 1.26+):

```bash
go install github.com/ghchinoy/binder@latest
```

Or clone and build from source:

```bash
git clone https://github.com/ghchinoy/binder.git
cd binder
make build        # -> bin/binder
```

## Usage

> **New to binder?** The **[tutorial](docs/tutorial.md)** is a hands-on,
> runnable walkthrough: ingest an existing corpus, gate it in CI, then author and
> stamp a fresh one. For the full reference (every command and flag, the OKF v0.2
> output layout, the complete trust vocabulary, the relationship-extraction rules,
> malformed-input recovery, CI usage, and worked end-to-end examples) see the
> **[user guide](docs/user_guide.md)**. This section is the quickstart.

Convert a plain markdown corpus into an OKF v0.2 bundle:

```bash
binder convert path/to/corpus -o path/to/bundle
```

```text
binder convert
  source: path/to/corpus
  output: path/to/bundle
  concepts: 2
  links: 2 (resolved 2, unresolved 0)

Concepts:
  metrics/revenue.md  [type=Metric]
  overview.md  [type=Note]
```

Preview a conversion without writing anything:

```bash
binder convert path/to/corpus --dry-run
```

Validate a bundle against OKF v0.2 §11 conformance rules:

```bash
binder validate path/to/bundle

# (Re)generate per-directory index.md navigation for a bundle.
binder index path/to/bundle

# Add a type-grouped catalog (with backlinks + outbound edges) to the root index.
binder index path/to/bundle --group-by-type --include-backlinks --include-graph

# Summarize a bundle: concepts by type, trust tiers, stale/attested,
# orphans, and broken internal links.
binder review path/to/bundle

# Lint a SOURCE corpus (before conversion, writes nothing): broken links
# (incl. #anchors), missing titles, orphans, stale, schema violations.
binder lint path/to/corpus

# Export the concept graph (edges = resolved links).
binder graph path/to/bundle --format dot   # dot|json|graphml|html
```

```text
bundle: path/to/bundle
concepts: 2, reserved files: 1
RESULT: conformant (OKF 0.2)
```

### Machine-readable output (`--json`) and exit codes

`convert`, `enrich`, `validate`, `review`, `lint`, and `graph` accept `--json` for
scripting and CI. Prose is the default and is byte-unchanged when `--json` is absent.

`convert`, `enrich`, `validate`, `review`, and `lint` wrap their existing report in a thin,
deterministic envelope (schema `binder.report/v1`) — same field names every run,
2-space indent, sorted keys, a trailing newline, and any `SOURCE_DATE_EPOCH`
honoured, so two runs on the same input are byte-identical:

```bash
binder validate path/to/bundle --json | jq '.result.findings'
```

```json
{
  "binder": "binder/0.1.0",
  "command": "validate",
  "schema": "binder.report/v1",
  "result": { "root": "path/to/bundle", "num_concepts": 2, "num_reserved": 1, "findings": [] }
}
```

`graph` is already machine-readable, so `graph --json` is an **alias for
`--format json`** — the raw `{nodes, edges}` export, **not** the envelope above.
Combining `--json` with a conflicting `--format {dot,graphml,html}` is a usage
error (exit 2).

Every command maps its outcome onto a stable **exit-code contract** (identical in
prose and `--json` mode):

| Code | Meaning |
|---|---|
| `0` | Success. Advisories (broken links, orphans, staleness, recovered frontmatter, missing trust) may be present — they are reported but never gate unless `--strict` is set. |
| `1` | Gating findings: `validate` spec §11 non-conformance (always), or, under `--strict`, the per-command advisory/finding set (see [Strict mode](#strict-mode)). |
| `2` | Usage error — unknown flag, missing/extra argument, or conflicting `--json`/`--format`. |
| `3` | I/O or internal error — unreadable corpus/bundle, write failure. |

Never-reject is preserved: a well-formed bundle with broken links or orphans
still exits `0`. See the [user guide](docs/user_guide.md) for the per-command
field lists, the discovery surface (`--version`/`--help`), and a CI example.

### convert flags

| Flag | Default | Purpose |
|---|---|---|
| `-o`, `--output` | — | Output bundle directory (required unless `--dry-run`). |
| `--dry-run` | `false` | Report what would be written without writing anything. |
| `--default-type` | `Note` (or config `default_type`) | Concept type applied when none is present or mapped. |
| `--type-map` | — | Per-directory type overrides, e.g. `"docs=Guide,adr=Decision"`. |
| `--status-map` | — | Per-directory `status`, e.g. `"archive=deprecated,drafts=draft,default=active"`; a `default=` key applies when no prefix matches. Set only when `status` is absent. |
| `--stale-after-map` | — | Per-directory `stale_after` relative to the run clock, e.g. `"07-benchmarks=+6m,legacy=+0d"` (grammar `+Nd`/`+Nm`/`+Ny`). Set only when absent. Malformed → exit 2. |
| `--verified-by` | config `verified_by` | Actor to append as a `verified` stamp, e.g. `"human:ghchinoy"` or `"binder/0.1.0"`. Invalid actor → exit 2. |
| `--strict` | `false` | Gate (exit 1) on unresolved links or recovery warnings (see [Strict mode](#strict-mode)). |
| `--workspace-root` | `<src>` root | Boundary within which `file://` links resolve to internal edges (see below). |
| `--report` | — | Also write the run report to this file. |
| `--json` | `false` | Emit the run report as deterministic JSON (schema `binder.report/v1`) instead of prose. Composes with `--report`. |
| `--group-by-type` | `false` | Append an additive `# Catalog` of all concepts, grouped by type, to the **root** `index.md` (see [Catalog](#catalog-in-the-root-indexmd)). |
| `--include-backlinks` | `false` | Annotate each catalog entry with its inbound resolved edges (requires `--group-by-type`). |
| `--include-graph` | `false` | Annotate each catalog entry with its outbound resolved edges (requires `--group-by-type`). |

The same three catalog flags are also accepted by `binder index`, which
regenerates indexes for an already-converted bundle.

### Catalog in the root index.md

`--group-by-type` **adds** a `# Catalog` section to the bundle-**root** `index.md`
only. It is purely additive: the existing per-directory nav (`# Concepts` /
`# Subdirectories`, spec §8) is left untouched, and non-root indexes are never
modified — so with the flag off, output is byte-identical to before.

The catalog lists **every** concept in the bundle, grouped under `## <type>`
subheaders:

- **Types are used verbatim** (no pluralization/humanization) and sorted
  alphabetically; concepts with an empty/unknown type go under a final
  `## (untyped)` group.
- Within a group, concepts are sorted by bundle-relative path, each linked by its
  bundle-relative-absolute path (`* [Title](/path/to/concept.md)`).
- Output is deterministic and idempotent — re-running `convert`/`index` on
  identical input yields a byte-identical `index.md`, and `binder validate` still
  passes (the root index remains the sole `okf_version` carrier).

`--include-backlinks` and `--include-graph` add, under each entry, a bounded,
sorted sub-list of inbound / outbound edges. The list is capped at **20 edges
per entry**; when an entry has more, exactly 20 are rendered followed by a single
`… and N more` line (the full edge set is always available via `binder graph`).
These annotations derive from the **same resolved-edge set** `binder graph` uses
(resolved links only), so the catalog and the graph can never disagree:

```markdown
# Catalog

## Pattern

* [Alpha](/patterns/alpha.md)
  * backlink: [Beta](/patterns/beta.md)
  * link: [Setup](/guides/setup.md)
```

### Enriching a source corpus in place (`binder enrich`)

`binder enrich <src>` adds the missing required OKF frontmatter (`type`, `title`,
`generated`) to the markdown files under `<src>`, **in place**. It is for authors
adopting OKF in an existing (usually git-tracked) repo who want the required
frontmatter added to their files without a convert-to-temp-and-copy-back dance.

```bash
binder enrich path/to/corpus            # write the injected frontmatter
binder enrich path/to/corpus --dry-run  # preview; writes nothing
```

```text
enrich path/to/corpus
3 file(s): 2 enriched, 1 unchanged, 0 skipped
  enriched getting-started.md (added: generated, title, type)
  enriched notes/idea.md (added: generated)
```

**enrich is not `convert --in-place`.** It touches **frontmatter only** — no link
rewriting, no `index.md` generation, no `## Related` section, no `#hashtag` merge.
Bodies are otherwise untouched. `binder convert` is unchanged (still strictly
out-of-place, still never touches `<src>`). Use `convert` to compile a bundle;
use `enrich` to bring a source tree's frontmatter up to spec.

The safety model is load-bearing, because enrich mutates the source:

- **Additive / never-clobber.** Only keys that are **absent** are added; an
  authored `type`/`title`/`generated` (or any other key) is never overwritten.
- **Idempotent.** A second run finds every key present → `unchanged` → **no
  write**. `generated.at` is stable across runs (set-when-absent).
- **Body + pre-existing keys byte-faithful.** enrich reuses the codec's
  byte-faithful serializer: on a file it changes, existing frontmatter subtrees
  are re-emitted verbatim (nested/list order and quoting preserved) and only the
  added keys are encoded fresh; the body is preserved.
- **Skip-unchanged (no git churn).** A file that needs no key is **not written at
  all** — no spurious diffs, no mtime bumps.
- **Skip-unparseable.** A file whose frontmatter will not parse is **skipped and
  reported**, never rewritten (unlike `convert`'s out-of-place recover-as-body,
  which would be destructive in place). Left byte-identical on disk.
- **Reserved files** (`index.md`/`log.md`) are skipped.
- **Atomic write.** enrich writes a temp file in the target's directory, `fsync`s
  it, then renames it over the original (same-filesystem atomic replace),
  preserving the file mode. An interrupt never leaves a partial/corrupt file.
- **Deterministic.** `generated.at` honours `SOURCE_DATE_EPOCH`.

> **Known normalization (only on files enrich changes):** the serializer
> normalizes body `\r\n`→`\n` and the frontmatter/body separator to a single
> blank line. A file that needs no key is never rewritten, so CRLF/spacing-only
> files are left exactly as-is. Because enrich mutates the source, run it on a
> clean working tree (git is your backup) and review the diff.

For no-frontmatter (plain-markdown) files, enrich prepends a fresh, valid block;
`title` derives from the first `# H1` else a humanized filename.

`--dry-run` reports `would-enrich` and writes nothing. `--json` emits the shared
deterministic envelope with `command: "enrich"` (schema `binder.report/v1`).
Skipped files (and preserved spec-invalid `verified` values) are advisory: bare
enrich exits `0`; `--strict` gates (exit 1) when any are present. A bad/unreadable
`<src>` → exit 2; a write failure → exit 3.

#### enrich flags

| Flag | Default | Purpose |
|---|---|---|
| `--dry-run` | `false` | Report what would be enriched without writing anything. |
| `--default-type` | `Note` (or config `default_type`) | Concept type applied when none is present or mapped. |
| `--type-map` | — | Per-directory type overrides, e.g. `"docs=Guide,adr=Decision"`. |
| `--status-map` | — | Per-directory `status` (set only when `status` is absent), e.g. `"archive=deprecated,default=active"`. |
| `--stale-after-map` | — | Per-directory `stale_after` relative to the run clock (grammar `+Nd`/`+Nm`/`+Ny`), set only when absent. Malformed → exit 2. |
| `--verified-by` | config `verified_by` | Actor to append as a `verified` stamp, e.g. `"human:ghchinoy"`. Invalid actor → exit 2. |
| `--strict` | `false` | Gate (exit 1) when any file is skipped or a preserve-or-advise finding is present. |
| `--json` | `false` | Emit the run report as deterministic JSON (schema `binder.report/v1`) instead of prose. |

The `--status-map`/`--stale-after-map`/`--verified-by` injectors are the same
declarative `#7` stamps `convert` offers, all set-when-absent; core enrichment is
`type`/`title`/`generated`.

### Linting a source corpus (`binder lint`)

`binder lint <corpus>` is a **read-only** pass over a **source markdown corpus**
(it writes nothing) that reports five health checks. It completes the command
triad — each surface is single-purpose:

| Command | Input | Purpose |
|---|---|---|
| `binder validate <bundle>` | emitted bundle | spec §11 **hard conformance** (always gates) |
| `binder review <bundle>` | emitted bundle | human **summary** (tiers, orphans, stale…) |
| `binder lint <corpus>` | **source** corpus | **pre-conversion** source health, writes nothing |

`lint` is distinct because it sees the corpus **as authored**: a missing title or
a missing `type:` is invisible in a bundle because `convert` *defaults* them.
`lint` reuses the exact converter pipeline (`convert.Analyze`) and the single
resolved-edge definition, so its "broken link" is by construction the converter's
"unresolved link" — no second resolver.

The five checks:

1. **Broken links** — an unresolved internal `.md` reference, a resolved link
   whose target concept is absent, a residual `[[wikilink]]`, and a broken
   **`#anchor`** (`foo.md#bar` whose target concept has no `bar` heading, or a
   same-doc `#bar`). `Detail` is the raw target.
2. **Missing titles** — no authored `title:` and no first-level (`# `) heading.
3. **Orphans** — a concept with **0 inbound AND 0 outbound** resolved edges (a
   truly disconnected node; stricter than `review`'s inbound-only orphan).
4. **Stale** — `stale_after` reached as of `--today` (or `SOURCE_DATE_EPOCH`).
5. **Schema violations** — a missing `type:` (`Detail: "missing type"`), or
   invalid frontmatter recovered under never-reject (`Detail: "invalid
   frontmatter: <err>"`).

**Anchor slug convention (GitHub-style, pinned):** an `#anchor` matches a heading
slugged by: lowercase; strip HTML tags; drop every character except `[a-z0-9]`,
space, hyphen; convert spaces to hyphens; collapse repeated hyphens; and give
duplicate headings the suffixes `-1`, `-2`, … in document order (so a second
`## Notes` is `#notes-1`). Slugging is code-region-aware — a `# heading` inside a
fenced code block is not a heading. This lives in `okf.HeadingSlugs`.

All findings are **spec-tolerated advisories**, so bare `binder lint` always exits
`0` (§11 hard conformance stays `binder validate`'s job over a bundle). `--strict`
gates **exit 1** when any finding is present — the same shared contract as
`convert`/`review`. The report is always emitted before the gate signals.

```bash
binder lint path/to/corpus            # report; exit 0 even with findings
binder lint path/to/corpus --strict   # exit 1 if any finding (CI)
binder lint path/to/corpus --json     # deterministic envelope, command:"lint"
```

| Flag | Default | Purpose |
|---|---|---|
| `--strict` | `false` | Gate (exit 1) when any finding is present; otherwise lint never gates. |
| `--today` | now (or `SOURCE_DATE_EPOCH`) | Date (`YYYY-MM-DD`) used for staleness. |
| `--json` | `false` | Emit the report as deterministic JSON (schema `binder.report/v1`, `command:"lint"`). |

A bad or non-directory `<corpus>` path is a usage error (exit 2).

### Relationship & trust flags (`convert`)

Relationship extraction that is always on: `[[wikilinks]]` / `[[Target|alias]]`
(resolved by path → filename → title-slug), standard `[text](a.md#anchor)`
links, in-workspace `file://` URIs (resolved to the same internal edge; see
below), and `#hashtags` merged into frontmatter `tags`. Unresolved links are left
in place **and** reported (spec §6/§11). These flags opt into the rest:

| Flag | Effect |
|---|---|
| `--fm-ref-keys related,parent` | treat the named frontmatter keys as relationship edges (keys are preserved; each resolved target is also materialized as a link in a trailing `## Related` section) |
| `--map-citations` | map a body `# Citations` list into `sources` entries |
| `--source-keys source,url` | map the named frontmatter keys into `sources` entries |
| `--map-draft` | map a `draft: true` marker to `status: draft` (never clobbers an existing status) |

Trust mapping is **off by default** and never fabricates provenance: with no
mapping flags, frontmatter round-trips byte-for-byte.

### Declarative trust & lifecycle flags (`convert`)

For CI/agentic bulk runs, `convert` can stamp trust and lifecycle metadata
declaratively across directory sections. All are **off by default** (byte-identical
output), all **set only when the field is absent** (never clobber authored values),
and all are deterministic. They share the same longest-prefix directory matcher as
`--type-map` (most-specific key wins; ties break lexicographically; keys are
trimmed of `/`).

| Flag | Effect |
|---|---|
| `--status-map "archive=deprecated,drafts=draft,default=active"` | set `status` per directory prefix; the special `default=` key applies when no prefix matches. Set only when `status` is absent. Unknown status values are not rejected (they surface as a `validate` advisory). |
| `--stale-after-map "07-benchmarks=+6m,03-transcription=+1y,legacy=+0d"` | set `stale_after` per directory prefix, computed relative to the run clock. Set only when absent. |
| `--verified-by "human:ghchinoy"` | append a `verified` actorstamp `{by, at}` to every concept. |

**Relative-date grammar** (`--stale-after-map`): `+<N><unit>` where unit is
`d` (days), `m` (months), or `y` (years) — e.g. `+6m`, `+1y`, `+0d` (today).
Dates are computed with UTC calendar arithmetic against the run clock
(`SOURCE_DATE_EPOCH`-aware, so reproducible) and written as `YYYY-MM-DD`. A
malformed map or date is a **usage error (exit 2)**.

**`--verified-by`** appends an actorstamp `{by: <actor>, at: <now, RFC3339 UTC>}`
to each concept's `verified` list, deduplicated by `(by, at)` so a re-run with a
fixed clock is idempotent. The actor comes from the flag, or from the config
`verified_by` default when the flag is absent; when **neither** is set, no
`verified` stamp is written (binder never auto-stamps). The trust **tier** stays
derived (`human:` → human-reviewed, else machine-confirmed) — no tier or score is
stored. The actor must follow the actor convention — valid forms are
`human:<id>`, `process:<id>`, `team:<id>`, or `<producer>/<version>` (e.g.
`binder/0.1.0`); an invalid value is a **usage error (exit 2)**.

### `binder config`

`binder config` shows the resolved effective configuration and, where cheap, the
source of each value. Configuration is resolved with the precedence **flag > env
> config file > built-in default**:

- **Config file** (first found wins): `./.binder.yaml`, then
  `$XDG_CONFIG_HOME/binder/config.yaml` (fallback
  `$HOME/.config/binder/config.yaml`). A missing config file is normal — defaults
  apply and it is never an error.
- **Environment:** prefix `BINDER_`, e.g. `BINDER_VERIFIED_BY`, `BINDER_DEFAULT_TYPE`.
- **Keys:** `verified_by` (default actor for `--verified-by`) and `default_type`
  (default for `--default-type`). The structure is namespaced and extensible.

A config `verified_by` must itself be a valid actor; a malformed value fails fast
at config-load with a **usage error (exit 2)**. `binder config` ships `--json`
(enveloped, schema `binder.config/v1`):

```bash
binder config
binder config --json | jq '.result.values.verified_by'
```

```yaml
# .binder.yaml
verified_by: human:ghchinoy
default_type: Guide
```

### Strict mode

Every command is **never-reject by default**: advisories (broken links, orphans,
staleness, recovered frontmatter, trust well-formedness) are reported but exit
`0`. `--strict` opts into gating them at exit `1` for CI. Hard `validate` spec §11
non-conformance always gates regardless of `--strict`.

| Command | What `--strict` gates |
|---|---|
| `validate --strict` | trust well-formedness advisories (in addition to hard non-conformance, which always gates) |
| `review --strict` | any review finding — orphans, stale, unresolved/broken edges, unparsed-frontmatter recoveries |
| `convert --strict` | unresolved links + recovery warnings |
| `enrich --strict` | skipped (unparseable-frontmatter) files + preserve-or-advise findings |
| `lint --strict` | any lint finding — broken links, missing titles, orphans, stale, schema violations |

**`file://` links.** IDE- and assistant-generated `file:///abs/path/doc.md` links
that point inside the workspace root resolve to internal edges rewritten to
`/<outRel>` — no absolute machine path leaks into the output and runs stay
byte-identical. The root defaults to the corpus `<src>` and can be widened with
`--workspace-root`. Paths are percent-decoded; an empty authority and
`file://localhost/…` are local while any other host stays external; `..`/symlink
escapes and out-of-root targets stay external. Unresolved `file://` links are
tolerated (recorded as advisories, exit code stays `0`), never fatal.

`convert` never rejects: a source file whose frontmatter will not parse (invalid
YAML or an unterminated fence) is preserved losslessly as a plain-markdown concept
— its original text, fence and all, becomes the body — stamped with a default
`type` so the bundle stays conformant, and reported. Such a file carries a
binder-namespaced marker `x_binder: { recovered: true, reason: ... }` in its
emitted frontmatter; `binder review` reads that same marker (not a body-shape
guess) to report recovered files, so `--report` and `review` always agree.

## Agent Skill / Plugin

binder ships an **Agent Plugin**, `okf-convert`, that teaches an AI agent to
*drive* binder — to ingest a plain-markdown corpus into a conformant OKF v0.2
bundle by reasoning over binder's `--json` output, with the ingestion-analysis
judgment the deterministic CLI cannot encode (flag-choice triage, the
`enrich`/`lint` remediation loop, and the never-fabricate-trust discipline). It
lives in this repo under [`plugins/okf-convert/`](plugins/okf-convert/) and is
standalone-installable.

**Install** (in a plugin-aware host such as Claude Code):

```text
/plugin marketplace add ghchinoy/binder
/plugin install okf-convert
```

This resolves binder's self-hosted marketplace
([`.claude-plugin/marketplace.json`](.claude-plugin/marketplace.json)) and the
`okf-convert` skill. The plugin **assumes the `binder` binary is installed** (see
[Installation](#installation)) — it drives the CLI, it does not embed it.

**Usage walkthrough.** A tiny sample corpus ships at
[`plugins/okf-convert/skills/okf-convert/assets/sample-corpus/`](plugins/okf-convert/skills/okf-convert/assets/sample-corpus/)
with three deliberate triage cases (an unresolved link, a missing-title file, a
no-frontmatter file). Following the skill, an agent drives:

```bash
cd plugins/okf-convert/skills/okf-convert/assets/sample-corpus

# 3. Dry-run triage — reason over structured output, never scrape prose
binder convert . --dry-run --json | jq '.result | {num_concepts, num_unresolved, num_recovered}'
binder lint . --json | jq '.result | {broken_links, missing_titles, schema_violations}'

# 4. Remediate the source frontmatter (additive, byte-faithful; preview first)
binder enrich . --dry-run --json | jq '.result.files'

# 5. Convert and validate for §11 conformance
binder convert . -o /tmp/sample-bundle --json | jq '.result.num_concepts'
binder validate /tmp/sample-bundle --json | jq '.result.findings'   # [] ⇒ conformant (exit 0)
```

The plugin's boundary is deliberate: it is the **binder-driven** ingestion
surface. For authoring or validating a *single* bundle by hand with no binaries,
use the tool-agnostic `okf-author` / `okf-validate` skills in
[`ghchinoy/agent-skills`](https://github.com/ghchinoy/agent-skills). The bundle
is validated in CI against Agent Plugins 1.0.0 by
[`scripts/validate-plugin.sh`](scripts/validate-plugin.sh) (the `plugin-validate`
job), independent of the Go gate.

## MCP server (`binder mcp`)

`binder mcp` runs binder as a stdio [Model Context Protocol](https://modelcontextprotocol.io)
server, exposing binder's **additive** verbs as MCP **tools** to an MCP-capable
agent harness (Claude Code, Cursor, Zed). Each tool returns the **same**
deterministic `binder.report/v1` payload as the corresponding `binder <cmd>
--json` — the handlers reuse the same internal functions and the same JSON
encoder, so there is no second serialization path and no drift from the CLI.

It is a transport, not a report-producing command: it has no `--json` flag (its
*outputs* are the structured tool payloads). It serves over stdio until the
client disconnects.

> **Output-routing flags are the deliberate 1:1 exception.** Every tool
> parameter mirrors its CLI flag one-to-one *except* the output-routing flags
> `--report` / `--output` / `--json`, which the tools do not expose: over MCP the
> transport **is** the JSON channel, so there is nothing to route and no `--json`
> flag to toggle. The tool payloads are byte-identical to the corresponding
> `binder <cmd> --json`. (`convert`'s `out`/`dry_run` and `graph`'s `format`
> select *what* is produced, not how the report is routed, so they remain.)

**Wire it into a harness** (Claude Code):

```bash
claude mcp add binder -- binder mcp
```

…or add an `.mcp.json` entry:

```json
{ "mcpServers": { "binder": { "command": "binder", "args": ["mcp"] } } }
```

**Tools** (each parameter mirrors the corresponding CLI flag 1:1):

| Tool | Key params | Returns |
|---|---|---|
| `convert` | `src` (req), `out` (req unless `dry_run`), `dry_run`, `default_type`, `type_map`, `fm_ref_keys`, `source_keys`, `map_citations`, `map_draft`, `status_map`, `stale_after_map`, `verified_by`, `workspace_root`, `group_by_type`, `include_backlinks`, `include_graph`, `strict` | `convert` report envelope (`dry_run:true` → the ingestion-analysis preview, writes nothing) |
| `validate` | `bundle` (req), `strict` | `validate` report envelope |
| `review` | `bundle` (req), `today`, `strict` | `review` report envelope |
| `lint` | `src` (req), `today`, `strict` | `lint` report envelope |
| `graph` | `bundle` (req), `format` (`dot`\|`json`\|`graphml`\|`html`, default `json`), `today` | raw export bytes — `format:json` is the raw `{nodes,edges}`, **not** the report envelope |
| `list_graphs` | `bundle` (req), `today`, `id_key` | `list_graphs` report envelope — the LPG **schema descriptor** (graph name, node labels = concept types, the single `LINKS` edge label, each with counts + property declarations). Read-only introspection derived from the same projection as `graph` |

The surface is deliberately additive (produce/validate). Source-mutating verbs
(`enrich`, `emit_concept`) and read/search tools are **not** exposed — the read
surface belongs to the knowledge store, and authoring over MCP is a later
concern. Invariants are preserved end to end: findings are returned **in** the
payload (a tool with findings is not an MCP error), `verified_by` is applied
**only** when explicitly passed (never auto-stamped; an invalid actor is a
usage error), and payloads honor `SOURCE_DATE_EPOCH`/`today` for determinism.

The [`okf-convert` plugin](#agent-skill--plugin) also ships a
[`.mcp.json`](plugins/okf-convert/.mcp.json) that registers this server, so a
plugin-aware host wires up `binder mcp` on install (the `binder` binary must be
on `PATH`).

See the [user guide](docs/user_guide.md#mcp-server-binder-mcp) for the full tool
schemas and examples.

## How it works

- `internal/okf` — the OKF domain model plus two small interfaces, `Codec`
  (parse/serialize concepts) and `LinkGraph` (extract/resolve links). Trust tiers,
  staleness, and the Attested flag are *derived* from frontmatter, never stored.
- `internal/okf/native` — the sole codec: [`goldmark`](https://github.com/yuin/goldmark)
  for markdown/link extraction and [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3)
  (`yaml.Node`) for order-preserving frontmatter. Unmodified frontmatter is passed
  through verbatim so round-trips are byte-faithful, including nested-map key order.
- `internal/convert` — the converter: concept discovery, link/wikilink rewriting,
  frontmatter-ref edges, hashtag/tag merge, per-directory index generation, and
  corpus-native trust mapping.
- `internal/bundle` — loads an on-disk bundle into the domain model (read side
  shared by `index`, `review`, and `graph`).
- `internal/review` — bundle summary (types, tiers, stale, attested, orphans,
  broken links).
- `internal/graph` — concept-graph export in dot/json/graphml/html.
- `internal/validate` — the §11 conformance checker.
- `internal/mcp` — the stdio MCP server (`binder mcp`): tool handlers that reuse
  the internal functions above + `internal/clijson`, returning the same
  `binder.report/v1` payloads as `--json`. The MCP SDK is confined here.
- `cmd` — the [Cobra](https://github.com/spf13/cobra) CLI; the concrete codec is
  injected once at the composition root (`cmd/root.go`) — every other package
  depends only on the `okf` interfaces.

## Development

```bash
git clone https://github.com/ghchinoy/binder.git
cd binder
make build        # -> bin/binder
make check        # gate: gofmt + go vet + go test ./...
```

Requires **Go 1.26+**. Dependencies are pinned via `go.mod`/`go.sum` and
fetched from the Go module proxy at build time (network required).

The full exit gate additionally cross-checks binder's verdicts against the
external `okfcli/okf` validator:

```bash
make okf-install  # go install github.com/okfcli/okf/cmd/okf@v0.3.0
make gate         # local checks + external differential validation
```

`make gate` runs `scripts/interop.sh`, which compares binder's and `okf`'s
verdicts in both directions and fails on any unexpected disagreement.

## Roadmap

The following is **planned, not yet shipped**:

- **A community-core codec adapter** (e.g. `--okf-impl=community`): a second
  `Codec` behind the existing interface, slotted in only after it is confirmed
  byte-complete against the golden bundles.

**Shipped, layered over the settled CLI contract:** the
[Agent Skill and Agent-Plugin bundle](#agent-skill--plugin) (`okf-convert`, #14),
then the [MCP server mode](#mcp-server-binder-mcp) (`binder mcp`, #15) — the
additive convert/validate/review/lint/graph tools over the same OKF core. (These
were sequenced Skill/Plugin **before** MCP, so MCP builds on already-settled
`--json` payloads.) Declarative trust/lifecycle flags (`--status-map`,
`--stale-after-map`, `--verified-by`; #7), `binder config` (#10), the standalone
`binder lint` (#8), in-place `binder enrich` (#5), and `--strict` mode have also
shipped (see above). The
[user guide](docs/user_guide.md#roadmap--planned-features) maps each to its issue.

Today's shipped surface is the `convert`, `validate`, `index`, `review`, `lint`,
`graph`, `config`, `enrich`, and `mcp` CLI described above, plus the
`okf-convert` Agent Skill/Plugin.

## Contributing

Pull requests are welcome. For major changes, please open an issue first to
discuss what you'd like to change. Make sure `make check` passes and add tests
for new behavior.

## License

Licensed under the [Apache License 2.0](LICENSE).
