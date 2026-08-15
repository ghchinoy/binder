# binder `--json` contract

Load this when you need the exact shape of what a binder command emits, so you
**parse structured output with `jq`** and never scrape prose. Every shape below
was taken from real `binder/0.1.0` output.

## The report envelope (`binder.report/v1`)

`convert`, `enrich`, `validate`, `review`, and `lint` all wrap their result in the
same envelope:

```json
{
  "binder":  "binder/0.1.0",
  "command": "convert",
  "schema":  "binder.report/v1",
  "result":  { }
}
```

- `binder` — the producing version.
- `command` — one of `convert | enrich | validate | review | lint`.
- `schema` — `binder.report/v1` for all five.
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
| `1` | findings / gate tripped | `validate` non-conformance (spec §11); or, under `--strict`, any advisory promoted to a gate (`review`/`lint`/`convert`/`enrich`) |
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
  "num_unresolved": 1, "num_recovered": 0, "dry_run": true
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
                  "message": "missing non-empty 'type' (spec §11.2)" } ]
}
```

`findings: []` ⇒ conformant ⇒ exit `0`. Any error finding ⇒ exit `1`.

```bash
binder validate <bundle> --json | jq '.result.findings'
```

### `review --json`

```jsonc
{
  "root": "bundle", "today": "2026-08-15", "num_concepts": 3,
  "by_type": { "Guide": 2, "Note": 1 },
  "tiers":   { "unverified": 3 },              // DERIVED, never stored
  "orphans": [ "docs/guide" ],
  "stale":   [],
  "attested": [],
  "unresolved": [ { "from": "docs/guide", "raw_target": "/docs/nope.md", "text": "missing" } ],
  "unparsed_frontmatter": [],
  "concepts": [ { "id": "docs/guide", "type": "Guide", "tier": "unverified",
                  "stale": false, "attested": false, "orphan": true } ]
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
  "warnings": []
}
```

`status` ∈ `enriched | unchanged | would-enrich | skipped`. `added` is the sorted
list of injected keys. Only ever adds **absent** keys (`type`/`title`/`generated`);
byte-faithful and idempotent.

```bash
binder enrich <corpus> --dry-run --json | jq '.result.files'
```

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
  "binder": "binder/0.1.0", "command": "config", "schema": "binder.config/v1",
  "result": { "config_file": "", "values": { "default_type": "…", "verified_by": "…" } }
}
```

Each value carries its resolved source (flag > env > file > default). Use it to
confirm what `--default-type`/`--verified-by` will be before a run.
