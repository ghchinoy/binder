// This file adds a read-only LPG *schema descriptor* over the existing
// projection model (Build/Node/Edge). It introduces NO new parsing and NO new
// edge logic: Describe calls Build and aggregates the already-computed Node/Edge
// sets into label + property-declaration summaries, so the schema binder
// advertises is exactly the schema its `graph` export already emits (parity by
// construction). It is additive and read-only — it never writes to a bundle,
// never mutates frontmatter, and never mints an identity (design
// pg-readonly-query-design.md §B.3/§B.5).
package graph

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// edgeLabel is the single, untyped v1 edge label: every resolved link (spec §6)
// is a LINKS edge. Future advisory/inferred edges would appear as additional
// labels; v1 has exactly this one.
const edgeLabel = "LINKS"

// SchemaSet is the `list_graphs` descriptor: the LPG schema(s) binder can
// project from an OKF corpus. The first slice is always a single local graph.
type SchemaSet struct {
	Graphs []Schema `json:"graphs"`
}

// Schema is one projected graph's LPG schema descriptor.
type Schema struct {
	Name       string      `json:"name"`
	Source     Source      `json:"source"`
	NodeKey    NodeKey     `json:"node_key"`
	Counts     Counts      `json:"counts"`
	NodeLabels []NodeLabel `json:"node_labels"`
	EdgeLabels []EdgeLabel `json:"edge_labels"`
}

// Source records where the projection is derived from (a local OKF bundle in
// v1; no live DB).
type Source struct {
	Kind string `json:"kind"` // "okf-bundle"
	Root string `json:"root"` // the bundle path this graph is projected from
}

// NodeKey describes the node-identity strategy used for this projection. The key
// is NEVER minted: it is either a concept's authored frontmatter value or the
// spec §2 path-derived Concept.ID (design §A.3).
type NodeKey struct {
	Strategy string `json:"strategy"` // "path" | "frontmatter"
	Key      string `json:"key"`      // the id_key requested, or "" for path identity
}

// Counts is the node/edge cardinality of the projected graph.
type Counts struct {
	Nodes int `json:"nodes"`
	Edges int `json:"edges"`
}

// NodeLabel is one node label (an OKF concept `type` present in the corpus) with
// its cardinality and property declarations.
type NodeLabel struct {
	Label      string   `json:"label"`
	Count      int      `json:"count"`
	Properties []string `json:"properties"`
}

// EdgeLabel is one edge label with its cardinality and property declarations.
type EdgeLabel struct {
	Label      string   `json:"label"`
	Count      int      `json:"count"`
	Properties []string `json:"properties"`
}

// nodeProperties and edgeProperties are derived from the Node/Edge json tags, so
// the advertised property declarations can never drift from what the `graph`
// export emits (parity by construction, design §B.3).
func nodeProperties() []string { return jsonFieldNames(reflect.TypeOf(Node{})) }
func edgeProperties() []string { return jsonFieldNames(reflect.TypeOf(Edge{})) }

// jsonFieldNames returns the json field names of a struct type in declaration
// order, skipping fields tagged "-".
func jsonFieldNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		names = append(names, name)
	}
	return names
}

// NodeKeyFor returns the projected node key for a concept and the strategy used
// to derive it. If idKey is non-empty and the concept carries it as a non-empty
// string in frontmatter, that authored value is the key (strategy
// "frontmatter"); otherwise the key falls back to Concept.ID — the spec §2
// path-derived identity — with strategy "path". binder NEVER mints an identity
// (design §A.3): the returned key is always either authored by the source or the
// existing path-derived ID.
func NodeKeyFor(c *okf.Concept, idKey string) (key, strategy string) {
	if idKey != "" && c.Frontmatter != nil {
		if v, ok := c.Frontmatter.Get(idKey); ok {
			if s, isStr := v.(string); isStr && s != "" {
				return s, "frontmatter"
			}
		}
	}
	return c.ID, "path"
}

// Describe builds the deterministic LPG schema descriptor for a loaded bundle as
// of `today` (for staleness). It reuses the existing Build projection and
// aggregates its Node/Edge sets — no new parsing, no new edge logic. When idKey
// is set and at least one concept carries it in frontmatter, the graph-level
// node-key strategy is "frontmatter" (individual concepts missing the key fall
// back to path identity, see NodeKeyFor); otherwise it is "path". The output is
// fully deterministic: node labels are sorted, counts are stable, and staleness
// honors the supplied `today` (SOURCE_DATE_EPOCH-derived at the call site).
func Describe(b *okf.Bundle, today, idKey string) *SchemaSet {
	m := Build(b, today)

	// Node labels = distinct concept types present, with per-type counts.
	byType := map[string]int{}
	for _, n := range m.Nodes {
		byType[n.Type]++
	}
	labels := make([]string, 0, len(byType))
	for t := range byType {
		labels = append(labels, t)
	}
	sort.Strings(labels)
	nodeLabels := make([]NodeLabel, 0, len(labels))
	for _, l := range labels {
		nodeLabels = append(nodeLabels, NodeLabel{
			Label:      l,
			Count:      byType[l],
			Properties: nodeProperties(),
		})
	}

	// Node-key strategy: "frontmatter" only when idKey is set AND resolves on at
	// least one concept; the requested key is echoed back regardless so callers
	// can see what was asked for.
	strategy := "path"
	key := ""
	if idKey != "" {
		key = idKey
		for _, c := range b.Concepts {
			if _, s := NodeKeyFor(c, idKey); s == "frontmatter" {
				strategy = "frontmatter"
				break
			}
		}
	}

	g := Schema{
		Name:       filepath.Base(b.Root),
		Source:     Source{Kind: "okf-bundle", Root: b.Root},
		NodeKey:    NodeKey{Strategy: strategy, Key: key},
		Counts:     Counts{Nodes: len(m.Nodes), Edges: len(m.Edges)},
		NodeLabels: nodeLabels,
		EdgeLabels: []EdgeLabel{{
			Label:      edgeLabel,
			Count:      len(m.Edges),
			Properties: edgeProperties(),
		}},
	}
	return &SchemaSet{Graphs: []Schema{g}}
}
