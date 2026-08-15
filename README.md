# binder

OKF application — a corpus→OKF v0.2 converter with relationship extraction, plus a
thin agent-tooling layer (CLI / MCP / Agent Skill / Agent Plugin) to follow.

Design of record: `design-v2.md` (project scratchpad).

**Status: Phase 1 (vertical slice) complete** — a Go CLI that converts a non-OKF
markdown corpus into a conformant [OKF v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle, validates bundles against the spec's §11 conformance rules, and preserves
trust frontmatter byte-for-byte on round-trip.

## Build

```sh
make build        # -> bin/binder
```

## Usage

```sh
# Convert a plain markdown corpus into an OKF v0.2 bundle.
binder convert path/to/corpus -o path/to/bundle

# Preview without writing anything.
binder convert path/to/corpus -o path/to/bundle --dry-run

# Validate a bundle against OKF v0.2 §11 conformance rules.
binder validate path/to/bundle
```

`convert` is deterministic: it honours `SOURCE_DATE_EPOCH` for any synthesised
timestamps, so identical input yields byte-identical output.

## Architecture (Phase 1)

- `internal/okf` — the OKF domain model plus two small interfaces, `Codec`
  (parse/serialize concepts) and `LinkGraph` (extract/resolve links). Trust tiers
  are *derived* from frontmatter, never stored.
- `internal/okf/native` — the sole Phase-1 codec: [`goldmark`](https://github.com/yuin/goldmark)
  for markdown/link extraction and [`gopkg.in/yaml.v3`](https://gopkg.in/yaml.v3)
  (`yaml.Node`) for order-preserving frontmatter. Unmodified frontmatter is passed
  through verbatim so round-trips are byte-faithful, including nested-map key order.
- `internal/convert` — the converter (concept discovery, link rewriting, index/trust
  synthesis).
- `internal/validate` — the §11 conformance checker.
- `cmd` — the Cobra CLI; dependencies are wired at the composition root
  (`cmd/root.go`).

## Verification

```sh
make check    # offline gate: gofmt + go vet + go test ./...
make gate     # full exit gate: offline checks + external differential validation
```

`make gate` additionally runs `scripts/interop.sh`, which cross-checks every
verdict against the vendor-neutral [`okfcli/okf`](https://github.com/okfcli/okf)
validator in both directions and captures any disagreement. Install it with:

```sh
make okf-install    # go install github.com/okfcli/okf/cmd/okf@v0.3.0
```
