# binder

A Go command-line tool that converts a plain-markdown corpus into a conformant
[Open Knowledge Format (OKF) v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle and validates OKF bundles against the spec's conformance rules.

![license](https://img.shields.io/badge/license-Apache--2.0-blue)
![go](https://img.shields.io/badge/go-1.26-00ADD8)

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

`binder` has two commands:

- **`convert`** walks a directory of ordinary markdown files and writes an OKF
  v0.2 bundle: one concept per non-reserved `.md`, standard markdown links
  rewritten to bundle-relative form, a root `index.md` declaring `okf_version`,
  and a generated provenance stamp. It never mutates the source.
- **`validate`** checks a bundle against the OKF v0.2 §11 conformance rules and
  reports trust/lifecycle well-formedness as advisories.

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

## How it works

- `internal/okf` — the OKF domain model plus two small interfaces, `Codec`
  (parse/serialize concepts) and `LinkGraph` (extract/resolve links). Trust tiers
  are *derived* from frontmatter, never stored.
- `internal/okf/native` — the sole codec today:
  [`goldmark`](https://github.com/yuin/goldmark) for markdown and link
  extraction, and [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3) (`yaml.Node`)
  for order-preserving frontmatter.
- `internal/convert` — the converter: concept discovery, link rewriting, and
  index/trust synthesis.
- `internal/validate` — the OKF v0.2 §11 conformance checker.
- `cmd` — the [Cobra](https://github.com/spf13/cobra) CLI; the concrete codec is
  selected and injected at the composition root (`cmd/root.go`).

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

- Full relationship extraction across concepts (Phase 2).
- An **MCP** (Model Context Protocol) server mode.
- An **Agent Skill** and an **Agent-Plugin** bundle, layering agent tooling over
  the same OKF core.

Today's shipped surface is the `convert` and `validate` CLI described above.

## Contributing

Pull requests are welcome. For major changes, please open an issue first to
discuss what you'd like to change. Make sure `make check` passes and add tests
for new behavior.

## License

Licensed under the [Apache License 2.0](LICENSE).
