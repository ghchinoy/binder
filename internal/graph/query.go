// This file adds a read-only, fixed-verb LPG *query* surface over the existing
// projection model (Build/Node/Edge). It introduces NO new parsing, NO new edge
// logic, and NO new build path: NewIndex consumes a *Model produced by Build and
// the five pure verb functions traverse the already-computed, already-sorted
// Node/Edge sets. It is additive and read-only — it never writes to a bundle,
// never mutates frontmatter, and never mints an identity (design
// query-graph-design.md §0/§8, sibling to schema.go's Describe).
//
// Every traversal is bounded BY CONSTRUCTION: a mandatory hard depth cap
// (MaxDepth) on k-hop/path verbs, a result-size cap (MaxResults) with
// sort-then-truncate-with-flag on the node result sets, and a visited-set cycle
// guard so cyclic corpora terminate with each node emitted once at its minimum
// depth (design §4).
package graph

import (
	"sort"
	"strconv"
)

// MaxDepth is the mandatory hard depth cap for k-hop traversal verbs
// (`neighborhood.depth`, `path.max_depth`). Those params are required and must be
// in 1..MaxDepth; anything outside that range is a usage error. There is no
// unbounded-traversal path anywhere in the tool (design §4, FROZEN value 5).
const MaxDepth = 5

// MaxResults is the result-size cap applied to the node result sets of
// lookup(label), neighbors, neighborhood, and pattern. On overflow the result is
// sorted (§5) THEN truncated to MaxResults and the payload flags truncated:true —
// never an error (never-reject, design §4, FROZEN value 1000).
const MaxResults = 1000

// ResultNodeKey echoes the identity basis a query actually used (design §14.1,
// binding amendment). Strategy is always "path" in v1 — traversal identity is the
// spec §2 path-derived Concept.ID, because Edge.From/To are defined in those
// terms. Key echoes the caller's id_key verbatim (empty when not supplied).
// Honored is always false in v1: a non-empty id_key is accepted for parity with
// list_graphs but does NOT re-key traversal identity, and it is never minted;
// honored:true is reserved for the future re-keying follow-up. Echoing this makes
// the limitation observable rather than a silent no-op, so a harness that read
// strategy=frontmatter from list_graphs can detect the difference here.
type ResultNodeKey struct {
	Strategy string `json:"strategy"`
	Key      string `json:"key"`
	Honored  bool   `json:"honored"`
}

// nodeKey builds the v1 node-key echo: path strategy, the id_key echoed verbatim,
// never honored (never minted).
func nodeKey(idKey string) ResultNodeKey {
	return ResultNodeKey{Strategy: "path", Key: idKey, Honored: false}
}

// WhereClause is the optional property predicate for the pattern verb: an exact
// match of a real node property (prop ∈ {type, tier, stale}) against eq. For
// stale, eq compares against "true"/"false".
type WhereClause struct {
	Prop string `json:"prop"`
	Eq   string `json:"eq"`
}

// Depth is one entry of a neighborhood's per-node BFS distance from the start.
type Depth struct {
	ID    string `json:"id"`
	Depth int    `json:"depth"`
}

// Per-verb echoed query objects. Field order is fixed by declaration so the
// encoded payload is deterministic.

// LookupQuery echoes a lookup's discriminator: exactly one of id or label.
type LookupQuery struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
}

// NeighborsQuery echoes a neighbors query.
type NeighborsQuery struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Rel       string `json:"rel"`
}

// NeighborhoodQuery echoes a neighborhood query.
type NeighborhoodQuery struct {
	ID        string `json:"id"`
	Depth     int    `json:"depth"`
	Direction string `json:"direction"`
	Rel       string `json:"rel"`
}

// PatternQuery echoes a pattern query. Where is omitted when absent.
type PatternQuery struct {
	Label   string       `json:"label"`
	ToLabel string       `json:"to_label"`
	Rel     string       `json:"rel"`
	Where   *WhereClause `json:"where,omitempty"`
}

// PathQuery echoes a path query.
type PathQuery struct {
	From      string `json:"from"`
	To        string `json:"to"`
	MaxDepth  int    `json:"max_depth"`
	Direction string `json:"direction"`
}

