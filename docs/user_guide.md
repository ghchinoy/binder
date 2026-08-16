# binder user guide

This is the deep reference for `binder`. The [README](../README.md) is the
concise landing page (what it is, install, quickstart), and the
[tutorial](tutorial.md) is a hands-on, runnable walkthrough for a first-time
user; this guide documents **every command and flag**, the OKF v0.2 output
layout, the full trust vocabulary, the relationship-extraction rules,
malformed-input recovery, CI usage, and worked end-to-end examples.

`binder` converts a plain-markdown corpus into a conformant
[Open Knowledge Format (OKF) v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle and reports on OKF bundles. It is **Phase 2 complete**: as of v0.3.0 every
Phase 2.x enhancement has shipped, along with the graph surface, `infer`, `config`
(including mutation), and the stdio MCP server. Only the community-core codec
adapter remains planned — see [Roadmap](#roadmap--planned-features).

> This guide grows alongside each feature as it ships. Any remaining planned work
> is listed under [Roadmap & planned features](#roadmap--planned-features), each
> item linking to its tracking issue.

## Table of Contents

- [Invariants](#invariants)
- [Concepts and terminology](#concepts-and-terminology)
- [Commands](#commands)
  - [`convert`](#convert)
  - [`enrich`](#enrich)
  - [`validate`](#validate)
  - [`index`](#index)
  - [`review`](#review)
  - [`lint`](#lint)
  - [`graph`](#graph)
  - [`infer`](#infer)
  - [`config`](#config)
- [JSON output (`--json`) and the exit-code contract](#json-output---json-and-the-exit-code-contract)
  - [Strict mode](#strict-mode)
- [Discovery surface (`--version` / `--help`)](#discovery-surface---version----help)
- [MCP server (`binder mcp`)](#mcp-server-binder-mcp)
- [The graph surface](#the-graph-surface)
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
| **Reserved file** | `index.md` and `log.md`. These are structural rather than concepts: they are not required to carry a `type`, and their structure is not examined by `validate`. |
| **Frontmatter** | The YAML block between `---` fences at the top of a concept. It is authoritative: `type` and the trust view are projections of it. |
| **Link / edge** | A directed relationship between concepts. Edges come from body markdown links (spec §6); resolved edges name a concept that exists in the bundle. |
| **Trust signals** | The v0.2 provenance/lifecycle vocabulary (`sources`, `generated`, `verified`, `status`, `stale_after`, …). |

## Commands

The binary exposes ten commands. All bundle-reading commands
(`validate`/`index`/`review`/`graph`) load a bundle through the same codec, so
their views of concepts, links, and trust always agree. `lint`, `enrich`, and
`infer` are the exceptions: they read (and, for `enrich`, mutate) a **source
corpus** rather than a bundle; `lint` and `enrich` do so through the same
converter machinery, and `infer` never writes at all. `mcp` is a transport rather than a
corpus/bundle command; it exposes the additive verbs over stdio (see
[MCP server](#mcp-server-binder-mcp)).

*Summary of the command set — written for this guide, **not** verbatim
`binder --help` output. Run `binder --help` for the authoritative strings.*

```text
binder convert    Convert a markdown corpus into an OKF v0.2 bundle
binder enrich     Inject missing frontmatter into a source tree, in place (frontmatter only)
binder validate   Check a bundle for OKF v0.2 conformance (spec §11)
binder index      (Re)generate the per-directory index.md nav tree (spec §8)
binder review     Summarize a bundle: concepts, links, orphans, trust tiers, stale
binder lint       Report source-corpus health before conversion (writes nothing)
binder graph      Export the bundle's concept graph (dot|json|graphml|html)
binder infer      Inspect a source markdown corpus and propose a --type-map
binder config     Show the resolved effective configuration and each value's source; get/set/unset to persist
binder mcp        Run binder as a stdio MCP server (convert/validate/review/lint/graph/list_graphs/query_graph)
```

Two further built-ins come from the CLI framework rather than from binder's own
surface, and are not counted among the ten: `binder help [command]` and
`binder completion <bash|zsh|fish|powershell>` (see
[Discovery surface](#discovery-surface---version----help)).

Every command supports `-h`/`--help`. The root binary supports `-v`/`--version`.
`validate`, `review`, `lint`, `convert`, `enrich`, and `infer` also support
`--strict` to gate advisories in CI (see [Strict mode](#strict-mode)); configuration is resolved
once with the precedence flag > env > file > default (see [`config`](#config)).

`convert` and `enrich` are complementary and single-purpose: `convert` compiles a
corpus into a **new bundle** out-of-place (and never touches the source);
`enrich` brings a **source tree's frontmatter** up to spec **in place** and
touches frontmatter only (no bodies, no links, no indexes). See [`enrich`](#enrich).

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
- standard markdown links, `[[wikilinks]]`, and in-workspace `file://` URIs
  rewritten to bundle-relative form (see [`file://` link resolution](#file-link-resolution));
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
| `--default-type` | `Note` (or config `default_type`) | Concept type applied when none is present or mapped. |
| `--type-map` | — | Per-directory type overrides, e.g. `"docs=Guide,adr=Decision"`. The longest (most specific) matching directory key wins. |
| `--status-map` | — | Per-directory `status`, e.g. `"archive=deprecated,drafts=draft,default=active"`. Longest-prefix match; `default=` is the fallback. Set **only when `status` is absent** (never clobbers an authored value). See [Declarative trust & lifecycle](#declarative-trust--lifecycle-flags). |
| `--stale-after-map` | — | Per-directory `stale_after` relative to now, e.g. `"07-benchmarks=+6m,legacy=+0d"`. Grammar `+Nd`/`+Nm`/`+Ny` (days/months/years, UTC `YYYY-MM-DD`). Longest-prefix match; set **only when `stale_after` is absent**. |
| `--verified-by` | config `verified_by` | Actor appended as a `verified` stamp, e.g. `"human:ghchinoy"` or `"binder/0.3.0"`. Validated with the actor grammar; an invalid value is a usage error (exit 2). Appends only — never rewrites the derived tier. See [Declarative trust & lifecycle](#declarative-trust--lifecycle-flags). |
| `--canonicalize-status` | `false` | **Opt-in**: rewrite a known `--status-map` alias to the OKF §5.4 vocabulary (`active`→`stable`, `wip`/`in-progress`→`draft`, `archived`/`legacy`→`deprecated`). The vocabulary **check** is always on; this flag controls only the rewrite. Each rewrite is reported in `status_notes`. See [Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status). |
| `--fm-ref-keys` | — | Frontmatter keys treated as relationship edges, e.g. `"related,parent"`. |
| `--workspace-root` | `<src>` root | Boundary within which `file://` links resolve to internal edges. See [`file://` link resolution](#file-link-resolution). |
| `--external-root` | — | Declare a **known** sibling-workspace root (repeatable). A `file://` link that resolves under it stays external (never internalized) but its outside-root advisory is suppressed. An empty value is a usage error (exit 2). See [`file://` link resolution](#file-link-resolution). |
| `--map-citations` | `false` | Map a body `# Citations` list into `sources` entries. |
| `--source-keys` | — | Frontmatter keys to map into `sources` entries, e.g. `"source,author"`. |
| `--map-draft` | `false` | Map a `draft: true` marker to `status: draft` (only when `status` is absent). |
| `--report` | — | Also write the run report (the same text printed to stdout) to this file. |
| `--json` | `false` | Emit the run report as deterministic JSON (schema `binder.report/v1`) instead of prose. Composes with `--report` (the file gets whichever format `--json` selects). See [JSON output](#json-output---json-and-the-exit-code-contract). |
| `--strict` | `false` | Gate (exit 1) on unresolved links or recovery warnings. Without it these never gate (never-reject; exit 0). See [Strict mode](#strict-mode). |
| `--group-by-type` | `false` | Append an additive `# Catalog` of all concepts, grouped by type, to the **root** `index.md`. See [The type-grouped catalog](#the-type-grouped-catalog). |
| `--include-backlinks` | `false` | Annotate each catalog entry with its inbound resolved edges (requires `--group-by-type`). |
| `--include-graph` | `false` | Annotate each catalog entry with its outbound resolved edges (requires `--group-by-type`). |

`--map-citations`, `--source-keys`, and `--map-draft` are the **trust-mapping**
flags — all off by default. See [The trust vocabulary](#the-trust-vocabulary).
`--status-map`, `--stale-after-map`, and `--verified-by` are the **declarative
trust & lifecycle** flags — also off by default; with none set, output is
byte-identical to a plain run. See [Declarative trust & lifecycle](#declarative-trust--lifecycle-flags).
`--group-by-type`, `--include-backlinks`, and `--include-graph` are the
**catalog** flags — also off by default and shared with `binder index`; see
[The type-grouped catalog](#the-type-grouped-catalog).

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

#### `file://` link resolution

IDEs (VS Code), OS tools, and AI coding assistants often emit clickable
`file:///absolute/path/to/project/doc.md` links. `convert` resolves a `file://`
URI that points **inside the workspace root** to the same internal, bundle-
relative edge a plain relative link would produce, so those links populate the
link graph instead of being dropped as external (`links: 0`).

Resolution rules:

- The URI is parsed with `net/url`; the path is **percent-decoded** (`%20` → space).
- **Authority/host:** an empty authority (`file:///path`) and `file://localhost/path`
  are local. Any other host (`file://otherhost/path`) is remote and left external.
- **Boundary:** the absolute path must fall inside `--workspace-root` (default: the
  corpus `<src>` root). Targets that escape the root — via `..` or a symlink — stay
  external. Set `--workspace-root` to a parent directory when the corpus is a
  subdirectory of a larger workspace.
- Only `.md` targets resolve; `#fragments` are preserved. A resolved edge is
  rewritten to `/<outRel>`, so **no absolute machine path leaks into the output**
  and runs stay byte-identical.
- A `file://` link that is remote, outside the root, or otherwise unresolved is
  **tolerated**, never fatal: it is recorded as an advisory/unresolved edge and the
  exit code stays `0`.

So for a corpus at `/home/me/notes` whose `intro.md` links to
`file:///home/me/notes/docs/doc.md`, the target is inside the root: the emitted
`intro.md` body carries `[doc](/docs/doc.md)` and the run reports
`links: 1 (resolved 1, unresolved 0)`.

```bash
binder convert /home/me/notes -o /tmp/bundle
```

##### Declaring known sibling roots (`--external-root`)

In an established multi-repo tree, an author often links across sibling
workspaces on purpose — `speech/` referencing
`file:///Users/me/projects/jibo/docs/...`. Those links are **genuinely
external** and `convert` leaves them exactly as written, but it also emits an
advisory for each one — *stand-in paths; the message shape is verbatim:*

```text
Warnings:
  - file:// link "file:///Users/me/projects/jibo/docs/a.md" resolves outside the workspace root; left external
```

`--external-root <path>` lets you acknowledge a known sibling root so its
advisory is suppressed. It is **repeatable** (`--external-root A --external-root B`),
consistent with the `file://`-boundary flag family. The behavior is deliberately
narrow:

- **Matched links stay external.** The flag *only* suppresses the advisory — it
  never internalizes, rewrites, inlines, or resolves the link. The emitted
  bundle bytes are **identical** with and without the flag for the same corpus.
- **Non-matching external links still advise.** A `file://` link that resolves
  outside every declared root warns exactly as before.
- **Matching is segment-safe.** Roots are compared at path-segment boundaries, so
  `--external-root /projects/jib` does **not** suppress `/projects/jibo/...`.
- **Symlinks stay coherent.** A link whose real target (through a symlink)
  resolves under a declared root is suppressed the same way as a lexical match.
- **Declared roots need not exist here.** A well-formed path is accepted even if
  it is absent on the converting machine (e.g. in CI) — that is the point, since
  the sibling lives outside this checkout. Only an *empty* value is a usage error
  (exit 2). No filesystem `stat` is performed, so runs stay deterministic and the
  ordering of declared roots never affects output.

Under `--strict`, declared sibling roots do **not** gate (external links never
counted as unresolved edges, so the exit code is unaffected); unknown external
links continue to behave exactly as they do today.

For the `speech/` tree above, that means declaring the parent of the sibling
workspaces:

```bash
binder convert ./speech -o ./bundle --external-root /Users/me/projects
```

This flag applies to `convert` only. `binder lint` reports its own health
findings and does not surface `convert`'s `file://` advisories (external
`file://` targets are not recorded as edges), so there is nothing there for
`--external-root` to suppress.

### `enrich`

```text
binder enrich <src> [flags]
```

Adds the missing required OKF frontmatter (`type`, `title`, `generated`) to the
markdown files under `<src>`, **in place**. It is for authors adopting OKF in an
existing (usually git-tracked) repo who want the required frontmatter added to
their files without a convert-to-temp-and-copy-back dance.

**enrich touches frontmatter ONLY.** Unlike `convert`, it does **no** link
rewriting, **no** `index.md` generation, **no** `## Related` section, and **no**
`#hashtag` merge — bodies are otherwise untouched. `binder convert` is unchanged
(still strictly out-of-place; it never touches `<src>`). Enrichment per file:

- `type` ensured (precedence: existing → `--type-map` per-directory →
  `--default-type`, default `Note`), set **only if absent**;
- `title` ensured (precedence: existing → first `# H1` → humanized filename), set
  **only if absent**;
- `generated` provenance stamp added **only if absent**;
- optionally the declarative `#7` stamps `status`/`stale_after`/`verified`
  (`--status-map`/`--stale-after-map`/`--verified-by`), all set-when-absent.

A plain-markdown file (no frontmatter fence) gets a fresh, valid block prepended.

#### The safety model (load-bearing — enrich mutates the source)

1. **Additive / never-clobber.** Only keys that are **absent** are added; an
   authored value (any key) is never overwritten. The **one** exception is the
   explicit, opt-in [`--overwrite-keys`](#opt-in-refresh---overwrite-keys) flag,
   which refreshes only the keys you name — trust keys are refused.
2. **Idempotent.** A second run finds every key present → `unchanged` → **no
   write**. `generated.at` is stable across runs (set-when-absent).
3. **Body + pre-existing keys byte-faithful (files that already have
   frontmatter).** enrich reuses the codec's byte-faithful serializer: on a file
   it changes, every unchanged frontmatter key is re-emitted **from its original
   source bytes** — nested-map and list order, flow-vs-block style, interior
   spacing, scalar quoting/folding, YAML tags (e.g. an `!!timestamp` never
   silently becomes an `!!str`), and comments are all preserved — and only the
   **added or changed** keys are encoded fresh. The **body** is re-emitted
   exactly, *including the original frontmatter/body separator*: a body that
   abutted the closing fence stays abutting it, and existing blank lines are
   neither added nor removed. This guarantee is scoped to files that **already
   have frontmatter** — see *Residual bounds* below for the byte-level cases it
   does **not** cover.

   This extends to **sibling granularity**: when a container legitimately changes
   (e.g. a stamp appended to `verified`), the **pre-existing entries** are
   re-emitted from their original bytes, so an existing human attestation is
   **never reshaped or retyped merely because a neighbour was added**. The entry
   keeps its bytes — flow style, interior spacing, `{by,at}` sub-key order, and
   the `!!timestamp` tag all intact — for a `verified` authored as:

   - a **block sequence** (`verified:` then `- { … }` entries),
   - a **flow sequence**, single- **or multi-line** (`verified: [{ … }]`), or
   - a bare inline **`{by,at}` mapping** (`verified: { … }`).

   Appending to a flow sequence or bare mapping re-emits the container as a block
   sequence (the natural shape once it has more than one entry), but each
   pre-existing entry keeps its exact bytes. Because preservation is a source-byte
   copy of the entry, it holds whatever the entry contains — nested mappings,
   multi-line block scalars, anchors/aliases, quoted scalars. A `{by,at}` written
   instead as an *indented block mapping* is re-indented into the first list item
   — its leading whitespace necessarily changes when a lone mapping value becomes
   a sequence item — but its tokens, sub-key order, and YAML tags are still
   preserved, so an `!!timestamp` never becomes an `!!str`.

   > **Known limitation.** Preservation inside a *changed* container is per
   > **entry**, not per byte of the whole container. YAML **comments** (and blank
   > separator lines) that sit *inside a container that changes* — before, between,
   > or beside its entries — are **not carried onto the rebuilt value**. This is a
   > limit of the underlying YAML node model (that trivia is not attached to the
   > entry nodes the serializer copies), not a deliberate normalization; comments
   > on **unchanged** keys are preserved as above. A comment interleaved in a
   > *changed* multi-line **flow sequence** (`verified: [` … `]` over several
   > lines) is likewise dropped — this used to make the output **unparseable** and
   > no longer does. Relatedly, an empty flow mapping (`verified: {}`) is reshaped
   > to a block item (`- {}`) when a stamp is appended, and a changed multi-line
   > flow **mapping** (`verified: {` … `}` over several lines) is copied into the
   > first block item from its source bytes **without re-indentation** — its
   > interior lines and closing `}` keep their original columns, so the output
   > re-parses but is not cleanly re-indented.
4. **Skip-unchanged (no git churn).** A file that needs no key is **not written
   at all** — no spurious diffs, no mtime bumps. Critical for git-tracked trees.
5. **Skip-unparseable.** A file whose frontmatter will not parse (invalid YAML or
   an unterminated fence) is **skipped and reported** (`status: skipped`, with a
   reason), **never rewritten** — and is left **byte-identical on disk**. This is
   deliberately unlike `convert`'s out-of-place recover-as-body (which moves the
   bad frontmatter into the body): silent recovery in place would be destructive.
6. **Reserved files** (`index.md`/`log.md`) are skipped.
7. **Atomic write.** enrich writes a temp file in the target's directory,
   `fsync`s it, then `rename`s it over the original (a same-filesystem atomic
   replace), preserving the file mode. An interrupt never leaves a partial or
   corrupt source file; a mid-write failure leaves the original intact.
8. **Deterministic.** `generated.at` (and any resolved `stale_after`) honour
   `SOURCE_DATE_EPOCH`.

> **Residual bounds (the body guarantee is bounded, not unconditional).** Even on
> a file enrich changes, four byte-level deviations remain that the body guarantee
> above does **not** cover:
> 1. **CRLF → LF.** Body line endings are normalized `\r\n`→`\n`, so a CRLF file
>    round-tripped is not byte-identical.
> 2. **Trailing newline.** A file whose content ends without a trailing newline
>    gains one on the closing `---` fence line.
> 3. **Empty frontmatter re-emission.** A file whose frontmatter block is empty
>    (`---` immediately followed by `---`) has that empty block re-emitted as
>    `{}`. The body boundary is still preserved.
> 4. **No frontmatter at all (synthesis, not round-trip).** A file with no
>    frontmatter is not a round-trip: enrich **synthesizes** a header and inserts a
>    single blank line between it and the body. Every body byte survives verbatim
>    and in order, but a blank is prepended — which is why the byte-faithfulness
>    guarantee is scoped to files that already have frontmatter, and does not apply
>    here.
>
> A file that needs no key is never rewritten, so untouched files (including CRLF-
> or spacing-only ones) are left exactly as-is. Because enrich mutates the source,
> run it on a clean working tree (git is your backup) and review the diff.

#### Flags

| Flag | Default | Purpose |
|---|---|---|
| `--dry-run` | `false` | Report what would be enriched (`status: would-enrich`) without writing anything. |
| `--default-type` | `Note` (or config `default_type`) | Concept type applied when none is present or mapped. |
| `--type-map` | — | Per-directory type overrides, e.g. `"docs=Guide,adr=Decision"`. Longest matching directory key wins. |
| `--status-map` | — | Per-directory `status`, e.g. `"archive=deprecated,default=active"`; `default=` is the fallback. Set **only when `status` is absent**. |
| `--stale-after-map` | — | Per-directory `stale_after` relative to the run clock (grammar `+Nd`/`+Nm`/`+Ny`, UTC `YYYY-MM-DD`); set **only when absent**. Malformed → exit 2. |
| `--verified-by` | config `verified_by` | Actor appended as a `verified` stamp, e.g. `"human:ghchinoy"`. Invalid actor → exit 2. Appends only (dedup by `by,at`). |
| `--canonicalize-status` | `false` | **Opt-in**: rewrite a known `--status-map` alias to the OKF §5.4 vocabulary (`active`→`stable`, `wip`/`in-progress`→`draft`, `archived`/`legacy`→`deprecated`). The vocabulary **check** is always on; this flag controls only the rewrite. Each rewrite is reported in `status_notes`. See [Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status). |
| `--overwrite-keys` | — | **Opt-in** comma-separated keys to **refresh in place even when present**, e.g. `"status,stale_after"`. Default is additive/never-clobber. Scoped strictly to the named keys; trust keys are **refused** (exit 2). See [Opt-in refresh](#opt-in-refresh---overwrite-keys). |
| `--strict` | `false` | Gate (exit 1) when any file is skipped or a preserve-or-advise finding is present. Without it these never gate (exit 0). |
| `--json` | `false` | Emit the run report as deterministic JSON (schema `binder.report/v1`, `command:"enrich"`) instead of prose. |

The `--status-map`/`--stale-after-map`/`--verified-by` injectors are the same
declarative `#7` stamps `convert` offers; core enrichment is `type`/`title`/`generated`.

#### Output report

*Report shape, with `path/to/corpus` and the filenames standing in for a real
corpus — not a capture of any particular run:*

```text
enrich path/to/corpus
3 file(s): 2 enriched, 1 unchanged, 0 skipped
  enriched getting-started.md (added: generated, title, type)
  enriched notes/idea.md (added: generated)
```

The `--json` report carries `src`, `dry_run`, the `num_files`/`num_enriched`/
`num_unchanged`/`num_skipped` counts, a per-file `files` array (`path`, `status`
∈ `enriched|unchanged|would-enrich|skipped`, sorted `added` keys, the sorted
`overwritten` keys when [`--overwrite-keys`](#opt-in-refresh---overwrite-keys)
refreshed a pre-existing key, and a `reason` for skips), and a `warnings` array
(preserve-or-advise notes). Empty arrays serialize as `[]`, and two runs on the
same input are byte-identical.

#### Opt-in refresh: `--overwrite-keys`

`enrich` is additive by default: a key that is already present is left exactly as
authored. In maintenance workflows you sometimes need to **refresh** a value
across a whole corpus — bump `stale_after` after a new benchmark release, flip a
`status`, or correct a taxonomy `type`. `--overwrite-keys <k1,k2,…>` is the
narrow, explicit exception that does this, and **only** for the keys you name:

```bash
# Refresh status and stale_after even where already present; everything else stays put.
binder enrich <src> \
  --status-map "07-benchmarks=deprecated" \
  --stale-after-map "07-benchmarks=+6m" \
  --overwrite-keys status,stale_after
```

Rules that make it safe to run on a git-tracked tree:

- **Scoped strictly to the named keys.** Every other pre-existing key, custom
  frontmatter, **key order**, and surrounding bytes are untouched and
  byte-faithful. A named key is refreshed **in place** (its position is kept).
- **Only when a value source exists.** A key is refreshed only if the run
  actually produces a value for it (e.g. `status` needs `--status-map`/
  `--default-type`; `stale_after` needs `--stale-after-map`). Naming a key with
  no source leaves the authored value untouched — it is never blanked.
- **The effective refreshable set is exactly four keys: `type`, `title`,
  `status`, `stale_after`.** Those are the only keys `enrich` ever computes, so
  they are the only keys `--overwrite-keys` can refresh. Naming any *other*
  non-trust key — `--overwrite-keys owner`, `--overwrite-keys foo` — is
  **accepted silently**: the run exits `0`, the named key is left exactly as
  authored, and nothing is reported. There is no "unknown key" diagnostic, so
  check your spelling. (Naming a *trust* key is the opposite: a loud refusal,
  exit `2`, nothing written — see **Trust keys are refused** below.)
- **Respects `--dry-run`, skip-unchanged, and the atomic write.** Refreshing a
  key to the value it already has rewrites nothing and is not counted as
  modified; two runs are byte-identical (`SOURCE_DATE_EPOCH`-deterministic).
- **Trust keys are refused (exit 2).** Naming a trust/attestation-carrying key —
  `verified`, `verified_by`, `sources`, `generated`, `usage_window`, `runtime`,
  `parameters`, `computation`, `executor`, `attester` — fails loudly with a
  message naming the offending key and **writes nothing**. These can carry human
  attestations; overwriting them would violate the **never-fabricate-trust**
  invariant. The lifecycle keys `status` and `stale_after` are the intended,
  allowed targets.

The report distinguishes keys that were **added** (were absent) from keys that
were **overwritten** (named and refreshed in place) — again a report shape, with
stand-in paths rather than a capture:

```text
enrich path/to/corpus
1 file(s): 1 enriched, 0 unchanged, 0 skipped
  enriched 07-benchmarks/mmlu.md (overwritten: stale_after, status)
```

#### `verified` preserve-or-advise

When `--verified-by` is set and a file already carries a **spec-invalid scalar**
`verified` value (spec §5.2 wants a `{by,at}` stamp or a list of them), enrich
**preserves the authored value unchanged** and does **not** append — it never
silently drops or reshapes authored data — reporting the file under `warnings`
instead. This same shared helper backs `convert`, which surfaces it as a warning.

```bash
binder enrich path/to/corpus            # write injected frontmatter
binder enrich path/to/corpus --dry-run  # preview; writes nothing
binder enrich path/to/corpus --json     # deterministic envelope, command:"enrich"
binder enrich path/to/corpus --strict   # exit 1 if any file is skipped
```

Exit codes: clean run → `0`; a bad/unreadable `<src>` → `2`; a bad flag value
(`--status-map`/`--stale-after-map`/`--verified-by`) → `2`; a write failure →
`3`; skipped/advisory findings gate to `1` only under `--strict`.

### `validate`

```text
binder validate <bundle>
```

Checks the **hard conformance rules** (spec §11): every non-reserved `.md` has a
parseable frontmatter block with a non-empty `type`. Everything else it
checks — trust well-formedness, actor conventions, date shapes — is reported as
an **advisory** and never rejects the bundle.

`validate` exits non-zero only when there is at least one hard violation
(unparseable frontmatter or a missing/empty `type`), which makes it a clean CI
gate. Reserved files (`index.md`/`log.md`) are counted but not required to carry
a `type`, and their structure (spec §8/§9) is not examined — so a `conformant`
verdict covers the concept files only.

When the bundle contains at least one reserved file, `validate` prints a
`scope:` line immediately after the counts line, ahead of any findings and the
`RESULT:` line; with no reserved files the line is omitted. It is a disclosure,
not a finding: it never affects the verdict or the exit code. Under `--json`
the same fact is the `reserved_structure_checked` field, which every result
carries regardless of the reserved count — see
[JSON output](#json-output---json-and-the-exit-code-contract).

```text
bundle: /tmp/bundle
concepts: 2, reserved files: 2
scope: reserved-file structure (index.md, log.md) not validated; verdict covers concept files only
RESULT: conformant (OKF 0.2)
```

A non-conformant bundle prints each finding and ends with
`RESULT: NOT conformant (N violation(s))` and a non-zero exit code.

| Flag | Default | Purpose |
|---|---|---|
| `--json` | `false` | Emit the validation result as deterministic JSON (schema `binder.report/v1`) instead of prose. The exit code is identical in both modes. See [JSON output](#json-output---json-and-the-exit-code-contract). |
| `--strict` | `false` | Also gate (exit 1) on trust advisories (malformed trust, actor-convention, or date-shape warnings). Hard conformance violations gate regardless. See [Strict mode](#strict-mode). |

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
| `--group-by-type` | `false` | Append an additive `# Catalog` of all concepts, grouped by type, to the **root** `index.md` (see [The type-grouped catalog](#the-type-grouped-catalog)). |
| `--include-backlinks` | `false` | Annotate each catalog entry with its inbound resolved edges (requires `--group-by-type`). |
| `--include-graph` | `false` | Annotate each catalog entry with its outbound resolved edges (requires `--group-by-type`). |

`convert` already generates these indexes; run `index` to refresh them after
hand-editing concepts in a bundle. The three catalog flags above are shared with
`convert` and behave identically in both commands.

#### The type-grouped catalog

`--group-by-type` **adds** a `# Catalog` section to the bundle-**root** `index.md`
only. It never replaces the per-directory nav and never touches non-root indexes,
so with the flag off the generated output is byte-identical to before.

The catalog lists **every** concept in the bundle, grouped under `## <type>`
subheaders:

- **Types are used verbatim** — no pluralization or humanization (`Pattern` stays
  `Pattern`, not `Patterns`) — and sorted alphabetically. Concepts with an
  empty/unknown type are collected under a final `## (untyped)` group.
- Within a group, concepts are sorted by their bundle-relative path and each is
  linked by its bundle-relative-absolute path, `* [Title](/path/to/concept.md)`,
  because the catalog spans directories.
- The section is deterministic and idempotent: re-running `convert`/`index` on
  identical input yields a byte-identical `index.md`.
- Adding the catalog does not change the bundle's frontmatter layout: the root
  index stays the sole `okf_version` carrier (spec §8/§12).

`--include-backlinks` and `--include-graph` add, under each catalog entry, a
bounded, sorted sub-list of **inbound** (who links to it) and **outbound** (its
dependency links) edges. Both are opt-in and compose with `--group-by-type`;
empty edge sets render nothing. The list is capped at **20 edges per entry**:
when an entry has more, exactly 20 are rendered followed by a single `… and N
more` line, keeping the catalog navigable (the complete edge set is always
available via `binder graph`). Crucially, these annotations derive from the
**same resolved-edge set** `binder graph` builds (resolved links only,
`From=concept → To=target`), via a single shared helper — so the catalog and the
graph can never drift apart.

Each edge entry is suffixed with the **link text in parentheses** — the
relationship label carried by the markdown link that produced the edge. The same
pair of concepts can therefore appear more than once under one entry when the
source document links to it with different text.

Running, from a checkout of the binder repo:

```bash
binder convert testdata/corpus-rich -o build/rich \
  --group-by-type --include-backlinks --include-graph
```

produces this `# Catalog` section at the end of `build/rich/index.md`:

```markdown
# Catalog

## Attested Computation

* [Revenue Calc](/attested/calc.md)
  * link: [Orders Table](/tables/orders.md) (Orders Table)

## BigQuery Table

* [Orders Table](/tables/orders.md)
  * backlink: [Revenue Calc](/attested/calc.md) (Orders Table)
  * backlink: [Setup Guide](/guides/setup.md) (Orders Table)
  * backlink: [Introduction](/intro.md) (Orders Table)
  * backlink: [Introduction](/intro.md) (orders schema)
  * link: [Introduction](/intro.md) (introduction)

## Guide

* [Setup Guide](/guides/setup.md)
  * backlink: [Introduction](/intro.md) (setup)
  * link: [Orders Table](/tables/orders.md) (Orders Table)

## Note

* [Guides Index](/guides/index-note.md)
* [Introduction](/intro.md)
  * backlink: [Orders Table](/tables/orders.md) (introduction)
  * link: [Setup Guide](/guides/setup.md) (setup)
  * link: [Orders Table](/tables/orders.md) (Orders Table)
  * link: [Orders Table](/tables/orders.md) (orders schema)
```

(This corpus has no untyped concepts, so no `## (untyped)` group is emitted.)

### `review`

```text
binder review <bundle> [flags]
```

Summarizes a loaded bundle: concept counts by type, **derived** trust tiers,
stale concepts, Attested Computations, files recovered from unparseable
frontmatter, entrypoints, orphans, and unresolved links. Trust tiers and
staleness are derived on demand, never stored.

| Flag | Default | Purpose |
|---|---|---|
| `--today` | now | Date (`YYYY-MM-DD`) used for the staleness check; honours `SOURCE_DATE_EPOCH`. |
| `--json` | `false` | Emit the review report as deterministic JSON (schema `binder.report/v1`) instead of prose. See [JSON output](#json-output---json-and-the-exit-code-contract). |
| `--strict` | `false` | Gate (exit 1) when any review finding is present (orphans, stale, unresolved, or unparsed-frontmatter recoveries). Entrypoints are advisory and never gate. Without it `review` never gates (exit 0). See [Strict mode](#strict-mode). |
| `--entrypoint` | — | Concept id or path (repeatable) to treat as an **entrypoint**, not an orphan, in addition to the general rule and the recognized root. |

```console
$ SOURCE_DATE_EPOCH=1700000000 binder convert testdata/corpus-lint-entrypoints -o /tmp/ep >/dev/null
$ SOURCE_DATE_EPOCH=1700000000 binder review /tmp/ep
binder review
  bundle: /tmp/ep
  concepts: 4
  by type:
    Guide: 1
    Note: 3
  trust tiers:
    human-reviewed: 0
    machine-confirmed: 0
    unverified: 4
  stale (as of 2023-11-14): 0
  attested computations: 0
  unparsed frontmatter (recovered as body): 0
  entrypoints (no inbound links): 2
    README
    start
  orphans (no inbound or outbound links): 1
    lonely
  unresolved links: 0
```

An **unresolved link** in `review` is a concept reference (a bundle-relative
`.md` target, or a residual `[[wikilink]]`) that names no concept in the bundle.
External URLs, `mailto:`/`tel:`/`ftp:` targets, same-document `#anchors`, and
links to non-concept files (assets, scripts) are **not** concept references and
are never reported.

**Entrypoints vs. orphans (node roles).** A concept with no inbound resolved edge
is classified by whether it links outward:

- **Entrypoint** — no inbound but **has outbound** resolved edges (it indexes into
  the corpus rather than being linked into), *or* it is the recognized root
  entrypoint (`README.md` at the corpus root), *or* it was named via
  `--entrypoint`. A root `README.md` that links out is an entrypoint, **not** an
  orphan — reporting it as an orphan was a false positive (issue #24).
- **Orphan** — a **true** orphan has **no inbound AND no outbound** resolved edges:
  a genuinely disconnected node, reported for you to wire up or accept, never
  removed.

`README.md` is the only name recognized this way, and only at the corpus root.
The name is matched **case-insensitively** — `README.md`, `readme.md`, and
`ReadMe.md` all qualify — but a nested `docs/README.md` is **not** recognized and
stays a true orphan.
An **authored** root `index.md` is a reserved name: it is renamed to
`index-note.md` on conversion so binder can generate its own index (spec §3.1;
see [OKF v0.2 output structure](#okf-v02-output-structure)), so it never matches
by name and is classified on its **edges** like any other concept — one that
links out is still reported as an entrypoint, while one that links nothing is
reported as an orphan.
[#71](https://github.com/ghchinoy/binder/issues/71)

Both classifications are **advisory only** — entrypoints never gate, and the
reclassification never changes an exit code that did not change before. `review`
and `lint` apply the **same rule**, so with each used on its intended input —
`review` on a bundle, `lint` on the source corpus that bundle was converted from
— the two usually report the same entrypoints and the same orphans, though
agreement is **not guaranteed**: the rule is shared, the graph is not, because
conversion sits in between (see [`lint`](#lint) for the mechanism). Handing
`lint` a bundle rather than a source corpus changes the graph wholesale, and the
two then disagree systematically.

### `lint`

```text
binder lint <corpus> [flags]
```

`lint` is a **read-only** health check over a **source markdown corpus** — it
writes nothing. It completes the command triad. Each surface is single-purpose:

| Command | Input | Purpose |
|---|---|---|
| `validate` | emitted bundle | spec §11 **hard conformance** (always gates on non-conformance) |
| `review` | emitted bundle | human **summary** of a finished bundle |
| `lint` | **source** corpus | **pre-conversion** source health; writes nothing |

`lint` earns its own command because it sees the corpus **as authored**, before
`convert` fills in defaults: a missing `title:` or a missing `type:` is invisible
in a bundle because `convert` synthesises both. To guarantee it sees exactly what
`convert` would, `lint` runs the **same converter pipeline** (`convert.Analyze`)
and the **same single resolved-edge definition** — its "broken link" is by
construction the converter's "unresolved link". There is no second resolver and
the codec is untouched.

It reports these checks:

1. **Broken links** — an unresolved internal `.md` reference, a resolved link
   whose target concept is absent, a residual `[[wikilink]]`, or a broken
   `#anchor` (`foo.md#bar` whose target has no `bar` heading, or a same-doc
   `#bar`). The finding `Detail` is the raw target.
2. **Missing titles** — no authored `title:` **and** no first-level (`# `)
   heading (`convert` would humanize the filename; `lint` flags the gap).
3. **Orphans** — a concept with **0 inbound AND 0 outbound** resolved edges: a
   truly disconnected node. A concept with no inbound but **outbound** edges (or
   the recognized root `README.md`, or one named via `--entrypoint`) is an
   **entrypoint** instead, reported separately and never as an orphan (issue #24).
   `review` applies the identical rule, so with each used on its intended input —
   `lint` on a source corpus, `review` on the bundle converted from it — the two
   usually report the same orphans and the same entrypoints, though agreement is
   **not guaranteed**: same rule, different graph. `convert --fm-ref-keys
   related` materializes edges out of a frontmatter key that `lint` has no flag
   to read, so a note `lint` calls an orphan can reach `review` as an
   entrypoint. `lint`'s input is a **source corpus**, and given a bundle it
   misleads rather than refuses (binder never rejects): it re-converts the
   bundle's files including its generated per-directory `index.md` tree, in
   which each index links that directory's concepts plus its subdirectory
   indexes. Each ordinary concept therefore gains an inbound edge from its own
   directory's index, and each directory index gains one from its **parent's**
   index (an index never links itself), leaving the bundle's own **generated**
   root `index.md` as the only file with no inbound edge: the orphan list
   collapses to empty, and that generated index — renamed by the re-conversion,
   so not a recognized root — re-derives as an ordinary concept reported as an
   entrypoint for its outbound edges.
4. **Entrypoints** — no inbound resolved edge but not a true orphan: an outward
   index into the corpus (advisory only; entrypoints never gate).
5. **Stale** — `stale_after` reached as of `--today` (honours `SOURCE_DATE_EPOCH`).
6. **Schema violations** — a missing `type:` (`Detail: "missing type"`), or
   invalid frontmatter recovered under never-reject
   (`Detail: "invalid frontmatter: <err>"`). A recovered file is reported once as
   invalid frontmatter, never also as "missing type".

| Flag | Default | Purpose |
|---|---|---|
| `--today` | now | Date (`YYYY-MM-DD`) used for the staleness check; honours `SOURCE_DATE_EPOCH`. |
| `--json` | `false` | Emit the report as deterministic JSON (schema `binder.report/v1`, `command:"lint"`). See [JSON output](#json-output---json-and-the-exit-code-contract). |
| `--strict` | `false` | Gate (exit 1) when any finding is present. Entrypoints are advisory and never gate. Without it `lint` never gates (exit 0). See [Strict mode](#strict-mode). |
| `--entrypoint` | — | Concept id or path (repeatable) to treat as an **entrypoint**, not an orphan, in addition to the general rule and the recognized root. A trailing `.md` is tolerated **in any case** (`x`, `x.md`, and `x.MD` all name concept `x`), but the concept id itself is matched **case-sensitively** — `X` and `X.md` do **not** match concept `x`. |

All findings are **spec-tolerated advisories**: bare `binder lint` always exits
`0` even with findings (§11 hard conformance stays `validate`'s job over a
bundle). `--strict` gates **exit 1** when any finding is present — the same shared
contract as `convert`/`review`. The report is always emitted before the gate
signals. A missing or non-directory `<corpus>` path is a usage error (exit 2).

```bash
binder lint path/to/corpus            # report; exit 0 even with findings
binder lint path/to/corpus --strict   # exit 1 if any finding (CI)
binder lint path/to/corpus --json     # deterministic envelope, command:"lint"
```

**Anchor slug convention (GitHub-style, single-sourced).** This is the
**heading-anchor** slug (`okf.HeadingSlugs`), which is a deliberately separate
function — with deliberately different rules — from the **title-resolution**
slug that `convert` uses to resolve wikilinks by title (see [Wikilink and ref
resolution](#wikilink-and-ref-resolution)); that one *does* collapse runs, this
one does not. An `#anchor` matches a heading whose slug is produced by applying
this pipeline to the heading's text:

1. **Lowercase, then strip HTML tags** — `## <code>API</code>` slugs to `#api`,
   not `#codeapicode`.
2. **Keep `[a-z0-9_-]` verbatim, drop everything else.** Underscores are word
   characters and round-trip (`## snake_case` → `#snake_case`); hyphens already
   in the heading are left exactly as authored (`## a--b` → `#a--b`). Any other
   character is dropped outright, leaving nothing in its place — `## v0.3.0
   Release` → `#v030-release`.
3. **Turn each space into one hyphen, without collapsing runs.** Two adjacent
   spaces produce `--`, which is how a dropped character between two spaces
   shows up: `## Agent Skill / Plugin` → `#agent-skill--plugin`, *not*
   `#agent-skill-plugin`.
4. **Disambiguate duplicates** with the suffixes `-1`, `-2`, … in document
   order, counted per document: three `## Notes` headings yield `#notes`,
   `#notes-1`, and `#notes-2`.

Slugging is code-region-aware: a `# heading` inside a fenced code block is not a
heading and contributes no anchor.

Letters and digits are **ASCII-only** — `[a-z0-9]` rather than the full Unicode
word class github-slugger uses — so a non-ASCII heading slugs differently than it
would on GitHub, and the anchor GitHub would accept is reported as a broken link.
`## Café` slugs to `#caf`, so `#café` is a false positive; `## 配置` contributes no
slug characters at all (`## 配置 A` slugs to `#-a`). The divergence is deliberate
and is disclosed in `okf.HeadingSlugs` itself; it is tracked as
[#85](https://github.com/ghchinoy/binder/issues/85). Read "GitHub-style" as
holding for ASCII headings.

**What is load-bearing here is the single source, not the character set.** The
convention has exactly one unit-tested implementation, `okf.HeadingSlugs` (a
design commitment from [#8](https://github.com/ghchinoy/binder/issues/8)), and
every part of binder that has to decide what `#bar` matches resolves through it —
today that is `lint`'s anchor check — so binder cannot hold two answers at once.
The character rules above describe what that one implementation does in v0.3.0;
they are a fact about this release rather than a frozen promise, and v0.3.0 itself
changed two of them.

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

### `infer`

```text
binder infer <corpus> [flags]
```

Inspects a source markdown corpus and proposes a `--type-map` string (e.g.
`"docs=Guide,subsystems=Subsystem"`) and structured JSON report.

`infer` evaluates a tiered signal ladder from cheapest/deterministic to richest:
1. **Tier 1 (Folder):** Humanizes and singularizes directory names (`subsystems/` → `Subsystem`, `runbooks/` → `Runbook`, `proposals/` → `Proposal`).
2. **Tier 2 (Patterns):** Inspects filename and heading conventions (`*-spec.md` → `Specification`, `troubleshooting*` → `Runbook`, `adr-*` → `Decision`).
3. **Tier 3 (Frontmatter):** Recognizes authored frontmatter type majorities and key hints (`goal:` → `Proposal`, `runtime:` → `Attested Computation`).
4. **Tier 4 (Gemini):** Opt-in semantic inference (`--gemini`) sending directory names and sample titles to Gemini, supporting both API key and Google Cloud Vertex AI.

`infer` is strictly **proposal-only** and writes nothing to disk. Review the
proposal, then apply it with `convert` or `enrich`:

```bash
# Deterministic proposal:
binder infer path/to/corpus

# With Gemini semantic inference:
binder infer path/to/corpus --gemini

# Apply proposal to conversion:
binder convert path/to/corpus -o path/to/bundle --type-map "$(binder infer path/to/corpus)"

# Or stamp types into the source tree in place:
binder enrich path/to/corpus --type-map "$(binder infer path/to/corpus)"
```

| Flag | Default | Purpose |
|---|---|---|
| `--default-type` | `Note` (or config `default_type`) | Fallback concept type when none is inferred or mapped. |
| `--gemini` | `false` | Enable Gemini semantic inference tier. |
| `--gemini-model` | `gemini-3.5-flash-lite` | Gemini model name for semantic inference. |
| `--location` | `global` | Google Cloud location for Vertex AI. |
| `--project` | — | Google Cloud project for Vertex AI (defaults to ADC / `GOOGLE_CLOUD_PROJECT`). |
| `--backend` | `auto` | Gemini auth backend: `auto`, `api`, or `vertex`. |
| `--gemini-required` | `false` | Fail on Gemini inference error instead of degrading to deterministic tiers. |
| `--json` | `false` | Emit the report as deterministic JSON (schema `binder.report/v1`). |
| `--strict` | `false` | Gate (exit 1) if any warning or inference failure occurs. |

#### Prose output: a bare `--type-map` string

`infer` is the one command whose default output is **not** a report. It writes a
single line to stdout — the proposed `--type-map` value and nothing else, no
header, no counts, no trailing prose:

```console
$ binder infer testdata/corpus-rich
attested=Attested Computation,guides=Guide,tables=BigQuery Table
```

That is precisely why the `"$(binder infer …)"` substitution above works: the
whole of stdout is a valid `--type-map` argument. Diagnostics go to **stderr**,
so they never contaminate the substitution. Use `--json` when you want the
per-directory rationale, the `warnings` array, and the tier attribution.

That holds on a corpus with no directory signal too, which is the case worth
checking before you script the idiom. There stdout carries **nothing at all**,
the note `No directory type mappings inferred (use --default-type: Note)` goes to
stderr, and the exit code is still `0`, so the substitution expands to an empty
`--type-map`. Both `convert` and `enrich` accept that as a map that matches
nothing and go on to do the rest of their work exactly as if the flag had been
omitted — on `testdata/corpus-basic`, the bundle `convert --type-map ""` emits is
byte-identical to the one it emits with no `--type-map` at all (`concepts: 4`,
`links: 5 (resolved 4, unresolved 1)`). The idiom is therefore safe to write once
and run against **every** corpus, not only the ones with a directory signal to
find.

**For scripting, read `result.type_map` out of `--json`.** It is the same string
the prose form prints, on a stable, parseable path, and it is `""` when no
directory mapping is inferred:

```console
$ binder infer testdata/corpus-rich --json | jq -r '.result.type_map'
attested=Attested Computation,guides=Guide,tables=BigQuery Table
$ binder infer testdata/corpus-lint-entrypoints --json | jq -r '.result.type_map'

```

That extraction substitutes directly into `--type-map`:

```bash
binder convert path/to/corpus -o path/to/bundle \
  --type-map "$(binder infer path/to/corpus --json | jq -r '.result.type_map')"
```

#### The Gemini tier: degrade by default, `--gemini-required` to fail

The opt-in Gemini tier (`--gemini`) is the only part of `infer` that can fail for
reasons outside the corpus. Two flags decide what a failure means, and they map
onto three different exit codes:

| Invocation | On a Gemini failure |
|---|---|
| `binder infer <src> --gemini` | **Degrades** to the deterministic tiers, still prints the proposal, exits **0**. In prose mode the degradation is **silent** — the reason is recorded only in the `--json` `warnings` array. |
| `binder infer <src> --gemini --strict` | Degrades and still prints the proposal on stdout, then gates: `binder: infer encountered 1 warning(s) (--strict)` on stderr, exit **1**. |
| `binder infer <src> --gemini --gemini-required` | Does **not** degrade. Nothing on stdout; the error on stderr; exit **3** — the io/external-failure code, not `1` or `2`. |

The Gemini tier has **two** distinct failure points and the message names which
one you hit. Building the client fails first (no credentials, no project, an
unusable backend) — that is the common case on a fresh machine and in CI, and it
is reported as `binder: gemini client initialization: <error>`. If the client is
built and the model call itself fails (a bad model name, a quota or network
error), it is reported as `binder: gemini semantic inference: <error>`. Both exit
**3** under `--gemini-required`; when degrading, they surface in the `--json`
`warnings` array as `gemini tier disabled: <error>` and `gemini inference
warning: <error>` respectively. Grep for the pair, not for one string.

So a prose `--gemini` run that quietly returns a deterministic-looking proposal
may in fact be a degraded one. If it matters whether the model was actually
consulted, use `--json` and inspect `warnings` (and `mappings[].source`), or make
the failure loud with `--gemini-required` / `--strict`.

`--gemini-required` without `--gemini` has no effect: the Gemini tier is never
attempted, so there is nothing to require, and the run is an ordinary
deterministic exit `0`.

Unlike every other binder output, a **Gemini-tier proposal is not deterministic
even with `SOURCE_DATE_EPOCH` pinned** — the same corpus can yield different
`suggested_type` values on successive runs. The deterministic tiers (1–3) are
reproducible; tier 4 is a model call. Do not pin CI on `--gemini` output.

### `config`

```text
binder config [subcommand] [flags]
```

Manages persistent configuration and prints the **resolved effective configuration** —
the value binder uses for each key and which precedence layer supplied it
(`flag`, `env`, `file`, or `default`).

Configuration is resolved once, at startup, with a strict precedence:

```text
flag  >  environment variable  >  config file  >  built-in default
```

- **Config files:**
  - Local repository config: `./.binder.yaml`
  - Global user config: `$XDG_CONFIG_HOME/binder/config.yaml` (fallback `$HOME/.config/binder/config.yaml`)

  Exactly **one** file is loaded — the two are not merged. If `./.binder.yaml`
  exists it is the config file, and the global file is ignored in its entirety
  (not just for the keys the local file sets). `binder config` prints the file
  actually in effect on its `config file:` line, so that line is the way to tell
  which one you are editing.
- **Environment variables** use the `BINDER_` prefix, e.g. `BINDER_VERIFIED_BY`, `BINDER_DEFAULT_TYPE`, `BINDER_GEMINI_PROJECT`.

#### Configuration Keys

| Key (snake or dotted) | Env | Default | Purpose |
|---|---|---|---|
| `default_type` | `BINDER_DEFAULT_TYPE` | `Note` | Concept type applied when none is present or mapped. |
| `verified_by` | `BINDER_VERIFIED_BY` | — | Default actor appended as a `verified` stamp by `convert` / `enrich`. Validated against actor grammar. |
| `gemini_model` (`gemini.model`) | `BINDER_GEMINI_MODEL` | `gemini-3.5-flash-lite` | Default Gemini model for `binder infer --gemini`. |
| `gemini_location` (`gemini.location`) | `BINDER_GEMINI_LOCATION` | `global` | Default Google Cloud location for Vertex AI in `binder infer --gemini`. |
| `gemini_project` (`gemini.project`) | `BINDER_GEMINI_PROJECT` | — | Default Google Cloud project for Vertex AI in `binder infer --gemini`. |
| `gemini_backend` (`gemini.backend`) | `BINDER_GEMINI_BACKEND` | `auto` | Default Gemini backend (`auto`, `api`, or `vertex`) in `binder infer --gemini`. |

#### Subcommands

- **`binder config`** (or **`binder config list`**) — prints all resolved settings and their attribution source (`flag`, `env`, `file`, or `default`).
- **`binder config get <key>`** — outputs the single resolved value for `<key>`.
- **`binder config set <key> <value> [--global]`** — sets a persistent value. By default writes to `./.binder.yaml`; pass `--global` / `-g` to write to user config. Uses isolated file mutation to touch only the specified key.
- **`binder config unset <key> [--global]`** — removes a key from the configuration file so it reverts to its environment variable or built-in default.

```bash
# Set GCP project locally in ./.binder.yaml:
binder config set gemini.project my-gcp-project

# Set default model globally in the user config file:
binder config set --global gemini.model gemini-3.5-flash-lite

# Inspect a single resolved value (see the transcript below for its output):
binder config get gemini.project

# Revert setting:
binder config unset gemini.project
```

| Flag | Default | Purpose |
|---|---|---|
| `--json` | `false` | Emit the configuration report as deterministic JSON (schema `binder.config/v1`). |
| `-g`, `--global` | `false` | (`set`/`unset` only) Target the global user config — `$XDG_CONFIG_HOME/binder/config.yaml`, falling back to `$HOME/.config/binder/config.yaml` — instead of `./.binder.yaml`. |

#### Key spellings: `gemini.project` and `gemini_project`

Every Gemini key has two accepted spellings — dotted (`gemini.project`) and
snake_case (`gemini_project`) — and they are the **same key**, not two keys.
Whichever you type, the config file is always written in snake_case and every
report echoes the snake_case name:

```console
$ binder config set gemini.project my-gcp-project
Set gemini_project = "my-gcp-project" in .binder.yaml

$ cat .binder.yaml
gemini_project: my-gcp-project

$ binder config get gemini_project
my-gcp-project
```

So a value written with one spelling reads back with the other. The dotted form
is a **key spelling on the command line only** — it is not YAML nesting. A
hand-edited file containing

```yaml
gemini:
  project: nested-project
```

is parsed without error but the value is **not picked up**: `binder config` still
reports `gemini_project: "" (source: default)`. Only the flat snake_case keys are
read, so prefer `binder config set` over editing the file by hand.

#### `config` prose output

`binder config` (equivalently `binder config list`) prints a header line, the
config file in effect, and one line per key with its value and attribution
source (`flag`, `env`, `file`, or `default`). With nothing configured anywhere:

```console
$ binder config
binder config
  config file: (none; using defaults)
  default_type: "Note" (source: default)
  verified_by: "" (source: default)
  gemini_model: "gemini-3.5-flash-lite" (source: default)
  gemini_location: "global" (source: default)
  gemini_project: "" (source: default)
  gemini_backend: "auto" (source: default)
```

…and after `binder config set gemini.project my-gcp-project`, with
`BINDER_DEFAULT_TYPE=Guide` exported:

```console
$ BINDER_DEFAULT_TYPE=Guide binder config
binder config
  config file: .binder.yaml
  default_type: "Guide" (source: env)
  verified_by: "" (source: default)
  gemini_model: "gemini-3.5-flash-lite" (source: default)
  gemini_location: "global" (source: default)
  gemini_project: "my-gcp-project" (source: file)
  gemini_backend: "auto" (source: default)
```

The mutating subcommands each print a single confirmation line naming the file
they touched, and `get` prints the bare value with no decoration — which is what
makes it safe in a shell substitution:

```console
$ binder config set gemini.project my-gcp-project
Set gemini_project = "my-gcp-project" in .binder.yaml

$ binder config get gemini.project
my-gcp-project

$ binder config unset gemini.project
Unset gemini_project in .binder.yaml (reverted to default)
```

Unsetting a key that is not set is a **no-op, not an error** — it prints
`Key <name> is not set in .binder.yaml` and exits `0`. Removing the last key
from a config file removes the file itself rather than leaving an empty one.

#### `config --json` — the `binder.config/v1` payloads

`config` uses the envelope shape described under
[JSON output](#json-output---json-and-the-exit-code-contract) but with
`schema: "binder.config/v1"`, and `command` distinguishes the four subcommands.
Each `result` is a different object.

**`binder config --json` / `binder config list --json`** — `command: "config"`.
`config_file` is the file in effect (`""` when none), and `values` is a **map**,
so its keys are sorted alphabetically (unlike the struct-backed envelope, which
is field-ordered):

```json
{
  "binder": "binder/0.3.0",
  "command": "config",
  "schema": "binder.config/v1",
  "result": {
    "config_file": ".binder.yaml",
    "values": {
      "default_type": {
        "value": "Note",
        "source": "default"
      },
      "gemini_backend": {
        "value": "auto",
        "source": "default"
      },
      "gemini_location": {
        "value": "global",
        "source": "default"
      },
      "gemini_model": {
        "value": "gemini-3.5-flash-lite",
        "source": "default"
      },
      "gemini_project": {
        "value": "my-gcp-project",
        "source": "file"
      },
      "verified_by": {
        "value": "",
        "source": "default"
      }
    }
  }
}
```

Every key is always present, `value` is always a string (`""` when unset), and
`source` ∈ `flag` | `env` | `file` | `default`.

**`binder config get <key> --json`** — `command: "config get"`; `result` carries
the resolved `key` (always the snake_case spelling), its `source`, and its
`value`:

```json
{
  "binder": "binder/0.3.0",
  "command": "config get",
  "schema": "binder.config/v1",
  "result": {
    "key": "gemini_project",
    "source": "file",
    "value": "my-gcp-project"
  }
}
```

**`binder config set <key> <value> --json`** — `command: "config set"`; `result`
names the `file` written, the `key`, the `value`, and a `status`:

```json
{
  "binder": "binder/0.3.0",
  "command": "config set",
  "schema": "binder.config/v1",
  "result": {
    "file": ".binder.yaml",
    "key": "default_type",
    "status": "updated",
    "value": "Guide"
  }
}
```

**`binder config unset <key> --json`** — `command: "config unset"`; `result` is
the same minus `value`. `status` is `removed` when the key was present and
`noop` when it was not (both exit `0`):

```json
{
  "binder": "binder/0.3.0",
  "command": "config unset",
  "schema": "binder.config/v1",
  "result": {
    "file": ".binder.yaml",
    "key": "default_type",
    "status": "removed"
  }
}
```

#### `config` error semantics

`config` fails fast and it fails as a **usage error** (exit `2`), never as a
findings code. Both failure classes print a plain `binder: …` line on stderr and
emit **no envelope** — even under `--json`, so a consumer must branch on the exit
code, not on parsing stdout.

An unrecognized key, on any of `get`/`set`/`unset`, is rejected with the valid
set listed:

```console
$ binder config get nope; echo $?
binder: unknown configuration key "nope" (valid keys: default_type, verified_by, gemini_model, gemini_location, gemini_project, gemini_backend)
2
```

`verified_by` is validated against the actor grammar at **write** time, so an
invalid actor can never be persisted:

```console
$ binder config set verified_by "bogus actor!"; echo $?
binder: invalid actor "bogus actor!"; valid forms: human:<id>, process:<id>, team:<id>, or <producer>/<version> (e.g. binder/0.3.0)
2
```

The same validation runs again at config-**load**, so a `verified_by` that was
hand-edited into the file is caught on the next run rather than silently stamped
into a bundle.

## JSON output (`--json`) and the exit-code contract

`convert`, `enrich`, `validate`, `review`, `lint`, `infer`, `graph`, and `config`
(including `config list`, `config get`, `config set`, and `config unset`) accept
`--json` for
scripting, agents, and CI. Prose is the default and is **byte-unchanged** when
`--json` is absent — `--json` is a presentation layer over the already-computed
report, it changes no behavior and fabricates no fields or trust data.

### The envelope (schema `binder.report/v1`)

`convert`, `enrich`, `validate`, `review`, `lint`, and `infer` wrap their existing
report struct in a thin envelope that carries the provenance and schema tag a
consumer needs to parse it safely:

```json
{
  "binder": "binder/0.3.0",
  "command": "convert",
  "schema": "binder.report/v1",
  "result": { }
}
```

| Envelope field | Meaning |
|---|---|
| `binder` | The producing binder version, `binder/<version>` (same string as `--version`). |
| `command` | `convert` \| `enrich` \| `validate` \| `review` \| `lint` \| `infer`. |
| `schema` | The report contract identifier. Bumped **only** on a breaking change to a payload's shape or field names, so a consumer can branch on it. |
| `result` | The command's report object (see per-command fields below). |

`config` uses the **same envelope shape with a different contract**: `schema` is
`binder.config/v1` and `command` is `config` (for both bare `config` and
`config list`), `config get`, `config set`, or `config unset`. See
[`config`](#config).

`graph` is the deliberate exception — see [graph JSON](#graph-json--a-raw-export-not-the-envelope).

### Determinism

JSON output is deterministic: fixed 2-space indentation, HTML escaping **off**
(so `<`, `>`, `&` are literal), a stable field order, and a trailing newline.
Struct-backed objects — including the envelope itself — emit in
field-declaration order (`binder`, `command`, `schema`, `result`), which is
deterministic but not alphabetical; map-valued objects such as `by_type`,
`tiers`, and `config`'s `values` do have their keys sorted alphabetically. All
timestamps derive from `SOURCE_DATE_EPOCH` (via `--today` for the read
commands), so two runs on the same input are byte-identical:

```bash
SOURCE_DATE_EPOCH=1700000000 binder review bundle --json > a.json
SOURCE_DATE_EPOCH=1700000000 binder review bundle --json > b.json
diff a.json b.json
```

The two files are byte-identical, so that `diff` prints nothing and exits `0`.

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
| `status_notes` | array of string | Status-vocabulary messages — canonicalization rewrites and non-conformance warnings — sorted. Always present; `[]` on a conformant run. See [Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status). |

Each `concepts[]` object: `rel_path`, `type`, `title`, `num_links`,
`num_unresolved`. Each `unresolved[]` object: `from` (source concept rel path),
`raw_target` (target exactly as written), `text` (link text / relationship label).

### `enrich --json` — `result` fields

| Field | Type | Meaning |
|---|---|---|
| `src` | string | Source corpus path. |
| `dry_run` | bool | Whether this was a `--dry-run`. |
| `num_files` | int | Non-reserved files considered. |
| `num_enriched` | int | Files written (or `would-enrich` under `--dry-run`). |
| `num_unchanged` | int | Files needing no key — not written. |
| `num_skipped` | int | Files skipped (unparseable frontmatter), never mutated. |
| `files` | array | One object per file (see below), sorted by path. |
| `warnings` | array of string | Preserve-or-advise notices (`path: message`). |
| `status_notes` | array of string | Status-vocabulary messages — canonicalization rewrites and non-conformance warnings — sorted. Always present; `[]` on a conformant run. See [Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status). |

Each `files[]` object: `path` (source-relative), `status` ∈
`enriched|unchanged|would-enrich|skipped`, `added` (sorted keys injected),
`overwritten` (sorted keys **refreshed in place** under
[`--overwrite-keys`](#opt-in-refresh---overwrite-keys)), and `reason` (for
`skipped`, e.g. `unparseable frontmatter: <err>`). A key is reported under
exactly one of `added` (it was absent) or `overwritten` (it was present and was
replaced).

**Emptiness convention — it is not uniform, so branch on presence.** Inside
`files[]`, `added`, `overwritten`, and `reason` are all omitted when empty, so a
file that needed nothing serializes as just `{"path": …, "status": "unchanged"}`.
The top-level arrays behave the other way: `warnings` and `status_notes` are
**always present** and serialize as `[]` when empty (as do `entrypoints` in
`review --json` and `lint --json`). Treat an absent `files[].overwritten` as
"nothing was overwritten", and do not expect the same rule at both levels:

A second `enrich` pass over a tree whose `status` was already set by a first
pass, this time refreshing it:

```bash
binder enrich src --overwrite-keys status --status-map "archive=stable" --json
```

```json
{
  "binder": "binder/0.3.0",
  "command": "enrich",
  "schema": "binder.report/v1",
  "result": {
    "src": "src",
    "dry_run": false,
    "num_files": 2,
    "num_enriched": 1,
    "num_unchanged": 1,
    "num_skipped": 0,
    "files": [
      {
        "path": "README.md",
        "status": "unchanged"
      },
      {
        "path": "archive/a.md",
        "status": "enriched",
        "overwritten": [
          "status"
        ]
      }
    ],
    "warnings": [],
    "status_notes": []
  }
}
```

### `validate --json` — `result` fields

| Field | Type | Meaning |
|---|---|---|
| `root` | string | Bundle path. |
| `num_concepts` | int | Non-reserved concepts checked. |
| `num_reserved` | int | Reserved files (`index.md`/`log.md`) counted, not required to carry a `type`, and not structurally examined. |
| `findings` | array | Each `{ concept_id, severity, message }`. |
| `reserved_structure_checked` | bool | Whether the structure of the reserved files (spec §8/§9) was examined. Always `false`: reserved-file structure is outside `validate`'s scope. |

`severity` is `error` (a hard §11 violation — gates the exit code) or `advisory`
(trust/lifecycle well-formedness — reported, never gates). See the
[exit-code contract](#exit-code-contract).

`reserved_structure_checked` is **unconditional** — every `validate --json`
result carries it, including one where `num_reserved` is `0`. The prose output
states the same fact as a `scope:` line, but only when `num_reserved` is
greater than zero.

### `review --json` — `result` fields

| Field | Type | Meaning |
|---|---|---|
| `root` | string | Bundle path. |
| `today` | string | The `YYYY-MM-DD` staleness date used. |
| `num_concepts` | int | Concept count. |
| `by_type` | object | `{ "<type>": count }` (types with no value show as `(none)`). |
| `tiers` | object | `{ "<tier>": count }` over `unverified` / `machine-confirmed` / `human-reviewed`. |
| `orphans` | array of string | Concept IDs with **no inbound AND no outbound** resolved edges (true orphans). |
| `entrypoints` | array of string | Concept IDs with no inbound edge that are not orphans: outbound edges, the recognized root (`README.md`), or designated via `--entrypoint` (issue #24). Advisory; never gates. |
| `stale` | array of string | Concept IDs stale as of `today`. |
| `attested` | array of string | Attested-Computation concept IDs. |
| `unresolved` | array | Broken concept references, each `{ from, raw_target, text }`. |
| `unparsed_frontmatter` | array of string | Concept IDs recovered from unparseable frontmatter. |
| `concepts` | array | Per-concept view: `{ id, type, tier, stale, attested, orphan, entrypoint }`. |

`by_type` and `tiers` are JSON objects with sorted keys; all list fields are
`[]` when empty.

### `lint --json` — `result` fields

| Field | Type | Meaning |
|---|---|---|
| `src` | string | Source corpus path. |
| `num_concepts` | int | Concepts analysed. |
| `broken_links` | array | Each `{ concept, detail }`; `detail` is the raw target. |
| `missing_titles` | array of string | Concept IDs with no authored title and no `# H1`. |
| `orphans` | array of string | Concept IDs with 0 inbound **and** 0 outbound resolved edges (true orphans). |
| `entrypoints` | array of string | Concept IDs with 0 inbound edge that are not orphans: outbound edges, the recognized root (`README.md`), or designated via `--entrypoint` (issue #24). Advisory; never gates. |
| `stale` | array of string | Concept IDs stale as of `today`. |
| `schema_violations` | array | Each `{ concept, detail }` — `"missing type"` or `"invalid frontmatter: <err>"`. |

All list fields are `[]` when empty. `lint`'s exit code follows the shared
contract: `0` by default (findings are advisories), `1` under `--strict` when any
finding is present. See the [exit-code contract](#exit-code-contract).

### `infer --json` — `result` fields

Like every other `--json` command except `graph`, `infer` wraps its payload in the
standard [`binder.report/v1` envelope](#the-envelope-schema-binderreportv1) with
`"command": "infer"`; the object below is the `result` value, shown on its own.
*Field shape only, with `path/to/corpus` and the `subsystems/` directory standing
in for a real corpus — not a capture of any particular run:*

```json
{
  "src": "path/to/corpus",
  "type_map": "docs=Guide,subsystems=Subsystem",
  "default_type": "Note",
  "mappings": [
    {
      "dir": "subsystems",
      "suggested_type": "Subsystem",
      "source": "folder",
      "rationale": "well-known directory name \"subsystems\"",
      "sample_files": ["subsystems/audio.md"]
    }
  ],
  "warnings": []
}
```

- **`type_map`** (`string`) — comma-separated `dir=Type` string formatted for `--type-map`.
- **`mappings`** (`[]object`) — list of per-directory proposals with `dir`, `suggested_type`, `source` (`folder` | `pattern` | `frontmatter` | `gemini`), `rationale`, and optional `sample_files`, `model`, `backend`.
- **`warnings`** (`[]string`) — always present, `[]` on a clean run. A degraded
  Gemini tier lands here as `gemini tier disabled: <error>` (the client could not
  be built — the common no-credentials case) or `gemini inference warning:
  <error>` (the client was built but the model call failed); in prose mode this
  is the **only** place the degradation is visible.

`model` and `backend` appear **only** on entries the Gemini tier produced
(`"source": "gemini"`) — they name the model actually called and the auth
backend that was resolved for the call.

*A single `mappings[]` entry captured from a live tier-4 run of `binder infer
testdata/corpus-rich --gemini --json` against a Vertex-enabled Google Cloud
project. **Unlike every other transcript in this guide, you cannot reproduce this
block at all without working Google Cloud ADC (or a Gemini API key)** — without
them the tier degrades, `result.warnings[0]` carries `gemini tier disabled: …`,
no `"source": "gemini"` entry is emitted, and the run still exits `0`. **Exactly
one field varies: `suggested_type`.** It is model output, so it is not
deterministic — successive runs against this same `attested/` directory returned
both `Reference` (shown here) and `Specification`. Every other field repeats:
`dir` and `sample_files` are corpus facts, and `rationale`, `model`, and
`backend` are fixed by the code and the `--gemini-model` default:*

```json
{
  "dir": "attested",
  "suggested_type": "Reference",
  "source": "gemini",
  "rationale": "suggested by Gemini semantic analysis",
  "sample_files": [
    "attested/calc.md"
  ],
  "model": "gemini-3.5-flash-lite",
  "backend": "vertex"
}
```

`suggested_type` in a tier-4 entry is model output and will not necessarily
repeat run to run; the deterministic tiers will.

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
| `1` | findings-present | A gating condition: `validate` spec §11 non-conformance (unparseable frontmatter or a missing/empty `type`), or — under [`--strict`](#strict-mode) — any advisory promoted to a gating finding. | `validate`, `review`, `lint`, `convert`, `enrich`, `infer` (with `--strict`) |
| `2` | usage-error | Bad flags/args — an unknown subcommand or flag, a missing/extra argument, a conflicting `--json`/`--format`, an unparseable `--today`, a malformed `--status-map`, or an `--overwrite-keys` list naming a trust-provenance key. Also: a missing source path for `lint`/`enrich`/`infer` (see below). | any command |
| `3` | io-error | Cannot read the corpus/bundle, a write failure, an external-service failure, or an internal error. Includes a missing path for `convert`/`validate`/`review`/`index`/`graph` (see below), and a failing Gemini tier under [`infer --gemini-required`](#the-gemini-tier-degrade-by-default---gemini-required-to-fail). | any command |

#### The missing-path asymmetry (`2` vs `3`)

A nonexistent path is **not** classified the same way by every command, and the
split is deliberate. `lint`, `enrich`, and `infer` treat it as **argument
validation** and exit `2`; `convert`, `validate`, `review`, `index`, and `graph`
treat it as an **I/O failure** and exit `3`. Script accordingly — do not assume a
single code for "path not found":

```console
$ binder lint     /no/such/path; echo $?
binder: corpus "/no/such/path" is not a readable directory
2
$ binder enrich   /no/such/path; echo $?
binder: source "/no/such/path" is not a readable directory
2
$ binder infer    /no/such/path; echo $?
binder: corpus "/no/such/path" is not a readable directory
2
$ binder convert  /no/such/path -o /tmp/x; echo $?
binder: convert: source "/no/such/path": stat /no/such/path: no such file or directory
3
$ binder validate /no/such/path; echo $?
binder: stat /no/such/path: no such file or directory
3
$ binder review   /no/such/path; echo $?
binder: stat /no/such/path: no such file or directory
3
$ binder index    /no/such/path; echo $?
binder: stat /no/such/path: no such file or directory
3
$ binder graph    /no/such/path; echo $?
binder: stat /no/such/path: no such file or directory
3
```

Every `binder: …` line above is on **stderr** — none of these commands writes
anything to stdout, so the bare number is the `echo $?`. The wording tracks the
classification: the exit-`2` commands report a failed check against the
argument, the exit-`3` ones surface the underlying `stat` error.

#### Other observed exit-`2` cases

```console
$ binder bogus; echo $?                                        # unknown subcommand
binder: unknown command "bogus" for "binder"
2
$ binder graph build/bundle --format bogus; echo $?            # unknown --format
binder: unknown graph format "bogus" (want dot|json|graphml|html)
2
$ binder graph build/bundle --today notadate; echo $?          # unparseable --today
binder: --today "notadate" is not a valid date (expected YYYY-MM-DD)
2
$ binder graph build/bundle --json --format dot; echo $?       # conflicting output flags
binder: --json conflicts with --format dot; --json selects --format json
2
$ binder enrich docs/ --overwrite-keys sources; echo $?        # refused trust key
binder: --overwrite-keys: refusing to overwrite trust-provenance key "sources" (protected: attester, computation, executor, generated, parameters, runtime, sources, usage_window, verified, verified_by); these can carry human attestations and overwriting them would violate the never-fabricate-trust invariant
2
```

The three `graph` cases fail on flag validation before the bundle path is
touched, which is why they exit `2` even though `build/bundle` does not exist.

The refused-trust-key case is **fail-fast and total**: naming any of the
protected trust-provenance keys (`attester`, `computation`, `executor`,
`generated`, `parameters`, `runtime`, `sources`, `usage_window`, `verified`,
`verified_by`) in `--overwrite-keys` aborts the whole run before any file is
touched — nothing is written, not even for files the list would not have
affected. See [Opt-in refresh: `--overwrite-keys`](#opt-in-refresh---overwrite-keys).

**Never-reject is preserved by default.** Broken links, orphans, staleness,
recovered frontmatter, and missing optional trust are all **advisories** — by
default they are reported (in prose and JSON) and exit `0`. The only default
non-zero for a well-formed run is `validate`'s spec §11 hard non-conformance,
which is a genuine violation, not an advisory. Opting into [`--strict`](#strict-mode)
promotes those advisories into gating findings (exit `1`) **only for the run that
requests it** — the default posture is unchanged.

> **Compatibility note.** This refines the previous behavior (where non-IO
> failures collapsed to exit `1`): `0` still means success, and failures are now
> more specific (`1`/`2`/`3`). No consumer that only checked "zero vs non-zero"
> is affected.

### Strict mode

By default binder is **never-reject**: advisories are reported but exit `0`.
`--strict` opts a single run into promoting advisories to gating findings
(exit `1`), so CI can fail the build on conditions that are informational
locally. It changes only the exit code — the report (prose or JSON) is
byte-identical with and without the flag — and it never affects any other run.

The per-command contract:

| Command | `--strict` gates (exit 1) on | Always gates, regardless |
|---|---|---|
| `validate` | trust advisories (malformed trust, actor-convention, date-shape warnings) | spec §11 hard non-conformance (unparseable frontmatter, missing/empty `type`) |
| `review` | any review finding: orphans, stale concepts, unresolved links, unparsed-frontmatter recoveries | — |
| `lint` | any lint finding: broken links, missing titles, orphans, stale, schema violations | — |
| `convert` | unresolved links, recovery warnings, or non-conformant `--status-map` status values | — (a clean run is exit `0` even under `--strict`) |
| `enrich` | skipped (unparseable-frontmatter) files, preserve-or-advise findings, or non-conformant `--status-map` status values | — (a clean run is exit `0` even under `--strict`) |
| `infer` | any warning or inference failure (in practice, Gemini-tier warnings — the deterministic tiers do not warn) | — |

The `--status-map` vocabulary gate is the one that fires **before** anything is
written: `convert --strict` exits `1` without creating the output directory, and
`enrich --strict` exits `1` without touching a single source file. Adding
`--canonicalize-status` resolves a known alias and the run returns to exit `0`.
See [Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status).

`--strict` is available on `validate`, `review`, `lint`, `convert`, `enrich`, and
`infer`. A clean run stays exit `0` even with `--strict` set, so the flag is safe
to leave on permanently in CI. `index`, `graph`, and `config` have no advisory
surface and do not take it.

```bash
# Fail CI on any unresolved link or recovered file, not just spec violations
SOURCE_DATE_EPOCH=1700000000 binder convert docs/ -o build/bundle --strict
binder validate build/bundle --strict
binder review   build/bundle --strict
# Fail CI on source-corpus health before conversion
binder lint     docs/ --strict
```

## Discovery surface (`--version` / `--help`)

An agent or script can enumerate binder's surface without parsing prose reports:

- **`binder --version`** prints `binder/<version>` (e.g. `binder/0.3.0`) — the
  exact string that appears in the JSON envelope's `binder` field. (`--version`
  is a root flag; passing it to a subcommand is a usage error.)
- **`binder --help`** lists every command; **`binder <cmd> --help`** prints that
  command's `Usage:` line and a `Flags:` section (name, shorthand, default,
  description) — a stable, documented shape sufficient to discover every flag,
  including `--json`.
- **`binder completion <bash|zsh|fish|powershell>`** writes a shell-completion
  script to stdout. It is the stock generator supplied by the CLI framework, with
  no binder-specific behaviour and no `--json`; it is not a report-producing
  command and emits no envelope:

  ```console
  $ binder completion bash | head -1
  # bash completion V2 for binder                               -*- shell-script -*-
  ```

  Install it the way your shell expects — e.g.
  `binder completion bash > /etc/bash_completion.d/binder`, or
  `binder completion zsh > "${fpath[1]}/_binder"`. Note that `binder completion`
  with a missing or unrecognized shell prints the sub-command list and exits `0`
  rather than failing as a usage error, so check the output rather than the exit
  code when scripting it.

A structured tool/flag catalog (a machine-readable command manifest) is not part
of this surface; it is delivered natively by the [MCP server mode](#mcp-server-binder-mcp)
(`binder mcp`), whose tools advertise typed input schemas.

## MCP server (`binder mcp`)

`binder mcp` runs binder as a stdio [Model Context Protocol](https://modelcontextprotocol.io)
server, exposing binder's **additive** verbs as MCP **tools** to an MCP-capable
harness (Claude Code, Cursor, Zed). Each tool returns the **same** deterministic
[`binder.report/v1` envelope](#the-envelope-schema-binderreportv1) as the
corresponding `binder <cmd> --json`: the handlers call the same internal
functions and the same JSON encoder the CLI uses, so the payloads are
byte-identical and can never drift from `--json`.

The server registers **seven** tools: `convert`, `validate`, `review`, `lint`,
`graph`, `list_graphs`, and `query_graph`. (`enrich` is excluded because it
mutates the source tree — see [Behavior and invariants](#behavior-and-invariants).)

`binder mcp` is a **transport, not a report-producing command** — it takes no
positional args and has no `--json` flag (its *outputs* are the structured tool
payloads). It serves over stdio until the client disconnects.

> **Parity is a claim about the five verbs that have a CLI counterpart**, and
> **output-routing flags are its deliberate exception.** For `convert`,
> `validate`, `review`, `lint` and `graph`, every tool parameter mirrors its CLI
> flag or positional argument one-to-one *except* the output-routing flags —
> `convert --report`, `graph --output`, and the report-envelope `--json` of
> `convert`, `validate`, `review` and `lint` — which the tools do not expose:
> over MCP the transport **is** the JSON channel, so there is nothing to route
> and no `--json` flag to toggle. The tool payloads are byte-identical to the
> corresponding `binder <cmd> --json`, with `graph` the exception: its payload is
> the raw export in whatever `format` was asked for, so it equals
> `binder graph --json` only at `format:json`, and `binder graph --format dot` at
> `format:dot`. (`convert`'s `out`/`dry_run` and `graph`'s `format` select *what*
> is produced, not how the report is routed, so they remain — though `format` is
> also the one parameter whose **default** diverges from its flag's: the CLI
> defaults to `dot`, the tool to `json`. Two of those flag names mean different
> things on different commands. `convert`'s `-o`/`--output` is the output
> **bundle directory**, a destination rather than a report route, which is why it
> is exposed as `out`; `graph`'s `-o`/`--output` writes the export to a file
> instead of stdout and is genuinely routing. And `graph --json` is merely an
> alias for `--format json` — a format selector, not the report envelope, which
> `graph` never emits — so it is already covered by the exposed `format` param.
> `lint` and `review` have no `--output` flag at all.)
>
> `list_graphs` and `query_graph` have **no CLI equivalent**: `binder list-graphs`
> and `binder query-graph` are unknown commands. They are MCP-only tools, so
> parity does not apply to any of their parameters.

### Wiring it into a harness

Claude Code:

```bash
claude mcp add binder -- binder mcp
```

…or an `.mcp.json` entry:

```json
{ "mcpServers": { "binder": { "command": "binder", "args": ["mcp"] } } }
```

### Tools and input schemas

For the five verbs that have a CLI counterpart — `convert`, `validate`,
`review`, `lint` and `graph` — every tool parameter mirrors the corresponding
CLI flag or positional argument 1:1, with one **default** divergence: `graph`'s
`format` defaults to `dot` on the CLI and to `json` here. `list_graphs` and
`query_graph` are **MCP-only**: there is no `binder list-graphs` and no
`binder query-graph`, so none of their parameters mirrors a CLI flag,
`query_graph`'s whole parameter set included. Map/list params (`type_map`,
`fm_ref_keys`, …) use the same `"k=v,k=v"` / `"a,b"` grammar as the flags.
Required params are marked **(req)**.

| Tool | Params | Result |
|---|---|---|
| `convert` | `src` **(req)**, `out` (req unless `dry_run`), `dry_run`, `default_type`, `type_map`, `fm_ref_keys`, `source_keys`, `map_citations`, `map_draft`, `status_map`, `canonicalize_status`, `stale_after_map`, `verified_by`, `workspace_root`, `external_root`, `group_by_type`, `include_backlinks`, `include_graph`, `strict` | `convert` report envelope. `dry_run:true` → `convert.Analyze`, the ingestion-analysis preview (writes nothing); `dry_run:false` → writes the bundle to `out`. |
| `validate` | `bundle` **(req)**, `strict` | `validate` report envelope. |
| `review` | `bundle` **(req)**, `today`, `strict`, `entrypoints` | `review` report envelope. |
| `lint` | `src` **(req)**, `today`, `strict`, `entrypoints` | `lint` report envelope (read-only source-corpus health). |
| `graph` | `bundle` **(req)**, `format` (`dot`\|`json`\|`graphml`\|`html`, default `json`), `today` | The **raw** export bytes. `format:json` is the raw `{nodes,edges}` object — **not** the report envelope (see [graph JSON](#graph-json--a-raw-export-not-the-envelope)). |
| `list_graphs` | `bundle` **(req)**, `today`, `id_key` | `list_graphs` report envelope: the LPG **schema descriptor** for the graph binder projects from the bundle — graph name, `node_key` strategy, counts, node labels (the concept `type`s present) and the single `LINKS` edge label, each with property declarations. Read-only introspection derived from the same `graph.Build` projection; `id_key` prefers an authored stable-id frontmatter key as the node key when present, else path identity (never minted). |
| `query_graph` | `bundle` **(req)**, `op` **(req)** (`lookup`\|`neighbors`\|`neighborhood`\|`pattern`\|`path`), `today`, `id_key`, `id`, `label` (**req for `pattern`**), `direction` (`out`\|`in`\|`both`, default `out`), `rel`, `depth` (**req for `neighborhood`**, `1..5`), `to_label`, `where` (`{prop, eq}`, both required; `prop` ∈ `type`\|`tier`\|`stale`), `from` and `to` (**both req for `path`**), `max_depth` (**req for `path`**, `1..5`) | `query_graph` report envelope, one result shape per `op`. Which params apply depends on `op`; `id_key` is accepted for parity with `list_graphs` but is **never honored** here (the response echoes `node_key.honored: false`). The five operations, their per-`op` result shapes and the bounds are documented in full under [`query_graph`: asking questions of the graph](#query_graph-asking-questions-of-the-graph). |

`convert`'s `canonicalize_status` is the MCP parity param for the CLI's
`--canonicalize-status` — same always-on check, same opt-in rewrite, same
`status_notes` in the payload (see
[Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status)).
`external_root` is the parity param for the repeatable `--external-root` flag.
`review`'s and `lint`'s `entrypoints` is the parity param for the repeatable
`--entrypoint` flag: named concepts move out of `orphans` and into
`entrypoints`, exactly as on the CLI. A root `README.md` is still recognized
automatically without naming it.

`query_graph`'s JSON schema marks only `bundle` and `op` as `required`; the
per-`op` requirements marked **(req for …)** above are enforced at call time
instead and come back as tool errors — `pattern requires label`,
`path requires from and to`, `depth must be in 1..5`.

`strict` is accepted for parity with the CLI flag, but since a tool call has no
exit code it does **not** change the payload (the report is returned either way).
`today`/`SOURCE_DATE_EPOCH` default the staleness date exactly as on the CLI.

### Behavior and invariants

The server adds **no business logic** — it is a thin, invariant-preserving
transport over the existing internal functions:

- **Never-reject.** Findings (non-conformance, broken links, orphans, stale, …)
  are returned **in** the payload. A tool that produces findings is **not** an
  MCP error. Tool errors are reserved for **usage** (bad/missing params, an
  invalid `verified_by`, an unknown `graph` format) and **IO** (an unreadable
  path).
- **A zero-match query is a result, not an error.** `query_graph` extends the
  never-reject rule to the query surface: a `lookup` for an id that does not
  exist comes back as an ordinary result: the normal envelope with
  `result.not_found: true`, and **no** `isError` field on the wire at all (MCP
  treats an absent `isError` as false, so do not wait for a literal
  `"isError": false` — it is never sent). What *is* a tool error is malformed
  usage — an unknown `op`
  (`unknown op "bogus" (want lookup|neighbors|neighborhood|pattern|path)`)
  or an out-of-range `depth` (`depth must be in 1..5`) come back as
  `isError: true` with **plain text**, not the envelope. So a consumer must
  check for `isError: true` before attempting to parse the content as JSON.
- **Never-fabricate-trust.** `verified_by` is applied **only** when explicitly
  passed; the server never auto-stamps `verified` and never invents `sources`.
  An invalid actor is a usage-class tool error.
- **Determinism.** Payloads honor `SOURCE_DATE_EPOCH` / `today` the same as the
  CLI, so identical inputs yield identical bytes.
- **Additive only.** Source-mutating verbs (`enrich`, `emit_concept`) and
  read/search tools are deliberately **not** exposed: the read surface belongs to
  the knowledge store, and authoring over MCP is a later concern.
- **`infer` is not exposed either.** A `tools/list` returns exactly the seven
  tools above — no `enrich`, and no `infer`. `infer` writes nothing, so it is not
  excluded for the mutation reason `enrich` is; two consequences follow for a
  harness. First, there is no MCP route to a proposed `--type-map`: derive one
  yourself (the harness already has a model) and pass it to the `convert` tool's
  `type_map` param. Second, a pinned clock is all a harness needs to make every
  payload on the MCP surface reproducible, because `infer`'s Gemini tier — the
  one binder output that is not reproducible **even with the clock pinned** — is
  not reachable over it. Pin it in the **server's environment**: the `convert`
  tool has no clock param of its own, so `SOURCE_DATE_EPOCH` set for the
  `binder mcp` process is what reaches it.

## The graph surface

Every binder command that touches relationships works from the same derived
structure: a **graph** binder projects from the bundle's resolved links. binder does
not store a graph — it rebuilds one from the concepts and their links on every call,
hands you a view, and forgets it.

That projection is a labeled property graph. For the model itself — what nodes,
edges, labels and property maps are, how an adjacency index makes neighbor lookups
cheap, and when an in-process graph stops being the right answer — see the
[in-memory labeled property graph primer](lpg-inmemory-primer.md).

Three surfaces read that projection:

- **`binder graph`** — a CLI command that exports the *whole* graph in one of four
  formats (see the [`graph`](#graph) flag table).
- **`list_graphs`** — an MCP tool that describes the graph's *schema* (its labels,
  counts, and property declarations).
- **`query_graph`** — an MCP tool that answers *data* questions about the graph:
  lookup, neighbors, k-hop neighborhood, pattern match, and path existence.

`list_graphs` and `query_graph` are **MCP-only**: there is no `binder list_graphs`
or `binder query-graph` CLI verb. Running one is an unknown-command usage error:

```console
$ binder list_graphs ./bundle
binder: unknown command "list_graphs" for "binder"
$ echo $?
2
```

To call the MCP tools, run binder as an MCP server (see
[MCP server (`binder mcp`)](#mcp-server-binder-mcp) for wiring it into a harness).
`binder mcp` reads newline-delimited JSON-RPC on stdin and exits as soon as stdin
closes, so **stdin must stay open until the response has been read** — a real harness
holds the connection open, and in a shell the trailing `sleep` below stands in for
that. A bare `printf ... | binder mcp` closes the pipe immediately and the server
exits before it flushes the reply, so you get nothing back.

Over stdio a single `tools/call` looks like this — the report is a JSON string in
`result.content[0].text`:

```bash
{ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"c","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_graphs","arguments":{"bundle":"docs/examples/graph-sample/orders-kb"}}}' ; \
  sleep 2; } | binder mcp | jq -r 'select(.id==2).result.content[0].text'
```

A runnable sample bundle and its graph views live in
[`docs/examples/graph-sample/`](examples/graph-sample/); the examples below are
reproducible against it and against the shipped `testdata/okf-bundles/acme_retail`.

### The graph model

binder projects a **labeled property graph**:

- **Nodes are concepts.** A node's **label** is its concept `type` (`Table`,
  `Metric`, `Policy`, …). Every node carries the same five queryable **properties**:
  `id`, `title`, `type`, `tier` (the derived trust tier), and `stale`.
- **Edges are resolved links.** There is exactly **one edge label, `LINKS`** —
  binder's links are untyped. Each edge carries three properties: `from`, `to`, and
  `text` (the link's text, which serves as a relationship label *by convention
  only*).

`list_graphs` reports that schema directly. For the sample bundle
(`docs/examples/graph-sample/orders-kb`):

```json
{
  "binder": "binder/0.3.0",
  "command": "list_graphs",
  "schema": "binder.report/v1",
  "result": {
    "graphs": [
      {
        "name": "orders-kb",
        "source": {
          "kind": "okf-bundle",
          "root": "docs/examples/graph-sample/orders-kb"
        },
        "node_key": {
          "strategy": "path",
          "key": ""
        },
        "counts": {
          "nodes": 2,
          "edges": 1
        },
        "node_labels": [
          {
            "label": "Concept",
            "count": 1,
            "properties": ["id", "title", "type", "tier", "stale"]
          },
          {
            "label": "Table",
            "count": 1,
            "properties": ["id", "title", "type", "tier", "stale"]
          }
        ],
        "edge_labels": [
          {
            "label": "LINKS",
            "count": 1,
            "properties": ["from", "to", "text"]
          }
        ]
      }
    ]
  }
}
```

(The real output puts each property on its own line; it is folded here for reading.)
The node labels are exactly the concept types present, each with its count; the
single `LINKS` edge label carries the edge count. The `binder` field is the runtime
version string of the binary that produced the report — **not** a fixed constant;
yours reads whatever `binder --version` prints. This is read-only introspection
derived from the same projection `binder graph` exports.

### `binder graph`: exporting the whole graph

`binder graph <bundle>` writes the entire projection in one shot; the
[`graph` flag table](#graph) is the reference. `--format json` (or its `--json`
alias) is the raw `{nodes, edges}` export — **not** the `binder.report/v1` envelope
the other commands wrap their reports in (see
[graph JSON — a raw export, not the envelope](#graph-json--a-raw-export-not-the-envelope)):

```json
{
  "nodes": [
    { "id": "customer", "title": "Customer", "type": "Concept", "tier": "unverified", "stale": false },
    { "id": "orders", "title": "Orders", "type": "Table", "tier": "unverified", "stale": false }
  ],
  "edges": [
    { "from": "orders", "to": "customer", "text": "customer" }
  ]
}
```

(Folded for reading; the real output is 2-space-indented, one field per line.) Object
keys follow Go **struct declaration order** — `id, title, type, tier, stale` for
nodes; `from, to, text` for edges — deterministic and reproducible run-to-run, but
**not** alphabetically sorted. Do not build a consumer that assumes sorted keys.

`--format dot` emits Graphviz you can pipe to `dot -Tsvg`:

```console
$ binder graph docs/examples/graph-sample/orders-kb --format dot
digraph okf {
  rankdir=LR;
  node [shape=box];
  "customer" [label="Customer"];
  "orders" [label="Orders"];
  "orders" -> "customer" [label="customer"];
}
```

`--format graphml` is XML with typed node keys and an edge `rel` key (excerpt):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<graphml xmlns="http://graphml.graphdrawing.org/xmlns">
  <key id="title" for="node" attr.name="title" attr.type="string"></key>
  <key id="type" for="node" attr.name="type" attr.type="string"></key>
  <key id="tier" for="node" attr.name="tier" attr.type="string"></key>
  <key id="stale" for="node" attr.name="stale" attr.type="boolean"></key>
  <key id="rel" for="edge" attr.name="rel" attr.type="string"></key>
  <graph edgedefault="directed">
    <node id="customer">
      <data key="title">Customer</data>
```

`--format html` is a self-contained page — a readable node/edge table plus the same
JSON embedded as a `<script type="application/json">` island (49 lines for this
two-node bundle; excerpt):

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>binder graph</title>
...
<table>
<caption>Concepts</caption>
<tr><th>id</th><th>title</th><th>type</th><th>tier</th><th>stale</th></tr>
<tr><td>customer</td><td>Customer</td><td>Concept</td><td>unverified</td><td>false</td></tr>
```

`-o/--output` writes to a file instead of stdout. An empty `--format ""` is accepted
and means `dot`; an unrecognized value is a usage error:

```console
$ binder graph docs/examples/graph-sample/orders-kb --format bogus
binder: unknown graph format "bogus" (want dot|json|graphml|html)
$ echo $?
2
```

`--today YYYY-MM-DD` sets the date used for each node's `stale` flag; a malformed or
calendar-invalid date is a usage error:

```console
$ binder graph docs/examples/graph-sample/orders-kb --today 2026-13-45
binder: --today "2026-13-45" is not a valid date (expected YYYY-MM-DD)
$ echo $?
2
```

`--json` combined with a conflicting explicit `--format` is also a usage error:

```console
$ binder graph docs/examples/graph-sample/orders-kb --json --format dot
binder: --json conflicts with --format dot; --json selects --format json
$ echo $?
2
```

### Node identity: path, or a read-honored `id_key`

A node's identity is its **path-derived id** (`orders`, `tables/orders`). This is
what edges reference and what you pass to `query_graph`. binder **never mints** an
identity.

`list_graphs` accepts an optional `id_key`: the name of a frontmatter key to
*prefer* as the node key when a concept carries it. It is a read preference, not a
rename — nothing is written. The sample bundle's concepts carry a `slug` key:

Each line below is a `list_graphs` call made with the wrapper above, its payload
filtered with `jq '.result.graphs[0].node_key'`:

```text
arguments: {"bundle":"docs/examples/graph-sample/orders-kb"}
node_key:  {"strategy":"path","key":""}                     # default: path identity

arguments: {"bundle":"docs/examples/graph-sample/orders-kb","id_key":"slug"}
node_key:  {"strategy":"frontmatter","key":"slug"}          # id_key resolves, so honored
```

`strategy` is
`frontmatter` only when the key resolves on at least one concept; otherwise it stays
`path` with the key still echoed. The value is never invented — a concept missing the
key keeps its path id.

### `query_graph`: asking questions of the graph

`query_graph` is a single MCP tool with a required `op` selector. Every call takes a
`bundle` and an `op`; each `op` adds its own parameters. Every response is a
`binder.report/v1` envelope whose `result` carries the `op`, the echoed `query`, a
[`node_key`](#the-node_key-echo) object, and (where a node/edge set applies)
`nodes[]` and `edges[]`. `nodes` are sorted by `id`; `edges` by `from, to, text`. The
examples below run against `testdata/okf-bundles/acme_retail`.

#### The five operations

**`lookup`** — fetch a node by exact `id`, or all nodes of a `label`:

```json
{
  "binder": "binder/0.3.0",
  "command": "query_graph",
  "schema": "binder.report/v1",
  "result": {
    "op": "lookup",
    "query": { "id": "metrics/revenue" },
    "node_key": { "strategy": "path", "key": "", "honored": false },
    "nodes": [
      { "id": "metrics/revenue", "title": "Revenue", "type": "Metric", "tier": "human-reviewed", "stale": false }
    ],
    "not_found": false
  }
}
```

**`neighbors`** — one-hop neighbors of `id` in a `direction` (`out`|`in`|`both`,
default `out`), optionally filtered by `rel`:

```json
{
  "result": {
    "op": "neighbors",
    "query": { "id": "metrics/gross-margin", "direction": "out", "rel": "" },
    "node_key": { "strategy": "path", "key": "", "honored": false },
    "nodes": [
      { "id": "computations/gross-margin-period", "title": "Gross margin for a period", "type": "Attested Computation", "tier": "human-reviewed", "stale": false },
      { "id": "metrics/gross-margin-legacy", "title": "Gross Margin (legacy, pre-FY2026)", "type": "Metric", "tier": "human-reviewed", "stale": false },
      { "id": "metrics/revenue", "title": "Revenue", "type": "Metric", "tier": "human-reviewed", "stale": false }
    ],
    "edges": [
      { "from": "metrics/gross-margin", "to": "computations/gross-margin-period", "text": "computations/gross-margin-period.md" },
      { "from": "metrics/gross-margin", "to": "metrics/gross-margin-legacy", "text": "gross-margin-legacy" },
      { "from": "metrics/gross-margin", "to": "metrics/gross-margin-legacy", "text": "metrics/gross-margin-legacy.md" },
      { "from": "metrics/gross-margin", "to": "metrics/revenue", "text": "Revenue" }
    ],
    "truncated": false,
    "not_found": false
  }
}
```

(Envelope fields `binder`/`command`/`schema` omitted from here on; every response
carries them.)

**`neighborhood`** — bounded k-hop BFS from `id` up to `depth`, with each node's
minimum depth in `depths[]` (`nodes` and `edges` omitted here — excerpt):

```json
{
  "result": {
    "op": "neighborhood",
    "query": { "id": "metrics/gross-margin", "depth": 2, "direction": "out", "rel": "" },
    "node_key": { "strategy": "path", "key": "", "honored": false },
    "depths": [
      { "id": "metrics/gross-margin", "depth": 0 },
      { "id": "computations/gross-margin-period", "depth": 1 },
      { "id": "metrics/gross-margin-legacy", "depth": 1 },
      { "id": "metrics/revenue", "depth": 1 },
      { "id": "computations/revenue-ytd", "depth": 2 }
    ],
    "truncated": false,
    "not_found": false
  }
}
```

**`pattern`** — source nodes of `label` that link to a node matching `to_label`
and/or a `where` property filter over `type`/`tier`/`stale`:

```json
{
  "result": {
    "op": "pattern",
    "query": { "label": "Policy", "to_label": "Metric", "rel": "" },
    "node_key": { "strategy": "path", "key": "", "honored": false },
    "nodes": [
      { "id": "policies/margin-standard", "title": "Acme Retail — Cost Allocation & Margin Standard (FY2026)", "type": "Policy", "tier": "human-reviewed", "stale": false },
      { "id": "policies/revenue-recognition", "title": "Acme Retail — Revenue Recognition Policy (FY2026)", "type": "Policy", "tier": "human-reviewed", "stale": false }
    ],
    "edges": [
      { "from": "policies/margin-standard", "to": "metrics/gross-margin", "text": "metrics/gross-margin" },
      { "from": "policies/margin-standard", "to": "metrics/gross-margin-legacy", "text": "metrics/gross-margin-legacy" },
      { "from": "policies/revenue-recognition", "to": "metrics/gross-margin", "text": "metrics/gross-margin" },
      { "from": "policies/revenue-recognition", "to": "metrics/revenue", "text": "metrics/revenue" }
    ],
    "truncated": false
  }
}
```

`nodes` are the matching **source** nodes; `edges` are the satisfying links. A
property filter uses `"where": { "prop": "tier", "eq": "human-reviewed" }` in place
of (or alongside) `to_label`.

**`path`** — bounded existence and shortest hop-path from `from` to `to`:

```json
{
  "result": {
    "op": "path",
    "query": { "from": "policies/revenue-recognition", "to": "computations/revenue-ytd", "max_depth": 3, "direction": "out" },
    "node_key": { "strategy": "path", "key": "", "honored": false },
    "exists": true,
    "length": 1,
    "path": ["policies/revenue-recognition", "computations/revenue-ytd"],
    "not_found": false
  }
}
```

#### Bounds: depth and result caps

Traversal is bounded by construction. Depth is capped at **5**:
`neighborhood.depth` and `path.max_depth` are required and must be `1..5`. A value
outside that range is a usage error, surfaced as an MCP tool error (not a payload
finding):

```json
{ "content": [ { "type": "text", "text": "depth must be in 1..5" } ], "isError": true }
```

Node result sets are capped at **1000**. On overflow the results are **sorted, then
truncated** to the cap and the payload flags `truncated: true` — this is not an
error. To see it, build a corpus larger than the cap:

```bash
mkdir -p /tmp/big/items
for i in $(seq 0 1199); do n=$(printf '%05d' "$i"); \
  printf -- '---\ntype: Item\ntitle: Item %s\n---\n# Item %s\n' "$n" "$n" > "/tmp/big/items/item-$n.md"; done
binder convert /tmp/big -o /tmp/bigbundle
```

A `lookup` by label against it returns the first 1000 ids in sort order. The call and
its payload, filtered to a summary with `jq`:

```text
arguments: {"bundle":"/tmp/bigbundle","op":"lookup","label":"Item"}
result:    {"count":1000,"truncated":true,"first":"items/item-00000","last":"items/item-00999"}
```

#### Empty results are answers, not errors

A well-formed query that matches nothing returns a normal result (`isError` unset),
not a failure. `lookup`/`neighbors`/`neighborhood`/`pattern` return an empty `nodes`
list; a named-but-absent start id adds `not_found: true`; an unreachable `path`
returns `exists: false`.

```json
{
  "result": {
    "op": "lookup",
    "query": { "label": "Nonexistent" },
    "node_key": { "strategy": "path", "key": "", "honored": false },
    "nodes": [],
    "truncated": false
  }
}
```

#### Filtering by relationship: `rel` matches `Edge.Text` exactly

`neighbors`, `neighborhood`, and `pattern` accept an optional `rel` that keeps only
edges whose `text` **equals** the value. The match is exact — **not**
case-insensitive, **not** a prefix, **not** a substring. Every reader assumes
otherwise, so it is worth proving. The edge
`metrics/gross-margin → metrics/revenue` has `text: "Revenue"`; running
`op:"neighbors", id:"metrics/gross-margin"` with each `rel`:

```text
rel = "Revenue"   → nodes: ["metrics/revenue"]   # exact match
rel = "revenue"   → nodes: []                      # case differs
rel = "Rev"       → nodes: []                      # a prefix is not a match
rel = "even"      → nodes: []                      # a substring is not a match
```

#### The `node_key` echo

Every `query_graph` result echoes the identity basis it actually used:

```json
"node_key": { "strategy": "path", "key": "<your id_key, or empty>", "honored": false }
```

In this version `query_graph` **accepts** an `id_key` but does **not** re-key
traversal identity — traversal is always by path id. Rather than ignore the
parameter silently, it says so: `strategy` stays `path`, `key` echoes what you sent,
and `honored` is `false`. Supplying `id_key: "slug"` against the sample bundle:

```json
{
  "result": {
    "op": "lookup",
    "query": { "id": "orders" },
    "node_key": { "strategy": "path", "key": "slug", "honored": false },
    "nodes": [
      { "id": "orders", "title": "Orders", "type": "Table", "tier": "unverified", "stale": false }
    ],
    "not_found": false
  }
}
```

This matters for harnesses. `list_graphs` may report
`node_key.strategy: "frontmatter"` for the very same `id_key`, but `query_graph`
still traverses by path and returns `honored: false`. **A harness that passes an
`id_key` here must read `honored` and know its key was not applied** — the returned
ids are path ids (`orders`, not `urn:acme:orders`). Joining `query_graph` output on
your authored id without checking `honored` will silently mis-join results.

#### Errors and exit codes

Over MCP the convention is: a handler error becomes a tool result with
`isError: true` (its text is the message), while findings ride in the payload with no
`isError`. Usage problems — an unknown `op`, a missing required parameter, a `depth`
outside `1..5`, a bad `direction`, or a `lookup` given both `id` and `label` — are
tool errors. An unreadable `bundle` is an IO tool error. A query that simply matches
nothing is **not** an error (see above).

On the CLI, `binder graph` follows the standard
[exit-code contract](#json-output---json-and-the-exit-code-contract): `0` on success;
`2` for usage (an unknown or invalid `--format`, a malformed `--today`, a conflicting
`--json`/`--format`, or an unknown subcommand); `3` for an unreadable bundle or a
write failure.

### A read-only projection

The graph is a **read model**: derived from the bundle on every call, never stored,
never written back. `binder graph`, `list_graphs`, and `query_graph` cannot change a
bundle — not its frontmatter, not its links, not an identity. You can run any of them
against a production bundle, in any order, as often as you like, and the bundle bytes
are unchanged. If you need the graph again, ask again: it is always recomputed from
the current bundle, so it never drifts from the source.

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
    by: binder/0.3.0
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
   single hyphens, matched against concept titles). This **title-resolution**
   slug is a separate function from the **heading-anchor** slug that [`lint`](#lint)
   matches `#anchor`s against, which does *not* collapse runs; the two rules
   diverge deliberately, and the divergence is tracked as
   [#86](https://github.com/ghchinoy/binder/issues/86).

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
(e.g. `binder/0.3.0`, `reference_agent/gemini`), or one of the `human:`,
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

### Declarative trust & lifecycle flags

Three `convert` flags let you assign lifecycle and verification metadata
declaratively, per run, without hand-editing frontmatter. All three are off by
default; with none set, output is **byte-identical** to a plain run. Each obeys
the same discipline as the trust-mapping flags: deterministic, additive, and
**never clobbering an authored value**.

- **`--status-map "archive=deprecated,drafts=draft,default=active"`** — assigns
  `status` by the concept's source directory. Keys are directory prefixes matched
  **longest-first** (the most specific directory wins; ties are broken
  **lexicographically**, and each key is trimmed of surrounding `/`) — the same
  matcher `--type-map` and `--stale-after-map` use; the reserved `default=`
  key is the fallback for anything unmatched. `status` is set **only when the
  concept has none** — an authored `status` is always preserved. Values are the
  spec `status` enum (`draft`, `stable`, `deprecated`) — a value outside it is
  **checked and reported** on every run, and rewritten only if you ask. See
  [Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status).

- **`--stale-after-map "07-benchmarks=+6m,legacy=+0d"`** — assigns `stale_after`
  by directory prefix (same longest-first matching). Values use the relative-date
  grammar below, resolved against "now" (honouring `SOURCE_DATE_EPOCH`), and
  `stale_after` is set **only when absent**.

  | Spec | Meaning | Example (now = 2023-11-14) |
  |---|---|---|
  | `+Nd` | `N` days from now | `+30d` → `2023-12-14` |
  | `+Nm` | `N` months from now | `+6m` → `2024-05-14` |
  | `+Ny` | `N` years from now | `+1y` → `2024-11-14` |

  The result is emitted as a UTC `YYYY-MM-DD` date. `+0d` marks a concept stale
  as of now. A malformed spec (or a malformed `name=value` pair in either map) is
  a **usage error** (exit 2), not a silent skip.

- **`--verified-by "human:ghchinoy"`** — appends a `verified` stamp
  `{ by: <actor>, at: <now> }` (RFC 3339, `SOURCE_DATE_EPOCH`-aware) to each
  concept. It **appends** to any existing `verified` list and de-duplicates by
  `(by, at)`; it never rewrites the derived trust tier — the tier stays computed
  from the `verified` events (see [Derived trust tiers](#derived-trust-tiers)).
  The actor is validated with the actor grammar (`human:<id>`, `process:<id>`,
  `team:<id>`, or `<producer>/<version>` such as `binder/0.3.0`); an invalid
  value is a **usage error** (exit 2) that lists the valid forms. If unset, the
  flag falls back to the `verified_by` [config](#config) key (which is itself
  validated fail-fast at config-load).

These flags leave `binder validate` conformant: stamped output round-trips
byte-faithfully and never introduces a hard violation.

### Status vocabulary and `--canonicalize-status`

OKF §5.4 fixes the `status` enum at `draft` | `stable` | `deprecated`, but the
vocabularies real corpora use rarely match — `wip`, `active`, `archived`. Binder
splits this into two separate behaviours, and the distinction is the whole point
of the feature:

- **The check is always on.** Every `--status-map` value is compared against the
  §5.4 enum on every `convert` and `enrich` run, whether or not you pass a flag.
- **The rewrite is opt-in.** `--canonicalize-status` is the only thing that
  changes a value. Without it binder **writes your value unchanged** and tells
  you what it would have mapped it to. Binder never silently rewrites a status.

The alias table is **fixed and closed** — there is no way to extend it, and
anything not in it is never rewritten:

| Authored value | Canonicalizes to |
|---|---|
| `active` | `stable` |
| `wip` | `draft` |
| `in-progress` | `draft` |
| `archived` | `deprecated` |
| `legacy` | `deprecated` |

`--canonicalize-status` is a `bool`, default `false`, and is available on **both**
`convert` and `enrich`.

The transcripts below all run against `testdata/corpus-clean` from a checkout of
this repository, whose `overview.md` has no authored `status` (so `default=wip`
applies to it) and whose `metrics/revenue.md` has `status: stable` (so it is
never touched).

**Default: warn, write unchanged, exit `0`.** `convert` reports under a
`Status vocabulary (OKF §5.4):` heading; the written frontmatter really does keep
`status: wip`:

```console
$ binder convert testdata/corpus-clean -o /tmp/q/a --status-map default=wip; echo $?
binder convert
  source: testdata/corpus-clean
  output: /tmp/q/a
  concepts: 2
  links: 2 (resolved 2, unresolved 0)

Concepts:
  metrics/revenue.md  [type=Metric]
  overview.md  [type=Note]

Status vocabulary (OKF §5.4):
  - status value "wip" (from --status-map key "default") is not one of draft|stable|deprecated (OKF §5.4); wrote it unchanged — pass --canonicalize-status to map it to "draft"
0

$ grep '^status:' /tmp/q/a/overview.md
status: wip
```

**With the flag: rewrite, report, exit `0`.** Only the `Status vocabulary` block
and the written value change:

```console
$ binder convert testdata/corpus-clean -o /tmp/q/b --status-map default=wip --canonicalize-status; echo $?
binder convert
  source: testdata/corpus-clean
  output: /tmp/q/b
  concepts: 2
  links: 2 (resolved 2, unresolved 0)

Concepts:
  metrics/revenue.md  [type=Metric]
  overview.md  [type=Note]

Status vocabulary (OKF §5.4):
  - status value "wip" (from --status-map key "default") canonicalized to "draft" (OKF §5.4)
0

$ grep '^status:' /tmp/q/b/overview.md
status: draft
```

The check is on the **map value**, not on what was ultimately written: a
`--status-map` entry whose directory contains only concepts with an authored
`status` still produces a note, even though nothing was assigned from it.

`enrich` performs exactly the same check and rewrite, and folds the note into its
per-run warnings with a `status:` prefix instead of a separate heading:

```console
$ cp -r testdata/corpus-clean /tmp/enrsrc
$ binder enrich /tmp/enrsrc --status-map default=wip --canonicalize-status; echo $?
enrich /tmp/enrsrc
2 file(s): 1 enriched, 1 unchanged, 0 skipped
  enriched overview.md (added: generated, status, title, type)
  status: status value "wip" (from --status-map key "default") canonicalized to "draft" (OKF §5.4)
0

$ grep '^status:' /tmp/enrsrc/overview.md
status: draft
```

**A non-alias value is never rewritten, even with the flag on.** It is warned
about and written through, and the `— pass --canonicalize-status …` hint is
suppressed because the flag is already set:

```console
$ binder convert testdata/corpus-clean -o /tmp/q/c --status-map default=frobnicate --canonicalize-status; echo $?
binder convert
  source: testdata/corpus-clean
  output: /tmp/q/c
  concepts: 2
  links: 2 (resolved 2, unresolved 0)

Concepts:
  metrics/revenue.md  [type=Metric]
  overview.md  [type=Note]

Status vocabulary (OKF §5.4):
  - status value "frobnicate" (from --status-map key "default") is not one of draft|stable|deprecated (OKF §5.4); wrote it unchanged
0

$ grep '^status:' /tmp/q/c/overview.md
status: frobnicate
```

**Under `--strict`, a non-conformant value gates — and it gates before any
write.** The output directory is never created; on `enrich`, no source file is
touched:

```console
$ binder convert testdata/corpus-clean -o /tmp/q/d --strict --status-map default=wip; echo $?
binder: --status-map has non-conformant status value(s) and --strict is set: status value "wip" (from --status-map key "default") is not one of draft|stable|deprecated (OKF §5.4) — pass --canonicalize-status to map it to "draft"
1

$ ls /tmp/q/d
ls: cannot access '/tmp/q/d': No such file or directory
```

Combining the two flags resolves a known alias and the run returns to exit `0`:

```console
$ binder convert testdata/corpus-clean -o /tmp/q/e --strict --canonicalize-status --status-map default=wip; echo $?
binder convert
  source: testdata/corpus-clean
  output: /tmp/q/e
  concepts: 2
  links: 2 (resolved 2, unresolved 0)

Concepts:
  metrics/revenue.md  [type=Metric]
  overview.md  [type=Note]

Status vocabulary (OKF §5.4):
  - status value "wip" (from --status-map key "default") canonicalized to "draft" (OKF §5.4)
0
```

A **malformed** `--status-map` — a missing `=`, not a bad vocabulary value — is a
usage error and is caught before anything else, with or without either flag:

```console
$ binder convert testdata/corpus-clean -o /tmp/q/f --status-map default; echo $?
binder: invalid --status-map entry "default" (want dir=value)
2
```

Summary of the four outcomes:

| Run | Frontmatter written | Exit |
|---|---|---|
| non-conformant, no flags | value unchanged | `0` |
| non-conformant alias, `--canonicalize-status` | rewritten to the §5.4 value | `0` |
| non-conformant, `--strict` | **nothing written** | `1` |
| non-conformant alias, `--strict --canonicalize-status` | rewritten to the §5.4 value | `0` |

Under `--json`, both `convert` and `enrich` carry every one of these messages in
`result.status_notes` — always present, sorted, and `[]` on a conformant run:

```json
[
  "status value \"legacy\" (from --status-map key \"metrics\") canonicalized to \"deprecated\" (OKF §5.4)",
  "status value \"wip\" (from --status-map key \"default\") canonicalized to \"draft\" (OKF §5.4)"
]
```

(from `binder convert testdata/corpus-clean -o /tmp/q/g --status-map
"default=wip,metrics=legacy" --canonicalize-status --json`.)

Over MCP the same behaviour is reachable as the `convert` tool's
`canonicalize_status` boolean — see [Tools and input schemas](#tools-and-input-schemas).

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

`binder enrich` handles the unparseable case **differently and deliberately**:
because it mutates the source in place, recover-as-body would be destructive, so
enrich instead **skips** such a file (`status: skipped`, with a reason), leaves it
**byte-identical on disk**, and reports it as an advisory (gating only under
`--strict`). See [`enrich`](#enrich).

## Determinism and reproducible builds

Given identical input and the same clock, `convert` produces byte-identical
output, and re-converting an already-converted bundle is idempotent. To pin
synthesised timestamps (the `generated.at` stamp), set `SOURCE_DATE_EPOCH` to a
Unix timestamp:

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert corpus -o bundle
```

The same variable seeds the default `--today` used by `review`, `lint`, and
`graph` staleness, so a fully pinned pipeline is reproducible end to end.

## CI usage

`validate` is the CI gate: on a well-formed run it exits non-zero only on a
hard conformance violation, and under [`--strict`](#strict-mode) also on
advisories (see the [exit-code contract](#exit-code-contract)). The gate covers
concept files; reserved-file structure is outside its scope — see
[`validate`](#validate). A typical pipeline converts, then validates:

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

# lint the SOURCE corpus before conversion; --strict gates on any finding.
binder lint    docs/ --strict                   # fail on broken links / orphans / …

# review is advisory by default; --strict makes it (and convert) gate.
binder convert docs/ -o build/bundle --strict   # fail on unresolved links / recoveries
binder review  build/bundle --strict            # fail on orphans / stale / unresolved
```

By default `review` and `graph` never fail the build (they always exit `0`); use
them for reporting and artifacts. To fail a build on unresolved links, orphans,
or staleness, pass [`--strict`](#strict-mode) to `convert`, `review`, `lint`, or
`validate` — it promotes those advisories to gating findings (exit `1`) for that
run only. `binder lint` gates on **source-corpus** health before conversion; the
others gate on the emitted bundle.

The project's own exit gate additionally cross-checks binder's verdicts against
the external [`okfcli/okf`](https://github.com/okfcli/okf) validator in both
directions (`make gate`); see
[Differential-validation exit gate](RELEASING.md#differential-validation-exit-gate).

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
scope: reserved-file structure (index.md, log.md) not validated; verdict covers concept files only
RESULT: conformant (OKF 0.2)
```

Both generated indexes are counted but not examined, so the verdict covers
`overview.md` and `metrics/revenue.md`.

**3. Review** (revenue is human-reviewed via `verified.by: human:alice`;
overview is unverified). Keep the clock pinned here too — `review` derives its
staleness date from `SOURCE_DATE_EPOCH`, so an unpinned run prints today's date
and the example stops reproducing:

```bash
SOURCE_DATE_EPOCH=1700000000 binder review bundle
```

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
  stale (as of 2023-11-14): 0
  attested computations: 0
  unparsed frontmatter (recovered as body): 0
  entrypoints (no inbound links): 0
  orphans (no inbound or outbound links): 0
  unresolved links: 0
```

`1700000000` is `2023-11-14`, which is why the staleness date reads that way:
the whole point of pinning is that the report does not change from one day to
the next.

**4. Graph** (render with Graphviz):

```bash
binder graph bundle --format dot | dot -Tsvg -o graph.svg
```

## Roadmap & planned features

Each tracked feature below is marked **✅ shipped** or *Planned*, and links to its
tracking issue where one has been filed — the community-core codec adapter has
none yet. As of v0.3.0 everything here has shipped except the
community-core codec adapter. This guide grows a full section for each as it lands.

### Phase 2.x — `convert`/CLI enhancements

- **In-place enrichment** — ✅ shipped: `binder enrich` injects the missing
  required frontmatter (`type`/`title`/`generated`) into a source tree **in
  place** — frontmatter-only, additive/never-clobber, idempotent, and
  byte-faithful. See [`enrich`](#enrich).
  [#5](https://github.com/ghchinoy/binder/issues/5)
- **`file://` edge resolution** — ✅ shipped: workspace-relative `file://` URIs
  that point inside the workspace root now resolve to internal concept edges. See
  [`file://` link resolution](#file-link-resolution).
  [#6](https://github.com/ghchinoy/binder/issues/6)
- **Declarative trust & lifecycle flags** — ✅ shipped: `--status-map`,
  `--stale-after-map`, and `--verified-by` stamp status, freshness, and
  verification across directories, plus a `--strict` mode that gates advisories in
  CI. See [Declarative trust & lifecycle](#declarative-trust--lifecycle-flags) and
  [Strict mode](#strict-mode).
  [#7](https://github.com/ghchinoy/binder/issues/7)
- **`binder lint`** — ✅ shipped: a standalone **source-corpus** linter reporting
  broken links, missing titles, orphans, staleness, and schema violations, with
  `--strict` for a non-zero CI gate. See [`lint`](#lint).
  [#8](https://github.com/ghchinoy/binder/issues/8)
- **Richer root `index.md`** — ✅ shipped: `--group-by-type` appends an additive,
  type-grouped `# Catalog` to the root index, and `--include-backlinks` /
  `--include-graph` annotate entries with inbound/outbound resolved edges. See
  [The type-grouped catalog](#the-type-grouped-catalog).
  [#9](https://github.com/ghchinoy/binder/issues/9)
- **`binder config`** — ✅ shipped: a viper-backed config (flag > env > file >
  default) for actor identity and defaults, so common flags need not be passed
  every run. See [`config`](#config).
  [#10](https://github.com/ghchinoy/binder/issues/10)
- **Config mutation** — ✅ shipped: `binder config set`/`get`/`unset` with
  `--global`/local scoping write single keys into `./.binder.yaml` or the user
  config file, validated fail-fast, so `config` is no longer read-only. See
  [`config`](#config).
  [#47](https://github.com/ghchinoy/binder/issues/47)
- **`binder infer`** — ✅ shipped: proposal-only type-map inference over a source
  corpus, with a deterministic three-tier signal ladder and an opt-in Gemini
  semantic tier. Writes nothing. See [`infer`](#infer).
  [#38](https://github.com/ghchinoy/binder/issues/38),
  [#43](https://github.com/ghchinoy/binder/issues/43)
- **Opt-in refresh (`--overwrite-keys`)** — ✅ shipped: `enrich` can refresh named
  keys in place instead of only adding absent ones, while still refusing every
  trust-provenance key. See [Opt-in refresh](#opt-in-refresh---overwrite-keys).
  [#22](https://github.com/ghchinoy/binder/issues/22)
- **Status-vocabulary check and `--canonicalize-status`** — ✅ shipped: every
  `--status-map` value is checked against the OKF §5.4 enum on `convert` and
  `enrich`, with an opt-in flag to rewrite the known aliases. See
  [Status vocabulary and `--canonicalize-status`](#status-vocabulary-and---canonicalize-status).
  [#23](https://github.com/ghchinoy/binder/issues/23)
- **Entrypoints vs orphans** — ✅ shipped: `review` and `lint` now separate true
  orphans from entrypoints (outbound-only nodes, a root `README.md`,
  or anything named via `--entrypoint`), so an outbound-only node no longer gates
  under `--strict`. See [`review`](#review) and [`lint`](#lint).
  [#24](https://github.com/ghchinoy/binder/issues/24)
- **`--external-root`** — ✅ shipped: declare a known sibling-workspace root so
  `file://` links under it stay external without raising an outside-root
  advisory. See [`file://` link resolution](#file-link-resolution).
  [#25](https://github.com/ghchinoy/binder/issues/25)

### Codec adapter and the reachability layer

The reachability layer was sequenced **Skill/Plugin before MCP**, so MCP builds
on already-settled `--json` payloads rather than the reverse.

- **Community-core codec adapter** (`--okf-impl=community`): a second `Codec`
  behind the existing interface, slotted in only after it is confirmed
  byte-complete against the golden bundles. *Planned.*
- **Agent Skill + Agent-Plugin bundle** (`okf-convert`): — ✅ shipped
  ([#14](https://github.com/ghchinoy/binder/issues/14)). An isolated, optional
  plugin (plugin manifest + skill) driving the convert → validate → review
  workflow; deleting it leaves binder fully functional. See
  [Agent Skill / Plugin](../README.md#agent-skill--plugin).
- **MCP server mode** (`binder mcp`): — ✅ shipped
  ([#15](https://github.com/ghchinoy/binder/issues/15)). binder's *additive*
  `convert`/`validate`/`review`/`lint`/`graph`/`list_graphs`/`query_graph` tools
  over stdio (no read/search
  re-implementation), returning the same `binder.report/v1` payloads as `--json`.
  See [MCP server (`binder mcp`)](#mcp-server-binder-mcp).
- **Graph introspection and query over MCP** — ✅ shipped: `list_graphs`
  ([#32](https://github.com/ghchinoy/binder/issues/32), a #15 follow-on) returns
  the LPG schema descriptor for the graph binder projects from a bundle, and
  `query_graph` ([#33](https://github.com/ghchinoy/binder/issues/33)) answers
  data questions about that graph through five read-only traversal operations.
  Both are **MCP-only** — there is no CLI equivalent. See
  [`query_graph`](#query_graph-asking-questions-of-the-graph).

This guide is seeded per [issue #11](https://github.com/ghchinoy/binder/issues/11)
and grows alongside each feature above.
