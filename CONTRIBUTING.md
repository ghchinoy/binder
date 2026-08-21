# Contributing to binder

Pull requests are welcome. For major changes, please open an issue first to
discuss what you'd like to change. Make sure `make check` passes and add tests
for new behavior.

*Using* binder is documented in the [README](README.md), the
[tutorial](docs/tutorial.md), and the [user guide](docs/user_guide.md). Using
binder needs nothing but an installed binary; you only need a clone to get the
sample corpus those walkthroughs convert, which ships in this repo (under
`plugins/okf-convert/skills/okf-convert/assets/sample-corpus/`) and **not** in
the release archive: a release archive holds the binary, `LICENSE`, and
`README.md`, nothing else. This file is for changing binder itself.

## Development

```bash
git clone https://github.com/ghchinoy/binder.git
cd binder
make build        # -> bin/binder
make check        # gate: gofmt + go vet + go test ./...
```

Requires **Go 1.26.1+** (the floor declared in `go.mod`). Dependency pinning and
the module-proxy requirement are described under
[Reproducible build invariants](docs/RELEASING.md#reproducible-build-invariants).
`make check` is the toolchain-only gate; the project's full exit gate adds an
external cross-check, described under
[Differential-validation exit gate](docs/RELEASING.md#differential-validation-exit-gate).
The release process itself is [docs/RELEASING.md](docs/RELEASING.md).

## Command reference (generated, drift-gated)

The CLI command reference under [`docs/commands/`](docs/commands/) is **generated
from binder's own Cobra command tree** (never hand-edited), so it cannot assert
a flag the binary does not have. The generator is `cmd/gendocs` (it walks the
live command tree via `github.com/spf13/cobra/doc` and writes deterministic
markdown), and the generation logic is shared with the drift test in
`internal/gendocs`.

After you add or change a command or flag, regenerate and commit the reference:

```bash
make docs         # regenerates docs/commands/ from the command tree
git add docs/commands
```

`make check` runs `internal/gendocs`'s **drift test**, which regenerates into a
temp dir and asserts byte-equality with the committed `docs/commands/`. If you
add a flag without running `make docs`, the drift test — and therefore CI —
fails until you regenerate and commit. `make docs` is deterministic (no
timestamps, no host paths, sorted), so running it twice yields no diff.

`make build` is a plain `go build` and passes no `-ldflags`, so the binary it
produces reports a Go module pseudo-version rather than a release version. That
matters, because that string is stamped into every converted concept's
`generated.by` provenance. Day to day that is fine; nothing about developing
binder needs a pinned version. When you do need to rehearse the release stamp,
follow
[How the version reaches the binary](docs/RELEASING.md#how-the-version-reaches-the-binary-single-source-the-tag),
which carries the authoritative versioned build command and explains why the
canonical stamp has no leading `v`.

## Feature history and sequencing

What is planned but not yet shipped, and today's shipped surface, are stated in
the [user guide roadmap](docs/user_guide.md#roadmap--planned-features). This section records how that surface was
sequenced and which issue each feature came from.

**Shipped, layered over the settled CLI contract:** the
[Agent Skill and Agent-Plugin bundle](/binder/agent/plugin/)
(`okf-convert`, #14), then the
[MCP server mode](/binder/agent/mcp/) (`binder mcp`, #15) — the
additive convert/validate/review/lint/graph tools over the same OKF core, since
joined by the read-only graph tools `list_graphs`
([#32](https://github.com/ghchinoy/binder/pull/32)) and `query_graph`
([#33](https://github.com/ghchinoy/binder/issues/33)). (These were sequenced
Skill/Plugin **before** MCP, so MCP builds on already-settled `--json` payloads.)
Declarative trust/lifecycle flags (`--status-map`, `--stale-after-map`,
`--verified-by`; #7), `binder config` (#10), the standalone `binder lint` (#8),
in-place `binder enrich` (#5), the type-map proposer `binder infer` (#38), and
`--strict` mode have also shipped. The v0.3.0 cycle added the
opt-in `--canonicalize-status` (#23), `convert --external-root` (#25), the
entrypoint-versus-orphan reclassification with `--entrypoint` on `review` and
`lint` (#24), and `enrich --overwrite-keys` (#22). The
[user guide](docs/user_guide.md#roadmap--planned-features) maps each to its issue.
