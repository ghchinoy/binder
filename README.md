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
round-trip. It also reports and visualizes a bundle (`review`, `graph`).

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
- **`index`** (re)generates per-directory `index.md` navigation for a bundle.
- **`review`** summarizes a bundle: concepts by type, trust tiers,
  stale/attested, orphans, and unresolved links.
- **`graph`** exports the concept graph (edges = resolved links) as
  dot/json/graphml/html.

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

### convert flags

| Flag | Default | Purpose |
|---|---|---|
| `-o`, `--output` | — | Output bundle directory (required unless `--dry-run`). |
| `--dry-run` | `false` | Report what would be written without writing anything. |
| `--default-type` | `Note` | Concept type applied when none is present or mapped. |
| `--type-map` | — | Per-directory type overrides, e.g. `"docs=Guide,adr=Decision"`. |
| `--report` | — | Also write the run report to this file. |

### Relationship & trust flags (`convert`)

Relationship extraction that is always on: `[[wikilinks]]` / `[[Target|alias]]`
(resolved by path → filename → title-slug), standard `[text](a.md#anchor)`
links, and `#hashtags` merged into frontmatter `tags`. Unresolved links are left
in place **and** reported (spec §6/§11). These flags opt into the rest:

| Flag | Effect |
|---|---|
| `--fm-ref-keys related,parent` | treat the named frontmatter keys as relationship edges (keys are preserved; each resolved target is also materialized as a link in a trailing `## Related` section) |
| `--map-citations` | map a body `# Citations` list into `sources` entries |
| `--source-keys source,url` | map the named frontmatter keys into `sources` entries |
| `--map-draft` | map a `draft: true` marker to `status: draft` (never clobbers an existing status) |

Trust mapping is **off by default** and never fabricates provenance: with no
mapping flags, frontmatter round-trips byte-for-byte.

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

- An **MCP** (Model Context Protocol) server mode.
- An **Agent Skill** and an **Agent-Plugin** bundle, layering agent tooling over
  the same OKF core.

Today's shipped surface is the `convert`, `validate`, `index`, `review`, and
`graph` CLI described above.

## Contributing

Pull requests are welcome. For major changes, please open an issue first to
discuss what you'd like to change. Make sure `make check` passes and add tests
for new behavior.

## License

Licensed under the [Apache License 2.0](LICENSE).
