# binder

OKF application — a corpus→OKF v0.2 converter with relationship extraction, plus a
thin agent-tooling layer (CLI / MCP / Agent Skill / Agent Plugin) to follow.

Design of record: `design-v2.md` (project scratchpad).

**Status: Phase 2 complete** — a Go CLI that converts a non-OKF markdown corpus
into a conformant [OKF v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle, extracts every relationship signal (wikilinks, anchor links, frontmatter
refs, hashtags), maps corpus-native provenance into the trust vocabulary,
generates per-directory index navigation, validates bundles against the spec's
§11 conformance rules, and preserves trust frontmatter byte-for-byte on
round-trip. It also reports and visualizes a bundle (`review`, `graph`).

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

# (Re)generate per-directory index.md navigation for a bundle.
binder index path/to/bundle

# Summarize a bundle: concepts by type, trust tiers, stale/attested,
# orphans, and broken internal links.
binder review path/to/bundle

# Export the concept graph (edges = resolved links).
binder graph path/to/bundle --format dot   # dot|json|graphml|html
```

`convert` is deterministic: it honours `SOURCE_DATE_EPOCH` for any synthesised
timestamps, so identical input yields byte-identical output.

### Relationship & trust flags (`convert`)

Relationship extraction that is always on: `[[wikilinks]]` / `[[Target|alias]]`
(resolved by path → filename → title-slug), standard `[text](a.md#anchor)`
links, and `#hashtags` merged into frontmatter `tags`. Unresolved links are left
in place **and** reported (spec §6/§11). These flags opt into the rest:

| Flag | Effect |
|---|---|
| `--fm-ref-keys related,parent` | treat the named frontmatter keys as relationship edges (keys are preserved) |
| `--map-citations` | map a body `# Citations` list into `sources` entries |
| `--source-keys source,url` | map the named frontmatter keys into `sources` entries |
| `--map-draft` | map a `draft: true` marker to `status: draft` (never clobbers an existing status) |
| `--report FILE` | also write the run report (unresolved links, warnings) to `FILE` |

Trust mapping is **off by default** and never fabricates provenance: with no
mapping flags, frontmatter round-trips byte-for-byte.

## Architecture

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
- `cmd` — the Cobra CLI; the concrete codec is injected once at the composition
  root (`cmd/root.go`) — every other package depends only on the `okf` interfaces.

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
