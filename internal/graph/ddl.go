// This file adds the offline property-graph DDL projection over the existing
// in-memory model (Build/Node/Edge). It introduces NO new parsing and NO new
// edge or identity logic: Project calls Build and EdgesFromConcepts VERBATIM and
// derives node identity via NodeKeyFor, so the schema `binder project` emits is
// in edge/identity/tier parity — by construction — with what `binder graph`,
// `list_graphs` and `query_graph` already produce. There is no second projection
// path. It is additive and read-only: it never writes to a bundle, never mutates
// frontmatter, and NEVER mints an identity (design v0.4.0 §4.1–§4.5).
//
// DDL renders the schema text (CREATE TABLE Nodes/Edges/NodeVerified + CREATE
// PROPERTY GRAPH); the Projection's Nodes/Edges/verified slices are the seam the
// companion emitters read to produce the row data (nodes.csv, edges.csv,
// node_verified.csv), the DML loader (load.sql) and the derivation view
// (derivation.sql). Each emitter is a pure read over the same in-memory model.
package graph

import (
	"bytes"
	"encoding/csv"
	"sort"
	"strconv"
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

	// Verified is the concept's verified[] attestations projected VERBATIM
	// (okf.ProjectTrust order-preserving; see projectActorstamps). It is the
	// lossless source the frozen Tier derives from and the verbatim input to
	// the NodeVerified child table (OQ-8 item 3). Copied, never re-derived.
	Verified []okf.Actorstamp
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

	// keyByID maps concept path-ID → emitted node_key, used only by row emission
	// (edges reference node_key). Unexported: it is a derivation of the Nodes
	// slice, not part of the report contract.
	keyByID map[string]string
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
	// keyByID maps a concept's path-derived ID (the Edge.From/To vocabulary) to its
	// emitted node_key, so edge row emission can carry referential integrity to the
	// Nodes table without a second identity path (row data, G2).
	keyByID := make(map[string]string, len(m.Nodes))
	pathFallback := 0
	for _, n := range m.Nodes {
		// Default to the path identity so a node missing from byID (impossible in
		// practice — Build's nodes come from b.Concepts) is never minted a key.
		key, strategy := n.ID, "path"
		staleAfter := ""
		var verified []okf.Actorstamp
		if c := byID[n.ID]; c != nil {
			key, strategy = NodeKeyFor(c, opts.IDKey)
			staleAfter = c.Trust.StaleAfter
			verified = c.Trust.Verified
		}
		if strategy != "frontmatter" {
			pathFallback++
		}
		keyByID[n.ID] = key
		nodes = append(nodes, ProjectedNode{
			Key:        key,
			Strategy:   strategy,
			Title:      n.Title,
			Type:       n.Type,
			Tier:       n.Tier,
			Stale:      n.Stale,
			StaleAfter: staleAfter,
			Verified:   verified,
		})
	}

	// Graph-level node-key descriptor: shared with schema.go's Describe via
	// aggregateNodeKey so the list_graphs and `binder project` surfaces cannot
	// drift (one definition of the graph-level strategy, as there is one
	// NodeKeyFor per node).
	nk := aggregateNodeKey(b.Concepts, opts.IDKey)

	return &Projection{
		Target:  target,
		Nodes:   nodes,
		Edges:   EdgesFromConcepts(b.Concepts),
		NodeKey: nk,
		Identity: IdentityStability{
			ReRootingStable:   nk.Strategy == "frontmatter" && pathFallback == 0,
			PathFallbackCount: pathFallback,
		},
		Counts:  Counts{Nodes: len(nodes), Edges: len(m.Edges)},
		AsOf:    opts.Today,
		keyByID: keyByID,
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
-- Deterministic; identifier sanitization and column order are fixed.
--
-- This file declares the tables (Nodes, Edges, and the NodeVerified attestation
-- child table) and the property graph. binder project emits it alongside the
-- companion row files nodes.csv / edges.csv / node_verified.csv, the DML loader
-- load.sql that populates Nodes and Edges, and derivation.sql (a view that
-- recomputes tier/stale from stale_after + NodeVerified).
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
// TABLE Edges, CREATE TABLE NodeVerified, then the CREATE PROPERTY GRAPH wrapper.
// The DDL is a pure function of the fixed column definitions (it carries no row
// data and no bundle-derived identifiers), so it is byte-identical run-to-run;
// the row data lives in the companion CSV/loader artifacts.
func (p *Projection) DDL() []byte {
	var b strings.Builder
	b.WriteString(ddlHeader)
	b.WriteString("\n")
	writeCreateTable(&b, "Nodes", nodeColumns, "node_key")
	b.WriteString("\n")
	writeCreateTable(&b, "Edges", edgeColumns, "from_key, to_key, rel")
	b.WriteString("\n")
	// OQ-8 item 3: the NodeVerified child table (verified[] copied verbatim). It is
	// a relational detail table keyed under Nodes, NOT a graph node/edge, so it
	// does not appear in CREATE PROPERTY GRAPH below.
	writeCreateTable(&b, "NodeVerified", nodeVerifiedColumns, "node_key, seq")
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

// ---------------------------------------------------------------------------
// G2 — loader row data (nodes.csv / edges.csv) + DML load statements.
//
// Row emission is additive over the G1 Projection: it reuses the SAME node
// identity (node_key) and edge set the schema was declared from — there is no
// second projection path — so the emitted rows and the schema.ddl columns cannot
// disagree. CSV column order and the DML column lists are BOTH driven by the same
// nodeColumns/edgeColumns slices that drive schema.ddl, and round-trip
// consistency is asserted by test (TestRowsRoundTripWithSchema). Ordering is
// deterministic: nodes by node_key, edges in EdgesFromConcepts order. Emission is
// a pure function of the Projection, so it is byte-identical run-to-run and uses
// no cloud credentials.
// ---------------------------------------------------------------------------

// columnNames returns the sanitized identifier for each column, in declaration
// order. Sanitizing here (as the DDL does) keeps the CSV header, the DML column
// list, and the CREATE TABLE column names byte-identical.
func columnNames(cols []ddlColumn) []string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = sanitizeIdentifier(c.name)
	}
	return names
}

// sortedNodes returns the projected nodes ordered by node_key, without mutating
// the Projection's Nodes slice (which stays in Build order for parity). The sort
// is stable so a corpus with a duplicated node_key still emits deterministically.
func (p *Projection) sortedNodes() []ProjectedNode {
	out := append([]ProjectedNode(nil), p.Nodes...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// nodeKeyFor maps a concept path-ID (the Edge.From/To vocabulary) to its emitted
// node_key. It NEVER mints: a resolved edge always targets a bundle concept, so
// the map hit is the normal path; the fallback returns the path id verbatim.
func (p *Projection) nodeKeyFor(id string) string {
	if k, ok := p.keyByID[id]; ok {
		return k
	}
	return id
}

// writeCSV renders a header + rows deterministically: comma-delimited, LF line
// terminator (encoding/csv default), fields quoted only when they contain a
// comma, quote, or newline. bytes.Buffer never errors, so the writes cannot fail.
func writeCSV(header []string, rows [][]string) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(header)
	for _, r := range rows {
		_ = w.Write(r)
	}
	w.Flush()
	return buf.Bytes()
}

// NodesCSV emits the Nodes row data: a header naming the columns in schema.ddl
// order, then one row per node ordered by node_key. tier/stale are the frozen
// snapshot (as of AsOf); an empty optional value is an empty CSV field.
func (p *Projection) NodesCSV() []byte {
	rows := make([][]string, 0, len(p.Nodes))
	for _, n := range p.sortedNodes() {
		rows = append(rows, []string{
			n.Key,
			n.Title,
			n.Type,
			string(n.Tier),
			strconv.FormatBool(n.Stale),
			n.StaleAfter,
		})
	}
	return writeCSV(columnNames(nodeColumns), rows)
}

// EdgesCSV emits the Edges row data in EdgesFromConcepts order. from_key/to_key
// are mapped from the edge's path IDs to the emitted node_key so the rows carry
// referential integrity to the Nodes table; rel is the raw link text.
func (p *Projection) EdgesCSV() []byte {
	rows := make([][]string, 0, len(p.Edges))
	for _, e := range p.Edges {
		rows = append(rows, []string{
			p.nodeKeyFor(e.From),
			p.nodeKeyFor(e.To),
			e.Text,
		})
	}
	return writeCSV(columnNames(edgeColumns), rows)
}

// loadHeader documents the loader artifact honestly: GoogleSQL (Spanner) has no
// SQL statement that bulk-loads a CSV, so the loader is emitted as DML INSERTs
// carrying the same rows as the sibling CSVs (which are the bulk-import
// representation for tooling such as `gcloud spanner databases import`). It
// carries no version/timestamp so it stays a stable byte-golden.
const loadHeader = `-- load.sql — deterministic loader for the OKF property-graph projection.
-- Populates the Nodes and Edges tables declared in schema.ddl. GoogleSQL
-- (Spanner) has no SQL statement that bulk-loads a CSV file, so this loader is
-- emitted as DML INSERT statements carrying the same rows, in the same fixed
-- column order, as the sibling nodes.csv / edges.csv (the bulk-import
-- representation for tooling such as ` + "`gcloud spanner databases import`" + `). Rows
-- are deterministically ordered: nodes by node_key, edges in resolved-link order.
-- No cloud credentials are used to emit this file.
`

// LoadSQL emits the DML loader: an INSERT for Nodes (rows ordered by node_key)
// and an INSERT for Edges (EdgesFromConcepts order), populating the tables
// declared in schema.ddl. The rows are identical to the CSV rows; only the
// serialization differs (SQL literals vs CSV fields).
func (p *Projection) LoadSQL() []byte {
	var b strings.Builder
	b.WriteString(loadHeader)
	b.WriteString("\n")
	writeInsert(&b, "Nodes", columnNames(nodeColumns), p.nodeValueTuples())
	b.WriteString("\n")
	writeInsert(&b, "Edges", columnNames(edgeColumns), p.edgeValueTuples())
	return []byte(b.String())
}

// nodeValueTuples renders each node (node_key order) as a GoogleSQL VALUES tuple.
// node_key/tier are NOT NULL; title/type/stale_after are nullable; stale is BOOL;
// stale_after, when present, is a DATE literal from the raw authored input.
func (p *Projection) nodeValueTuples() []string {
	tuples := make([]string, 0, len(p.Nodes))
	for _, n := range p.sortedNodes() {
		vals := []string{
			sqlString(n.Key),
			sqlNullableString(n.Title),
			sqlNullableString(n.Type),
			sqlString(string(n.Tier)),
			sqlBool(n.Stale),
			sqlNullableDate(n.StaleAfter),
		}
		tuples = append(tuples, "("+strings.Join(vals, ", ")+")")
	}
	return tuples
}

// edgeValueTuples renders each edge (EdgesFromConcepts order) as a VALUES tuple.
// from_key/to_key are NOT NULL (mapped to node_key); rel is nullable link text.
func (p *Projection) edgeValueTuples() []string {
	tuples := make([]string, 0, len(p.Edges))
	for _, e := range p.Edges {
		vals := []string{
			sqlString(p.nodeKeyFor(e.From)),
			sqlString(p.nodeKeyFor(e.To)),
			sqlNullableString(e.Text),
		}
		tuples = append(tuples, "("+strings.Join(vals, ", ")+")")
	}
	return tuples
}

// writeInsert renders one INSERT ... VALUES statement (one tuple per line,
// trailing semicolon). With no rows it emits a comment instead of an invalid
// empty INSERT, so an edge-free corpus still produces valid SQL.
func writeInsert(b *strings.Builder, table string, cols, tuples []string) {
	name := sanitizeIdentifier(table)
	if len(tuples) == 0 {
		b.WriteString("-- no rows for ")
		b.WriteString(name)
		b.WriteString(".\n")
		return
	}
	b.WriteString("INSERT INTO ")
	b.WriteString(name)
	b.WriteString(" (")
	b.WriteString(strings.Join(cols, ", "))
	b.WriteString(") VALUES\n")
	for i, t := range tuples {
		b.WriteString("  ")
		b.WriteString(t)
		if i == len(tuples)-1 {
			b.WriteString(";\n")
		} else {
			b.WriteString(",\n")
		}
	}
}

// sqlString renders s as a single-quoted GoogleSQL string literal, escaping so
// no input can break out of the literal (injection-safe, deterministic).
func sqlString(s string) string { return "'" + escapeSQL(s) + "'" }

// sqlNullableString renders "" as the NULL keyword and any other value as a
// quoted string literal.
func sqlNullableString(s string) string {
	if s == "" {
		return "NULL"
	}
	return sqlString(s)
}

// sqlBool renders a GoogleSQL BOOL literal.
func sqlBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// sqlNullableDate renders "" as NULL and any other value as a DATE literal from
// the raw authored input (validity is the author's concern and is reported
// separately by `binder validate`; the projection emits what was authored).
func sqlNullableDate(s string) string {
	if s == "" {
		return "NULL"
	}
	return "DATE " + sqlString(s)
}

// escapeSQL escapes the characters that are significant inside a single-quoted
// GoogleSQL string literal, using backslash escapes (the GoogleSQL convention).
func escapeSQL(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
