# binder tutorial

A hands-on, task-oriented walkthrough. You will ingest an existing corpus into
an OKF v0.2 bundle, inspect and gate it, then author a fresh corpus and stamp its
lifecycle and verification metadata. Every command below runs against shipped
binder, and the output shown is real.

For the concise landing page see the [README](../README.md); for the exhaustive
per-flag reference see the [user guide](user_guide.md). This tutorial is the
guided path between them.

## Before you start

Install binder with Homebrew:

```bash
brew install ghchinoy/tap/binder
```

…or build it from source (requires Go 1.26.1+, the floor declared in `go.mod`):

```bash
git clone https://github.com/ghchinoy/binder.git
cd binder
make build          # -> bin/binder
export PATH="$PWD/bin:$PATH"
```

Dependencies are pinned via `go.mod`/`go.sum` and fetched from the Go module
proxy at build time (network required). Either way, check the binary:

```bash
binder --version
```

```text
binder/<version>
```

A Homebrew or direct-download install of v0.3.0 prints exactly `binder/0.3.0`.
The banner is always `binder/<version>` and **never** carries a leading `v`. This
is not cosmetic: it is the exact string binder stamps into every concept's
`generated.by`, so the value you see here is the value that will show up in your
bundles.

A source build prints something longer, like
`binder/0.2.2-0.20260816074947-7f4ca6b4c816`. That is the Go module
pseudo-version, and the reason is the build command, not the clone: `make build`
runs a plain `go build` with **no `-ldflags`**, so nothing injects the release
version and binder falls back to what the module graph knows. A fully tagged
clone behaves identically. Install a release if you want a clean stamp, or pass
the ldflag yourself — see
[docs/RELEASING.md](RELEASING.md#how-the-version-reaches-the-binary-single-source-the-tag).

Two habits make every run in this tutorial reproducible:

- Pin the clock with `SOURCE_DATE_EPOCH` so any synthesised timestamp
  (`generated.at`, a resolved `stale_after`, a `verified.at`) is byte-stable.
- Read the structured `--json` output, not the prose, when you script or gate.

Throughout, keep two invariants in mind. `binder convert` never mutates its
source, and binder never fabricates trust: it derives trust tiers from
frontmatter, stamps an honest `generated` provenance for content it produced, and
never invents a source or auto-stamps `verified`.

## Part 1: brownfield, ingesting a corpus you already have

Brownfield means the knowledge already exists (a docs site, an Obsidian vault, a
folder of runbooks) and the job is to get it into OKF with its relationships
intact. The binder **repository** carries a small sample corpus with three
deliberate triage cases (an unresolved link, a file with no title, and files with
no frontmatter). Use it as the corpus for this part.

The corpus lives in the git repository, **not** in the release archive — a
Homebrew or direct-download install gives you the `binder` binary plus `LICENSE`
and `README.md`, and nothing else. So grab the repo, whichever way you installed:

```bash
git clone --depth 1 https://github.com/ghchinoy/binder.git /tmp/binder-src
CORPUS=/tmp/binder-src/plugins/okf-convert/skills/okf-convert/assets/sample-corpus
```

If you built from source above you already have the clone; point `CORPUS` at your
own checkout instead — every command below uses `"$CORPUS"`, so nothing else
changes. The pasted output shows the path from the shallow clone; yours will
differ in that one line.

### Step 1: triage with a dry run

Preview the conversion before anything lands on disk. A dry run reports the
concept count, the resolved and unresolved link counts, and any files whose
frontmatter had to be recovered:

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert "$CORPUS" --dry-run
```

```text
binder convert --dry-run (no files written)
  source: /tmp/binder-src/plugins/okf-convert/skills/okf-convert/assets/sample-corpus
  output: 
  concepts: 5
  links: 3 (resolved 2, unresolved 1)

Concepts:
  README.md  [type=Note]
  notes/scratch.md  [type=Note]
  topics/architecture.md  [type=Reference]
  topics/glossary.md  [type=Reference]
  topics/onboarding.md  [type=Playbook]  (1 unresolved links)

Unresolved links:
  topics/onboarding.md -> /topics/deploy.md
```

The one unresolved link is a fact about the corpus: `onboarding` points at a
`deploy.md` that does not exist. binder reports it and keeps it in place rather
than dropping it. You decide whether to fix the source or accept it.

### Step 2: lint the source as authored

`convert` supplies defaults (a missing `type` becomes `Note`, a missing title is
humanized from the filename), so some gaps are invisible once a bundle exists.
`binder lint` reads the corpus *as authored*, before those defaults are applied,
and writes nothing:

```bash
binder lint "$CORPUS"
```

```text
binder lint
  corpus: /tmp/binder-src/plugins/okf-convert/skills/okf-convert/assets/sample-corpus
  concepts: 5
  broken links: 1
    topics/onboarding -> /topics/deploy.md
  missing titles: 1
    topics/glossary
  schema violations: 2
    README: missing type
    notes/scratch: missing type
  entrypoints (no inbound links): 1
    README
  orphans (no inbound or outbound links): 2
    notes/scratch
    topics/glossary
  stale: 0
```

An orphan here is a concept with **no inbound and no outbound** resolved edge: a
document no reader will reach by following links, and one that leads nowhere.
Treat the orphan list as a to-do list for the corpus.

A concept with nothing pointing *at* it is an **entrypoint** rather than an
orphan when any of three things holds: it links out to something, it is a root
`README.md`, or you named it with `--entrypoint` (repeatable, and it tolerates a
trailing `.md`). `README` here qualifies on the **second** count: it is the
corpus-root `README.md`. It has no outbound edges of its own — every path it
mentions sits inside a code span or a fenced block, so none of them is a
markdown link — and renaming the file makes it a true orphan. binder reports
entrypoints separately and does **not** count them as findings, so a corpus
with a legitimate front door is not penalised for having one, and `--strict`
never gates on them.

One edge of that rule is worth knowing: "root" means *root*, and `README.md` is
the only name recognized this way. A nested `docs/README.md` is **not**
recognized automatically and stays a true orphan until you pass
`--entrypoint docs/README.md`, and a root `index.md` is not recognized in any
spelling — it is classified on its edges like any other concept (`convert`
renames an authored lowercase `index.md` to `index-note.md` first).

`binder review` applies the same rule to a bundle, but over a different graph:
`lint` reads the corpus **as authored**, `review` reads the **emitted bundle**,
and conversion happens in between. The two usually agree about what is an
orphan, but agreement is not guaranteed — `convert --fm-ref-keys related`, for
instance, materializes edges out of a frontmatter key that `lint` has no flag to
read, so a note `lint` calls an orphan can reach `review` as an entrypoint.

### Step 2.5: let binder propose a type map

The dry run typed almost everything `Note`, because that is the fallback when a
file declares no `type:`. `binder infer` reads the corpus and **proposes** a
`--type-map` from what it finds — directory names, filename patterns, and the
types files already declare. It is proposal-only and writes nothing:

```bash
binder infer "$CORPUS"
```

```text
notes=Note,topics=Reference
```

That single line is the whole prose output, which is what makes it composable.
Ask `--json` for the reasoning behind each mapping:

```bash
binder infer "$CORPUS" --json | jq '.result.mappings'
```

```json
[
  {
    "dir": "notes",
    "suggested_type": "Note",
    "source": "folder",
    "rationale": "inferred from directory name \"notes\"",
    "sample_files": [
      "notes/scratch.md"
    ]
  },
  {
    "dir": "topics",
    "suggested_type": "Reference",
    "source": "frontmatter",
    "rationale": "majority of files carry authored type \"Reference\"",
    "sample_files": [
      "topics/architecture.md",
      "topics/glossary.md",
      "topics/onboarding.md"
    ]
  }
]
```

Read the proposal before you use it — `infer` is a suggestion, not an authority.
When you agree with it, feed it straight into the conversion:

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert "$CORPUS" -o /tmp/tut-typed \
  --type-map "$(binder infer "$CORPUS")"
```

On *this* corpus that produces a bundle byte-identical to the plain conversion in
Step 3, because the proposal simply reproduces the types the files already
declare. `infer` earns its keep on a corpus where the types are implied by
structure rather than written down — a `docs/` tree of guides, an `adr/` tree of
decisions — where the alternative is hand-writing the map.

By default `infer` uses deterministic offline signals only, so it needs neither a
network nor an API key. `--gemini` opts into an extra semantic tier that calls a
Gemini model; that is the one place in binder where a model enters the loop, and
it is off unless you ask for it.

### Step 3: convert to a bundle

`convert` never touches the source. Write the bundle to a separate directory:

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert "$CORPUS" -o /tmp/tut-bundle
```

The report is the dry run's, with two differences: the header loses its
"(no files written)" banner, and `output:` now names the bundle directory. The
bundle has a root `index.md` declaring `okf_version: "0.2"`, per-directory
navigation, and one concept per source file.

### Step 4: review and validate the bundle

`review` summarizes the finished bundle. Pin the staleness date for a stable
report:

```bash
binder review /tmp/tut-bundle --today 2026-08-15
```

```text
binder review
  bundle: /tmp/tut-bundle
  concepts: 5
  by type:
    Note: 2
    Playbook: 1
    Reference: 2
  trust tiers:
    human-reviewed: 0
    machine-confirmed: 0
    unverified: 5
  stale (as of 2026-08-15): 0
  attested computations: 0
  unparsed frontmatter (recovered as body): 0
  entrypoints (no inbound links): 1
    README
  orphans (no inbound or outbound links): 2
    notes/scratch
    topics/glossary
  unresolved links: 1
    topics/onboarding -> /topics/deploy.md
```

`validate` checks the bundle's concept files against the OKF v0.2 §11
conformance rules. The only hard requirement is that every non-reserved concept
has a parseable frontmatter block with a non-empty `type`; everything else it
checks is an advisory:

```bash
binder validate /tmp/tut-bundle
echo "exit=$?"
```

```text
bundle: /tmp/tut-bundle
concepts: 5, reserved files: 3
scope: reserved-file structure (index.md, log.md) not validated; verdict covers concept files only
RESULT: conformant (OKF 0.2)
exit=0
```

The `scope:` line tells you how far to trust the verdict. The bundle's three
reserved files — the generated `index.md` in each directory — were counted, but
their structure was not examined, so `conformant` here is a claim about the
five concept files and nothing else. That line appears only when a bundle
contains reserved files; under `--json` the same fact is the
`reserved_structure_checked` field, which every result carries — always
`false` — regardless of the reserved count. It is a disclosure, not a
finding — it does not change the verdict or the exit code.

The bundle is conformant and exits `0` even though it still has broken links and
orphans. That is the never-reject posture: advisories are reported, not gated,
unless you ask for gating.

### Step 5: gate CI on JSON and exit codes

Every command maps its outcome onto a stable exit-code contract, identical in
prose and `--json` mode:

| Code | Meaning |
|---|---|
| `0` | Success. Advisories may be present but never gate unless `--strict` is set. |
| `1` | Gating findings: `validate` §11 non-conformance (always), or any advisory under `--strict`. |
| `2` | Usage error — anything wrong with the command line, **or with the `.binder.yaml` that feeds it**: an unknown subcommand or flag, the wrong number of arguments, an invalid actor, a malformed `--today`/`--type-map`/`--status-map`/`--stale-after-map` value, an unknown `graph --format`, and an unreadable corpus for `lint`/`enrich`/`infer`. |
| `3` | I/O or internal error — an unreadable bundle or source for `convert`/`validate`/`index`/`review`/`graph`, or a write failure. |

Never-reject governs the *corpus*, not the inputs that configure the run: binder
will not refuse your documents for being imperfect, but it will refuse a value it
cannot parse rather than quietly computing against something you did not mean.
That applies to the config file too — a bad `verified_by:` in `.binder.yaml` is
resolved before the subcommand runs, so *every* command exits `2` until you fix
it, even one that never uses the value.

`--json` wraps the report in a deterministic envelope (schema
`binder.report/v1`) with a stable field order and a trailing newline, so two runs
on the same input are byte-identical. The order is declaration order, not
alphabetical — only map-valued objects such as `by_type` have their keys sorted.
Gate on the source corpus before conversion with `lint --strict`, which turns
advisories into a non-zero exit:

```bash
binder lint "$CORPUS" --strict --json > /tmp/lint.json
echo "exit=$?"
jq '{broken_links: .result.broken_links, missing_titles: .result.missing_titles}' /tmp/lint.json
```

```text
exit=1
{
  "broken_links": [
    {
      "concept": "topics/onboarding",
      "detail": "/topics/deploy.md"
    }
  ],
  "missing_titles": [
    "topics/glossary"
  ]
}
```

Under `--strict`, binder also prints a one-line summary to stderr
(`binder: lint found 6 finding(s) (--strict)`); the JSON on stdout is unaffected.
That is 1 broken link + 1 missing title + 2 schema violations + 2 orphans — the
lone entrypoint is reported but not counted. A clean corpus exits `0` even with
`--strict` set, so the flag is safe to leave on permanently in CI.

`--strict` is available on `convert`, `enrich`, `validate`, `review`, `lint`, and
`infer`. Use `validate --strict` to gate on trust well-formedness in addition to
hard conformance, and `convert --strict` to gate on unresolved links or
recoveries.

### Multi-repo corpora: `--workspace-root`

When your corpus is one repository among sibling repositories and you want
`file://` links that point into those siblings treated as internal edges rather
than external references, widen the resolution boundary with `--workspace-root`:

```bash
binder convert repo-a/docs -o out --workspace-root /path/to/monorepo
```

The boundary defaults to the corpus root. A `file://` link that resolves inside
the boundary is rewritten to a bundle-relative edge, and no absolute machine path
leaks into the output. A link that points to another host, or escapes the root
through `..` or a symlink, stays external and is tolerated as an advisory. See the
[user guide](user_guide.md#file-link-resolution) for the full resolution rules.

## Part 2: greenfield, authoring OKF from the start

Greenfield means you are writing OKF fresh. The shipped path is `binder enrich`,
which adds the required frontmatter (`type`, `title`, `generated`) to a source
tree *in place*, frontmatter only, with no body rewriting. Because it mutates the
source, run it on a clean git tree and review the diff.

### Step 1: create a small corpus

Work somewhere outside any binder checkout — including the shallow clone Part 1
made — because `enrich` mutates the tree it is pointed at and the advice above is
to run it on a clean git tree:

```bash
cd /tmp        # anywhere outside a binder checkout

mkdir -p kb/drafts
cd kb

cat > payments.md <<'EOF'
# Payments

How we process customer payments. See [refunds](refunds.md).
EOF

cat > refunds.md <<'EOF'
---
type: Playbook
title: Refunds
---
# Refunds

Steps to issue a refund. Back to [payments](payments.md).
EOF

cat > drafts/idea.md <<'EOF'
# Loyalty program idea

Rough notes, not ready yet.
EOF
```

`payments.md` and `drafts/idea.md` have no frontmatter; `refunds.md` already
declares a `type` and `title`.

### Step 2: preview the enrichment

```bash
SOURCE_DATE_EPOCH=1700000000 binder enrich . --dry-run
```

```text
enrich .
(dry run — no files written)
3 file(s): 3 would enrich, 0 unchanged, 0 skipped
  would enrich drafts/idea.md (add: generated, title, type)
  would enrich payments.md (add: generated, title, type)
  would enrich refunds.md (add: generated)
```

`refunds.md` only needs `generated`: its authored `type` and `title` are already
present and will be left untouched. This is the additive, never-clobber rule.

### Step 3: enrich with status and verification

Stamp lifecycle and verification metadata as you enrich. `--status-map` assigns
`status` by source directory (longest directory prefix wins; `default=` is the
fallback), and `--verified-by` appends a `verified` actor stamp. Both are set only
where the field is absent:

```bash
SOURCE_DATE_EPOCH=1700000000 binder enrich . \
  --status-map "drafts=draft,default=stable" \
  --verified-by "human:alice"
```

```text
enrich .
3 file(s): 3 enriched, 0 unchanged, 0 skipped
  enriched drafts/idea.md (added: generated, status, title, type, verified)
  enriched payments.md (added: generated, status, title, type, verified)
  enriched refunds.md (added: generated, status, verified)
```

Look at the drafts file: it received `status: draft` from the `drafts=` prefix,
`title` derived from its `# H1`, a `Note` type, and the `verified` and `generated`
stamps:

```bash
cat drafts/idea.md
```

```text
---
type: Note
title: Loyalty program idea
status: draft
verified:
  - at: "2023-11-14T22:13:20Z"
    by: human:alice
generated:
  at: "2023-11-14T22:13:20Z"
  by: binder/0.3.0
---

# Loyalty program idea

Rough notes, not ready yet.
```

`payments.md` and `refunds.md` got `status: stable` from the `default=` fallback.
The `generated.at` and `verified.at` timestamps come from `SOURCE_DATE_EPOCH`, so
the run is reproducible.

### Step 4: confirm it is idempotent

A second identical run finds every key present and writes nothing, so there is no
git churn:

```bash
SOURCE_DATE_EPOCH=1700000000 binder enrich . \
  --status-map "drafts=draft,default=stable" \
  --verified-by "human:alice"
```

```text
enrich .
3 file(s): 0 enriched, 3 unchanged, 0 skipped
```

### Step 5: stop retyping the actor — `binder config`

You have now typed `--verified-by "human:alice"` twice. `binder config set`
persists it so every later run picks it up:

```bash
binder config set verified_by "human:alice"
```

```text
Set verified_by = "human:alice" in .binder.yaml
```

**There is no `--local` flag.** Local is the default: `config set` writes
`./.binder.yaml` in the current directory, which is the repository-scoped file
you commit alongside the corpus. `-g`/`--global` is the opt-out, writing
`$XDG_CONFIG_HOME/binder/config.yaml` (`~/.config/binder/config.yaml` by default)
instead. `binder config set --local …` fails with `unknown flag: --local` and
exit `2`.

`binder config` with no arguments lists every resolved value **with the source it
came from** — the fastest way to explain a surprising run, because the precedence
is flag > env > config file > built-in default:

```bash
binder config
```

```text
binder config
  config file: .binder.yaml
  default_type: "Note" (source: default)
  verified_by: "human:alice" (source: file)
  gemini_model: "gemini-3.5-flash-lite" (source: default)
  gemini_location: "global" (source: default)
  gemini_project: "" (source: default)
  gemini_backend: "auto" (source: default)
```

Now add a new document and enrich **without** passing `--verified-by` at all:

```bash
cat > pricing.md <<'EOF'
# Pricing

How we price. See [payments](payments.md).
EOF

SOURCE_DATE_EPOCH=1700000000 binder enrich .
```

```text
enrich .
4 file(s): 1 enriched, 3 unchanged, 0 skipped
  enriched pricing.md (added: generated, title, type, verified)
```

```bash
cat pricing.md
```

```text
---
type: Note
title: Pricing
verified:
  - at: "2023-11-14T22:13:20Z"
    by: human:alice
generated:
  at: "2023-11-14T22:13:20Z"
  by: binder/0.3.0
---

# Pricing

How we price. See [payments](payments.md).
```

The actor came from the config file, not the command line. This does **not**
weaken never-fabricate-trust: binder still stamps `verified` only because *you*
configured an actor. With no flag and no configured `verified_by`, nothing is
written.

`config get` prints one resolved value (handy in scripts), and `config unset`
reverts a key to its default:

```bash
binder config get verified_by
binder config unset verified_by
```

```text
human:alice
Unset verified_by in .binder.yaml (reverted to default)
```

The other keys are `default_type`, `gemini_model`, `gemini_location`,
`gemini_project`, and `gemini_backend`; dotted spellings like `gemini.project`
are accepted and written back as snake_case. An unknown key is a usage error
(exit `2`).

### The status vocabulary and the actor convention

Two conventions keep a greenfield bundle honest.

Keep `status` in the OKF §5.4 vocabulary: `draft`, `stable`, or `deprecated`.
The `--status-map` you passed above is already conformant, so it ran silently.
Had you written `--status-map "drafts=wip,default=stable"`, `enrich` would have
checked the value at **input** time — the check is always on — written it
unchanged, and told you so:

```text
  status: status value "wip" (from --status-map key "drafts") is not one of draft|stable|deprecated (OKF §5.4); wrote it unchanged — pass --canonicalize-status to map it to "draft"
```

(`convert` reports the same messages under a `Status vocabulary (OKF §5.4):`
heading; `enrich` folds them into its per-file list.)

binder still does not reject the value: bare `enrich` exits `0`. But
`--strict` turns it into a gate (exit `1`) that fires **before** anything is
written, and `--canonicalize-status` opts into rewriting the handful of known
aliases (`active`→`stable`, `wip`/`in-progress`→`draft`,
`archived`/`legacy`→`deprecated`) so the gate has nothing left to catch. Both
flags exist on `convert` as well. See
[Status vocabulary](../README.md#status-vocabulary-and---canonicalize-status).

Attest with a real actor. `--verified-by` requires the actor convention:
`human:<id>`, `process:<id>`, `team:<id>`, or `<producer>/<version>` such as
`binder/0.3.0`. There is no `agent:` form. An invalid actor is a usage error and
exits `2`:

```bash
binder enrich . --verified-by "agent:bot"
echo "exit=$?"
```

```text
binder: invalid actor "agent:bot"; valid forms: human:<id>, process:<id>, team:<id>, or <producer>/<version> (e.g. binder/0.3.0)
exit=2
```

### Never fabricate trust

This is the load-bearing invariant. binder derives a trust tier from the
`verified` signals in the frontmatter (`human:` verification yields
`human-reviewed`, other verification yields `machine-confirmed`, none yields
`unverified`); it never stores a credibility score. It stamps an honest
`generated: binder/<version>` for content it produced, and it never auto-stamps
`verified`: with no `--verified-by` and no configured `verified_by`, no
verification is written. When an agent drives binder, the same rule binds the
agent: do not stamp trust you cannot assert.

## Which surface: CLI, Agent Skill, or MCP

The CLI you used above is the deterministic core. Two other surfaces ship today
and rest on the same `binder.report/v1` payloads; choose by the integration depth
you want.

- **CLI.** Deterministic at runtime, with no model in the loop by default: every
  command runs without a network or an API key, the sole exception being the
  opt-in `binder infer --gemini` semantic tier. Reach for it for batch ingestion,
  pipelines, and any gate that must run unattended. It is the foundation the
  other two build on. (Runtime only — *building* binder still needs network to
  fetch its pinned modules from the Go module proxy.)
- **Agent Skill / Plugin (`okf-convert`).** A skill teaches an agent harness you
  already run (Claude Code, Cursor, Zed) how to drive the CLI for judgment-laden
  work: reading a dry-run triage and deciding remediate-versus-accept, choosing
  conversion flags, reading the trust-extraction review. Install it from binder's
  self-hosted marketplace: `/plugin marketplace add ghchinoy/binder`, then
  `/plugin install okf-convert`. It assumes the `binder` binary is on your `PATH`.
  See [Agent Skill / Plugin](../README.md#agent-skill--plugin).
- **MCP server (`binder mcp`).** Runs binder as a stdio MCP server. It registers
  **seven** tools: the additive verbs `convert`, `validate`, `review`, `lint` and
  `graph`, which return the same payloads as `binder <cmd> --json`, plus the two
  read-only graph tools `list_graphs` (schema introspection) and `query_graph`
  (traversal). It is a transport, not a report-producing command, and the surface
  stays deliberately narrow: source-mutating verbs such as `enrich` are not
  exposed, and neither is `infer`, which is proposal-only and may call out to a
  model. Wire it into a host with
  `claude mcp add binder -- binder mcp`, or let the `okf-convert` plugin's bundled
  `.mcp.json` register it on install. See
  [MCP server](../README.md#mcp-server-binder-mcp).

The through-line: binder keeps the mechanical, reproducible work in a tool you can
audit, and leaves the semantics to you or your agent. That boundary holds
whichever surface you reach for.

## Where to go next

- The [user guide](user_guide.md) documents every command, flag, and the full
  trust vocabulary.
- [CI usage](user_guide.md#ci-usage) shows a complete convert-validate-review
  pipeline with `--json` and the exit-code contract.
- [Relationship extraction](user_guide.md#relationship-extraction) covers
  wikilinks, frontmatter refs, hashtags, and `file://` resolution in depth.
