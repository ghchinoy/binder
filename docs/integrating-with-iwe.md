# Integrating with iwe

A hands-on mini tutorial. You will take an existing plain-markdown corpus,
convert it to an OKF v0.2 bundle with binder, and query that bundle as a
knowledge graph with [iwe](https://github.com/iwe-org/iwe) — no adapter, no
import step. Then you will walk the seam between the two tools honestly: what
carries across, what does not, and the ways **provenance gets rewritten,
relocated, or quietly outlived** when iwe writes to a bundle binder produced.

For the binder side on its own, see the [tutorial](tutorial.md) and the [user
guide](user_guide.md). This document is about the seam.

**What was run to write this.** Every command and every block of output below
was executed on 2026-08-17 in a Debian 12 container, against:

| Tool | Version | Provenance of the artifact |
|---|---|---|
| binder | `binder/0.3.2-0.20260816233049-fc3c8c9bffdb` | `go build` of this repo at commit `fc3c8c9` (`v0.3.1-2-gfc3c8c9`) |
| iwe | `iwe 0.19.1` | release `iwe-v0.19.1` (published 2026-08-15T02:18:02Z); tarball SHA-256 `b48035a05d58f3fe185056b7b3078d7b6f6bbee8cc44b29eb401f20fec046a97`, matching the published checksum |

Claims about binder are grounded in that tree, naming the file and the function.
Claims about iwe are grounded in either observed behaviour of that binary or
iwe's own docs at `main`, fetched 2026-08-17 (`docs/okf.md`, `docs/mcp.md`,
`docs/cli-normalize.md`, `docs/cli-retrieve.md`, `docs/query-language.md`).
Anything not confirmed by one of those is marked **UNVERIFIED** and is not
rounded up.

---

## What iwe is, and why compose it with binder

**iwe** (`iwe-org/iwe`) is a markdown knowledge-graph tool written in Rust: an
LSP server for editors, plus a CLI and an MCP server that give an agent search,
retrieval and refactoring over a directory of markdown files. Its
self-description is *"Markdown knowledge graph — LSP for your editor, CLI + MCP
memory for your AI agents."* Pulled from the GitHub REST API on **2026-08-17**:
Apache-2.0, created 2024-09-20, 1,521 stars, 1 open issue, not archived, last
push 2026-08-15, latest release `iwe-v0.19.1` on 2026-08-15. Of the OKF-aware
tools surveyed for this document it is the healthiest by a clear margin — nearly
two years old, releasing roughly weekly, under a real OSI licence. Caveats worth
carrying: it is effectively single-maintainer, and its query language is
**self-labelled experimental** — *"Syntax, operators, defaults, and CLI flag
names may change without warning"* (`docs/query-language.md`, fetched
2026-08-17).

**Why the pair composes.** binder turns a markdown corpus you already have into
a conformant OKF v0.2 bundle: one concept per file, links rewritten to
bundle-absolute form, a root `index.md`, and a `generated` provenance stamp on
every concept. An OKF bundle is a directory of markdown files with YAML
frontmatter — which is exactly iwe's native data model. iwe's own OKF page puts
it plainly: *"IWE has no OKF mode because it doesn't need one: markdown,
frontmatter, and links are its native data model"* (`docs/okf.md`, fetched
2026-08-17). So binder's output is iwe's input, with nothing in between.

**Is this "an answer" for the querying path?** Yes, with one correction that the
rest of this tutorial earns. The composition is real: it ran out of the box, at
zero build cost. But iwe supplies the **retrieval** answer, not the **trust**
answer. OKF's trust vocabulary is *derived*, not stored, so a tool that filters
raw frontmatter cannot produce it — and the derived values are the one thing
binder contributes that iwe cannot. The honest one-line form of the composition
is:

> **binder onboards and derives trust; iwe queries and retrieves; the
> derived-trust vocabulary does not
> cross the seam, and everything iwe writes lands in files whose provenance
> binder stamped.**

---

## Before you start

