package convert

import (
	"path"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// frontmatterRefEdges reads the configured frontmatter-ref keys (--fm-ref-keys,
// e.g. "related,parent") and emits a graph edge for each referenced concept
// (design-v2 §4.2). The original frontmatter key/value is PRESERVED unchanged —
// these edges are additive, so a `related:`/`parent:` field survives round-trip
// while also becoming a first-class edge. Values may be a single reference or a
// YAML list of references; each is resolved via the corpus index (path, then
// filename, then title-slug). Unresolved refs are reported like any other edge.
func frontmatterRefEdges(fm *okf.OrderedMap, outRel string, keys []string, ix *corpusIndex) []okf.Link {
	if len(keys) == 0 {
		return nil
	}
	fromDir := path.Dir(outRel)
	if fromDir == "." {
		fromDir = ""
	}
	var links []okf.Link
	for _, key := range keys {
		v, ok := fm.Get(key)
		if !ok {
			continue
		}
		for _, ref := range refValues(v) {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			target, resolved, _ := ix.resolve(fromDir, ref)
			link := okf.Link{RawTarget: ref, Text: key, Resolved: resolved}
			if resolved {
				link.TargetID = strings.TrimSuffix(target, ".md")
			}
			links = append(links, link)
		}
	}
	return links
}

// refValues normalizes a frontmatter-ref value (scalar, list, or a wikilink-ish
// "[[Target]]" scalar) to a slice of reference strings.
func refValues(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{unwrapWiki(t)}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := okf.AsString(item); s != "" {
				out = append(out, unwrapWiki(s))
			}
		}
		return out
	default:
		return nil
	}
}

// unwrapWiki strips a surrounding [[...]] (and any |alias) from a ref value so a
// frontmatter ref written as a wikilink resolves the same way an inline one does.
func unwrapWiki(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[[") && strings.HasSuffix(s, "]]") {
		inner := s[2 : len(s)-2]
		target, _ := splitWikilink(inner)
		return target
	}
	return s
}

// ParseFMRefKeys parses a --fm-ref-keys value ("related,parent,see-also") into a
// clean, de-duplicated key list.
func ParseFMRefKeys(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, k := range strings.Split(s, ",") {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}
