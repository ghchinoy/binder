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
// preserves every key/value and the top-level key order. Unmodified frontmatter
// is re-emitted from the original source bytes verbatim, so nested-mapping key
// order and scalar quoting/folding style survive exactly. When the converter
// changes or adds a TOP-LEVEL key (e.g. a `generated` stamp), the mapping is
// rebuilt from the original order-preserving *yaml.Node: every unchanged
// top-level key reuses its source subtree verbatim, so nested-map key order and
// list-item order are preserved at every level; only the added/changed values
// are encoded fresh, deterministically.
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
	// No source frontmatter (converter-synthesised concept): encode the ordered
	// map deterministically.
	if len(con.OriginalFrontmatter) == 0 {
		return encodeOrderedMap(con.Frontmatter)
	}

	// Re-parse the original block to its order-preserving *yaml.Node tree (plus a
	// plain-value OrderedMap for equality checks). If it is no longer a mapping we
	// can round-trip, fall back to a clean encode.
	origRoot, origOM, err := parseFrontmatterNode(string(con.OriginalFrontmatter))
	if err != nil {
		return encodeOrderedMap(con.Frontmatter)
	}

	// Fast path: nothing the converter touched changed the frontmatter — emit the
	// original bytes verbatim, so scalar quoting/folding style is preserved too.
	if orderedMapEqual(origOM, con.Frontmatter) {
		return con.OriginalFrontmatter, nil
	}

	// Something changed at the TOP level (e.g. a synthesised `generated` stamp, a
	// defaulted `type`/`title`, an appended `status`). Re-emit each unchanged
	// top-level key from its ORIGINAL SOURCE BYTES and encode only the
	// added/changed keys freshly. Splicing source text — rather than reusing
	// yaml.Node values and re-encoding — is the only mechanism that is truly
	// byte-faithful: yaml.v3's encoder does not preserve flow-mapping interior
	// spacing (`{ a: b }` → `{a: b}`), re-decides scalar quoting, drops the
	// `!!timestamp` tag (retyping it to `!!str`), and loses top-level head
	// comments. This applies to ALL pre-existing frontmatter, with no key
	// singled out (design-v2 §3.2).
	return spliceFrontmatter(string(con.OriginalFrontmatter), origRoot, origOM, con.Frontmatter)
}

// spliceFrontmatter reconstructs a frontmatter block that is byte-identical to
// the source for every UNCHANGED top-level key and freshly encoded only for
// added or changed keys. It relies on the source line numbers yaml.v3 records on
// every node: each top-level key's block spans from its own line (and any head
// comment/blank lines preceding it) through the last source line of its value.
//
// Layout of the output, in order:
//   - the prefix: any comment/blank lines before the first key (defect A: these
//     top-level head comments were previously dropped);
//   - each pre-existing key IN SOURCE ORDER — verbatim when its value is
//     unchanged, else its head-comment lines verbatim followed by a fresh encode;
//   - any trailing comment/blank lines after the last key's value;
//   - each key present in fm but absent from the source, freshly encoded, in fm
//     order (the additive keys enrich/convert appended).
func spliceFrontmatter(fmText string, origRoot *yaml.Node, origOM, fm *okf.OrderedMap) ([]byte, error) {
	lines := strings.SplitAfter(fmText, "\n") // each element keeps its trailing "\n"

	type span struct {
		key             string
		keyLine, valEnd int        // 0-indexed line indices into lines
		valNode         *yaml.Node // original value node, for sibling-level splicing
	}
	var pairs []span
	for i := 0; i+1 < len(origRoot.Content); i += 2 {
		kn, vn := origRoot.Content[i], origRoot.Content[i+1]
		keyLine := kn.Line - 1
		valEnd := maxNodeLine(vn) - 1
		if valEnd < keyLine {
			valEnd = keyLine
		}
		pairs = append(pairs, span{kn.Value, keyLine, valEnd, vn})
	}
	// No parseable top-level keys (empty/degenerate block): fall back to a clean
	// deterministic encode of the desired map.
	if len(pairs) == 0 {
		return encodeOrderedMap(fm)
	}

	inSource := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		inSource[p.key] = true
	}

	var out bytes.Buffer
	ensureNL := func() {
		if b := out.Bytes(); len(b) > 0 && b[len(b)-1] != '\n' {
			out.WriteByte('\n')
		}
	}
	joinLines := func(lo, hi int) string {
		if lo < 0 {
			lo = 0
		}
		if hi > len(lines) {
			hi = len(lines)
		}
		if lo >= hi {
			return ""
		}
		return strings.Join(lines[lo:hi], "")
	}

	firstKeyLine := pairs[0].keyLine
	out.WriteString(joinLines(0, firstKeyLine)) // prefix (top-level head comments)

	for j, p := range pairs {
		spanStart := firstKeyLine
		if j > 0 {
			spanStart = pairs[j-1].valEnd + 1
		}
		v, present := fm.Get(p.key)
		if !present {
			continue // key removed from the desired map: drop its source block
		}
		ov, _ := origOM.Get(p.key)
		if reflect.DeepEqual(ov, v) {
			out.WriteString(joinLines(spanStart, p.valEnd+1)) // unchanged: verbatim
			continue
		}
		// Changed key. Keep the head-comment region verbatim either way.
		out.WriteString(joinLines(spanStart, p.keyLine))
		// Sibling-level preservation: when the value is a CHANGED block sequence
		// (e.g. a `verified` list with a stamp appended), re-emit the unchanged
		// entries from their ORIGINAL bytes and encode only the added/changed
		// entries freshly. The pre-existing entries — which can be human
		// attestations — must not be reshaped just because a neighbour arrived.
		if items, ok := spliceSequenceItems(p.valNode, v, lines); ok {
			out.WriteString(joinLines(p.keyLine, p.keyLine+1)) // the "key:" line verbatim
			out.Write(items)
			continue
		}
		// Otherwise re-encode the whole value fresh.
		fresh, err := encodeOrderedMapPair(p.key, v)
		if err != nil {
			return nil, err
		}
		ensureNL()
		out.Write(fresh)
	}

	// Trailing comment/blank lines after the last key's value.
	out.WriteString(joinLines(pairs[len(pairs)-1].valEnd+1, len(lines)))

	// Additive keys (present in fm, absent from source), in fm order.
	for _, k := range fm.Keys() {
		if inSource[k] {
			continue
		}
		v, _ := fm.Get(k)
		fresh, err := encodeOrderedMapPair(k, v)
		if err != nil {
			return nil, err
		}
		ensureNL()
		out.Write(fresh)
	}
	return out.Bytes(), nil
}

