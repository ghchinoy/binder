## binder project

Project a bundle into offline property-graph DDL (Spanner SQL/PGQ)

### Synopsis

Project emits a deterministic, credential-free property-graph schema for a
loaded OKF bundle. It writes schema.ddl (CREATE TABLE Nodes, Edges and the
NodeVerified attestation table plus a CREATE PROPERTY GRAPH wrapper with a
single LINKS edge label) to --out and prints a binder.report/v1 summary to
stdout.

The projection reuses the same node/edge model as `binder graph`,
`list_graphs`, and `query_graph`, so it stays in edge/identity parity by
construction. Node identity (node_key) is the concept's authored frontmatter
value under --id-key when present and non-empty, otherwise the path-derived
concept id; binder NEVER mints a key. The tier/stale columns are the frozen
projection-time snapshot as of --today (SOURCE_DATE_EPOCH-honoring); stale_after
carries the raw authored input so stale stays re-derivable.

Alongside schema.ddl it emits the loader row data (nodes.csv, edges.csv,
load.sql) and the provenance artifacts node_verified.csv (the verified[]
attestations, byte-faithful: order preserved, by/at verbatim, is_human = the
human: prefix) and derivation.sql (a CREATE VIEW that recomputes tier/stale
from stale_after and NodeVerified for any date). --target defaults to spanner
and is the only accepted value in this release. No cloud credentials are used
or needed.

```
binder project <bundle> --out <dir> [flags]
```

### Options

```
  -h, --help            help for project
      --id-key string   authored frontmatter key to use as node identity; falls back to path identity per concept
      --out string      output directory for emitted artifacts (required)
      --target string   projection target dialect (only "spanner" in v0.4.0) (default "spanner")
      --today string    date (YYYY-MM-DD) used for the frozen tier/stale snapshot; defaults to now
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

