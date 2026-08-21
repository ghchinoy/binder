# Trust model & tiers

binder maps corpus-native provenance into the OKF v0.2 trust vocabulary,
**preserves** existing trust frontmatter byte-for-byte where it recognises the
fence, and **derives** trust tiers and staleness on demand. It never stores a
credibility score, and where the fence is recognised it never fabricates
provenance (spec §5.1).

> **That scoping is load-bearing if you rely on binder for provenance.**
> Byte-for-byte preservation is scoped to files whose frontmatter binder
> recognises and that need no read-boundary normalization: the fence opens with
> `---` and a newline, LF or CRLF, at the very start. A leading UTF-8 BOM or a
> lone-CR (classic-Mac) fence is now recognised via read-boundary normalization
> ([#124](https://github.com/ghchinoy/binder/issues/124)), so the `verified:`
> attestation it guards is preserved rather than demoted to body — but because
> that normalization (BOM strip, lone-CR to LF) is not byte-faithful, it is
> disclosed non-optionally (a `normalized` signal plus a top-level advisory)
> rather than a silent round-trip. A file with **no** frontmatter fence at all
> is still read as plain and synthesized over. See
> [Byte-faithful round-trip](byte-faithful.md).

## Vocabulary

| Field | Shape | Notes |
|---|---|---|
| `generated` | `{ by, at }` | Provenance of the producing run. `by` follows the actor convention; `at` is an ISO 8601 datetime. binder stamps `{ by: "binder/<ver>", at }` only when absent. |
| `verified` | `{ by, at }` or a list of them | Verification events. A bare mapping is treated as a one-element list. Drives the trust tier. |
| `sources` | list of `{ id, resource, title, author, usage_count, last_modified }` | `resource` is required within an entry; `author` follows the actor convention; `last_modified` is a `YYYY-MM-DD` date. |
| `status` | `draft` \| `stable` \| `deprecated` | Absent ⇒ `stable` (spec §5.4). |
| `stale_after` | `YYYY-MM-DD` | Absolute date; drives staleness. |
| `usage_window` | `{ from, to }` | A date range; both bounds are `YYYY-MM-DD`. |
| Attested-Computation family | `runtime`, `parameters`, `computation`, `executor`, `attester` | Preserved and shape-checked. A concept of type `Attested Computation` requires `runtime` (advisory). binder does **not** execute attestations (no runtime). |

The **actor convention** (spec §7): `"<producer>/<version>"` for tools/agents
(e.g. `binder/0.3.0`, `reference_agent/gemini`), or one of the `human:`,
`process:`, `team:` prefixes for people, processes, and teams (e.g.
`human:alice`).

## Derived trust tiers

Tiers are computed from `verified`, never stored:

| Tier | Condition |
|---|---|
| `human-reviewed` | at least one `verified[].by` uses the `human:` prefix |
| `machine-confirmed` | one or more `verified` events, none by a `human:` actor |
| `unverified` | no `verified` events |

## Derived staleness

A concept is **stale** when `today >= stale_after` (using `--today`, else now;
`SOURCE_DATE_EPOCH` honoured). A concept without `stale_after` is never stale.

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
