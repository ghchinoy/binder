# binder tutorial

A hands-on, task-oriented walkthrough. You will ingest an existing corpus into
an OKF v0.2 bundle, inspect and gate it, then author a fresh corpus and stamp its
lifecycle and verification metadata. Every command below runs against shipped
binder, and the output shown is real.

For the concise landing page see the [README](../README.md); for the exhaustive
per-flag reference see the [user guide](user_guide.md). This tutorial is the
guided path between them.

## Before you start

Build the binary and put it on your `PATH` (requires Go 1.26+):

```bash
git clone https://github.com/ghchinoy/binder.git
cd binder
make build          # -> bin/binder
export PATH="$PWD/bin:$PATH"
binder --version    # binder/0.1.0
```

Dependencies are pinned via `go.mod`/`go.sum` and fetched from the Go module
proxy at build time (network required).

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
intact. binder ships a small sample corpus with three deliberate triage cases (an
unresolved link, a file with no title, and files with no frontmatter). Use it as
the corpus for this part:

```bash
CORPUS=plugins/okf-convert/skills/okf-convert/assets/sample-corpus
```

### Step 1: triage with a dry run

Preview the conversion before anything lands on disk. A dry run reports the
concept count, the resolved and unresolved link counts, and any files whose
frontmatter had to be recovered:

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert "$CORPUS" --dry-run
```

```text
binder convert --dry-run (no files written)
  source: plugins/okf-convert/skills/okf-convert/assets/sample-corpus
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
  corpus: plugins/okf-convert/skills/okf-convert/assets/sample-corpus
  concepts: 5
  broken links: 1
    topics/onboarding -> /topics/deploy.md
  missing titles: 1
    topics/glossary
  schema violations: 2
    README: missing type
    notes/scratch: missing type
  orphans: 3
    README
    notes/scratch
    topics/glossary
  stale: 0
```

An orphan here is a concept with no inbound and no outbound resolved edge: a
document no reader will reach by following links. Treat the orphan list as a
to-do list for the corpus.

### Step 3: convert to a bundle

`convert` never touches the source. Write the bundle to a separate directory:

```bash
SOURCE_DATE_EPOCH=1700000000 binder convert "$CORPUS" -o /tmp/tut-bundle
```

The report is identical to the dry run, minus the "no files written" banner. The
bundle now has a root `index.md` declaring `okf_version: "0.2"`, per-directory
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
  orphans (no inbound links): 3
    README
    notes/scratch
    topics/glossary
  unresolved links: 1
    topics/onboarding -> /topics/deploy.md
```

`validate` checks the bundle against the OKF v0.2 §11 conformance rules. The only
hard requirement is that every non-reserved concept has a parseable frontmatter
block with a non-empty `type`; everything else is an advisory:

```bash
binder validate /tmp/tut-bundle
echo "exit=$?"
```

```text
bundle: /tmp/tut-bundle
concepts: 5, reserved files: 3
RESULT: conformant (OKF 0.2)
exit=0
```

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
| `2` | Usage error (unknown flag, missing argument, invalid actor). |
| `3` | I/O or internal error. |

`--json` wraps the report in a deterministic envelope (schema
`binder.report/v1`) with sorted keys and a trailing newline, so two runs on the
same input are byte-identical. Gate on the source corpus before conversion with
`lint --strict`, which turns advisories into a non-zero exit:

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
(`binder: lint found 7 finding(s) (--strict)`); the JSON on stdout is unaffected.
A clean corpus exits `0` even with `--strict` set, so the flag is safe to leave on
permanently in CI.

`--strict` is available on `convert`, `enrich`, `validate`, `review`, and `lint`.
Use `validate --strict` to gate on trust well-formedness in addition to hard
conformance, and `convert --strict` to gate on unresolved links or recoveries.

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

```bash
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
  by: binder/0.1.0
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

### The status vocabulary and the actor convention

Two conventions keep a greenfield bundle honest.

Keep `status` in the OKF §5.4 vocabulary: `draft`, `stable`, or `deprecated`.
binder does not reject an unfamiliar value, but it surfaces one as a `validate`
advisory, and `validate --strict` turns that advisory into a CI gate.

Attest with a real actor. `--verified-by` requires the actor convention:
`human:<id>`, `process:<id>`, `team:<id>`, or `<producer>/<version>` such as
`binder/0.1.0`. There is no `agent:` form. An invalid actor is a usage error and
exits `2`:

```bash
binder enrich . --verified-by "agent:bot"
echo "exit=$?"
```

```text
binder: invalid actor "agent:bot"; valid forms: human:<id>, process:<id>, team:<id>, or <producer>/<version> (e.g. binder/0.1.0)
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

- **CLI.** Deterministic, offline, no model in the loop. Reach for it for batch
  ingestion, pipelines, and any gate that must run without a network or an API
  key. It is the foundation the other two build on.
- **Agent Skill / Plugin (`okf-convert`).** A skill teaches an agent harness you
  already run (Claude Code, Cursor, Zed) how to drive the CLI for judgment-laden
  work: reading a dry-run triage and deciding remediate-versus-accept, choosing
  conversion flags, reading the trust-extraction review. Install it from binder's
  self-hosted marketplace: `/plugin marketplace add ghchinoy/binder`, then
  `/plugin install okf-convert`. It assumes the `binder` binary is on your `PATH`.
  See [Agent Skill / Plugin](../README.md#agent-skill--plugin).
- **MCP server (`binder mcp`).** Runs binder as a stdio MCP server, exposing the
  additive verbs (convert, validate, review, lint, graph) as MCP tools that return
  the same payloads as `binder <cmd> --json`. It is a transport, not a
  report-producing command, and it stays additive: source-mutating verbs and
  read/search tools are not exposed. Wire it into a host with
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
