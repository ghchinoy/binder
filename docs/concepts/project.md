# Graph projection (`project`, Spanner)

```text
binder project <bundle> --out <dir> [flags]
```

`binder project` projects a loaded bundle into a **deterministic,
credential-free** property-graph schema **and its loader row data**, writing them
to `--out`. It emits:

- `schema.ddl` — `CREATE TABLE Nodes`, `CREATE TABLE Edges`, `CREATE TABLE
  NodeVerified` (the attestation child table), and a `CREATE PROPERTY GRAPH`
  wrapper with a single `LINKS` edge label;
- `nodes.csv` and `edges.csv` — the row data for the two tables, one header row
  naming the columns in `schema.ddl` order followed by one row per node/edge;
- `load.sql` — DML `INSERT` statements that populate `Nodes` and `Edges` with the
  same rows (GoogleSQL has no SQL statement that bulk-loads a CSV, so the CSVs are
  the bulk-import representation for tooling such as `gcloud spanner databases
  import` and `load.sql` is the credential-free, tool-free loader);
- `node_verified.csv` — one row per `verified[]` attestation, copied losslessly
  from the source (see below);
- `derivation.sql` — a `CREATE VIEW` that recomputes `tier`/`stale` from the
  stored facts, so no consumer is stuck with the frozen snapshot.

It also prints a `binder.report/v1` summary (command `project`) to stdout, uses
**no cloud credentials**, and contacts no service.

The projection reuses the same node/edge model as `graph`, `list_graphs`, and
`query_graph` (see [Relationship extraction & the graph](graph.md)), so the
emitted edge set and node identity stay in parity by construction. No graph is
created or populated remotely. This command emits offline DDL text only.

| Flag | Default | Purpose |
|---|---|---|
| `--out` | *(required)* | Output directory for emitted artifacts (`schema.ddl`, `nodes.csv`, `edges.csv`, `load.sql`, `node_verified.csv`, `derivation.sql`). |
| `--target` | `spanner` | Projection target dialect. `spanner` (Spanner Graph, GoogleSQL SQL/PGQ) is the only accepted value in this release; any other value is a usage error (exit 2). |
| `--id-key` | *(none)* | Authored frontmatter key to use as the node identity (`node_key`). When a concept carries this key as a non-empty string, that value is the key (`strategy: frontmatter`); otherwise it falls back to the path-derived concept id (`strategy: path`). binder **never mints** a key. |
| `--today` | now | Date (`YYYY-MM-DD`) used for the frozen `tier`/`stale` snapshot and echoed as `projected_as_of`. Honors `SOURCE_DATE_EPOCH`. |

The `Nodes` table carries `node_key`, `title`, `type`, `tier`, `stale`, and
`stale_after`. `tier` and `stale` are the **projection-time snapshot** as of
`--today` (identical to what `graph`/`review` derive for the same date);
`stale_after` is the raw authored date input, so `stale` stays re-derivable. The
`Edges` table carries `from_key`, `to_key`, and the nullable `rel` (the link
text; labels are **not** derived from it). The DDL is byte-deterministic: fixed
column order and deterministic identifier sanitization. `binder project` never
mints a key and never writes back to the source bundle — it only reads and emits.

## Provenance completeness: `NodeVerified` + the derivation view

`tier` is a *derived* value, never stored on disk — the raw truth is each
concept's `verified[]` list. So the projection also emits that list losslessly.

`node_verified.csv` (and the matching `CREATE TABLE NodeVerified`, keyed
`(node_key, seq)`) carries one row per attestation: `node_key`, `seq` (the stable
index within the concept's `verified[]`), `by`, `at`, and `is_human`. The rows are
copied **losslessly**: authored order is preserved, `by`/`at` are verbatim as
authored, and `is_human` is exactly the `human:` actor-prefix test that drives the
trust tier (a node is `human-reviewed` when any attestation is human, else
`machine-confirmed`, else `unverified` — see [Trust model & tiers](trust.md)).

`derivation.sql` is a `CREATE VIEW NodeTrustDerived` that **recomputes** `tier`
and `stale` from the stored facts (`Nodes.stale_after` and the `NodeVerified`
table) rather than reading the frozen `Nodes.tier`/`Nodes.stale` columns. The
view recomputes as of `CURRENT_DATE()`; substitute a `DATE` literal to recompute
for any chosen date. Because the frozen `tier`/`stale` in `Nodes` are only a
snapshot as of `projected_as_of`, this view lets a consumer re-derive the current
verdict for any date without re-running binder.

For the full `project` reference, see the
[user guide](../user_guide.md#project).