Install binder (see [Before you start](tutorial.md#before-you-start) in the
tutorial for the full options) and iwe:

```bash
# iwe: prebuilt binaries, cargo, or npm — see https://github.com/iwe-org/iwe
cargo install iwe iwes iwec
```

```bash
binder --version
iwe --version
```

```text
binder/0.3.2-0.20260816233049-fc3c8c9bffdb
iwe 0.19.1
```

**Practical note, observed on 2026-08-17.** The published
`iwe-v0.19.1-x86_64-unknown-linux-gnu` binary needs glibc 2.39. On Debian 12
(glibc 2.36) it fails with ``version `GLIBC_2.39' not found``; it runs when
launched under a newer loader (this document's runs used a Debian 13 `libc6`
2.41 loader). On a current distro the friction does not exist. Two binaries
matter here: `iwe` (CLI) and `iwec` (MCP server).

Work on a copy, under version control. Every gotcha below is a file rewrite, and
`git diff` is the tool that makes them visible:

```bash
git init && git add -A && git commit -m "before iwe"
```

---

## Part 1: the onboarding walkthrough

### Step 1: start from a corpus you already have

Three ordinary markdown files with ordinary relative links. One of them already
carries a human attestation and a lifecycle date — the kind of frontmatter a
real corpus accumulates:

```text
src/
  metrics/revenue.md
  metrics/gross-margin.md
  policies/revenue-recognition.md
```

```markdown
---
type: Metric
verified: { by: human:ghchinoy, at: 2026-05-02T09:00:00Z }
stale_after: "2026-06-01"
---

# Revenue

Recognized revenue for the period, net of refunds.

Governed by the [revenue recognition policy](../policies/revenue-recognition.md).
```

### Step 2: binder converts the corpus to an OKF bundle

```bash
binder convert ./src -o ./bundle --type-map "metrics=Metric,policies=Policy"
```

```text
binder convert
  source: ./src
  output: ./bundle
  concepts: 3
  links: 3 (resolved 3, unresolved 0)

Concepts:
  metrics/gross-margin.md  [type=Metric]
  metrics/revenue.md  [type=Metric]
  policies/revenue-recognition.md  [type=Policy]
```

`convert` never mutates the source. What lands in `bundle/metrics/revenue.md`:

```markdown
---
type: Metric
verified: { by: human:ghchinoy, at: 2026-05-02T09:00:00Z }
stale_after: "2026-06-01"
title: Revenue
generated:
  at: "2026-08-17T02:23:33Z"
  by: binder/0.3.2-0.20260816233049-fc3c8c9bffdb
---

# Revenue

Recognized revenue for the period, net of refunds.

Governed by the [revenue recognition policy](/policies/revenue-recognition.md).
```

Three things to notice, because each one comes back in Part 2:

1. **The pre-existing trust frontmatter is preserved byte-for-byte** — the flow
   mapping, the quoting, the key order. binder does not clobber it (the package
   contract in `internal/convert`). That preservation is **scoped to a fence
   binder recognises**: a file whose frontmatter binder reads as plain is
   synthesized over, leaving the original `verified:` block in the body as text
   ([#124](https://github.com/ghchinoy/binder/issues/124), open). The bounds
   that remain even on a recognised fence are *Residual bounds* under
   [`enrich`](user_guide.md#enrich). This tutorial's corpus is inside that
   scope; a real one may not be, and the seam below assumes the guarantee holds.
2. **A `generated` stamp is added, and only when absent.** `stampGenerated`
   (`internal/convert/frontmatter.go`) returns early if the concept already
   carries one. binder does not re-stamp on a later run.
3. **The link was rewritten to bundle-absolute form**
   (`/policies/revenue-recognition.md`), and the generated `index.md` nav files
   use relative links.

binder's own view of the bundle, including the two values it *derives*:

```bash
binder review ./bundle
```

```text
  concepts: 3
  trust tiers:
    human-reviewed: 1
    machine-confirmed: 0
    unverified: 2
  stale (as of 2026-08-17): 1
    metrics/revenue
  unresolved links: 0
```

### Step 3: point iwe at the bundle

```bash
cd bundle
iwe init --okf
```

```text
6 files here · 8 markdown links
warning: link paths is mixed across the corpus — 3 links resolve relative to their own
directory, 3 from the library root
note: frontmatter fields: generated 50%, title 50%, type 50%, okf_version 16%, stale_after 16%, verified 16%
note: duplicate titles: Concepts (2 documents)
initialized .iwe/config.toml
wrote .iwe/schemas/okf.yaml
wrote .iwe/schemas/okf-index.yaml
wrote .iwe/schemas/okf-log.yaml
kept the existing index.md
iwe normalize would rewrite 4/6 files
```

`init --okf` writes only `.iwe/` — config plus three OKF conformance schemas.
**It does not touch your markdown.** Hold on to two lines of that output; they
are the first two gotchas: the *mixed link paths* warning, and *`iwe normalize`
would rewrite 4/6 files*.

Conformance, checked by iwe's own schemas:

```bash
iwe schema validate ; echo "exit=$?"
```

```text
exit=0
```

A binder-emitted bundle passes iwe's OKF schema validation with no findings.

### Step 4: query the graph

The bundle is now a knowledge graph. No import, no index build:

```bash
iwe find --filter '{type: Metric}' -f keys
```

```text
metrics/revenue
metrics/gross-margin
```

```bash
# documents that point AT revenue ($references: "this doc references the anchor")
iwe find --filter '{$references: metrics/revenue}' -f keys
```

```text
policies/revenue-recognition
metrics/gross-margin
metrics/index
```

```bash
# documents revenue points at ($referencedBy: "this doc is referenced by the anchor")
iwe find --filter '{$referencedBy: metrics/revenue}' -f keys
```

```text
policies/revenue-recognition
```

The default `find` output shows the graph and the OKF frontmatter together —
forward links after `->`, backlinks after `<-`:

```text
- [Revenue](metrics/revenue) -> [Revenue Recognition Policy](policies/revenue-recognition) <- [Gross Margin](metrics/gross-margin), [Concepts](metrics/index), [Revenue Recognition Policy](policies/revenue-recognition) · type: Metric · verified: {by: human:ghchinoy, at: 2026-05-02T09:00:00Z} · stale_after: 2026-06-01 · generated: {at: 2026-08-17T02:23:33Z, by: binder/0.3.2-...}
```

iwe resolved binder's rewritten links into a real bidirectional graph, and OKF
frontmatter families are ordinary query predicates. Bounded graph operators
(`$references`, `$referencedBy`, `$includes`, `$includedBy`, each with
`minDistance`/`maxDistance` and a `$size` cardinality test, composable under
`$and`/`$or`/`$nor`) give a richer filter surface than binder's five
`query_graph` verbs.

### Step 5: hand it to an agent

```bash
iwe retrieve -k metrics/revenue --expand-references 1
```

`````text
````markdown #metrics/revenue
---
title: Revenue
references:
- key: policies/revenue-recognition
  title: Revenue Recognition Policy
referencedBy:
- key: metrics/gross-margin
  title: Gross Margin
...
---

# Revenue

Recognized revenue for the period, net of refunds.
...
````
`````

One call, token-budgeted (`--max-tokens`, `--max-documents`), assembling seeds
plus graph expansion. binder has no equivalent. Look closely at that envelope,
though — `type`, `verified`, `stale_after` and `generated` are **not in it**.
That is Gotcha 3.

For an agent harness, iwe's MCP server is `iwec`:

```bash
iwec              # stdio
```

**That is the whole happy path.** It works. Now the part that decides whether
you should build on it.

---

## Part 2: gotchas — honestly

Nothing below is a bug report against iwe. iwe is an *authoring and refactoring*
tool and behaves correctly by its own contract. The losses happen **at the
seam**, and they are invisible unless you go looking — which is precisely the
failure mode binder exists to prevent.

### Gotcha 1 (the big one): provenance rewriting and relocation

binder's provenance model rests on two invariants, both readable in the source:

- **Path is identity.** A concept's id is its bundle-relative path minus `.md`
  (`Concept.ID` in `internal/okf/model.go`; spec §2). Nothing else keys it.
- **Stamps are additive and never fabricated.** `generated` is written only when
  absent (`stampGenerated`); `verified` is appended, never replaced, and
  `enrich --overwrite-keys` **refuses** every trust key ([Opt-in
  refresh](user_guide.md#opt-in-refresh---overwrite-keys)).

iwe knows about neither. Here is what that costs, in four measured forms.

#### 1a. Every iwe write re-serializes the provenance frontmatter

`iwe normalize` on the freshly converted bundle (no other change):

```diff
 ---
 type: Metric
-verified: { by: human:ghchinoy, at: 2026-05-02T09:00:00Z }
-stale_after: "2026-06-01"
+verified:
+  by: human:ghchinoy
+  at: 2026-05-02T09:00:00Z
+stale_after: 2026-06-01
 title: Revenue
 generated:
-  at: "2026-08-17T02:23:33Z"
+  at: 2026-08-17T02:23:33Z
   by: binder/0.3.2-0.20260816233049-fc3c8c9bffdb
 ---
```

The **values** are unchanged. The **serialization** is not: the flow mapping
becomes a block mapping, and two quoted scalars lose their quotes — which
changes their YAML *type* from `!!str` to `!!timestamp`. `okf_version: "0.2"`
likewise becomes `okf_version: '0.2'` in the root index.

How much does that matter? Measured, both ways:

- **binder is unaffected.** Its codec deliberately keeps timestamps as literal
  text — *"Timestamps are kept as their literal string so trust datetimes
  round-trip textually and never become `time.Time`"*
  (`internal/okf/native/native.go`, `nodeToValue`). After `iwe normalize`,
  `binder review` reports the same tiers and the same staleness, and
  `binder validate` still says **conformant (OKF 0.2)**.
- **A third consumer may not be.** A consumer decoding frontmatter into
  `map[string]any` now sees a different type for `generated.at`, `verified.at`
  and `stale_after` than it did before. Attestation bytes changing as a side
  effect of an operation that was not about the attestation is the hazard
  binder's byte-faithful round-trip exists to avoid — and the one its own
  *[Residual bounds](user_guide.md#enrich)* enumerate where it cannot. iwe is
  outside that guarantee, so nothing here contradicts it; the point is that the
  guarantee stops at the seam.
- **Every attestation shows up in the diff.** After any iwe write, `git diff` on
  a bundle contains provenance-block churn that has nothing to do with what you
  meant to change. Review fatigue is how a real provenance change slips through.

Note the scope: this is not confined to `normalize`. `iwe rename` and
`iwe extract` re-serialize every document they touch, by the same formatter.

#### 1b. `iwe extract` relocates content **out from under the attestation that covered it**

This is the sharpest one. Take a section of the human-verified `metrics/revenue`
and extract it:

```bash
iwe extract metrics/revenue --section "Measurement Notes"
```

```text
Extracting section 'Measurement Notes' to 'metrics/dfewgd8x'
Done
```

The parent loses the prose and gains a link; the prose lands in a brand-new
file:

```text
# Measurement Notes

Revenue is measured in USD, converted at the month-end rate.
```

That new file has **no frontmatter at all** — no `type`, no `generated`, no
`verified`. So:

- Content that a human signed off on now lives in a document with **no
  provenance and no trust**.
- The parent keeps `verified: { by: human:ghchinoy, at: 2026-05-02 }`
  **unchanged**, though its content changed and now attests to less text than it
  did.
- The bundle stops conforming. Both validators catch it, which is the good news:

```text
$ iwe schema validate
metrics/dfewgd8x › frontmatter: "type" is a required property
  hint: OKF v0.2 concept document — frontmatter with a non-empty type (SPEC §4.1, §11)
exit=1

$ binder validate .
[error] metrics/dfewgd8x: unparseable or missing YAML frontmatter (spec §11.1): missing frontmatter: document does not start with '---'
RESULT: NOT conformant (1 violation(s))
```

- The obvious repair makes it worse. `binder enrich` fills the gap the only way
  it can:

```text
$ binder enrich ./metrics
3 file(s): 1 enriched, 2 unchanged, 0 skipped
  enriched dfewgd8x.md (added: generated, title, type)
```

```markdown
---
type: Note
title: Measurement Notes
generated:
  at: "2026-08-17T02:26:22Z"
  by: binder/0.3.2-0.20260816233049-fc3c8c9bffdb
---
```

**Read that stamp.** binder now asserts that it generated, today, prose that a
human wrote and iwe moved. The human verification is gone; a machine-authorship
claim took its place. binder is behaving exactly as specified — the field was
absent, so it stamped — and the result is still a false provenance record. The
loss is structural, not a defect in either tool.

Also note the key: `metrics/dfewgd8x`. The scaffolded config uses
`key_template = "{{id}}"` (`.iwe/config.toml`, `[actions.extract]`), so the new
concept's **path — which is binder's identity** — is a random-looking slug. Set
`key_template = "{{slug}}"` if you want readable ids.

#### 1c. `iwe update` lets a trust stamp **outlive the content it attested to**

Body-overwrite mode preserves frontmatter — reasonable behaviour, dangerous
result:

```bash
iwe update -k metrics/revenue -c '# Revenue

Gross booked revenue for the period, INCLUDING refunds.
...'
```

```diff
 generated:
   at: 2026-08-17T02:23:33Z
   by: binder/0.3.2-0.20260816233049-fc3c8c9bffdb
 ---
-Recognized revenue for the period, net of refunds.
+Gross booked revenue for the period, INCLUDING refunds.
```

The definition of the metric was inverted.
`verified: { by: human:ghchinoy, at: 2026-05-02 }` did not move, and neither did
`generated.at`. binder, asked afterwards, still says:

```text
  trust tiers:
    human-reviewed: 1
```

**A human-reviewed tier is now being derived for text no human ever reviewed.**
OKF's model permits this by design — spec §5.2 keeps `verified` independent of
`generated`, *"content can change without re-confirmation"* — but the pair gives
you nothing that notices.

And binder cannot clean it up for you. `generated` is set-when-absent, so a
re-stamp never happens:

```text
$ binder enrich ./metrics
2 file(s): 0 enriched, 2 unchanged, 0 skipped
```

...and withdrawing the attestation is refused, on purpose:

```text
$ binder enrich ./metrics --overwrite-keys verified ; echo "exit=$?"
binder: --overwrite-keys: refusing to overwrite trust-provenance key "verified" (protected: attester,
computation, executor, generated, parameters, runtime, sources, usage_window, verified, verified_by);
these can carry human attestations and overwriting them would violate the never-fabricate-trust invariant
exit=2
```

**Mitigation is human and manual:** treat any iwe body write as invalidating the
concept's attestation, strip the `verified` block by hand (or `git revert`), and
re-verify. Gate it in CI — a diff that touches a concept body without touching
its `verified` block is the signature to catch. **UNVERIFIED:** no such gate
ships in binder today, and no verb was found that reports "body changed since
the last verification."

#### 1d. `iwe rename` reassigns binder's identity, silently

```bash
iwe rename metrics/revenue metrics/net-revenue
```

```text
Renaming 'metrics/revenue' to 'metrics/net-revenue'
Updated 3 document(s)
```

iwe does this well by its own lights: the file moves, and all three inbound
links are rewritten. But the concept's binder id is its path, so
`metrics/revenue` **ceases to exist** and `metrics/net-revenue` appears with the
same body, the same `generated` stamp and the same `verified` stamp:

```text
$ binder review .
  stale (as of 2026-08-17): 1
    metrics/net-revenue
```

Inside the bundle nothing breaks. Outside it, everything holding the old id
breaks at once — a stored `query_graph` result, a citation in a chat log, an
external index, another bundle's notes. Nothing is written to record that the
two ids are the same concept: no tombstone, no redirect, no `log.md` entry.
Path-as-identity is a binder invariant, not an iwe one ([Node
identity](user_guide.md#node-identity-path-or-a-read-honored-id_key)); iwe does
not create the weakness, but it exercises it with one command.

#### 1e. The write surface cannot be turned off

`iwec` exposes 14 MCP tools; the writing and refactoring ones — `iwe_create`,
`iwe_update`, `iwe_delete`, `iwe_query` (`update`/`delete`), `iwe_rename`,
`iwe_extract`, `iwe_inline`, `iwe_normalize`, `iwe_attach` — are always among
them (`docs/mcp.md`, fetched 2026-08-17). There is no read-only flag:

```text
$ iwec --help
Options:
      --transport <TRANSPORT>  [default: stdio] [possible values: stdio, http]
      --host <HOST>            [default: 127.0.0.1]
      --port <PORT>            [default: 8000]
```

binder's `query_graph` and `list_graphs` are read-only by construction. **An
agent handed iwe's MCP server can rewrite a provenance stamp, and 1a–1d are what
that looks like.** That is a governance decision to make deliberately rather
than inherit. Mitigations that do work: point `iwec` at a throwaway copy or a
read-only mount of the bundle, or allow-list only the reading tools in the
harness. `iwe_query`'s mutating kinds are at least strict — every mutation must
carry an `expect` guard or it is refused.

### Gotcha 2: derived trust does not exist on disk, so iwe cannot filter it

`tier` and `stale` are **derived, never stored** — spec §5.3/§5.5, implemented
by `TrustTier` and `IsStale` in `internal/okf/trust.go` (`TrustTier`: no
`verified` ⇒ `unverified`; any `human:` verifier ⇒ `human-reviewed`; else
`machine-confirmed`. `IsStale`: `today >= stale_after`). binder's `query_graph`
hands them to an agent as first-class fields on every node ([The graph
surface](user_guide.md#the-graph-surface)). Calling the `query_graph` MCP tool
with `{"bundle": ".", "op": "lookup", "label": "Metric", "today": "2026-08-17"}`
returns, inside the usual `binder.report/v1` envelope:

```json
"nodes": [
  { "id": "metrics/gross-margin", "title": "Gross Margin", "type": "Metric",
    "tier": "unverified", "stale": false },
  { "id": "metrics/revenue", "title": "Revenue", "type": "Metric",
    "tier": "human-reviewed", "stale": true }
]
```

iwe returns nothing for either, because neither field is in any file:

```text
$ iwe find --filter '{tier: {$exists: true}}' -f keys      # (no output)
$ iwe find --filter '{stale: {$exists: true}}' -f keys     # (no output)
```

You can *reconstruct* them — and the reconstruction is exactly as fragile as it
sounds:

```text
$ iwe find --filter '{$and: [{verified.by: human:ghchinoy}, {stale_after: {$lt: "2026-08-17"}}]}' -f keys
metrics/revenue
```

That works only because this concept has a **single** verifier. Give it the
spec's own canonical shape — a human sign-off plus a nightly process, `verified`
as a list of events (spec §5.2) — and the same filter goes silent:

```markdown
verified:
  - by: process:nightly-check
    at: 2026-08-10T02:00:00Z
  - by: human:ghchinoy
    at: 2026-05-02T09:00:00Z
```

```text
$ iwe find --filter '{verified.by: human:ghchinoy}' -f keys
                                          # (no output, exit 0)
$ binder review .
  trust tiers:
    human-reviewed: 1
```

**Same file, same question, opposite answers — and iwe's is a silent false
negative, not an error.** iwe's filter language has no documented operator for
matching inside a list of mappings (no `$elemMatch` equivalent in
`docs/query-language.md`, fetched 2026-08-17; `{verified: {$exists: true}}`
still matches, but the `by` of each event is unreachable). So a caller
reconstructing trust in iwe must additionally (i) re-implement OKF's derivation
rules, (ii) know the `human:`/`process:`/`team:` actor-prefix convention, (iii)
pass today's date in by hand as a string comparison, and (iv) get the
single-mapping versus list form right. Every one of those is a place for an
agent to get trust wrong quietly.

**Mitigation:** ask binder for trust and iwe for retrieval — do not reconstruct.
Getting the derived values *inside* iwe filters would take a projection binder
does not emit today, and any such projection would have to be a sidecar or an
export format and **not** be written back into concept frontmatter — storing a
derived value is precisely what OKF forbids (spec §5.3).

### Gotcha 3: `iwe retrieve` does not carry provenance into the agent's context

The retrieval envelope carries graph metadata — *"YAML frontmatter with `key`,
`title`, `includedBy`, and `referencedBy`"* (`docs/cli-retrieve.md`, fetched
2026-08-17) — and the document's own OKF frontmatter is not part of it.
Confirmed in both output formats: `-f json` returns `key`, `title`, `content`,
`references`, `includes`, `referencedBy`, `includedBy`, and nothing else.

So the highest-value part of the composition — one-call, token-budgeted context
assembly — is also the place where the agent sees **content stripped of its
trust signal**. It cannot tell a human-reviewed concept from an unverified
draft, or a fresh one from a stale one, from what it was handed.

**Mitigation:** use `iwe find` (whose default output does print the frontmatter
fields) to accompany a retrieve, or call binder's `query_graph` for the same
keys and join on the id, or read the raw files.

### Gotcha 4: the two tools disagree about link form, and the disagreement churns

binder writes concept-to-concept links bundle-absolute
(`/policies/revenue-recognition.md`) and leaves the generated index nav links
relative. iwe detects the mix at `init` and picks one:

```text
warning: link paths is mixed across the corpus — 3 links resolve relative to their own directory,
3 from the library root
iwe normalize would rewrite 4/6 files
```

With the detected default (`refs_path = "relative"`), `iwe normalize` flips
binder's absolute concept links to relative:

```diff
-Gross margin is derived from [revenue](/metrics/revenue.md) less cost of goods sold.
+Gross margin is derived from [revenue](revenue.md) less cost of goods sold.
```

Setting `refs_path = "absolute"` in `.iwe/config.toml` before normalizing helps,
but **does not eliminate the disagreement — it moves it**. Measured: with
`absolute`, iwe leaves every concept link exactly as binder wrote it, and
instead rewrites the *index* nav links to absolute, which binder's
`convert`/`index` writes relative. Either way, alternating `binder convert` and
`iwe normalize` produces byte churn forever.

Both forms are legal OKF (spec §6.1/§6.2) and both resolve: `binder validate`
reports conformant after either normalization. This is a diff-noise and
pipeline-determinism problem, not a correctness one.

### Gotcha 5: the pipeline is one-way — decide who owns the bundle

`binder convert` reads a source corpus and writes a bundle. iwe edits the
**bundle**. There is no path back. Measured: after iwe normalized the bundle and
created a new document, re-running `binder convert ./src -o ./bundle` overwrote
the three source-derived concepts (reverting iwe's link form and re-stamping
`generated.at` with a new timestamp, since the source files carry no stamp)
while leaving iwe's new `metrics/churn.md` sitting in the bundle — unreferenced
by the conversion and, because `iwe create` writes content verbatim with no
frontmatter added, non-conformant:

```text
$ binder validate ./bundle
[error] metrics/churn: unparseable or missing YAML frontmatter (spec §11.1): missing frontmatter
RESULT: NOT conformant (1 violation(s))
```

Pick one ownership model before you build anything:

- **Model A — corpus is the source of truth, iwe is read-only downstream.**
  Re-run `binder convert` freely; never write through iwe (no `normalize`, no
  refactors, reading MCP tools only). You keep determinism and provenance; you
  give up iwe's authoring value.
- **Model B — the bundle is the source of truth.** Convert once, retire the
  source tree, let iwe author and refactor, and use binder in place as the
  conformance and trust gate (`validate`, `review`, `lint`, `enrich` for missing
  families). You keep iwe's full value; you take on Gotchas 1a–1d and must gate
  them in CI.

Mixing the two models is what produces the half-reverted, half-stranded bundle
above.

### Gotcha 6: two limits that are neither tool's fault

- **No cross-bundle anything.** iwe is single-workspace, and binder's graph is
  projected from one bundle's resolved links ([The graph
  surface](user_guide.md#the-graph-surface)). OKF v0.2 defines no inter-bundle
  reference form, so neither tool has one to implement. Adding iwe does not
  change this.
- **The query language is experimental.** iwe says so itself: syntax, operators
  and flag names *"may change without warning"*, it is *"not exposed as a
  library API"*, and it has *"no stable on-disk format"*
  (`docs/query-language.md`, fetched 2026-08-17). Automation built on the filter
  syntax should expect churn and pin the iwe version.

---

## What ports, and what does not

| Capability | Across the binder → iwe seam | Notes |
|---|---|---|
| The bundle itself — markdown + frontmatter + links | **Ports** | No adapter, no import, no index build. `iwe init --okf` writes only `.iwe/`. |
| OKF frontmatter as query predicates (`type`, `status`, nested fields) | **Ports** | Ordinary frontmatter to iwe; dotted paths work on single mappings. |
| The link graph, bidirectional | **Ports** | binder's rewritten links resolve into iwe's forward links and backlinks. |
| Bounded graph operators | **Ports, and improves** | `$references`/`$referencedBy`/`$includes`/`$includedBy` with distance bounds and `$size`, composable — richer than binder's five verbs. |
| OKF conformance checking | **Ports (both directions)** | `iwe schema validate` exits 0 on a binder bundle; both validators catch the same breakage after a refactor. |
| Token-budgeted context assembly for agents | **iwe only** | `iwe retrieve` / `iwe_retrieve`; binder has no equivalent — but see Gotcha 3. |
| Derived trust tier (`unverified` / `machine-confirmed` / `human-reviewed`) | **Does NOT port** | Derived at query time, never on disk. Reconstruction breaks silently on the spec's list form of `verified`. |
| Derived staleness (`stale` as of a date) | **Does NOT port** | Same reason; `stale_after` is filterable only as a hand-passed string comparison. |
| Provenance in retrieved context | **Does NOT port** | `iwe retrieve` emits `key`/`title`/edge lists; `generated`, `verified`, `type`, `stale_after` are dropped from the envelope. |
| Frontmatter byte-fidelity of attestations | **Does NOT survive an iwe write** | Values preserved; flow style, quoting and YAML scalar tags are rewritten. |
| Path-as-identity stability | **Does NOT survive `rename`** | The concept id changes; no tombstone or forwarding record is written. |
| Attestation validity across an edit | **Does NOT survive** | `verified` and `generated.at` are preserved verbatim through a body rewrite; nothing invalidates them. |
| Read-only safety posture | **Does NOT port** | binder's MCP surface is read-only by construction; `iwec` has no read-only mode. |
| Cross-bundle query | **Neither tool** | Format-level gap in OKF v0.2. |

---

## A safe default recipe

If you want the composition today with the smallest exposure:

```bash
# 1. Convert, on a pinned clock, and commit the bundle.
SOURCE_DATE_EPOCH=$(date +%s) binder convert ./src -o ./bundle --type-map "..."
git -C ./bundle init && git -C ./bundle add -A && git -C ./bundle commit -m "binder convert"

# 2. Scaffold iwe. Nothing but .iwe/ is written.
cd bundle && iwe init --okf

# 3. Read with iwe; do not write with it.
iwe find --filter '{type: Metric}' -f keys
iwe retrieve -k metrics/revenue --expand-references 1

# 4. Ask binder — not iwe — for trust, and join on the concept id.
binder review .
#   or, for an agent: the query_graph MCP tool, which returns tier and stale per node.

# 5. Gate every run.
iwe schema validate && binder validate .
```

If you do let iwe write (Model B), add these to CI:

- `binder validate` **and** `iwe schema validate` on every commit — extraction
  and `create` both produce frontmatter-less documents that break conformance.
- A diff rule: a change to a concept's body with no change to its `verified`
  block is a stale attestation until a human says otherwise.
- A rename rule: a renamed concept file is an identity change; record the old id
  somewhere durable.
- Pin `refs_path` in `.iwe/config.toml` and pin the iwe version.

---

## Open questions and what is not verified here

- **Open, not decided here:** whether binder should emit a derived-trust
  projection (sidecar or export, never frontmatter) that iwe could filter on. It
  would make the pair compose losslessly. Nothing of the sort is on the
  [roadmap](user_guide.md#roadmap--planned-features) today, and naming it here
  is **not** a recommendation to build it.
- **UNVERIFIED:** whether any binder verb could detect "body changed since the
  last `verified` event." No such verb exists today; the gate described above is
  a CI convention, not a feature.
- **UNVERIFIED:** iwe's behaviour on large corpora (thousands of concepts) in
  this composition. Everything here was measured on a 3-concept bundle plus
  targeted variants.
- **UNVERIFIED:** whether `iwe extract` can be configured to seed frontmatter (a
  `type`, at minimum) on the extracted document. `docs/cli-extract.md` documents
  `key_template` and `link_type` for the `extract` action and mentions template
  modes elsewhere; no frontmatter option was found in those docs, and none was
  found by experiment.
- **UNVERIFIED:** whether the MCP `iwe_query` `update` operation's `expect`
  guards would catch any of the provenance hazards above. They guard block
  content, not frontmatter validity.

## Reproduction

```text
binder : this repo @ fc3c8c9, go build, self-reports
         binder/0.3.2-0.20260816233049-fc3c8c9bffdb
iwe    : release iwe-v0.19.1 (2026-08-15), x86_64-unknown-linux-gnu,
         sha256 b48035a05d58f3fe185056b7b3078d7b6f6bbee8cc44b29eb401f20fec046a97
date   : 2026-08-17 (all fetches and all runs)
```

Sources fetched 2026-08-17: `iwe-org/iwe` via the GitHub REST API (stars,
licence, dates, release); `docs/okf.md`, `docs/mcp.md`, `docs/cli-normalize.md`,
`docs/cli-extract.md`, `docs/cli-rename.md`, `docs/cli-retrieve.md`,
`docs/query-language.md` at `main`. binder behaviour from the cited files in the
`fc3c8c9` tree and from the runs shown. Re-run the walkthrough against a newer
binder before relying on any output above: the versions are pinned here
precisely so a drift is visible.
