// Package factileadapter implements okf.Codec and okf.LinkGraph by wrapping
// factile's pkg/okf and pkg/graph (both Apache-2.0). It is the default codec,
// selected once at the composition root (cmd/root.go) and injected as an
// okf.Codec/okf.LinkGraph. It is the ONLY binder package that imports factile.
package factileadapter

import (
	"fmt"
	"sort"

	fgraph "github.com/factile/factile/pkg/graph"
	fokf "github.com/factile/factile/pkg/okf"

	"github.com/ghchinoy/binder/internal/okf"
)

// Adapter satisfies both okf.Codec and okf.LinkGraph.
type Adapter struct{}

// New returns the factile-backed codec/link-graph.
func New() *Adapter { return &Adapter{} }

var (
	_ okf.Codec     = (*Adapter)(nil)
	_ okf.LinkGraph = (*Adapter)(nil)
)

// ParseConcept parses raw at relPath into a typed Concept, preserving key order
// and unknown keys and projecting TrustSignals via binder-owned logic.
func (a *Adapter) ParseConcept(relPath string, raw []byte) (*okf.Concept, error) {
	id, _ := fokf.ConceptIDFromRel(relPath)
	doc, err := fokf.ParseConcept(id, raw)
	if err != nil {
		return nil, err
	}
	fm := toOrderedMap(doc.Frontmatter, doc.Order)
	typ := ""
	if v, ok := fm.Get("type"); ok {
		typ = asString(v)
	}
	c := &okf.Concept{
		ID:          doc.ConceptID,
		RelPath:     relPath,
		Type:        typ,
		Frontmatter: fm,
		Body:        doc.Markdown,
	}
	c.Trust = okf.ProjectTrust(fm, typ)
	return c, nil
}

// Serialize renders a Concept back to bytes, order-stable and deterministic.
func (a *Adapter) Serialize(c *okf.Concept) ([]byte, error) {
	values, order := fromOrderedMap(c.Frontmatter)
	doc := fokf.Document{
		ConceptID:   c.ID,
		Frontmatter: values,
		Order:       order,
		Markdown:    c.Body,
	}
	return fokf.Serialize(doc), nil
}

// ConceptIDFromRel maps a bundle-relative path to a concept ID.
func (a *Adapter) ConceptIDFromRel(rel string) (string, bool) {
	return fokf.ConceptIDFromRel(rel)
}

// RelFromConceptID is the inverse of ConceptIDFromRel.
func (a *Adapter) RelFromConceptID(id string) (string, error) {
	return fokf.RelFromConceptID(id)
}

// IsReservedFile reports whether name is index.md or log.md.
func (a *Adapter) IsReservedFile(name string) bool {
	return fokf.IsReservedFile(name)
}

// ExtractLinks reads markdown-link edges from an OKF concept body.
func (a *Adapter) ExtractLinks(fromConceptID, body string) []okf.Link {
	raw := fgraph.ExtractMarkdownLinks(body)
	fromRel, _ := fokf.RelFromConceptID(fromConceptID)
	out := make([]okf.Link, 0, len(raw))
	for _, l := range raw {
		id, ok := fgraph.ResolveLink("/"+fromRel, l.Target)
		out = append(out, okf.Link{
			RawTarget: l.Target,
			TargetID:  trimLeadingSlash(id),
			Resolved:  ok,
		})
	}
	return out
}

// ResolveLink resolves a raw target to a bundle-relative concept ID.
func (a *Adapter) ResolveLink(fromConceptID, rawTarget string) (string, bool) {
	fromRel, _ := fokf.RelFromConceptID(fromConceptID)
	id, ok := fgraph.ResolveLink("/"+fromRel, rawTarget)
	return trimLeadingSlash(id), ok
}

func trimLeadingSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func toOrderedMap(values map[string]any, order []string) *okf.OrderedMap {
	m := okf.NewOrderedMap()
	seen := map[string]bool{}
	for _, k := range order {
		if _, ok := values[k]; ok && !seen[k] {
			m.Set(k, values[k])
			seen[k] = true
		}
	}
	// Defensive: append any value not covered by Order, deterministically.
	var rest []string
	for k := range values {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		m.Set(k, values[k])
	}
	return m
}

func fromOrderedMap(m *okf.OrderedMap) (map[string]any, []string) {
	values := map[string]any{}
	var order []string
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		values[k] = v
		order = append(order, k)
	}
	return values, order
}
