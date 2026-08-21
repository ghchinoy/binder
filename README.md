# binder

A Go command-line tool that converts a plain-markdown corpus into a conformant
[Open Knowledge Format (OKF) v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle and validates OKF bundles against the spec's conformance rules.

![license](https://img.shields.io/badge/license-Apache--2.0-blue)
![go](https://img.shields.io/badge/go-1.26.1+-00ADD8)

**Available today** — a Go CLI that converts a non-OKF markdown corpus
into a conformant [OKF v0.2](https://github.com/GoogleCloudPlatform/knowledge-catalog)
bundle, extracts every relationship signal (wikilinks, anchor links, frontmatter
refs, hashtags), maps corpus-native provenance into the trust vocabulary,
generates per-directory index navigation, validates bundles against the spec's
§11 conformance rules, and preserves trust frontmatter losslessly on
round-trip — scoped to files whose frontmatter binder recognises **and that need
no read-boundary normalization**: the fence must open with `---` and a newline
(LF or CRLF) at the very start. A leading UTF-8 BOM or a lone-CR (classic-Mac)
fence is now recognised too, but is normalized before recognition (#124) — a
disclosed change that deliberately does not preserve the original encoding. It
also reports and visualizes a bundle (`review`,
`graph`), lints a source corpus before conversion (`lint`), declaratively stamps
trust/lifecycle metadata (`--status-map`, `--stale-after-map`, `--verified-by`),
is configurable via `binder config`, and supports `--strict` CI gating.

**That scoping is load-bearing if you rely on binder for provenance.**
Recognising the fence is a scanner, not a property of the file. A leading UTF-8
BOM or a lone-CR-delimited fence used to leave the file looking plain, so
`convert` and `enrich` synthesized a fresh block and left the original — any
`verified:` attestation with it — in the body as text, exiting `0` with nothing
skipped and no warning. As of
[#124](https://github.com/ghchinoy/binder/issues/124) that input is normalized
at the read boundary before recognition, so the fence — and the attestation it
guards — is recognised; because the normalization (BOM strip, lone-CR → LF)
does not preserve the original encoding it is disclosed non-optionally via a
per-file `normalized` signal and a top-level advisory. Recognition still leaves
byte-level bounds; for the ones known today, see *Residual bounds* under
[`enrich`](docs/user_guide.md#enrich).

## Table of Contents

- [What it does](#what-it-does)
- [Installation](#installation)
- [Usage](#usage)
- [Agent Skill / Plugin](#agent-skill--plugin)
- [MCP server (`binder mcp`)](#mcp-server-binder-mcp)
- [How it works](#how-it-works)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

## What it does

`binder` has these commands:

- **`convert`** walks a directory of ordinary markdown files and writes an OKF
  v0.2 bundle: one concept per non-reserved `.md`, standard markdown links and
  `[[wikilinks]]` rewritten to bundle-relative form, frontmatter-ref edges,
  `#hashtags` merged into `tags`, per-directory `index.md` navigation, and a
  generated provenance stamp.
- **`enrich`** adds the missing required frontmatter (`type`, `title`,
  `generated`) to a source markdown tree **in place**: frontmatter only, no body
  changes. It is additive/never-clobber, atomic, and safe on a git-tracked repo,
  and idempotent unless a `verified` stamp advances (see
  [`enrich`](docs/user_guide.md#enrich)).
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
- **`infer`** inspects a source markdown corpus and proposes a directory-to-type
  mapping string (e.g. `docs=Guide,subsystems=Subsystem`) using deterministic
  heuristics (folders, patterns, frontmatter) and optional Gemini semantic inference
  (`--gemini`). It is proposal-only and never writes to disk.
- **`config`** manages persistent configuration (`get`, `set`, `unset`, `list`)
  and displays the resolved effective configuration with source attribution.
- **`mcp`** runs binder as a stdio MCP server, exposing **seven** tools: the
  additive verbs (`convert`/`validate`/`review`/`lint`/`graph`) plus the two
  read-only graph tools `list_graphs` (schema introspection) and `query_graph`
  (bounded traversal). They return the same `binder.report/v1` payloads as
  `--json` (see [MCP server](/binder/agent/mcp/)).

`convert` can also declaratively stamp trust and lifecycle metadata across
directory sections — `--status-map`, `--stale-after-map`, and `--verified-by`
(#7) — and six commands support `--strict` to gate advisories in CI (see
[Declarative trust & lifecycle flags](docs/user_guide.md#declarative-trust--lifecycle-flags)
and [Strict mode](docs/user_guide.md#strict-mode)).

Two properties make it trustworthy for pipelines:

- **Deterministic output.** `convert` honours `SOURCE_DATE_EPOCH` for any
  synthesised timestamps, so identical input yields byte-identical output.
- **Lossless frontmatter round-trip, where binder recognises the fence.**
  Unmodified YAML frontmatter is re-emitted verbatim — including nested-map
  and list key order — so every authored key and value survives untouched and
  a round-trip changes nothing it did not have to change. This is scoped to
  files whose frontmatter binder recognises **and that need no read-boundary
  normalization**: the fence must open with `---` and a newline (LF or CRLF) at
  the very start. A leading UTF-8 BOM or a lone-CR (classic-Mac) fence is now
  recognised too, but is first normalized at the read boundary
  ([#124](https://github.com/ghchinoy/binder/issues/124)) — the fence and any
  `verified:` block it guards are preserved, but the round-trip does not
  preserve the original encoding and is disclosed via a `normalized` signal and
  a top-level advisory. Recognition still leaves byte-level bounds; for the ones
  known today, see *Residual bounds* under [`enrich`](docs/user_guide.md#enrich).
  Scoped to the frontmatter: `convert` also pipelines the body and synthesises
  `index.md`, so the guarantee does not extend to a whole-file comparison.

## Installation

**Homebrew** (macOS and Linux, the recommended path):

```bash
brew install ghchinoy/tap/binder
```

`ghchinoy/tap` is Homebrew's shorthand for the
[`ghchinoy/homebrew-tap`](https://github.com/ghchinoy/homebrew-tap) repository,
whose `Formula/binder.rb` is regenerated by GoReleaser on every release. To
upgrade later: `brew upgrade binder`.

**Direct download.** Grab a prebuilt archive for your platform from the
[latest release](https://github.com/ghchinoy/binder/releases/latest) and put the
`binder` binary on your `PATH`. Every release publishes
`linux`/`darwin`/`windows` × `amd64`/`arm64` archives named
`binder_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows), plus a
`checksums.txt`. The v0.2.1 release, for example, carries:

```text
binder_0.2.1_darwin_amd64.tar.gz  binder_0.2.1_linux_amd64.tar.gz   binder_0.2.1_windows_amd64.zip
binder_0.2.1_darwin_arm64.tar.gz  binder_0.2.1_linux_arm64.tar.gz   binder_0.2.1_windows_arm64.zip
checksums.txt
```

Note: The `v` is in the tag but not in the filename.

Set `VERSION` to the tag on the releases page **without its leading `v`** (the
snippet puts the `v` back for the tag path and leaves it off the filename), pick
your platform, and the rest is mechanical:

```bash
export VERSION=...  OS=darwin  ARCH=arm64   # OS: linux|darwin|windows · ARCH: amd64|arm64
BASE="https://github.com/ghchinoy/binder/releases/download/v${VERSION}"   # tag keeps its "v"
curl -fsSL -O "${BASE}/binder_${VERSION}_${OS}_${ARCH}.tar.gz"
curl -fsSL -O "${BASE}/checksums.txt"
shasum -a 256 --ignore-missing -c checksums.txt   # or: sha256sum --ignore-missing -c checksums.txt
tar -xzf "binder_${VERSION}_${OS}_${ARCH}.tar.gz"
sudo mv binder /usr/local/bin/
binder --version    # binder/<version> — the stamp never carries a leading "v"
```

Each archive also contains `LICENSE` and `README.md`, and nothing else: no
docs directory, no sample corpus. Cosign signatures and SBOMs are **not**
published yet, so `checksums.txt` is the integrity artifact today; see
[what is deferred, and why](docs/RELEASING.md#deferred-phase-4).

**Go toolchain** (requires Go 1.26.1+, the floor declared in `go.mod`):

```bash
go install github.com/ghchinoy/binder@latest
```

`go install` stamps the binary cleanly, exactly like Homebrew and the direct
download: `binder --version` prints `binder/0.3.0`, no leading `v`.

> **Winget**: not published yet (tracked in
> [#40](https://github.com/ghchinoy/binder/issues/40)). Windows users: take the
> release `.zip` above.

**From source.** For anyone who prefers building their own binaries, and the way
to run a commit that has not been released yet: clone the repo and run
`make build` with Go 1.26.1+. The resulting binary reports a Go module
pseudo-version rather than a release version;
[CONTRIBUTING.md](CONTRIBUTING.md#development) covers what that affects, and is
where to start if you want to change binder rather than just run it.

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

# Propose a --type-map from the corpus (proposal only; writes nothing).
binder infer path/to/corpus
```

```text
bundle: path/to/bundle
concepts: 2, reserved files: 1
scope: reserved-file structure (index.md, log.md) not validated; verdict covers concept files only
RESULT: conformant (OKF 0.2)
```

The `scope:` line appears whenever the bundle has reserved files, because
structural validation of `index.md`/`log.md` is not implemented in this release.
A `conformant` verdict covers the concept files only. It is not a claim about a
surface the validator never examined.

### Agent-ready machine-readable output (`--json`) and exit codes

`convert`, `enrich`, `validate`, `review`, `lint`, `infer`, and `graph` accept
`--json` for scripting and CI; `binder config` (and `config list`/`get`/`set`/`unset`)
does too, with its own `binder.config/v1` schema. `index` and `mcp` have no
`--json` flag. Prose is the default and is byte-unchanged when `--json` is absent.

`convert`, `enrich`, `validate`, `review`, `lint`, and `infer` wrap their existing report
in a thin, deterministic envelope (schema `binder.report/v1`) — same field names every
run, 2-space indent, a stable field order, a trailing newline, and any
`SOURCE_DATE_EPOCH` honoured, so two runs on the same input are byte-identical:

```bash
binder validate path/to/bundle --json
```

```json
{
  "binder": "binder/0.3.0",
  "command": "validate",
  "schema": "binder.report/v1",
  "result": {
    "root": "path/to/bundle",
    "num_concepts": 2,
    "num_reserved": 1,
    "findings": [],
    "reserved_structure_checked": false
  }
}
```

The `binder` field is whatever version *your* binary reports (`binder --version`);
the stamp never carries a leading `v`. The field order is stable but **not**
alphabetical — it is the order the fields are declared in, so the envelope is
always `binder`, `command`, `schema`, `result`. Only *map*-valued objects
(`review`'s `by_type` and `tiers`, `config`'s `values`) have their keys sorted. A
consumer that re-sorts the output and then compares bytes will not match.

`graph` is already machine-readable, so `graph --json` is an **alias for
`--format json`**, the raw `{nodes, edges}` export, **not** the envelope above.
Combining `--json` with a conflicting `--format {dot,graphml,html}` is a usage
error (exit 2).

Every command maps its outcome onto a stable **exit-code contract** (identical in
prose and `--json` mode):

| Code | Meaning |
|---|---|
| `0` | Success. Advisories (broken links, true orphans, staleness, recovered frontmatter, missing trust, non-conformant `status` values) may be present — they are reported but never gate unless `--strict` is set. Entrypoints are reported and **never** gate. |
| `1` | Gating findings: `validate` spec §11 non-conformance (always), or, under `--strict`, the per-command advisory/finding set (see [Strict mode](docs/user_guide.md#strict-mode)). |
| `2` | Usage error — anything wrong with the command line **or with the config file that feeds it**. Unknown subcommand (`binder bogus`); unknown flag; wrong number of positional arguments; `convert` with neither `-o` nor `--dry-run`; `--json` conflicting with `--format`; an unknown `graph --format`; a malformed `--today`, `--verified-by`, `--type-map`, `--status-map`, `--stale-after-map`, or empty `--external-root` value; `enrich --overwrite-keys` naming a protected trust-provenance key (the whole run is refused and nothing is written); an unknown `config` key; and an unreadable `<corpus>`/`<src>` for `lint`, `enrich`, and `infer`. A bad value in `.binder.yaml` fails the same way, before the command runs — e.g. `verified_by: "agent:bot"` makes *every* subcommand exit 2. |
| `3` | I/O or internal error — an unreadable bundle or source for `convert`, `validate`, `index`, `review`, and `graph`, or a write failure. |

**Never-reject governs corpus content. The command line has its own contract.**
binder never refuses a corpus for being imperfect: a well-formed bundle with
broken links or orphans still exits `0`. A malformed *flag value* is a different
act, and binder does refuse it with exit `2` rather than silently computing
against a value you did not mean. See the [user guide](docs/user_guide.md) for
the per-command field lists, the discovery surface (`--version`/`--help`), and a
CI example.

### Command reference

The examples above are the quickstart. The full per-command reference (every
flag, the precise semantics, and worked end-to-end examples) lives in the
**[user guide](docs/user_guide.md)**:

- **[`convert`](docs/user_guide.md#convert)** — all flags, plus
  [`file://` link resolution](docs/user_guide.md#file-link-resolution) and
  [known sibling roots (`--external-root`)](docs/user_guide.md#declaring-known-sibling-roots---external-root).
- **[`enrich`](docs/user_guide.md#enrich)** — the in-place safety model and
  [opt-in refresh (`--overwrite-keys`)](docs/user_guide.md#opt-in-refresh---overwrite-keys).
- **[`index` and the type-grouped catalog](docs/user_guide.md#the-type-grouped-catalog)** —
  `--group-by-type`, `--include-backlinks`, `--include-graph`.
- **[`review`](docs/user_guide.md#review)** and **[`lint`](docs/user_guide.md#lint)** —
  the entrypoint/orphan rule and the [anchor-slug convention](docs/user_guide.md#lint).
- **[`infer`](docs/user_guide.md#infer)** — the `--type-map` proposal idiom and the Gemini tier.
- **[`config`](docs/user_guide.md#config)** — precedence, keys, and subcommands.
- **[Relationship extraction](docs/user_guide.md#relationship-extraction)** and the
  **[trust vocabulary](docs/user_guide.md#the-trust-vocabulary)**, including
  [declarative trust & lifecycle flags](docs/user_guide.md#declarative-trust--lifecycle-flags)
  and [status vocabulary & `--canonicalize-status`](docs/user_guide.md#status-vocabulary-and---canonicalize-status).
- **[Strict mode](docs/user_guide.md#strict-mode)** and
  **[malformed-input recovery](docs/user_guide.md#malformed-input-recovery)**.


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
[Installation](/binder/install/)) — it drives the CLI, it does not embed it.

**Usage walkthrough.** A tiny sample corpus ships at
[`plugins/okf-convert/skills/okf-convert/assets/sample-corpus/`](plugins/okf-convert/skills/okf-convert/assets/sample-corpus/)
with three deliberate triage cases (an unresolved link, a missing-title file, a
no-frontmatter file). Following the skill, an agent drives:

```bash
cd plugins/okf-convert/skills/okf-convert/assets/sample-corpus

# 3. Dry-run triage — reason over structured output, never scrape prose
binder convert . --dry-run --json | jq '.result | {num_concepts, num_unresolved, num_recovered}'
binder lint . --json | jq '.result | {broken_links, missing_titles, schema_violations}'

# 4. Remediate the source frontmatter (additive, lossless; preview first)
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
server, exposing **seven** MCP **tools** to an MCP-capable agent harness (Claude
Code, Cursor, Zed): binder's **additive** verbs `convert`, `validate`, `review`,
`lint` and `graph`, plus the **read-only** graph tools `list_graphs` and
`query_graph`. Each report-producing tool returns the **same** deterministic
`binder.report/v1` payload as the corresponding `binder <cmd> --json`: the
handlers reuse the same internal functions and the same JSON encoder, so there is
no second serialization path and no drift from the CLI.

It is a transport. It produces no report of its own and has no `--json` flag
(its *outputs* are the structured tool payloads). It serves over stdio until the
client disconnects.

> **Parity, in brief:** each MCP tool mirrors its CLI verb one-to-one; the
> exact per-parameter parity and its output-routing exceptions are in the
> [user guide](docs/user_guide.md#mcp-server-binder-mcp).

**Wire it into a harness** (Claude Code):

```bash
claude mcp add binder -- binder mcp
```

…or add an `.mcp.json` entry:

```json
{ "mcpServers": { "binder": { "command": "binder", "args": ["mcp"] } } }
```

**Tools** (for the five verbs with a CLI counterpart, each parameter mirrors the
corresponding CLI flag or positional argument 1:1, `graph`'s `format` **default**
excepted — `dot` on the CLI, `json` here; the MCP-only `list_graphs` and
`query_graph` have no CLI flags to mirror):

| Tool | Key params | Returns |
|---|---|---|
| `convert` | `src` (req), `out` (req unless `dry_run`), `dry_run`, `default_type`, `type_map`, `fm_ref_keys`, `source_keys`, `map_citations`, `map_draft`, `status_map`, `canonicalize_status`, `stale_after_map`, `verified_by`, `workspace_root`, `external_root` (repeatable), `group_by_type`, `include_backlinks`, `include_graph`, `strict` | `convert` report envelope (`dry_run:true` → the ingestion-analysis preview, writes nothing) |
| `validate` | `bundle` (req), `strict` | `validate` report envelope |
| `review` | `bundle` (req), `entrypoints` (array of concept id or path — the parity param for the repeatable `--entrypoint`), `today`, `strict` | `review` report envelope |
| `lint` | `src` (req), `entrypoints` (array of concept id or path — the parity param for the repeatable `--entrypoint`), `today`, `strict` | `lint` report envelope |
| `graph` | `bundle` (req), `format` (`dot`\|`json`\|`graphml`\|`html`, default `json`), `today` | raw export bytes — `format:json` is the raw `{nodes,edges}`, **not** the report envelope |
| `list_graphs` | `bundle` (req), `today`, `id_key` | `list_graphs` report envelope — the LPG **schema descriptor** (graph name, node labels = concept types, the single `LINKS` edge label, each with counts + property declarations). Read-only introspection derived from the same projection as `graph` |
| `query_graph` | `bundle` (req), `op` (req: `lookup`\|`neighbors`\|`neighborhood`\|`pattern`\|`path`), `today`, `id_key`, `id`, `label`, `direction` (`out`\|`in`\|`both`, default `out`), `rel`, `depth` (`1..5`, required for `neighborhood`), `to_label`, `where` (`{prop, eq}`; `prop` ∈ `type`\|`tier`\|`stale`), `from`/`to`/`max_depth` (`1..5`, all required for `path`) | `query_graph` report envelope — bounded read-only traversal of the same projection. `additionalProperties: false` |

**`query_graph` details worth knowing before you call it.** Every response echoes
a `node_key` object; `id_key` is accepted **for parity with `list_graphs` only
and is never honored** in this version, so the echo is always
`{"strategy":"path","key":"…","honored":false}` and traversal identity is always
the path-derived concept id. A query that matches nothing comes back as a
**result** (`isError:false`, `not_found:true`); an unknown `op` or an
out-of-range `depth`/`max_depth` is a **tool error** (`isError:true`, plain text
rather than the envelope). Every traversal is depth-bounded by construction.

The surface is deliberately additive (produce/validate) plus **read-only** graph
introspection and traversal. Source-mutating verbs (`enrich`, `emit_concept`) are
**not** exposed — authoring over MCP is a later concern — and neither is `infer`,
which is proposal-only and can call out to a model. General read/search over a
knowledge store remains out of scope; `list_graphs` and `query_graph` read the
bundle binder itself produced.
Invariants are preserved end to end: findings are returned **in** the
payload (a tool with findings is not an MCP error), `verified_by` is applied
**only** when explicitly passed (never auto-stamped; an invalid actor is a
usage error), and payloads honor `SOURCE_DATE_EPOCH`/`today` for determinism.

The [`okf-convert` plugin](/binder/agent/plugin/) also ships a
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
  (`yaml.Node`) for order-preserving frontmatter. Unmodified frontmatter is
  re-emitted from the original source verbatim, which is how this codec stays
  lossless over the trust and Attested-Computation families it does not model
  well enough to re-encode; nested-map key order survives with it. Losslessness
  is the bar; byte-identity is an internal property of this codec.
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
  injected once at the composition root (`cmd/root.go`). Every other package
  depends only on the `okf` interfaces.

## Roadmap

**Shipped today** (the complete v0.3.0 surface, checkable against
`binder --help`):

- the CLI verbs `convert`, `validate`, `index`, `review`, `lint`, `infer`,
  `graph`, `config`, and `enrich`, plus the stdio
  [MCP server](#mcp-server-binder-mcp) (`binder mcp`);
- the `okf-convert` [Agent Skill / Plugin](#agent-skill--plugin);
- the declarative trust/lifecycle flags `--status-map`, `--stale-after-map`,
  and `--verified-by`;
- `--strict` gating, `--canonicalize-status`, `convert --external-root`,
  `--entrypoint` on `review` and `lint`, and `enrich --overwrite-keys`.

**Planned, not yet shipped** (what binder does *not* do today):

- **A community-core codec adapter** (e.g. `--okf-impl=community`): a second
  `Codec` behind the existing interface, slotted in only after it is confirmed
  byte-complete against the golden bundles.
- **Structural validation of the reserved files** `index.md` and `log.md`.
  `validate` checks concept files only and prints a `scope:` line saying so
  (see [Usage](#usage)).
- **Cosign signatures, SBOMs, and a `winget` package** for releases;
  `checksums.txt` is the integrity artifact today (see
  [Installation](#installation)).

The [user guide](docs/user_guide.md#roadmap--planned-features) maps each shipped
feature to the issue it came from;
[CONTRIBUTING.md](CONTRIBUTING.md#feature-history-and-sequencing) records how
that surface was sequenced.

## Contributing

Pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) covers how to
build binder from a clone, run `make check`, and how the shipped surface was
sequenced; the release process and the project's full exit gate are in
[docs/RELEASING.md](docs/RELEASING.md).

## License

Licensed under the [Apache License 2.0](LICENSE).