// Per-verb result payloads. Each carries op, the echoed query, and the node_key
// identity echo (§14.1); the remaining fields are the per-verb additions from
// design §9. Slice fields are always non-nil so they render as [] (an empty
// result is a RESULT, not null — never-reject).

// LookupResult is the payload for op "lookup". Truncated is present only for a
// by-label lookup (the only capped variant); NotFound only for a by-id lookup
// (the only variant with a named anchor), per design §9.1.
type LookupResult struct {
	Op        string        `json:"op"`
	Query     LookupQuery   `json:"query"`
	NodeKey   ResultNodeKey `json:"node_key"`
	Nodes     []Node        `json:"nodes"`
	Truncated *bool         `json:"truncated,omitempty"`
	NotFound  *bool         `json:"not_found,omitempty"`
}

// NeighborsResult is the payload for op "neighbors".
type NeighborsResult struct {
	Op        string         `json:"op"`
	Query     NeighborsQuery `json:"query"`
	NodeKey   ResultNodeKey  `json:"node_key"`
	Nodes     []Node         `json:"nodes"`
	Edges     []Edge         `json:"edges"`
	Truncated bool           `json:"truncated"`
	NotFound  bool           `json:"not_found"`
}

// NeighborhoodResult is the payload for op "neighborhood".
type NeighborhoodResult struct {
	Op        string            `json:"op"`
	Query     NeighborhoodQuery `json:"query"`
	NodeKey   ResultNodeKey     `json:"node_key"`
	Nodes     []Node            `json:"nodes"`
	Edges     []Edge            `json:"edges"`
	Depths    []Depth           `json:"depths"`
	Truncated bool              `json:"truncated"`
	NotFound  bool              `json:"not_found"`
}

// PatternResult is the payload for op "pattern". It has no not_found (there is no
// single named anchor); nodes are the matching SOURCE nodes, edges the satisfying
// edges (design §9.4).
type PatternResult struct {
	Op        string        `json:"op"`
	Query     PatternQuery  `json:"query"`
	NodeKey   ResultNodeKey `json:"node_key"`
	Nodes     []Node        `json:"nodes"`
	Edges     []Edge        `json:"edges"`
	Truncated bool          `json:"truncated"`
}

// PathResult is the payload for op "path": bounded existence plus the shortest
// hop-path (design §9.5).
type PathResult struct {
	Op       string        `json:"op"`
	Query    PathQuery     `json:"query"`
	NodeKey  ResultNodeKey `json:"node_key"`
	Exists   bool          `json:"exists"`
	Length   int           `json:"length"`
	Path     []string      `json:"path"`
	NotFound bool          `json:"not_found"`
}

// Index is an additive adjacency index built from a *Model. out/in map a node id
// to the indices (into Model.Edges) of the edges leaving/entering it; byLabel
// maps a concept type to its node ids; byID maps a node id to its node. Because
// Model.Nodes is sorted by ID and Model.Edges by (From, To, Text), every
// adjacency and label list is in a deterministic order, so traversal expansion is
// deterministic without any extra sorting (design §5, rule 1). The index adds no
// new edge logic — it only indexes Build's output.
type Index struct {
	model   *Model
	byID    map[string]*Node
	byLabel map[string][]string
	out     map[string][]int
	in      map[string][]int
}

// NewIndex builds the adjacency index over an already-built *Model. It is O(N+E)
// and allocates only maps; it never mutates the model.
func NewIndex(m *Model) *Index {
	idx := &Index{
		model:   m,
		byID:    make(map[string]*Node, len(m.Nodes)),
		byLabel: make(map[string][]string),
		out:     make(map[string][]int),
		in:      make(map[string][]int),
	}
	for i := range m.Nodes {
		n := &m.Nodes[i]
		idx.byID[n.ID] = n
		idx.byLabel[n.Type] = append(idx.byLabel[n.Type], n.ID) // sorted: Nodes is sorted by ID
	}
	for i := range m.Edges {
		e := m.Edges[i]
		idx.out[e.From] = append(idx.out[e.From], i) // sorted: Edges is sorted by From,To,Text
		idx.in[e.To] = append(idx.in[e.To], i)
	}
	return idx
}

