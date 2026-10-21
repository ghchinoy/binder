# Trust model & tiers

binder maps corpus-native provenance into the OKF v0.2 trust vocabulary,
**preserves** existing trust frontmatter losslessly where it recognises the
fence, and **derives** trust tiers and staleness on demand. It never stores a
credibility score, and where the fence is recognised it never fabricates
provenance (spec §5.1).

> **That preservation is bounded, and the bound matters if you depend on binder
> for provenance.**
> Lossless preservation is scoped to files whose frontmatter binder
> recognises and that need no read-boundary normalization: the fence opens with
> `---` and a newline, LF or CRLF, at the very start. A leading UTF-8 BOM or a
> lone-CR (classic-Mac) fence is now recognised via read-boundary normalization
> ([#124](https://github.com/ghchinoy/binder/issues/124)), so the `verified:`
> attestation it guards is preserved rather than demoted to body — but because
> that normalization (BOM strip, lone-CR to LF) does not preserve the original
> encoding, it is disclosed non-optionally rather than passing as a silent
> round-trip: a `normalized` signal plus a top-level advisory in the run report.
> That advisory never gates, under `--strict` or otherwise
> ([Strict mode](../user_guide.md#strict-mode)). A file with **no**
> frontmatter fence at all is still read as plain and synthesized over. See
> [Lossless frontmatter round-trip](byte-faithful.md).

## Vocabulary

| Field | Shape | Notes |
|---|---|---|
| `generated` | `{ by, at }` | Provenance of the producing run. `by` follows the actor convention; `at` is an ISO 8601 datetime. binder stamps `{ by: "binder/<ver>", at }` only when absent. |
| `verified` | `{ by, at }` or a list of them | Verification events. A bare mapping is treated as a one-element list. Drives the trust tier. |
| `sources` | list of `{ id, resource, title, author, usage_count, last_modified }` | `resource` is required within an entry; `author` follows the actor convention; `last_modified` is an ISO 8601 datetime or `YYYY-MM-DD` date. |
| `status` | `draft` \| `stable` \| `deprecated` | Absent ⇒ `stable` (spec §5.4). |
| `stale_after` | ISO 8601 datetime or `YYYY-MM-DD` | Absolute expiration timestamp or date; drives staleness. |
| `usage_window` | `{ from, to }` | A datetime or date range; both bounds accept ISO 8601 datetimes or `YYYY-MM-DD` dates. |
| Attested-Computation family | `runtime`, `parameters`, `computation`, `executor`, `attester` | Preserved and shape-checked. A concept of type `Attested Computation` requires `runtime` (advisory). binder does **not** execute attestations (no runtime). |

The **actor convention** (spec §7): `"<producer>/<version>"` for tools/agents
(e.g. `binder/0.3.0`, `reference_agent/gemini`), or one of the `human:`,
`process:`, `team:` prefixes for people, processes, and teams (e.g.
`human:alice`).

## Derived trust tiers

Tiers are computed from `verified` at read time, never stored in frontmatter
(spec §5.1, §5.3). They form a strict total order:

$$\text{unverified} < \text{machine-confirmed} < \text{human-reviewed}$$

| Tier | Condition | What it represents |
|---|---|---|
| `unverified` | no `verified` events | Generated or authored content that has not yet been attested by any automated check, team process, or human reviewer. |
| `machine-confirmed` | one or more `verified` events, none by a `human:` actor | Content attested by a tool, validator, or automated pipeline (`<producer>/<version>`, `process:<id>`, or `team:<id>`), but not yet signed off by a named person. |
| `human-reviewed` | at least one `verified[].by` uses the `human:` prefix | Content reviewed and attested by a named human (`human:<id>`), even if machine or process stamps are also present. |

### Consumer never-reject vs. producer CI gating (`--require-tier`)

OKF v0.2 separates **what makes a bundle valid** from **what makes a bundle
ready to publish in a given environment**:

- **Consumers never reject on tier (spec §11).** A reader, search indexer, or
  agent consuming an OKF bundle must not reject a document for an unknown `type`,
  a missing optional field family (`verified`, `sources`, `status`,
  `stale_after`), or a broken cross-link. The same bundle is valid OKF whether
  its concepts are `unverified` on a feature branch or `human-reviewed` in
  production. Bare `binder validate` and bare `binder review` follow this
  consumer posture by default and exit `0`.
- **Producers enforce tier floors in CI (`binder review --require-tier`).** A
  team curating or releasing a bundle can opt into a minimum trust tier floor at
  the application layer by passing `--require-tier` (`unverified`,
  `machine-confirmed`, or `human-reviewed`) to `binder review` (or
  `require_tier` on the MCP `review` tool; [#243](https://github.com/ghchinoy/binder/issues/243),
  [#247](https://github.com/ghchinoy/binder/issues/247)). When any concept falls
  below the requested floor, `review` exits `1` and reports *why* each concept
  fell short—distinguishing a concept with no `verified` entry at all
  (`no verified entry`) from one attested only by non-human actors
  (`verified only by non-human actor(s): ...`). That turns a failing CI check
  into a short remediation to-do list rather than a wall of errors, without
  storing a credibility score in frontmatter.

See [Trust-tier gating (`--require-tier`)](../user_guide.md#trust-tier-gating---require-tier-and-how-it-rides-on-okf-v02)
in the user guide for examples and JSON output.

## Derived staleness

A concept is **stale** when `today >= stale_after` (using `--today`, else now;
`SOURCE_DATE_EPOCH` honoured). When `stale_after` is an ISO 8601 datetime
(e.g. `2026-09-23T00:00:00Z`), its leading `YYYY-MM-DD` date prefix is compared
against `today`. A concept without `stale_after` is never stale.

## Opt-in trust mapping (`convert`)

All mapping is off by default, deterministic, and additive (original keys are
preserved):

- **`--source-keys "source,url,author"`** — each named frontmatter key becomes a
  `sources` entry. The `author` key maps to a source `author`; every other key
  maps to a source `resource`.
- **`--map-citations`** — list items under a body `# Citations` heading (any
  level) become `sources` entries: a markdown link yields `{ title, resource }`,
  a bare URL yields `{ resource }`, other text yields `{ title }`.
- **`--map-draft`** — a `draft: true` marker sets `status: draft`, but only when
  `status` is absent (it never clobbers an existing status).

Mapped sources are de-duplicated against existing `sources` by
`(resource, title, author)`.

For the full trust surface (declarative lifecycle flags, writing a `verified`
stamp, status canonicalization, and trust well-formedness advisories), see the
[user guide](../user_guide.md#the-trust-vocabulary).
