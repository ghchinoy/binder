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
round-trip. It also reports and visualizes a bundle (`review`, `graph`),
declaratively stamps trust/lifecycle metadata (`--status-map`,
`--stale-after-map`, `--verified-by`), is configurable via `binder config`, and
supports `--strict` CI gating.

## Table of Contents

- [What it does](#what-it-does)
- [Installation](#installation)
- [Usage](#usage)
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
- **`validate`** checks a bundle against the OKF v0.2 §11 conformance rules and
  reports trust/lifecycle well-formedness as advisories.
- **`index`** (re)generates per-directory `index.md` navigation for a bundle, and
  can add a type-grouped `# Catalog` to the root index (`--group-by-type`).
- **`review`** summarizes a bundle: concepts by type, trust tiers,
  stale/attested, orphans, and unresolved links.
- **`graph`** exports the concept graph (edges = resolved links) as
  dot/json/graphml/html.
- **`config`** shows the resolved effective configuration (viper-backed) and
  where each value came from (flag/env/file/default).

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

> **Looking for the full reference?** Every command and flag, the OKF v0.2 output
> layout, the complete trust vocabulary, the relationship-extraction rules,
> malformed-input recovery, CI usage, and worked end-to-end examples live in the
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

# Export the concept graph (edges = resolved links).
binder graph path/to/bundle --format dot   # dot|json|graphml|html
```

```text
bundle: path/to/bundle
concepts: 2, reserved files: 1
RESULT: conformant (OKF 0.2)
```

### Machine-readable output (`--json`) and exit codes

`convert`, `validate`, `review`, and `graph` accept `--json` for scripting and CI.
Prose is the default and is byte-unchanged when `--json` is absent.

`convert`, `validate`, and `review` wrap their existing report in a thin,
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
sorted sub-list of inbound / outbound edges. These annotations derive from the
**same resolved-edge set** `binder graph` uses (resolved links only), so the
catalog and the graph can never disagree:

```markdown
# Catalog

## Pattern

* [Alpha](/patterns/alpha.md)
  * backlink: [Beta](/patterns/beta.md)
  * link: [Setup](/guides/setup.md)
```

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
- `cmd` — the [Cobra](https://github.com/spf13/cobra) CLI; the concrete codec is
  injected once at the composition root (`cmd/root.go`) — every other package
  depends only on the `okf` interfaces.

## Development

```bash
git clone https://github.com/ghchinoy/binder.git
cd binder
make build        # -> bin/binder
make check        # offline gate: gofmt + go vet + go test ./...
```

Requires **Go 1.26+**. Dependencies are vendored (`vendor/`), so the offline
gate needs no network access.

The full exit gate additionally cross-checks binder's verdicts against the
external `okfcli/okf` validator:

```bash
make okf-install  # go install github.com/okfcli/okf/cmd/okf@v0.3.0
make gate         # offline checks + external differential validation
```

`make gate` runs `scripts/interop.sh`, which compares binder's and `okf`'s
verdicts in both directions and fails on any unexpected disagreement.

## Roadmap

The following are **planned, not yet shipped**:

- **Phase 3 — a community-core codec adapter** (e.g. `--okf-impl=community`): a
  second `Codec` behind the existing interface, slotted in only after it is
  confirmed byte-complete against the golden bundles.
- **Phase 4 — an MCP** (Model Context Protocol) server mode (`binder mcp`),
  exposing binder's *additive* convert/author/emit tools only (no read/search
  re-implementation).
- **Phase 5 — an Agent Skill** and **Phase 6 — an Agent-Plugin bundle**, layering
  agent tooling over the same OKF core.

Declarative trust/lifecycle flags (`--status-map`, `--stale-after-map`,
`--verified-by`; #7), `binder config` (#10), and `--strict` mode have **shipped**
(see above). Remaining near-term `convert`/CLI enhancements are tracked as open
issues (in-place enrichment and a standalone `lint`); the
[user guide](docs/user_guide.md#roadmap--planned-features) maps each to its issue.

Today's shipped surface is the `convert`, `validate`, `index`, `review`, `graph`,
and `config` CLI described above.

## Contributing

Pull requests are welcome. For major changes, please open an issue first to
discuss what you'd like to change. Make sure `make check` passes and add tests
for new behavior.

## License

Licensed under the [Apache License 2.0](LICENSE).
