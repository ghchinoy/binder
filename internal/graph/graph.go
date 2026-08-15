// Package graph exports an already-loaded OKF bundle's concept graph in
// dot/json/graphml/html form (design-v2 §4.5). Edges are exactly the bundle's
// RESOLVED links (spec §6), so the graph matches what `binder validate` and the
// review report see. Output is deterministic (nodes and edges sorted).
package graph

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// Node is a concept in the graph.
type Node struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Type  string   `json:"type"`
	Tier  okf.Tier `json:"tier"`
	Stale bool     `json:"stale"`
}

// Edge is a resolved directed link between concepts.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text,omitempty"`
}

// Model is the extracted node/edge set.
type Model struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// EdgesFromConcepts returns the resolved directed edge set for the given
// concepts, sorted deterministically (by From, then To, then Text). It is the
// SINGLE definition of what counts as an edge — a resolved link only
// (Link.Resolved && TargetID != ""), with From=c.ID → To=l.TargetID. Build uses
// it, and any caller that must stay in edge-parity with `binder graph` (e.g. the
// index catalog's backlinks/graph annotations) calls it too, so the two can
// never disagree. It does NOT reimplement link resolution; it reads the already
// resolved Links on each concept.
func EdgesFromConcepts(concepts []*okf.Concept) []Edge {
	var edges []Edge
	for _, c := range concepts {
		for _, l := range c.Links {
			if l.Resolved && l.TargetID != "" {
				edges = append(edges, Edge{From: c.ID, To: l.TargetID, Text: l.Text})
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Text < edges[j].Text
	})
	return edges
}

// Build extracts the deterministic node/edge model from a loaded bundle as of
// `today` (for staleness).
func Build(b *okf.Bundle, today string) *Model {
	m := &Model{}
	concepts := append([]*okf.Concept(nil), b.Concepts...)
	sort.Slice(concepts, func(i, j int) bool { return concepts[i].ID < concepts[j].ID })

	for _, c := range concepts {
		title := c.ID
		if v, ok := c.Frontmatter.Get("title"); ok {
			if s, _ := v.(string); s != "" {
				title = s
			}
		}
		m.Nodes = append(m.Nodes, Node{
			ID: c.ID, Title: title, Type: c.Type,
			Tier: okf.TrustTier(c), Stale: okf.IsStale(c, today),
		})
	}
	m.Edges = EdgesFromConcepts(b.Concepts)
	return m
}

// Export renders a loaded bundle in the requested format.
func Export(b *okf.Bundle, format, today string) ([]byte, error) {
	m := Build(b, today)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "dot":
		return m.DOT(), nil
	case "json":
		return m.JSON()
	case "graphml":
		return m.GraphML()
	case "html":
		return m.HTML()
	default:
		return nil, fmt.Errorf("unknown graph format %q (want dot|json|graphml|html)", format)
	}
}

