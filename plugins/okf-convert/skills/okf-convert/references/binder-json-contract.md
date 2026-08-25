# binder `--json` contract

Load this when you need the exact shape of what a binder command emits, so you
**parse structured output with `jq`** and never scrape prose. Every shape below
was captured from real `binder/0.5.1` output. The `internal/plugindocs` drift
gate mechanically checks each block's **shape** — its key set at every nesting
level — against the live binary, matching each object to its live shape by
structural position (not key similarity) so an object stays checked however far
it has drifted. It does **not** check **values**: version literals like the
`binder` field below are not verified and can go stale (tracked in #169).
Free-form data maps whose keys are data, not schema (`by_type`, `tiers`), are
exempt. Regenerate a block by recapturing it rather than hand-editing (issue
#106).

## The report envelope (`binder.report/v1`)

`convert`, `enrich`, `validate`, `review`, `lint`, and `infer` all wrap their
result in the same envelope:

```json
{
  "binder":  "binder/0.5.1",
  "command": "convert",
  "schema":  "binder.report/v1",
  "result":  { }
}
```

- `binder` — the producing version.
- `command` — one of `convert | enrich | validate | review | lint | infer`.
- `schema` — `binder.report/v1` for all six.
- `result` — the per-command payload (below).

**Two commands do NOT use this envelope — do not expect it:**

- **`binder graph --json`** (alias for `--format json`) emits the **raw graph
  export** `{ "nodes": [...], "edges": [...] }`, *not* the report envelope.
  Combining `--json` with a conflicting `--format` is a usage error (exit 2).
- **`binder config --json`** emits schema **`binder.config/v1`** (still an
  envelope with `binder`/`command`/`schema`/`result`, but a *different* schema
  string).

## Exit codes (the 4-value contract)

Check `$?`, not the text:

| Code | Meaning | When |
|------|---------|------|
| `0` | ok | conformant / no gating findings |
| `1` | findings / gate tripped | `validate` non-conformance (spec §11); or, under `--strict`, a command's advisory set promoted to gating findings (`review`/`lint`/`convert`/`enrich`) — for `enrich` that set is skipped files, preserve-or-advise warnings, and a non-conformant `--status-map` OKF §5.4 value. The read-boundary normalization advisory is excluded from those sets and never gates: an advisory-only run exits `0` with or without `--strict` |
| `2` | usage error | bad flags/args (e.g. `convert` with no `<src>`, conflicting `graph` flags) |
| `3` | I/O error | path missing / unreadable / unwritable |

Without `--strict`, `convert`/`enrich`/`lint`/`review` are **never-reject**: they
report advisories and still exit `0`. Only `validate` gates on the §11 hard rule
by default.

## Per-command `result` shapes

### `convert --json` (and `--dry-run`)

```jsonc
{
  "src": "corpus", "out": "bundle",           // out == "" under --dry-run
  "concepts": [ { "rel_path": "notes/a.md", "type": "Note",
                  "title": "A", "num_links": 1, "num_unresolved": 0 } ],
  "warnings":   [],                            // recovered/unparseable frontmatter etc.
  "unresolved": [ { "from": "docs/guide.md",
                    "raw_target": "/docs/nope.md", "text": "missing" } ],
  "num_concepts": 3, "num_links": 2, "num_resolved": 1,
  "num_unresolved": 1, "num_recovered": 0, "dry_run": true,
  "status_notes": [],                          // advisory notes about the run
  "verified": {                                // trust disclosure (0.3.1+); see below
    "actor": "", "source": "none",             // "" / "none" when nothing was stamped
    "stamped": [], "num_stamped": 0,           // out-relative paths that got the stamp
    "skipped": [], "num_skipped": 0            // co-signs declined (different identity)
  }                                            // + "note" when a repo-local config was ignored
}
```

```bash
binder convert <corpus> --dry-run --json \
  | jq '.result | {num_concepts, num_links, num_unresolved, num_recovered}'
binder convert <corpus> --dry-run --json | jq '.result.unresolved'
```

### `validate --json`

```jsonc
{
  "root": "bundle", "num_concepts": 3, "num_reserved": 2,
  "findings": [ { "concept_id": "x", "severity": "error",
                  "message": "missing non-empty 'type' (spec §11.2)" } ],
  "reserved_structure_checked": false          // §11.3 NOT verified — see below
}
```

`findings: []` ⇒ exit `0`. Any error finding ⇒ exit `1`.

**`reserved_structure_checked` is currently `false`.** validate enforces §11.1
(parseable frontmatter) and §11.2 (non-empty `type`) only; the §11.3 reserved-file
structure rules are not checked. Read `findings: []` as *"no §11.1/§11.2
violations"*, not *"fully §11-conformant"*.

```bash
binder validate <bundle> --json | jq -c '.result | {findings, reserved_structure_checked}'
```

### `review --json`

```jsonc
{
  "root": "bundle", "today": "2026-08-15", "num_concepts": 3,
  "by_type": { "Guide": 2, "Note": 1 },
  "tiers":   { "unverified": 3 },              // DERIVED, never stored
  "orphans": [ "docs/guide" ],
  "entrypoints": [ "README" ],                 // roots the graph is reachable from
  "stale":   [],
  "attested": [],
  "unresolved": [ { "from": "docs/guide", "raw_target": "/docs/nope.md", "text": "missing" } ],
  "unparsed_frontmatter": [],
  "concepts": [ { "id": "docs/guide", "type": "Guide", "tier": "unverified",
                  "stale": false, "attested": false, "orphan": true,
                  "entrypoint": false } ]              // entrypoint: reachable root vs orphan
}
```

```bash
binder review <bundle> --json | jq '.result | {by_type, tiers, orphans, stale}'
```

### `lint --json` (reads the SOURCE corpus, not a bundle)

```jsonc
{
  "src": "corpus", "num_concepts": 3,
  "broken_links":      [ { "concept": "docs/guide", "detail": "/docs/nope.md" } ],
  "missing_titles":    [ "docs/notitle" ],
  "orphans":           [ "docs/notitle" ],
  "entrypoints":       [ "README" ],
  "stale":             [],
  "schema_violations": [ { "concept": "adr/one", "detail": "missing type" } ]
}
```

`lint` sees the corpus *as authored* — a missing `title:`/`type:` that `convert`
would silently default is visible here. Advisory (exit 0) unless `--strict`.

```bash
binder lint <corpus> --json | jq '.result | {broken_links, missing_titles, schema_violations}'
```

### `enrich --json` (mutates SOURCE frontmatter in place; use `--dry-run` first)

```jsonc
{
  "src": "corpus", "dry_run": true,
  "num_files": 3, "num_enriched": 3, "num_unchanged": 0, "num_skipped": 0,
  "files": [ { "path": "adr/one.md", "status": "would-enrich",
               "added": [ "generated", "title", "type" ] } ],
  "warnings": [], "status_notes": [],
  // "normalizations": [ ... ]                 // present only when a file's bytes were
                                               // normalized at the read boundary (BOM
                                               // stripped / lone CR translated); reported,
                                               // never gates --strict
  "verified": {                                // trust disclosure (0.3.1+); same shape as convert
    "actor": "", "source": "none",
    "stamped": [], "num_stamped": 0,
    "skipped": [], "num_skipped": 0
  }
}
```

`status` ∈ `enriched | unchanged | would-enrich | skipped`. `added` is the sorted
list of injected keys.

**`.result.normalizations` is a disclosure, not a finding.** `enrich --strict`
gates on skipped files, preserve-or-advise `warnings`, and a non-conformant
`--status-map` OKF §5.4 value; a read-boundary normalization is
always reported (here and as a per-file `normalized` signal) and exits `0`
([#154](https://github.com/ghchinoy/binder/issues/154)).

**`.result.verified` is the run-level trust disclosure (0.3.1+).** It is `actor:
"", source: "none"` when nothing was stamped. A stamp is written only from an
explicit `--verified-by` or a default *you* set — `verified_by:` in your
**global** config. Neither `BINDER_VERIFIED_BY` (env) nor a repo-local
`.binder.yaml` stamps; each is refused and reported in `.result.verified.note`
instead. When a stamp is written, `actor`/`source` name it, `stamped` lists the
paths, and the per-file `added` also gains `verified`. When a *different* identity
already attested a concept, a non-explicit default declines to co-sign: the
concept appears under `skipped` (`{ "path": ..., "existing_actor": ... }`) and the
prior attestation is left untouched. Inspect the disclosure and per-file `added`
before applying, and pass `--verified-by ""` to suppress a global default you
cannot vouch for:

```bash
binder enrich <corpus> --dry-run --json | jq -c '.result.verified'
binder enrich <corpus> --dry-run --json | jq -c '.result.files[] | select(.added|length>0)'
```

Idempotent **only within one clock second** — `verified` stamps dedupe on
`(by, at)`, so reruns seconds apart append another stamp for the same actor. Pin
`SOURCE_DATE_EPOCH` for a repeatable run.

### `graph --json` (raw export, NOT the envelope)

```jsonc
{
  "nodes": [ { "id": "adr/one", "title": "ADR One", "type": "Note",
               "tier": "unverified", "stale": false } ],
  "edges": [ { "from": "docs/guide", "to": "adr/one", "text": "adr" } ]
}
```

```bash
binder graph <bundle> --json | jq '{n: (.nodes|length), e: (.edges|length)}'
```

### `config --json` (schema `binder.config/v1`)

```jsonc
{
  "binder": "binder/0.5.1", "command": "config", "schema": "binder.config/v1",
  "result": {
    "config_file": "/home/u/.config/binder/config.yaml",   // "" when none
    "values": {
      "default_type":    { "value": "Note",                  "source": "default" },
      "gemini_backend":  { "value": "auto",                  "source": "default" },
      "gemini_location": { "value": "global",                "source": "default" },
      "gemini_model":    { "value": "gemini-3.5-flash-lite", "source": "default" },
      "gemini_project":  { "value": "",                      "source": "default" },
      "verified_by":     { "value": "human:alice",           "source": "file"    }
    }
  }
}
```

Every value is an object with `value` and `source`, where `source` is one of
`default | env | file` (a `flag` source never appears — `config` takes no such
flags). Use this to confirm what `--default-type`/`--verified-by` will be
**before** a run.

`config_file` is the single file actually in effect. binder loads **exactly one**
config file: a repo-local `./.binder.yaml` suppresses the global
`$XDG_CONFIG_HOME/binder/config.yaml` entirely rather than merging with it. This
matters for trust — `source: "file"` alone cannot tell a deliberate per-corpus
`verified_by` from a machine-wide default, so read `config_file` alongside it:

```bash
binder config --json | jq -c '.result | {config_file, verified_by: .values.verified_by}'
```