// spliceSequenceItems re-emits a changed BLOCK sequence entry-by-entry, keeping
// each unchanged leading item byte-identical to the source and encoding only the
// added or changed items freshly. It returns ok=false — signalling the caller to
// fall back to a whole-value re-encode — when the value is not a block sequence
// this mechanism can safely address by source line (a single-line flow sequence,
// an empty source sequence, a non-slice desired value, or an item span this
// simple line model cannot bound). This is the sibling-preservation guarantee
// for the append path, and it is general: no key is singled out.
func spliceSequenceItems(valNode *yaml.Node, desired any, lines []string) ([]byte, bool) {
	if valNode == nil || valNode.Kind != yaml.SequenceNode {
		return nil, false
	}
	if valNode.Style&yaml.FlowStyle != 0 {
		return nil, false // single-line flow sequence: not line-addressable
	}
	want, ok := desired.([]any)
	if !ok {
		return nil, false
	}
	items := valNode.Content
	if len(items) == 0 {
		return nil, false
	}
	// Derive the block "- " marker indent from the first item's source line; the
	// dash is the first non-space rune. Bail if the line is not shaped that way.
	first := items[0].Line - 1
	if first < 0 || first >= len(lines) {
		return nil, false
	}
	dash := strings.IndexByte(lines[first], '-')
	if dash < 0 || strings.TrimSpace(lines[first][:dash]) != "" {
		return nil, false
	}
	indent := lines[first][:dash]

	var b bytes.Buffer
	for i, dv := range want {
		if i < len(items) && reflect.DeepEqual(nodeToValue(items[i]), dv) {
			start := items[i].Line - 1
			end := maxNodeLine(items[i]) - 1
			if start < 0 || end < start || end >= len(lines) {
				return nil, false
			}
			b.WriteString(strings.Join(lines[start:end+1], ""))
			continue
		}
		fresh, err := encodeSeqItem(dv, indent)
		if err != nil {
			return nil, false
		}
		b.Write(fresh)
	}
	return b.Bytes(), true
}

// encodeSeqItem encodes a single value as a block-sequence item ("- ...") and
// indents it (and any continuation lines) to the given marker indent. Only
// freshly added/changed items take this path, so their exact style need not be
// byte-faithful — the pre-existing siblings are what must be preserved.
func encodeSeqItem(v any, indent string) ([]byte, error) {
	node := &yaml.Node{}
	if err := node.Encode([]any{v}); err != nil {
		return nil, err
	}
	raw, err := encodeNode(node)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	for _, l := range strings.SplitAfter(string(raw), "\n") {
		if l == "" {
			continue
		}
		b.WriteString(indent)
		b.WriteString(l)
	}
	return b.Bytes(), nil
}

// maxNodeLine returns the greatest source line number reached by n or any of its
// descendants — the last line the value occupies in the source. yaml.v3 records
// a line on every node, so for block and flow structures alike this bounds the
// key's source span. (Multi-line block scalars have no child nodes and are
// under-counted; the surplus lines simply fall into the next key's head region
// and are still emitted verbatim, so the block stays intact.)
func maxNodeLine(n *yaml.Node) int {
	m := n.Line
	for _, c := range n.Content {
		if l := maxNodeLine(c); l > m {
			m = l
		}
	}
	return m
}

// encodeOrderedMapPair renders a single "key: value" mapping deterministically
// (2-space indent), used for added/changed frontmatter keys.
func encodeOrderedMapPair(key string, v any) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{}
	if err := valNode.Encode(v); err != nil {
		return nil, fmt.Errorf("encode frontmatter key %q: %w", key, err)
	}
	root.Content = append(root.Content, keyNode, valNode)
	return encodeNode(root)
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
	if strings.TrimSpace(fmText) == "" {
		return okf.NewOrderedMap(), nil
	}
	_, m, err := parseFrontmatterNode(fmText)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// parseFrontmatterNode parses a YAML frontmatter block into both its root
// mapping *yaml.Node (order- and style-preserving, at every nesting level) and
// an OrderedMap of plain values (for cheap top-level equality checks). It is the
// order-preserving representation design-v2 §3.2 relies on.
func parseFrontmatterNode(fmText string) (*yaml.Node, *okf.OrderedMap, error) {
	m := okf.NewOrderedMap()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fmText), &doc); err != nil {
		return nil, nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	empty := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if len(doc.Content) == 0 {
		return empty, m, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("invalid frontmatter: expected a mapping at the top level")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		m.Set(root.Content[i].Value, nodeToValue(root.Content[i+1]))
	}
	return root, m, nil
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
	return encodeNode(root)
}

// encodeNode renders a yaml.Node with a stable 2-space indent.
func encodeNode(root *yaml.Node) ([]byte, error) {
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
