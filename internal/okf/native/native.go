// Package native is binder's own OKF v0.2 codec, reproduced from the spec with
// gopkg.in/yaml.v3 (standard YAML frontmatter) and goldmark (markdown links).
// It implements okf.Codec and okf.LinkGraph and is the default codec, selected
// once at the composition root (cmd/root.go) and injected as an interface.
//
// It is the SOLE Phase-1 codec (design decision A / R-2 reversed): a
// community-core adapter over an external OKF library is a deferred, optional
// slot behind the same interfaces, to be added only if it is first validated
// against real-world YAML (the check that ruled factile out of Phase 1).
//
// Frontmatter fidelity (design-v2 §3.2 — byte-faithful round-trip): parsing
// preserves every key/value and the top-level key order; unmodified frontmatter
// is re-emitted from the original source bytes verbatim, so nested-mapping key
// order and scalar quoting/folding style survive exactly. Only frontmatter the
// converter actually changed is re-encoded, deterministically.
package native

import (
	"bytes"
	"fmt"
	"path"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ghchinoy/binder/internal/okf"
)

// Codec implements okf.Codec and okf.LinkGraph.
type Codec struct{}

// New returns the native OKF v0.2 codec.
func New() *Codec { return &Codec{} }

var (
	_ okf.Codec     = (*Codec)(nil)
	_ okf.LinkGraph = (*Codec)(nil)
)

// ParseConcept splits the frontmatter/body of raw, parses the frontmatter with
// yaml.v3 (preserving top-level key order and every key/value), retains the
// original frontmatter bytes for byte-faithful serialization, and projects
// TrustSignals. It errors only for structural non-conformance (spec §11.1):
// missing or unterminated frontmatter, or invalid YAML.
func (c *Codec) ParseConcept(relPath string, raw []byte) (*okf.Concept, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	fmText, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, err
	}

	fm, err := parseFrontmatter(fmText)
	if err != nil {
		return nil, err
	}

	id, _ := c.ConceptIDFromRel(relPath)
	typ := okf.AsString(mapGet(fm, "type"))
	con := &okf.Concept{
		ID:                  id,
		RelPath:             relPath,
		Type:                typ,
		Frontmatter:         fm,
		OriginalFrontmatter: []byte(fmText),
		Body:                body,
	}
	con.Trust = okf.ProjectTrust(fm, typ)
	return con, nil
}

// Serialize renders a Concept to bytes. Unmodified frontmatter is emitted
// verbatim from OriginalFrontmatter (byte-faithful); modified or synthesised
// frontmatter is re-encoded deterministically with a 2-space indent.
func (c *Codec) Serialize(con *okf.Concept) ([]byte, error) {
	fmBytes, err := c.encodeFrontmatter(con)
	if err != nil {
		return nil, err
	}
	body := strings.ReplaceAll(con.Body, "\r\n", "\n")

	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fmBytes)
	if !bytes.HasSuffix(fmBytes, []byte("\n")) {
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	if body != "" && !strings.HasPrefix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(body)
	return b.Bytes(), nil
}

func (c *Codec) encodeFrontmatter(con *okf.Concept) ([]byte, error) {
	// Fast path: if the frontmatter is byte-for-byte reproducible from the
	// original source (nothing the converter touched changed it), emit it
	// verbatim so nested order and scalar style are preserved exactly.
	if len(con.OriginalFrontmatter) > 0 {
		orig, err := parseFrontmatter(string(con.OriginalFrontmatter))
		if err == nil && orderedMapEqual(orig, con.Frontmatter) {
			return con.OriginalFrontmatter, nil
		}
	}
	return encodeOrderedMap(con.Frontmatter)
}

// ConceptIDFromRel maps a bundle-relative path to a concept ID (path minus
// ".md"); ok is false for reserved or non-concept/unsafe paths.
func (c *Codec) ConceptIDFromRel(rel string) (string, bool) {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", false
	}
	if !strings.HasSuffix(rel, ".md") || c.IsReservedFile(rel) {
		return "", false
	}
	id := strings.TrimSuffix(rel, ".md")
	if id == "" || id == "." {
		return "", false
	}
	return id, true
}

// RelFromConceptID is the inverse of ConceptIDFromRel.
func (c *Codec) RelFromConceptID(id string) (string, error) {
	id = normalizeConceptID(id)
	if id == "" {
		return "", fmt.Errorf("empty concept id")
	}
	for _, part := range strings.Split(id, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe concept id: %s", id)
		}
	}
	return id + ".md", nil
}

// IsReservedFile reports whether name's base is index.md or log.md (spec §3.1).
func (c *Codec) IsReservedFile(name string) bool {
	base := path.Base(strings.TrimSpace(name))
	return base == "index.md" || base == "log.md"
}

// ExtractLinks reads markdown-link edges from an OKF concept body (output side).
func (c *Codec) ExtractLinks(fromConceptID, body string) []okf.Link {
	raw := okf.ExtractMarkdownLinks(body)
	out := make([]okf.Link, 0, len(raw))
	for _, l := range raw {
		id, ok := c.ResolveLink(fromConceptID, l.Dest)
		out = append(out, okf.Link{
			RawTarget: l.Dest,
			TargetID:  id,
			Text:      l.Text,
			Resolved:  ok,
		})
	}
	return out
}

