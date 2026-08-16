# Ingestion-analysis workflow

The judgment the deterministic converter **cannot** make for you: which flags to
pass, what to fix before writing, and when to stop. binder surfaces the signals
(`--json`); *you* make the calls. Load this alongside
[`binder-json-contract.md`](binder-json-contract.md) for the exact shapes.

All flags named here are real at binder `main` — verify with
`binder convert --help` / `binder enrich --help` / `binder lint --help`.

## A. Pre-convert flag-choice triage (judgment)

`binder convert` cannot infer your corpus's conventions — it must be *told*.
Inspect the corpus, then choose:

### Types — every concept needs a non-empty `type` (§11)

- `--default-type <T>` — the fallback type when a file declares none (binder's
  own default is `Note`). Set it to whatever the corpus mostly is.
- `--type-map <dir=Type,…>` — per-directory overrides, e.g.
  `--type-map "docs=Guide,adr=Decision"`. Use when subdirectories are
  homogeneous in kind. Look at the tree shape before deciding.

### Edges — which links/keys become relationships (§6)

- Standard markdown links are edges automatically; you do not flag them.
- `--fm-ref-keys <k1,k2>` — frontmatter keys whose values are *also* edges, e.g.
  `--fm-ref-keys "related,parent"`. Choose these only after seeing which keys the
  corpus actually uses to point at other concepts.
- `--workspace-root <dir>` — the boundary within which `file://` links resolve to
  **internal** edges. Set it when the corpus links across a workspace you want
  treated as one bundle.

### Provenance — map real corpus signals into `sources` (never invent)

- `--source-keys <k1,k2>` — frontmatter keys to fold into `sources`, e.g.
  `--source-keys "source,author"`. Enable **only** for keys that carry genuine
  provenance; do not map noise.
- `--map-citations` — map a body `# Citations` list into `sources` entries.
- See [`trust-discipline.md`](trust-discipline.md) before enabling either: map
  what is *real*, never fabricate.

### Lifecycle — set only when absent (§5.4/§5.5)

- `--map-draft` — map a `draft: true` marker to `status: draft` (only when
  `status` is absent).
- `--status-map <dir=status,…>` — per-directory `status` for concepts that lack
  one, e.g. `--status-map "archive=deprecated,drafts=draft,default=active"`.
- `--stale-after-map <dir=+Nd|+Nm|+Ny,…>` — per-directory freshness horizon,
  e.g. `--stale-after-map "benchmarks=+6m,legacy=+0d"` (set only when absent).

### Gating

- `--strict` — make unresolved links / recovery warnings **gate** (exit 1).
  Default is never-reject (exit 0 with advisories). Turn it on in CI once you
  have decided the corpus should be clean.

## B. Read the dry-run triage, then decide remediate-vs-accept

Always dry-run first — it writes nothing:

```bash
binder convert <corpus> --dry-run --json > triage.json
jq '.result | {num_concepts, num_links, num_unresolved, num_recovered}' triage.json
jq '.result.unresolved' triage.json
jq '.result.warnings'   triage.json
```

For a corpus-as-authored view (a missing `title:`/`type:` that `convert` would
silently default is **masked** in convert output but visible to lint):

```bash
binder lint <corpus> --json \
  | jq '.result | {broken_links, missing_titles, orphans, entrypoints, schema_violations}'
```

Decision table:

| Signal (from `--json`) | Remediate the source | Accept it |
|---|---|---|
| **Unresolved link** | a genuine typo / wrong path — fix the source | a deliberate link to not-yet-written knowledge — legal, keep it |
| **`num_recovered` > 0** | frontmatter that failed to parse and was recovered as body — fix the YAML if the loss matters | a file that was never meant to have frontmatter |
| **`schema_violations` (missing type)** | enrich or set `--default-type`/`--type-map` | — (every concept needs a type; do not accept none) |
| **`missing_titles`** | enrich adds a `title` from the filename/first heading | acceptable if the concept is intentionally untitled |
| **`orphans`** | add a link from an index or related concept if it should be reachable | a legitimately standalone concept |
| **`entrypoints`** | empty, or missing the root you expect — the corpus has no reachable starting point; add or link a root document | the listed roots are the ones you intended |

**Never fabricate to make a number go to zero.** Broken links and missing
optional fields are legal in OKF; only fix what is genuinely wrong.

## C. Remediate at the source (additive, never destructive)

- **Missing required frontmatter** (`type`/`title`/`generated`): prefer
  `binder enrich`. It is frontmatter-only, additive (adds only *absent* keys),
  byte-faithful, and atomic. It rewrites the **source tree in place**, so commit
  or stash before running it on a git-tracked corpus. Preview first:

  ```bash
  binder enrich <corpus> --dry-run --json | jq -c '.result.files[] | select(.added|length>0)'
  binder enrich <corpus> --verified-by "" --json      # apply
  ```

  `enrich` never *invents* a verifier, but it **will** stamp `verified` from a
  configured actor (`BINDER_VERIFIED_BY`, or `verified_by:` in a `.binder.yaml`) —
  writing an attestation into your source that nobody typed. The dry-run discloses
  it: `added` will contain `verified`. `--verified-by ""` suppresses it and is the
  only mitigation, since nothing removes a stamp afterwards. See
  [`trust-discipline.md`](trust-discipline.md).

  Enrich is idempotent **only within a single clock second** — stamps dedupe on
  `(by, at)`, so runs seconds apart append a new stamp for the same actor. Pin
  `SOURCE_DATE_EPOCH` for a repeatable run.

  If a required key needs a real value you can assert (e.g. a specific `type`),
  pass `--type-map`/`--default-type` rather than letting it default blindly.

- **Structural / link issues**: edit the source markdown. Do not invent a target
  file just to satisfy a link.

- **Trust fields** (`verified`, `sources`): *propose to the user*, never invent.
  See [`trust-discipline.md`](trust-discipline.md).

## D. The accept loop

Iterate until the bundle is conformant and the residual advisories are
understood, not hidden:

```bash
binder convert <corpus> -o <bundle> --verified-by "" --json | jq '.result | {num_concepts, num_unresolved}'
binder validate <bundle> --json | jq '.result.findings'   # [] ⇒ conformant, exit 0
binder review   <bundle> --json | jq '.result | {by_type, tiers, orphans, stale, unresolved}'
```

- `--verified-by ""` on `convert` is not optional boilerplate. `convert` resolves
  `--verified-by` through the same flag > env > config file > default chain as
  `enrich`, so a configured actor stamps **every concept in the bundle** and
  `review` reports them `human-reviewed` instead of `unverified`. Run all of this
  from the directory you checked in step 1 — binder resolves `./.binder.yaml`
  against the current directory, so a pre-flight run elsewhere protects nothing.
  See [`trust-discipline.md`](trust-discipline.md).
- `validate` must reach `findings: []` (exit 0). That is the hard gate.
- `review` advisories (orphans, stale, unresolved, tiers) are for *understanding*,
  not zeroing — decide per the table in §B whether each is expected.
- Optionally `binder graph <bundle> --json` to inspect the resolved edge set.

Stop when `validate` is conformant and every remaining advisory is one you have
consciously accepted. Then hand off with a summary of remediated-vs-accepted.