// step is one adjacency hop: the edge index taken and the neighbor reached.
type step struct {
	edge int
	to   string
}

// steps returns the adjacency hops out of id in the given direction, filtered to
// edges whose Text equals rel when rel is non-empty. The order is the
// sorted-edge order (rule 1), so expansion — and therefore every BFS tie-break —
// is deterministic. For "both", the union of out/in edge indices is de-duplicated
// (a self-loop touches both) and sorted ascending.
func (idx *Index) steps(id, direction, rel string) []step {
	res := []step{}
	match := func(ei int) (Edge, bool) {
		e := idx.model.Edges[ei]
		if rel != "" && e.Text != rel {
			return e, false
		}
		return e, true
	}
	switch direction {
	case "in":
		for _, ei := range idx.in[id] {
			if e, ok := match(ei); ok {
				res = append(res, step{ei, e.From})
			}
		}
	case "both":
		seen := make(map[int]bool)
		var idxs []int
		for _, ei := range idx.out[id] {
			if !seen[ei] {
				seen[ei] = true
				idxs = append(idxs, ei)
			}
		}
		for _, ei := range idx.in[id] {
			if !seen[ei] {
				seen[ei] = true
				idxs = append(idxs, ei)
			}
		}
		sort.Ints(idxs)
		for _, ei := range idxs {
			e, ok := match(ei)
			if !ok {
				continue
			}
			nb := e.To
			if e.From != id { // reached via an in-edge: the neighbor is the source
				nb = e.From
			}
			res = append(res, step{ei, nb})
		}
	default: // "out"
		for _, ei := range idx.out[id] {
			if e, ok := match(ei); ok {
				res = append(res, step{ei, e.To})
			}
		}
	}
	return res
}

// Lookup implements op "lookup": fetch the single node with the given id, or all
// nodes of the given concept type (label). Exactly one of id/label is expected
// (the handler enforces the one-of); by-id sets not_found when absent, by-label
// is capped/truncated. Nodes are sorted by ID.
func (idx *Index) Lookup(idKey, id, label string) *LookupResult {
	r := &LookupResult{Op: "lookup", NodeKey: nodeKey(idKey), Nodes: []Node{}}
	if id != "" {
		r.Query = LookupQuery{ID: id}
		nf := true
		if n, ok := idx.byID[id]; ok {
			r.Nodes = append(r.Nodes, *n)
			nf = false
		}
		r.NotFound = &nf
		return r
	}
	r.Query = LookupQuery{Label: label}
	for _, nid := range idx.byLabel[label] { // already sorted by ID
		r.Nodes = append(r.Nodes, *idx.byID[nid])
	}
	nodes, truncated := truncateNodes(r.Nodes)
	r.Nodes = nodes
	r.Truncated = &truncated
	return r
}

// Neighbors implements op "neighbors": the one-hop neighbors of id in the given
// direction, optionally filtered by edge rel (Edge.Text). Nodes are the distinct
// neighbors; edges are the traversed edges. Both are sorted then the node set is
// truncated, with edges filtered to retained endpoints.
func (idx *Index) Neighbors(idKey, id, direction, rel string) *NeighborsResult {
	r := &NeighborsResult{
		Op:      "neighbors",
		Query:   NeighborsQuery{ID: id, Direction: direction, Rel: rel},
		NodeKey: nodeKey(idKey),
		Nodes:   []Node{},
		Edges:   []Edge{},
	}
	if _, ok := idx.byID[id]; !ok {
		r.NotFound = true
		return r
	}
	edges := []Edge{}
	nodes := []Node{}
	seen := make(map[string]bool)
	for _, s := range idx.steps(id, direction, rel) {
		edges = append(edges, idx.model.Edges[s.edge])
		if !seen[s.to] {
			seen[s.to] = true
			if n, ok := idx.byID[s.to]; ok {
				nodes = append(nodes, *n)
			}
		}
	}
	sortNodes(nodes)
	sortEdges(edges)
	nodes, truncated := truncateNodes(nodes)
	keep := map[string]bool{id: true}
	for _, n := range nodes {
		keep[n.ID] = true
	}
	r.Nodes = nodes
	r.Edges = retainEdges(edges, keep, true)
	r.Truncated = truncated
	return r
}

