package convert

import (
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// buildRootIndex renders the bundle-root index.md (spec §8). It is the only
// place okf_version is declared (spec §12); binder emits v0.2. Per-directory
// index generation is deferred to Phase 2. The body lists every concept under a
// single "# Concepts" section, sorted by path, with each concept's description
// when present. Output is deterministic.
func buildRootIndex(concepts []*okf.Concept, version okf.SpecVersion) []byte {
	ordered := append([]*okf.Concept(nil), concepts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].RelPath < ordered[j].RelPath })

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("okf_version: \"" + string(version) + "\"\n")
	b.WriteString("---\n\n")
	b.WriteString("# Concepts\n\n")
	for _, c := range ordered {
		title := conceptTitle(c)
		line := "* [" + title + "](" + c.RelPath + ")"
		if desc := conceptDescription(c); desc != "" {
			line += " - " + desc
		}
		b.WriteString(line + "\n")
	}
	return []byte(b.String())
}

func conceptTitle(c *okf.Concept) string {
	if v, ok := c.Frontmatter.Get("title"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return c.ID
}

func conceptDescription(c *okf.Concept) string {
	if v, ok := c.Frontmatter.Get("description"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return ""
}
