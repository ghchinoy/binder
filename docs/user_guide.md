# binder user guide

This is the deep reference for `binder`. The [README](../README.md) is the
concise landing page (what it is, install, quickstart), and the
[tutorial](tutorial.md) is a hands-on, runnable walkthrough for a first-time
user; this guide documents **every command and flag**, the OKF v0.2 output
layout, the full trust vocabulary, the relationship-extraction rules,
malformed-input recovery, CI usage, and worked end-to-end examples.

`binder` converts a plain-markdown corpus into a conformant
[Open Knowledge Format (OKF) v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle and reports on OKF bundles. It is **Phase 2 complete**.

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
| **Reserved file** | `index.md` and `log.md`. These are structural, not concepts, and are not required to carry a `type`. |
| **Frontmatter** | The YAML block between `---` fences at the top of a concept. It is authoritative: `type` and the trust view are projections of it. |
| **Link / edge** | A directed relationship between concepts. Edges come from body markdown links (spec §6); resolved edges name a concept that exists in the bundle. |
| **Trust signals** | The v0.2 provenance/lifecycle vocabulary (`sources`, `generated`, `verified`, `status`, `stale_after`, …). |

## Commands

The binary exposes nine commands. All bundle-reading commands
(`validate`/`index`/`review`/`graph`) load a bundle through the same codec, so
their views of concepts, links, and trust always agree. `lint` and `enrich` are
the exceptions: they read (and, for `enrich`, mutate) a **source corpus** (not a
bundle) through the same converter machinery. `mcp` is a transport rather than a
corpus/bundle command; it exposes the additive verbs over stdio (see
[MCP server](#mcp-server-binder-mcp)).

```text
binder convert    Convert a markdown corpus into an OKF v0.2 bundle
binder enrich     Inject missing frontmatter into a source tree, in place (frontmatter only)
binder validate   Check a bundle for OKF v0.2 conformance (spec §11)
binder index      (Re)generate the per-directory index.md nav tree (spec §8)
binder review     Summarize a bundle: concepts, links, orphans, trust tiers, stale
binder lint       Report source-corpus health before conversion (writes nothing)
binder graph      Export the bundle's concept graph (dot|json|graphml|html)
binder config     Show the resolved effective configuration and each value's source
binder mcp        Run binder as a stdio MCP server (convert/validate/review/lint/graph/list_graphs)
```

Every command supports `-h`/`--help`. The root binary supports `-v`/`--version`.
`validate`, `review`, `lint`, `convert`, and `enrich` also support `--strict` to
gate advisories in CI (see [Strict mode](#strict-mode)); configuration is resolved
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
| `--default-type` | `Note` | Concept type applied when none is present or mapped. |
| `--type-map` | — | Per-directory type overrides, e.g. `"docs=Guide,adr=Decision"`. The longest (most specific) matching directory key wins. |
| `--status-map` | — | Per-directory `status`, e.g. `"archive=deprecated,drafts=draft,default=active"`. Longest-prefix match; `default=` is the fallback. Set **only when `status` is absent** (never clobbers an authored value). See [Declarative trust & lifecycle](#declarative-trust--lifecycle-flags). |
| `--stale-after-map` | — | Per-directory `stale_after` relative to now, e.g. `"07-benchmarks=+6m,legacy=+0d"`. Grammar `+Nd`/`+Nm`/`+Ny` (days/months/years, UTC `YYYY-MM-DD`). Longest-prefix match; set **only when `stale_after` is absent**. |
| `--verified-by` | config `verified_by` | Actor appended as a `verified` stamp, e.g. `"human:ghchinoy"` or `"binder/0.1.0"`. Validated with the actor grammar; an invalid value is a usage error (exit 2). Appends only — never rewrites the derived tier. See [Declarative trust & lifecycle](#declarative-trust--lifecycle-flags). |
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

```text
# corpus at /home/me/notes, intro.md links to file:///home/me/notes/docs/doc.md
binder convert /home/me/notes -o /tmp/bundle
# → intro.md body now contains [doc](/docs/doc.md); links: 1 (resolved 1)
```

##### Declaring known sibling roots (`--external-root`)

In an established multi-repo tree, an author often links across sibling
workspaces on purpose — `speech/` referencing
`file:///Users/me/projects/jibo/docs/...`. Those links are **genuinely
external** and `convert` leaves them exactly as written, but it also emits an
advisory for each one:

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

This flag applies to `convert` only. `binder lint` reports its own health
findings and does not surface `convert`'s `file://` advisories (external
`file://` targets are not recorded as edges), so there is nothing there for
`--external-root` to suppress.

```text
binder convert ./speech -o ./bundle --external-root /Users/me/projects
# → links under /Users/me/projects stay external but no longer advise;
#   any file:// link elsewhere still warns. Bundle bytes are unchanged.
```

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
3. **Body + pre-existing keys byte-faithful.** enrich reuses the codec's
   byte-faithful serializer: on a file it changes, unchanged frontmatter subtrees
   are re-emitted verbatim (nested-map key order, list order, and scalar
   quoting/folding preserved) and only the **added** top-level keys are encoded
   fresh; the body is re-emitted as-is.
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

> **Known normalization (only on files enrich changes):** the serializer
> normalizes body `\r\n`→`\n` and the frontmatter/body separator to a single
> blank line. A file that needs no key is never rewritten, so CRLF- or
> spacing-only files are left exactly as-is. Because enrich mutates the source,
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
| `--overwrite-keys` | — | **Opt-in** comma-separated keys to **refresh in place even when present**, e.g. `"status,stale_after"`. Default is additive/never-clobber. Scoped strictly to the named keys; trust keys are **refused** (exit 2). See [Opt-in refresh](#opt-in-refresh---overwrite-keys). |
| `--strict` | `false` | Gate (exit 1) when any file is skipped or a preserve-or-advise finding is present. Without it these never gate (exit 0). |
| `--json` | `false` | Emit the run report as deterministic JSON (schema `binder.report/v1`, `command:"enrich"`) instead of prose. |

The `--status-map`/`--stale-after-map`/`--verified-by` injectors are the same
declarative `#7` stamps `convert` offers; core enrichment is `type`/`title`/`generated`.

#### Output report

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
were **overwritten** (named and refreshed in place):

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
  identical input yields a byte-identical `index.md`, and `binder validate` still
  passes — the root index stays the sole `okf_version` carrier (spec §8/§12).

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

```markdown
# Catalog

## Pattern

* [Alpha](/patterns/alpha.md)
  * backlink: [Beta](/patterns/beta.md)
  * link: [Setup](/guides/setup.md)

## (untyped)

* [misc/notes](/misc/notes.md)
```

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
| `--entrypoint` | — | Concept id or path (repeatable) to treat as an **entrypoint**, not an orphan, in addition to the general rule and the recognized roots. |

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
  entrypoints (outbound, no inbound): 1
    README
  orphans (no inbound or outbound links): 0
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
  the corpus rather than being linked into), *or* it is a recognized root
  entrypoint (`README.md` / `index.md` at the corpus root), *or* it was named via
  `--entrypoint`. A root `README.md` that links out is an entrypoint, **not** an
  orphan — reporting it as an orphan was a false positive (issue #24).
- **Orphan** — a **true** orphan has **no inbound AND no outbound** resolved edges:
  a genuinely disconnected node, reported for you to wire up or accept, never
  removed.

Both classifications are **advisory only** — entrypoints never gate, and the
reclassification never changes an exit code that did not change before. `review`
and `lint` apply the **same rule**, so the two surfaces agree.

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
   truly disconnected node. A concept with no inbound but **outbound** edges (or a
   recognized root `README.md`/`index.md`, or one named via `--entrypoint`) is an
   **entrypoint** instead, reported separately and never as an orphan (issue #24).
   `review` applies the identical rule, so the two surfaces agree.
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
| `--entrypoint` | — | Concept id or path (repeatable) to treat as an **entrypoint**, not an orphan, in addition to the general rule and the recognized roots. |

All findings are **spec-tolerated advisories**: bare `binder lint` always exits
`0` even with findings (§11 hard conformance stays `validate`'s job over a
bundle). `--strict` gates **exit 1** when any finding is present — the same shared
contract as `convert`/`review`. The report is always emitted before the gate
signals. A missing or non-directory `<corpus>` path is a usage error (exit 2).

```text
binder lint path/to/corpus            # report; exit 0 even with findings
binder lint path/to/corpus --strict   # exit 1 if any finding (CI)
binder lint path/to/corpus --json     # deterministic envelope, command:"lint"
```

**Anchor slug convention (GitHub-style, pinned).** An `#anchor` matches a heading
whose slug is produced by: lowercase; strip HTML tags; drop every character except
`[a-z0-9]`, space, and hyphen; convert spaces to hyphens; collapse repeated
hyphens; and disambiguate duplicate headings with the suffixes `-1`, `-2`, … in
document order (a second `## Notes` becomes `#notes-1`). Slugging is
code-region-aware: a `# heading` inside a fenced code block is not a heading. This
convention lives in `okf.HeadingSlugs` and is a stable, load-bearing commitment.

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
| `--default-type` | `Note` | Fallback concept type when none is inferred or mapped. |
| `--gemini` | `false` | Enable Gemini semantic inference tier. |
| `--gemini-model` | `gemini-3.5-flash-lite` | Gemini model name for semantic inference. |
| `--location` | `global` | Google Cloud location for Vertex AI. |
| `--project` | — | Google Cloud project for Vertex AI (defaults to ADC / `GOOGLE_CLOUD_PROJECT`). |
| `--backend` | `auto` | Gemini auth backend: `auto`, `api`, or `vertex`. |
| `--gemini-required` | `false` | Fail on Gemini inference error instead of degrading to deterministic tiers. |
| `--json` | `false` | Emit the report as deterministic JSON (schema `binder.report/v1`). |
| `--strict` | `false` | Gate (exit 1) if any warning or inference failure occurs. |

### `config`

```text
binder config [subcommand] [flags]
```

Manages persistent configuration and prints the **resolved effective configuration** —
the value binder uses for each key and where that value came from.

Configuration is resolved once, at startup, with a strict precedence:

```text
flag  >  environment variable  >  config file  >  built-in default
```

- **Config files:**
  - Local repository config: `./.binder.yaml`
  - Global user config: `$XDG_CONFIG_HOME/binder/config.yaml` (fallback `$HOME/.config/binder/config.yaml`)
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

# Set default model globally in ~/.config/binder/config.yaml:
binder config set --global gemini.model gemini-3.5-flash-lite

# Inspect a single resolved value:
binder config get gemini.project
# Output: my-gcp-project

# Revert setting:
binder config unset gemini.project
```

| Flag | Default | Purpose |
|---|---|---|
| `--json` | `false` | Emit the configuration report as deterministic JSON (schema `binder.config/v1`). |
| `-g`, `--global` | `false` | (`set`/`unset` only) Target global user config (`~/.config/binder/config.yaml`) instead of `./.binder.yaml`. |

## JSON output (`--json`) and the exit-code contract

`convert`, `enrich`, `validate`, `review`, `lint`, and `graph` accept `--json` for
scripting, agents, and CI. Prose is the default and is **byte-unchanged** when
`--json` is absent — `--json` is a presentation layer over the already-computed
report, it changes no behavior and fabricates no fields or trust data.

### The envelope (schema `binder.report/v1`)

`convert`, `enrich`, `validate`, `review`, and `lint` wrap their existing report
struct in a thin envelope that carries the provenance and schema tag a consumer
needs to parse it safely:

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
| `command` | `convert` \| `enrich` \| `validate` \| `review` \| `lint`. |
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

Each `files[]` object: `path` (source-relative), `status` ∈
`enriched|unchanged|would-enrich|skipped`, `added` (sorted keys injected, omitted
when empty), and `reason` (for `skipped`, e.g. `unparseable frontmatter: <err>`).
Empty arrays serialize as `[]`.

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
| `orphans` | array of string | Concept IDs with **no inbound AND no outbound** resolved edges (true orphans). |
| `entrypoints` | array of string | Concept IDs with no inbound edge that are not orphans: outbound edges, a recognized root (`README.md`/`index.md`), or designated via `--entrypoint` (issue #24). Advisory; never gates. |
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
| `entrypoints` | array of string | Concept IDs with 0 inbound edge that are not orphans: outbound edges, a recognized root (`README.md`/`index.md`), or designated via `--entrypoint` (issue #24). Advisory; never gates. |
| `stale` | array of string | Concept IDs stale as of `today`. |
| `schema_violations` | array | Each `{ concept, detail }` — `"missing type"` or `"invalid frontmatter: <err>"`. |

All list fields are `[]` when empty. `lint`'s exit code follows the shared
contract: `0` by default (findings are advisories), `1` under `--strict` when any
finding is present. See the [exit-code contract](#exit-code-contract).

### `infer --json` — `result` fields

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
      "rationale": "well-known directory name "subsystems"",
      "sample_files": ["subsystems/audio.md"]
    }
  ],
  "warnings": []
}
```

- **`type_map`** (`string`) — comma-separated `dir=Type` string formatted for `--type-map`.
- **`mappings`** (`[]object`) — list of per-directory proposals with `dir`, `suggested_type`, `source` (`folder` | `pattern` | `frontmatter` | `gemini`), `rationale`, and optional `sample_files`, `model`, `backend`.

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
| `1` | findings-present | A gating condition: `validate` spec §11 non-conformance (unparseable frontmatter or a missing/empty `type`), or — under [`--strict`](#strict-mode) — any advisory promoted to a gating finding. | `validate`, `review`, `lint`, `convert`, `enrich` (with `--strict`) |
| `2` | usage-error | Bad flags/args — unknown flag, missing/extra argument, or a conflicting `--json`/`--format`. | any command |
| `3` | io-error | Cannot read the corpus/bundle, a write failure, or an internal error. | any command |

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
| `convert` | unresolved links or recovery warnings | — (a clean run is exit `0` even under `--strict`) |
| `enrich` | skipped (unparseable-frontmatter) files + preserve-or-advise findings | — (a clean run is exit `0` even under `--strict`) |

`--strict` is available on `validate`, `review`, `lint`, `convert`, and `enrich`.
A clean run stays exit `0` even with `--strict` set, so the flag is safe to leave
on permanently in CI. `index`, `graph`, and `config` have no advisory surface and
do not take it.

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

- **`binder --version`** prints `binder/<version>` (e.g. `binder/0.1.0`) — the
  exact string that appears in the JSON envelope's `binder` field. (`--version`
  is a root flag; passing it to a subcommand is a usage error.)
- **`binder --help`** lists every command; **`binder <cmd> --help`** prints that
  command's `Usage:` line and a `Flags:` section (name, shorthand, default,
  description) — a stable, documented shape sufficient to discover every flag,
  including `--json`.

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

`binder mcp` is a **transport, not a report-producing command** — it takes no
positional args and has no `--json` flag (its *outputs* are the structured tool
payloads). It serves over stdio until the client disconnects.

> **Output-routing flags are the deliberate 1:1 exception.** Every tool
> parameter mirrors its CLI flag one-to-one *except* the output-routing flags
> `--report` / `--output` / `--json`, which the tools do not expose: over MCP the
> transport **is** the JSON channel, so there is nothing to route and no `--json`
> flag to toggle. The tool payloads are byte-identical to the corresponding
> `binder <cmd> --json`. (`convert`'s `out`/`dry_run` and `graph`'s `format`
> select *what* is produced, not how the report is routed, so they remain.)

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

Every tool parameter mirrors the corresponding CLI flag 1:1. Map/list params
(`type_map`, `fm_ref_keys`, …) use the same `"k=v,k=v"` / `"a,b"` grammar as the
flags. Required params are marked **(req)**.

| Tool | Params | Result |
|---|---|---|
| `convert` | `src` **(req)**, `out` (req unless `dry_run`), `dry_run`, `default_type`, `type_map`, `fm_ref_keys`, `source_keys`, `map_citations`, `map_draft`, `status_map`, `stale_after_map`, `verified_by`, `workspace_root`, `group_by_type`, `include_backlinks`, `include_graph`, `strict` | `convert` report envelope. `dry_run:true` → `convert.Analyze`, the ingestion-analysis preview (writes nothing); `dry_run:false` → writes the bundle to `out`. |
| `validate` | `bundle` **(req)**, `strict` | `validate` report envelope. |
| `review` | `bundle` **(req)**, `today`, `strict` | `review` report envelope. |
| `lint` | `src` **(req)**, `today`, `strict` | `lint` report envelope (read-only source-corpus health). |
| `graph` | `bundle` **(req)**, `format` (`dot`\|`json`\|`graphml`\|`html`, default `json`), `today` | The **raw** export bytes. `format:json` is the raw `{nodes,edges}` object — **not** the report envelope (see [graph JSON](#graph-json--a-raw-export-not-the-envelope)). |
| `list_graphs` | `bundle` **(req)**, `today`, `id_key` | `list_graphs` report envelope: the LPG **schema descriptor** for the graph binder projects from the bundle — graph name, `node_key` strategy, counts, node labels (the concept `type`s present) and the single `LINKS` edge label, each with property declarations. Read-only introspection derived from the same `graph.Build` projection; `id_key` prefers an authored stable-id frontmatter key as the node key when present, else path identity (never minted). |

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
- **Never-fabricate-trust.** `verified_by` is applied **only** when explicitly
  passed; the server never auto-stamps `verified` and never invents `sources`.
  An invalid actor is a usage-class tool error.
- **Determinism.** Payloads honor `SOURCE_DATE_EPOCH` / `today` the same as the
  CLI, so identical inputs yield identical bytes.
- **Additive only.** Source-mutating verbs (`enrich`, `emit_concept`) and
  read/search tools are deliberately **not** exposed: the read surface belongs to
  the knowledge store, and authoring over MCP is a later concern.

## The graph surface

Every binder command that touches relationships works from the same derived
structure: a **graph** binder projects from the bundle's resolved links. binder does
not store a graph — it rebuilds one from the concepts and their links on every call,
hands you a view, and forgets it. Three surfaces read that projection:

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
Over stdio a single `tools/call` looks like this — the report is a JSON string in
`result.content[0].text`:

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"c","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_graphs","arguments":{"bundle":"./bundle"}}}' \
  | binder mcp 2>/dev/null | jq -r 'select(.id==2).result.content[0].text'
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

```console
# default: path identity
$ ... "arguments":{"bundle":"docs/examples/graph-sample/orders-kb"} ...
{"strategy":"path","key":""}

# id_key honored because the frontmatter key resolves
$ ... "arguments":{"bundle":"docs/examples/graph-sample/orders-kb","id_key":"slug"} ...
{"strategy":"frontmatter","key":"slug"}
```

(Filtering the tool payload with `jq '.result.graphs[0].node_key'`.) `strategy` is
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
error. Against a 1200-node corpus, a `lookup` by label returns the first 1000 ids in
sort order:

```console
$ ... "op":"lookup","label":"Item" ...   # filtered with jq
{"count":1000,"truncated":true,"first":"items/item-00000","last":"items/item-00999"}
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

```console
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

### Declarative trust & lifecycle flags

Three `convert` flags let you assign lifecycle and verification metadata
declaratively, per run, without hand-editing frontmatter. All three are off by
default; with none set, output is **byte-identical** to a plain run. Each obeys
the same discipline as the trust-mapping flags: deterministic, additive, and
**never clobbering an authored value**.

- **`--status-map "archive=deprecated,drafts=draft,default=active"`** — assigns
  `status` by the concept's source directory. Keys are directory prefixes matched
  **longest-first** (the most specific directory wins); the reserved `default=`
  key is the fallback for anything unmatched. `status` is set **only when the
  concept has none** — an authored `status` is always preserved. Values are the
  spec `status` enum.

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
  `team:<id>`, or `<producer>/<version>` such as `binder/0.1.0`); an invalid
  value is a **usage error** (exit 2) that lists the valid forms. If unset, the
  flag falls back to the `verified_by` [config](#config) key (which is itself
  validated fail-fast at config-load).

These flags leave `binder validate` conformant: stamped output round-trips
byte-faithfully and never introduces a hard violation.

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
  convert/validate/review/lint/graph tools over stdio (no read/search
  re-implementation), returning the same `binder.report/v1` payloads as `--json`.
  See [MCP server (`binder mcp`)](#mcp-server-binder-mcp).

This guide is seeded per [issue #11](https://github.com/ghchinoy/binder/issues/11)
and grows alongside each feature above.