// ResolveLink resolves a raw markdown target to a bundle-relative concept ID.
// External (scheme/anchor-only) and non-.md targets are unresolvable.
func (c *Codec) ResolveLink(fromConceptID, rawTarget string) (string, bool) {
	target := strings.TrimSpace(rawTarget)
	if hash := strings.IndexByte(target, '#'); hash >= 0 {
		target = target[:hash]
	}
	if target == "" || isExternal(target) || !strings.HasSuffix(target, ".md") {
		return "", false
	}
	fromRel, err := c.RelFromConceptID(fromConceptID)
	if err != nil {
		return "", false
	}
	var resolved string
	if strings.HasPrefix(target, "/") {
		resolved = path.Clean(target)
	} else {
		resolved = path.Clean(path.Join("/"+path.Dir(fromRel), target))
	}
	id := strings.TrimPrefix(strings.TrimSuffix(resolved, ".md"), "/")
	if id == "" || strings.HasPrefix(id, "..") {
		return "", false
	}
	return id, true
}

func isExternal(target string) bool {
	if i := strings.Index(target, "://"); i > 0 {
		return true
	}
	return strings.HasPrefix(target, "mailto:")
}

func normalizeConceptID(id string) string {
	id = strings.TrimSpace(strings.ReplaceAll(id, "\\", "/"))
	id = strings.TrimPrefix(id, "/")
	id = path.Clean(id)
	if id == "." {
		return ""
	}
	return strings.TrimSuffix(id, ".md")
}

// splitFrontmatter returns the bytes between the opening and closing "---"
// fences (the closing fence line excluded) and the body after the closing
// fence. It errors if the document does not open with "---" or the fence is
// never closed (spec §11.1).
func splitFrontmatter(text string) (fmText, body string, err error) {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != "---" {
		return "", "", fmt.Errorf("missing frontmatter: document does not start with '---'")
	}
	offset := len(lines[0])
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r\n") == "---" {
			end := offset + len(lines[i])
			return text[len(lines[0]):offset], text[end:], nil
		}
		offset += len(lines[i])
	}
	return "", "", fmt.Errorf("invalid frontmatter: unterminated '---' block")
}

// parseFrontmatter parses a YAML frontmatter block into an OrderedMap, keeping
// the top-level key order from the source. An empty block yields an empty map.
func parseFrontmatter(fmText string) (*okf.OrderedMap, error) {
	m := okf.NewOrderedMap()
	if strings.TrimSpace(fmText) == "" {
		return m, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fmText), &doc); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	if len(doc.Content) == 0 {
		return m, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid frontmatter: expected a mapping at the top level")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		m.Set(key, nodeToValue(root.Content[i+1]))
	}
	return m, nil
}

// nodeToValue converts a yaml.Node to a plain Go value (map[string]any, []any,
// or scalar). Timestamps are kept as their literal string so trust datetimes
// round-trip textually and never become time.Time.
func nodeToValue(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return nodeToValue(n.Content[0])
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			out[n.Content[i].Value] = nodeToValue(n.Content[i+1])
		}
		return out
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, nodeToValue(c))
		}
		return out
	case yaml.AliasNode:
		if n.Alias != nil {
			return nodeToValue(n.Alias)
		}
		return nil
	default: // ScalarNode
		switch n.Tag {
		case "!!null":
			return nil
		case "!!bool":
			var b bool
			if n.Decode(&b) == nil {
				return b
			}
		case "!!int":
			var i int64
			if n.Decode(&i) == nil {
				return i
			}
		case "!!float":
			var f float64
			if n.Decode(&f) == nil {
				return f
			}
		}
		// Strings, timestamps, and anything else: keep the literal text.
		return n.Value
	}
}

// encodeOrderedMap renders an OrderedMap as a YAML mapping, preserving the
// top-level key order and using a stable 2-space indent. Nested Go maps are
// emitted with sorted keys by yaml.v3, which is deterministic; this path is only
// used for frontmatter the converter synthesised or changed.
func encodeOrderedMap(m *okf.OrderedMap) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
		valNode := &yaml.Node{}
		if err := valNode.Encode(v); err != nil {
			return nil, fmt.Errorf("encode frontmatter key %q: %w", k, err)
		}
		root.Content = append(root.Content, keyNode, valNode)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func orderedMapEqual(a, b *okf.OrderedMap) bool {
	ak, bk := a.Keys(), b.Keys()
	if len(ak) != len(bk) {
		return false
	}
	for i := range ak {
		if ak[i] != bk[i] {
			return false
		}
		av, _ := a.Get(ak[i])
		bv, _ := b.Get(bk[i])
		if !reflect.DeepEqual(av, bv) {
			return false
		}
	}
	return true
}

func mapGet(m *okf.OrderedMap, key string) any {
	v, _ := m.Get(key)
	return v
}
