# Public-domain OKF bundles

To explore `binder` against multi-type, provenance-attested knowledge graphs
without preparing a corpus first, use the companion
[`ghchinoy/okf-bundles`](https://github.com/ghchinoy/okf-bundles) repository
(published under CC0-1.0) or the four official Google Cloud reference bundles
vendored under `testdata/okf-bundles/`.

## The `ghchinoy/okf-bundles` collection

Clone the repository anywhere on disk:

```bash
git clone --depth 1 https://github.com/ghchinoy/okf-bundles.git /tmp/okf-bundles
```

It contains four source corpora (`corpora/`), their converted OKF v0.2 bundles
(`bundles/`), and their pre-computed Spanner SQL/PGQ and graph projections
(`projections/`):

| Bundle | Concepts | Edges | Trust Tiers | OKF v0.2 Features Exercised |
|---|:---:|:---:|---|---|
| **`nist-ai-rmf`** | 26 | 104 | 26 `human-reviewed` | NIST AI RMF 1.0 (`Function`, `Category`, `Trustworthiness Characteristic`), 2 `Attested Computation` SQL fairness/drift metrics, `parent`/`related` frontmatter edges |
| **`ietf-rfcs`** | 17 | 51 | 17 `machine-confirmed` | Core Web/Security RFCs (`RFC`, `Internet-Draft`), full `status` lifecycle (`stable`, `draft`, `deprecated`), `stale_after` dates |
| **`cybersecurity-cwe-attack`** | 18 | 60 | 1 `human-reviewed`, 17 `machine-confirmed` | MITRE CWE (`Weakness`), ATT&CK (`Technique`), OWASP/MITRE (`Mitigation`), and an `Attested Computation` SIEM SQL detection |
| **`classical-mythology`** | 24 | 151 | 24 `human-reviewed` | Bulfinch's Mythology (Project Gutenberg #4928): 5-type entity graph (`Deity`, `Hero`, `Place`, `Quest`, `Artifact`) for multi-hop graph traversal |

For a step-by-step walkthrough running `review`, `graph`, and `project` on these
bundles, see [Part 3 of the tutorial](../tutorial.md#part-3-exploring-and-projecting-public-domain-knowledge-graphs-okf-bundles).

## Reproducible corpus-to-bundle pipeline

`ghchinoy/okf-bundles` uses a deterministic six-step `binder` pipeline
(`scripts/build.sh`) that you can adapt for your own knowledge repositories:

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

Keep non-Markdown artifacts (`graph.json`, `graph.dot`, `viz.html`, `schema.ddl`,
`*.csv`) in a sibling directory (`projections/`) rather than inside `bundles/` so
the bundle directory remains a pure OKF v0.2 Markdown tree.

## Querying bundles over MCP (`binder mcp`)

When `binder mcp` is registered in your agent host (see [MCP server](/binder/agent/mcp/)),
your agent can inspect a bundle's graph schema with `list_graphs` and run bounded
traversals with `query_graph` without loading all concept files into the context
window:

1. **Schema introspection (`list_graphs`):** Call `list_graphs` with
   `{"bundle": "/tmp/okf-bundles/bundles/classical-mythology"}` to discover the
   node labels (`Artifact`, `Deity`, `Hero`, `Place`, `Quest`), node/edge counts
   (24 nodes, 151 edges), and node properties (`id`, `title`, `type`, `tier`,
   `stale`).
2. **Neighborhood & path queries (`query_graph`):**
   - `op: "neighbors"` on `id: "heroes/odysseus"` returns every directly linked
     deity, hero, place, and quest.
   - `op: "bfs"` with `id: "functions/measure"` and `depth: 2` on
     `bundles/nist-ai-rmf` traverses from the `MEASURE` function into its
     categories (`measure-1`, `measure-2`, `measure-3`) and out to the
     sanctioned `Attested Computation` metrics (`demographic-parity-ratio`,
     `calibration-drift-score`).
   - `op: "path"` between `id: "weaknesses/cwe-89"` and
     `to: "mitigations/parameterized-queries"` on
     `bundles/cybersecurity-cwe-attack` verifies the defensive path in the
     concept graph.

See the [user guide](../user_guide.md#the-graph-surface) for the complete
`list_graphs` and `query_graph` input/output schemas.