// DOT renders Graphviz DOT.
func (m *Model) DOT() []byte {
	var b strings.Builder
	b.WriteString("digraph okf {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box];\n")
	for _, n := range m.Nodes {
		label := n.Title
		if label == "" {
			label = n.ID
		}
		fmt.Fprintf(&b, "  %s [label=%s];\n", dotID(n.ID), dotStr(label))
	}
	for _, e := range m.Edges {
		if e.Text != "" {
			fmt.Fprintf(&b, "  %s -> %s [label=%s];\n", dotID(e.From), dotID(e.To), dotStr(e.Text))
		} else {
			fmt.Fprintf(&b, "  %s -> %s;\n", dotID(e.From), dotID(e.To))
		}
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

// JSON renders the node/edge model as indented JSON.
func (m *Model) JSON() ([]byte, error) {
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// graphml XML structures.
type graphml struct {
	XMLName xml.Name `xml:"graphml"`
	XMLNS   string   `xml:"xmlns,attr"`
	Keys    []gmlKey `xml:"key"`
	Graph   gmlGraph `xml:"graph"`
}
type gmlKey struct {
	ID       string `xml:"id,attr"`
	For      string `xml:"for,attr"`
	AttrName string `xml:"attr.name,attr"`
	AttrType string `xml:"attr.type,attr"`
}
type gmlGraph struct {
	EdgeDefault string    `xml:"edgedefault,attr"`
	Nodes       []gmlNode `xml:"node"`
	Edges       []gmlEdge `xml:"edge"`
}
type gmlNode struct {
	ID   string    `xml:"id,attr"`
	Data []gmlData `xml:"data"`
}
type gmlEdge struct {
	Source string    `xml:"source,attr"`
	Target string    `xml:"target,attr"`
	Data   []gmlData `xml:"data,omitempty"`
}
type gmlData struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}

// GraphML renders GraphML XML.
func (m *Model) GraphML() ([]byte, error) {
	g := graphml{
		XMLNS: "http://graphml.graphdrawing.org/xmlns",
		Keys: []gmlKey{
			{ID: "title", For: "node", AttrName: "title", AttrType: "string"},
			{ID: "type", For: "node", AttrName: "type", AttrType: "string"},
			{ID: "tier", For: "node", AttrName: "tier", AttrType: "string"},
			{ID: "stale", For: "node", AttrName: "stale", AttrType: "boolean"},
			{ID: "rel", For: "edge", AttrName: "rel", AttrType: "string"},
		},
		Graph: gmlGraph{EdgeDefault: "directed"},
	}
	for _, n := range m.Nodes {
		g.Graph.Nodes = append(g.Graph.Nodes, gmlNode{
			ID: n.ID,
			Data: []gmlData{
				{Key: "title", Value: n.Title},
				{Key: "type", Value: n.Type},
				{Key: "tier", Value: string(n.Tier)},
				{Key: "stale", Value: fmt.Sprintf("%t", n.Stale)},
			},
		})
	}
	for _, e := range m.Edges {
		var data []gmlData
		if e.Text != "" {
			data = []gmlData{{Key: "rel", Value: e.Text}}
		}
		g.Graph.Edges = append(g.Graph.Edges, gmlEdge{Source: e.From, Target: e.To, Data: data})
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(g); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// HTML renders a self-contained, dependency-free page: the graph as an embedded
// JSON island plus a readable node/edge table. It is the zero-extra-tool
// fallback (NG-1), not a viewer.
func (m *Model) HTML() ([]byte, error) {
	data, err := m.JSON()
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<title>binder graph</title>\n")
	b.WriteString("<style>body{font-family:system-ui,sans-serif;margin:2rem;}")
	b.WriteString("table{border-collapse:collapse;margin-bottom:2rem;}")
	b.WriteString("th,td{border:1px solid #ccc;padding:4px 8px;text-align:left;}")
	b.WriteString("caption{font-weight:bold;text-align:left;margin-bottom:.5rem;}</style>\n")
	b.WriteString("</head>\n<body>\n<h1>binder graph</h1>\n")

	b.WriteString("<table>\n<caption>Concepts</caption>\n")
	b.WriteString("<tr><th>id</th><th>title</th><th>type</th><th>tier</th><th>stale</th></tr>\n")
	for _, n := range m.Nodes {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%t</td></tr>\n",
			esc(n.ID), esc(n.Title), esc(n.Type), esc(string(n.Tier)), n.Stale)
	}
	b.WriteString("</table>\n")

	b.WriteString("<table>\n<caption>Edges</caption>\n")
	b.WriteString("<tr><th>from</th><th>to</th><th>relationship</th></tr>\n")
	for _, e := range m.Edges {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n", esc(e.From), esc(e.To), esc(e.Text))
	}
	b.WriteString("</table>\n")

	b.WriteString("<script type=\"application/json\" id=\"graph-data\">\n")
	b.Write(bytes.TrimRight(data, "\n"))
	b.WriteString("\n</script>\n</body>\n</html>\n")
	return []byte(b.String()), nil
}

func esc(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// dotID quotes an identifier for DOT.
func dotID(s string) string { return dotStr(s) }

// dotStr renders a DOT double-quoted string with escaping.
func dotStr(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return "\"" + s + "\""
}
