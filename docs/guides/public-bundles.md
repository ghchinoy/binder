# Public-domain OKF bundles

You can test `binder` against real knowledge graphs right away without building a
corpus from scratch. Clone the companion
[`ghchinoy/okf-bundles`](https://github.com/ghchinoy/okf-bundles) repository
(CC0-1.0 public domain) or inspect the four Google Cloud reference bundles in
`testdata/okf-bundles/`.

## The `ghchinoy/okf-bundles` collection

Clone the repository anywhere on disk:

```bash
git clone --depth 1 https://github.com/ghchinoy/okf-bundles.git /tmp/okf-bundles
```

The checkout holds four source corpora (`corpora/`), their converted OKF v0.2
bundles (`bundles/`), and their Spanner SQL/PGQ and graph exports
(`projections/`):

| Bundle | Concepts | Edges | Trust Tiers | OKF v0.2 Features Exercised |
|---|:---:|:---:|---|---|
| **`nist-ai-rmf`** | 26 | 104 | 26 `human-reviewed` | NIST AI RMF 1.0 (`Function`, `Category`, `Trustworthiness Characteristic`), 2 `Attested Computation` SQL metrics, `parent`/`related` edges |
| **`ietf-rfcs`** | 17 | 51 | 17 `machine-confirmed` | Core Web/Security RFCs (`RFC`, `Internet-Draft`), `status` lifecycle (`stable`, `draft`, `deprecated`), `stale_after` dates |
| **`cybersecurity-cwe-attack`** | 18 | 60 | 1 `human-reviewed`, 17 `machine-confirmed` | MITRE CWE (`Weakness`), ATT&CK (`Technique`), OWASP/MITRE (`Mitigation`), and an `Attested Computation` SIEM SQL detection |
| **`classical-mythology`** | 24 | 151 | 24 `human-reviewed` | Bulfinch's Mythology (Project Gutenberg #4928): 5-type entity graph (`Deity`, `Hero`, `Place`, `Quest`, `Artifact`) |

For a step-by-step walkthrough of `review`, `graph`, and `project` on these
bundles, see [Part 3 of the tutorial](../tutorial.md#part-3-exploring-and-projecting-public-domain-knowledge-graphs-okf-bundles).

## Reproducible corpus-to-bundle pipeline

`ghchinoy/okf-bundles` uses a six-step `binder` script (`scripts/build.sh`).
You can adapt this pipeline for your own knowledge repositories:

```bash
export SOURCE_DATE_EPOCH=1785456000 # 2026-07-31T00:00:00Z for byte-identical builds

# 1. Lint the source corpus before conversion
binder lint --strict corpora/nist-ai-rmf

# 2. Convert to an OKF v0.2 bundle with frontmatter edges and a type-grouped catalog
binder convert corpora/nist-ai-rmf -o bundles/nist-ai-rmf \
  --fm-ref-keys "parent,related" \
  --group-by-type \
  --include-graph \
  --include-backlinks

# 3. Validate hard §11 conformance + §5/§7/§10 trust advisories
binder validate --strict bundles/nist-ai-rmf

# 4. Gate on a minimum trust tier floor
binder review --require-tier human-reviewed bundles/nist-ai-rmf

# 5. Project to offline Spanner SQL/PGQ DDL & CSV loader artifacts
binder project bundles/nist-ai-rmf \
  --target spanner \
  --today 2026-07-31 \
  --out projections/nist-ai-rmf

# 6. Export DOT, JSON, and self-contained HTML graph views
binder graph --format json bundles/nist-ai-rmf > projections/nist-ai-rmf/graph.json
binder graph --format dot  bundles/nist-ai-rmf > projections/nist-ai-rmf/graph.dot
binder graph --format html bundles/nist-ai-rmf > projections/nist-ai-rmf/viz.html
```

Write non-Markdown outputs (`graph.json`, `graph.dot`, `viz.html`, `schema.ddl`,
`*.csv`) to a sibling directory (`projections/`), not inside `bundles/`. That
keeps the bundle directory a pure OKF v0.2 Markdown tree.

## Querying bundles over MCP (`binder mcp`)

Register `binder mcp` in your agent host (see [MCP server](/binder/agent/mcp/)).
Your agent can then inspect a bundle's schema with `list_graphs` and run bounded
traversals with `query_graph` without reading every Markdown file into context:

1. **Schema introspection (`list_graphs`):** Pass
   `{"bundle": "/tmp/okf-bundles/bundles/classical-mythology"}`. The tool
   returns the node labels (`Artifact`, `Deity`, `Hero`, `Place`, `Quest`),
   node and edge counts (24 nodes, 151 edges), and node properties (`id`,
   `title`, `type`, `tier`, `stale`).
2. **One-hop neighbors (`query_graph`):** Run `op: "neighbors"` on
   `id: "heroes/odysseus"` to list every linked deity, hero, place, and quest.
3. **Bounded BFS (`query_graph`):** Run `op: "neighborhood"` with
   `id: "functions/measure"` and `depth: 2` on `bundles/nist-ai-rmf`. It steps
   from `MEASURE` into its categories (`measure-1`, `measure-2`, `measure-3`)
   and reaches the two `Attested Computation` metrics (`demographic-parity-ratio`,
   `calibration-drift-score`).
4. **Shortest path (`query_graph`):** Run `op: "path"` from
   `from: "weaknesses/cwe-89"` to `to: "mitigations/parameterized-queries"` on
   `bundles/cybersecurity-cwe-attack` to verify the mitigation chain.

See the [user guide](../user_guide.md#the-graph-surface) for the complete
`list_graphs` and `query_graph` schemas.
