-- schema.ddl — offline OKF property-graph projection (binder project).
-- Target: Spanner (GoogleSQL SQL/PGQ). The relational schema (tables/columns/
-- keys) is target-neutral; only CREATE PROPERTY GRAPH is Spanner-dialect.
-- Deterministic; identifier sanitization and column order are fixed.
--
-- This file declares the tables (Nodes, Edges, and the NodeVerified attestation
-- child table) and the property graph. binder project emits it alongside the
-- companion row files nodes.csv / edges.csv / node_verified.csv, the DML loader
-- load.sql that populates Nodes and Edges, and derivation.sql (a view that
-- recomputes tier/stale from stale_after + NodeVerified).

CREATE TABLE Nodes (
  node_key    STRING(MAX) NOT NULL,  -- OQ-6: NodeKeyFor(concept, id_key); never minted
  title       STRING(MAX),
  type        STRING(MAX),           -- OQ-7: the OKF concept type
  tier        STRING(MAX),           -- OQ-8: frozen trust-tier snapshot as of projected_as_of
  stale       BOOL,                  -- OQ-8: frozen staleness snapshot as of projected_as_of
  stale_after DATE,                  -- OQ-8: raw authored input; keeps stale re-derivable
) PRIMARY KEY (node_key);

CREATE TABLE Edges (
  from_key STRING(MAX) NOT NULL,  -- = Edge.From (source node_key)
  to_key   STRING(MAX) NOT NULL,  -- = Edge.To (target node_key)
  rel      STRING(MAX),           -- OQ-7: Edge.Text, nullable; single LINKS label
) PRIMARY KEY (from_key, to_key, rel);

CREATE TABLE NodeVerified (
  node_key STRING(MAX) NOT NULL,  -- = Nodes.node_key; never minted
  seq      INT64 NOT NULL,        -- OQ-8: stable index within the concept's verified[]
  by       STRING(MAX),           -- OQ-8: verified[].by, verbatim as authored
  at       STRING(MAX),           -- OQ-8: verified[].at, verbatim as authored
  is_human BOOL,                  -- OQ-8: okf.IsHumanActor(by) — the TrustTier predicate
) PRIMARY KEY (node_key, seq);

CREATE PROPERTY GRAPH OkfGraph
  NODE TABLES (Nodes)
  EDGE TABLES (
    Edges
      SOURCE KEY (from_key) REFERENCES Nodes (node_key)
      DESTINATION KEY (to_key) REFERENCES Nodes (node_key)
      LABEL LINKS
  );