// Neighborhood implements op "neighborhood": the bounded k-hop BFS neighborhood
// of id up to depth (1..MaxDepth, enforced by the handler), in the given
// direction, optionally rel-filtered. It returns every reached node (including
// the start at depth 0) with its minimum depth. Edges are the subgraph induced on
// the retained nodes (matching rel). The BFS distance map doubles as the visited
// set, so cycles terminate and each node is emitted once at its minimum depth.
func (idx *Index) Neighborhood(idKey, id string, depth int, direction, rel string) *NeighborhoodResult {
	r := &NeighborhoodResult{
		Op:      "neighborhood",
		Query:   NeighborhoodQuery{ID: id, Depth: depth, Direction: direction, Rel: rel},
		NodeKey: nodeKey(idKey),
		Nodes:   []Node{},
		Edges:   []Edge{},
		Depths:  []Depth{},
	}
	if _, ok := idx.byID[id]; !ok {
		r.NotFound = true
		return r
	}
	dist := map[string]int{id: 0}
	queue := []string{id}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		d := dist[cur]
		if d == depth { // depth bound: do not expand past it
			continue
		}
		for _, s := range idx.steps(cur, direction, rel) {
			if _, ok := dist[s.to]; !ok { // visited guard (min-depth is BFS-first)
				dist[s.to] = d + 1
				queue = append(queue, s.to)
			}
		}
	}
	nodes := []Node{}
	for nid := range dist {
		if n, ok := idx.byID[nid]; ok {
			nodes = append(nodes, *n)
		}
	}
	sortNodes(nodes)
	nodes, truncated := truncateNodes(nodes)
	keep := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		keep[n.ID] = true
	}
	edges := []Edge{}
	for _, e := range idx.model.Edges { // Model.Edges is already sorted
		if rel != "" && e.Text != rel {
			continue
		}
		if keep[e.From] && keep[e.To] {
			edges = append(edges, e)
		}
	}
	depths := []Depth{}
	for nid, d := range dist {
		if keep[nid] {
			depths = append(depths, Depth{ID: nid, Depth: d})
		}
	}
	sort.Slice(depths, func(i, j int) bool {
		if depths[i].Depth != depths[j].Depth {
			return depths[i].Depth < depths[j].Depth
		}
		return depths[i].ID < depths[j].ID
	})
	r.Nodes = nodes
	r.Edges = edges
	r.Depths = depths
	r.Truncated = truncated
	return r
}

// Pattern implements op "pattern": source nodes of type label that link (via an
// out-edge, optionally rel-filtered) to a node satisfying to_label and/or the
// where predicate over type/tier/stale. Returns the matching SOURCE nodes and the
// satisfying edges. The handler enforces that at least one of to_label/where is
// present.
func (idx *Index) Pattern(idKey, label, toLabel, rel string, where *WhereClause) *PatternResult {
	r := &PatternResult{
		Op:      "pattern",
		Query:   PatternQuery{Label: label, ToLabel: toLabel, Rel: rel, Where: where},
		NodeKey: nodeKey(idKey),
		Nodes:   []Node{},
		Edges:   []Edge{},
	}
	nodes := []Node{}
	edges := []Edge{}
	for _, sid := range idx.byLabel[label] { // sorted by ID
		matched := false
		for _, ei := range idx.out[sid] {
			e := idx.model.Edges[ei]
			if rel != "" && e.Text != rel {
				continue
			}
			tgt, ok := idx.byID[e.To]
			if !ok {
				continue
			}
			if toLabel != "" && tgt.Type != toLabel {
				continue
			}
			if where != nil && !matchWhere(tgt, where) {
				continue
			}
			edges = append(edges, e)
			matched = true
		}
		if matched {
			nodes = append(nodes, *idx.byID[sid])
		}
	}
	sortNodes(nodes)
	sortEdges(edges)
	nodes, truncated := truncateNodes(nodes)
	keep := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		keep[n.ID] = true
	}
	r.Nodes = nodes
	r.Edges = retainEdges(edges, keep, false) // targets are not in the node set
	r.Truncated = truncated
	return r
}

