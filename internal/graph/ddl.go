// This file adds the offline property-graph DDL projection over the existing
// in-memory model (Build/Node/Edge). It introduces NO new parsing and NO new
// edge or identity logic: Project calls Build and EdgesFromConcepts VERBATIM and
// derives node identity via NodeKeyFor, so the schema `binder project` emits is
// in edge/identity/tier parity — by construction — with what `binder graph`,
// `list_graphs` and `query_graph` already produce. There is no second projection
// path. It is additive and read-only: it never writes to a bundle, never mutates
// frontmatter, and NEVER mints an identity (design v0.4.0 §4.1–§4.5).
//
// G1 emits schema only (CREATE TABLE Nodes + Edges + CREATE PROPERTY GRAPH). The
// Projection's Nodes/Edges slices are the seam a later phase reads to emit row
// data additively, so row emission is an extension of this file, not a rewrite.
package graph

import (
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// Target is the SQL/PGQ dialect the projection emits. Spanner (GoogleSQL) is the
// only supported target in v0.4.0. The relational schema (tables/columns/keys)
// is target-neutral; only the CREATE PROPERTY GRAPH wrapper is dialect-specific,
// so a future target (e.g. BigQuery) is an additive emitter, not a rewrite
// (design §4.5, Non-Goal 4).
type Target string

// TargetSpanner emits GoogleSQL SQL/PGQ (Spanner Graph). It is the default and
// the only accepted value in v0.4.0.
const TargetSpanner Target = "spanner"

// ProjectOptions configures a bundle→property-graph projection.
type ProjectOptions struct {
	Target Target // dialect target; TargetSpanner when empty
	IDKey  string // authored frontmatter key for node identity; "" ⇒ path identity
	Today  string // YYYY-MM-DD used for the frozen tier/stale snapshot
}

// ProjectedNode is one node's projected values: the never-minted node_key, the
// frozen tier/stale snapshot mirrored VERBATIM from the in-memory Node, and the
// raw stale_after input that keeps the frozen stale re-derivable (OQ-8).
type ProjectedNode struct {
	Key        string   // node_key: NodeKeyFor(concept, id_key); NEVER minted
	Strategy   string   // per-node identity strategy: "frontmatter" | "path"
	Title      string   // = Node.Title
	Type       string   // = Node.Type (the OKF concept type)
	Tier       okf.Tier // = Node.Tier (frozen snapshot as of AsOf)
	Stale      bool     // = Node.Stale (frozen snapshot as of AsOf)
	StaleAfter string   // raw authored stale_after (spec §5.5); "" if absent
}

// IdentityStability is the graph-level re-rooting signal (OQ-6). The projection
// is re-rooting-stable only when an authored id_key resolved on EVERY node; any
// path fallback makes the populated graph vulnerable to a moved corpus root.
type IdentityStability struct {
	ReRootingStable   bool `json:"re_rooting_stable"`
	PathFallbackCount int  `json:"path_fallback_count"`
}

// Projection is the deterministic result of projecting a loaded bundle into the
// relational property-graph model. It is computed ONCE from Build and
// EdgesFromConcepts, so nodes/edges and the frozen tier/stale snapshot are in
// parity with the shipped surfaces by construction. Edges carry the path-derived
// From/To (EdgesFromConcepts verbatim), which the parity assertion checks; the
// mapping to node_key for row emission is a later, additive phase.
type Projection struct {
	Target   Target
	Nodes    []ProjectedNode
	Edges    []Edge
	NodeKey  NodeKey // strategy + requested key (echoes the list_graphs vocabulary)
	Identity IdentityStability
	Counts   Counts
	AsOf     string // projected_as_of: the `today` the frozen snapshot reflects
}

// Project builds the deterministic projection. tier/stale are read from Build's
// output VERBATIM (the frozen-at-projection snapshot, identical to what
// `list_graphs`/`query_graph` compute for the same `today`); node_key comes from
// NodeKeyFor (authored-or-path, NEVER minted); edges are EdgesFromConcepts
// verbatim. Target defaults to TargetSpanner when unset (validation of an
// unsupported target is the caller's job — a usage error at the CLI boundary).
func Project(b *okf.Bundle, opts ProjectOptions) *Projection {
	target := opts.Target
	if target == "" {
		target = TargetSpanner
	}

	m := Build(b, opts.Today)

	byID := make(map[string]*okf.Concept, len(b.Concepts))
	for _, c := range b.Concepts {
		byID[c.ID] = c
	}

	nodes := make([]ProjectedNode, 0, len(m.Nodes))
	pathFallback := 0
	frontmatterCount := 0
	for _, n := range m.Nodes {
		// Default to the path identity so a node missing from byID (impossible in
		// practice — Build's nodes come from b.Concepts) is never minted a key.
		key, strategy := n.ID, "path"
		staleAfter := ""
		if c := byID[n.ID]; c != nil {
			key, strategy = NodeKeyFor(c, opts.IDKey)
			staleAfter = c.Trust.StaleAfter
		}
		if strategy == "frontmatter" {
			frontmatterCount++
		} else {
			pathFallback++
		}
		nodes = append(nodes, ProjectedNode{
			Key:        key,
			Strategy:   strategy,
			Title:      n.Title,
			Type:       n.Type,
			Tier:       n.Tier,
			Stale:      n.Stale,
			StaleAfter: staleAfter,
		})
	}

	// Graph-level strategy echoes the list_graphs vocabulary (schema.go Describe):
	// "frontmatter" only when an id_key was requested AND resolved on ≥1 concept;
	// the requested key is echoed back regardless so a caller sees what was asked.
	strategy := "path"
	key := ""
	if opts.IDKey != "" {
		key = opts.IDKey
		if frontmatterCount > 0 {
			strategy = "frontmatter"
		}
	}

	return &Projection{
		Target:  target,
		Nodes:   nodes,
		Edges:   EdgesFromConcepts(b.Concepts),
		NodeKey: NodeKey{Strategy: strategy, Key: key},
		Identity: IdentityStability{
			ReRootingStable:   strategy == "frontmatter" && pathFallback == 0,
			PathFallbackCount: pathFallback,
		},
		Counts: Counts{Nodes: len(nodes), Edges: len(m.Edges)},
		AsOf:   opts.Today,
	}
}

// ddlColumn is one relational column, rendered in declaration order. The column
// SLICES below are the single source of column order for the DDL; changing a
// slice is the only way to change emitted column order, so the order is fixed and
// auditable in one place.
type ddlColumn struct {
	name    string // emitted (and sanitized) identifier
	typ     string // target-neutral SQL type
	notNull bool   // NOT NULL constraint
	comment string // trailing "-- comment" (provenance/OQ note), or ""
}

// nodeColumns is the fixed Nodes column order (OQ-8, G1 scope: snapshot + raw
// inputs; the NodeVerified child table is G3, not here).
var nodeColumns = []ddlColumn{
	{"node_key", "STRING(MAX)", true, "OQ-6: NodeKeyFor(concept, id_key); never minted"},
	{"title", "STRING(MAX)", false, ""},
	{"type", "STRING(MAX)", false, "OQ-7: the OKF concept type"},
	{"tier", "STRING(MAX)", false, "OQ-8: frozen trust-tier snapshot as of projected_as_of"},
	{"stale", "BOOL", false, "OQ-8: frozen staleness snapshot as of projected_as_of"},
	{"stale_after", "DATE", false, "OQ-8: raw authored input; keeps stale re-derivable"},
}

// edgeColumns is the fixed Edges column order (OQ-7: single LINKS label, link
// text carried as the nullable rel property; labels are NOT derived from rel).
var edgeColumns = []ddlColumn{
	{"from_key", "STRING(MAX)", true, "= Edge.From (source node_key)"},
	{"to_key", "STRING(MAX)", true, "= Edge.To (target node_key)"},
	{"rel", "STRING(MAX)", false, "OQ-7: Edge.Text, nullable; single LINKS label"},
}

// ddlHeader is a fixed, provenance-free banner. It deliberately carries NO
// binder version, timestamp, or bundle path so schema.ddl is a stable byte-golden
// across environments and releases; the run-specific provenance (version,
// projected_as_of, counts) lives in the binder.report/v1 envelope instead.
const ddlHeader = `-- schema.ddl — offline OKF property-graph projection (binder project).
-- Target: Spanner (GoogleSQL SQL/PGQ). The relational schema (tables/columns/
-- keys) is target-neutral; only CREATE PROPERTY GRAPH is Spanner-dialect.
-- Deterministic; identifier sanitization and column order are fixed. G1 emits
-- schema only — no row data.
`

// propertyGraphDDL is the Spanner (GoogleSQL SQL/PGQ) CREATE PROPERTY GRAPH
// wrapper: one node table, one edge table, and the single LINKS edge label
// (OQ-7). This is the only Spanner-dialect fragment; the CREATE TABLE statements
// above it are target-neutral.
const propertyGraphDDL = `CREATE PROPERTY GRAPH OkfGraph
  NODE TABLES (Nodes)
  EDGE TABLES (
    Edges
      SOURCE KEY (from_key) REFERENCES Nodes (node_key)
      DESTINATION KEY (to_key) REFERENCES Nodes (node_key)
      LABEL LINKS
  );
`

// DDL renders the deterministic schema.ddl bytes: CREATE TABLE Nodes, CREATE
// TABLE Edges, then the CREATE PROPERTY GRAPH wrapper. In G1 the DDL is a pure
// function of the fixed column definitions (it carries no row data and no
// bundle-derived identifiers), so it is byte-identical run-to-run.
func (p *Projection) DDL() []byte {
	var b strings.Builder
	b.WriteString(ddlHeader)
	b.WriteString("\n")
	writeCreateTable(&b, "Nodes", nodeColumns, "node_key")
	b.WriteString("\n")
	writeCreateTable(&b, "Edges", edgeColumns, "from_key, to_key, rel")
	b.WriteString("\n")
	b.WriteString(propertyGraphDDL)
	return []byte(b.String())
}

// writeCreateTable emits one CREATE TABLE with columns aligned to the widest
// sanitized column name in the table (deterministic padding) and the PRIMARY KEY
// on the closing-paren line. primaryKey is the raw key expression; each column
// name and the table name are passed through sanitizeIdentifier.
func writeCreateTable(b *strings.Builder, table string, cols []ddlColumn, primaryKey string) {
	// defs holds each column's "<name><pad> <type>[ NOT NULL]," fragment. Comments
	// are aligned to the widest fragment so the emitted block is deterministic and
	// readable; both widths derive only from the fixed column slice.
	nameWidth := 0
	for _, c := range cols {
		if w := len(sanitizeIdentifier(c.name)); w > nameWidth {
			nameWidth = w
		}
	}
	defs := make([]string, len(cols))
	defWidth := 0
	for i, c := range cols {
		name := sanitizeIdentifier(c.name)
		def := name + strings.Repeat(" ", nameWidth-len(name)+1) + c.typ
		if c.notNull {
			def += " NOT NULL"
		}
		def += ","
		defs[i] = def
		if len(def) > defWidth {
			defWidth = len(def)
		}
	}
	b.WriteString("CREATE TABLE ")
	b.WriteString(sanitizeIdentifier(table))
	b.WriteString(" (\n")
	for i, c := range cols {
		b.WriteString("  ")
		b.WriteString(defs[i])
		if c.comment != "" {
			b.WriteString(strings.Repeat(" ", defWidth-len(defs[i])))
			b.WriteString("  -- ")
			b.WriteString(c.comment)
		}
		b.WriteString("\n")
	}
	b.WriteString(") PRIMARY KEY (")
	b.WriteString(primaryKey)
	b.WriteString(");\n")
}

// sanitizeIdentifier renders s as a safe, deterministic GoogleSQL identifier: it
// keeps ASCII letters, digits and underscore, replaces every other rune with a
// single underscore, and prefixes an underscore when the result would otherwise
// begin with a digit. It is applied to EVERY emitted identifier so the DDL is
// injection-safe and byte-stable regardless of input. Every G1 identifier is a
// fixed constant, so this is the identity function for them today; it is the seam
// that keeps a later data/name-derived identifier deterministic. It never mints
// an identity — it only rewrites an identifier it was given.
func sanitizeIdentifier(s string) string {
	if s == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(s) + 1)
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