// Path implements op "path": bounded reachability from `from` to `to` within
// max_depth (1..MaxDepth, enforced by the handler) in the given direction, plus
// the shortest hop-path. A missing endpoint is a finding (not_found), not an
// error (never-reject). Ties are broken by the sorted-edge expansion order, so
// the returned path is unique and stable (design §5, rule 5).
func (idx *Index) Path(idKey, from, to, direction string, maxDepth int) *PathResult {
	r := &PathResult{
		Op:      "path",
		Query:   PathQuery{From: from, To: to, MaxDepth: maxDepth, Direction: direction},
		NodeKey: nodeKey(idKey),
		Path:    []string{},
	}
	_, fromOK := idx.byID[from]
	_, toOK := idx.byID[to]
	if !fromOK || !toOK {
		r.NotFound = true
		return r
	}
	if from == to {
		r.Exists = true
		r.Length = 0
		r.Path = []string{from}
		return r
	}
	prev := map[string]string{}
	visited := map[string]bool{from: true}
	type item struct {
		id    string
		depth int
	}
	queue := []item{{from, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth == maxDepth {
			continue
		}
		for _, s := range idx.steps(cur.id, direction, "") {
			if s.to == to {
				prev[to] = cur.id
				path := reconstructPath(prev, from, to)
				r.Exists = true
				r.Length = len(path) - 1
				r.Path = path
				return r
			}
			if !visited[s.to] {
				visited[s.to] = true
				prev[s.to] = cur.id
				queue = append(queue, item{s.to, cur.depth + 1})
			}
		}
	}
	return r // exists:false, length:0, path:[]
}

// matchWhere evaluates a pattern where-predicate against a target node's real
// properties (type/tier/stale). An unknown prop never matches (the handler
// rejects it as a usage error before we get here).
func matchWhere(n *Node, w *WhereClause) bool {
	switch w.Prop {
	case "type":
		return n.Type == w.Eq
	case "tier":
		return string(n.Tier) == w.Eq
	case "stale":
		return strconv.FormatBool(n.Stale) == w.Eq
	}
	return false
}

// reconstructPath walks the predecessor map from `to` back to `from` and returns
// the node ids in forward order.
func reconstructPath(prev map[string]string, from, to string) []string {
	rev := []string{}
	for cur := to; ; cur = prev[cur] {
		rev = append(rev, cur)
		if cur == from {
			break
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// sortNodes sorts a node slice by ID ascending (design §5, rule 2).
func sortNodes(ns []Node) {
	sort.Slice(ns, func(i, j int) bool { return ns[i].ID < ns[j].ID })
}

// sortEdges sorts an edge slice by From, then To, then Text (design §5, rule 3;
// identical to graph/list_graphs).
func sortEdges(es []Edge) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].From != es[j].From {
			return es[i].From < es[j].From
		}
		if es[i].To != es[j].To {
			return es[i].To < es[j].To
		}
		return es[i].Text < es[j].Text
	})
}

// truncateNodes applies the MaxResults cap AFTER sorting (design §4/§5, rule 6),
// returning the retained prefix and whether truncation occurred.
func truncateNodes(nodes []Node) ([]Node, bool) {
	if len(nodes) > MaxResults {
		return nodes[:MaxResults], true
	}
	return nodes, false
}

// retainEdges drops edges whose endpoints were truncated away. requireTo controls
// whether the To endpoint must also be retained: neighbors/neighborhood keep only
// edges with both endpoints in the node set; pattern keeps edges by source (the
// target is not part of the returned node set).
func retainEdges(edges []Edge, keep map[string]bool, requireTo bool) []Edge {
	out := []Edge{}
	for _, e := range edges {
		if !keep[e.From] {
			continue
		}
		if requireTo && !keep[e.To] {
			continue
		}
		out = append(out, e)
	}
	return out
}
