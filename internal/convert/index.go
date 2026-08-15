package convert

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf"
)

// IndexOptions toggles the additive, opt-in catalog rendering added by issue #9.
// The zero value (all false) makes GenerateIndexes byte-identical to its prior
// behaviour, so unset flags never change existing output.
type IndexOptions struct {
	// GroupByType appends a "# Catalog" section to the ROOT index.md only,
	// listing every concept grouped under "## <type>" subheaders. It is purely
	// additive: the existing per-directory "# Concepts"/"# Subdirectories" nav
	// (spec §8) is left untouched, and non-root indexes are not modified.
	GroupByType bool
	// IncludeBacklinks annotates each catalog entry with a bounded, sorted
	// sub-list of inbound resolved edges (concepts that link TO it). Requires
	// GroupByType; empty inbound sets render nothing.
	IncludeBacklinks bool
	// IncludeGraph annotates each catalog entry with a bounded, sorted sub-list
	// of the concept's outbound resolved edges (its dependency links). Requires
	// GroupByType; empty outbound sets render nothing.
	IncludeGraph bool
}

// maxCatalogEdges bounds how many backlink/graph annotations render under a
// single catalog entry, keeping the catalog navigable rather than exhaustive
// (the full edge set is available via `binder graph`). When more edges exist a
// single "… and N more" marker line is appended. The value is fixed so output
// stays deterministic and idempotent.
const maxCatalogEdges = 20

// GenerateIndexes builds the per-directory index.md nav tree for a bundle (spec
// §8). It returns a map of bundle-relative index path → file bytes: one per
// directory that contains concepts anywhere in its subtree, plus the bundle root.
//
// The bundle-root index.md is the ONLY place okf_version is declared (spec §12)
// and is the only index that carries frontmatter; every other index.md has no
// frontmatter (spec §8). Each index lists the directory's own concepts (with
// their descriptions) under "# Concepts" and its immediate subdirectories under
// "# Subdirectories", each section omitted when empty. Output is deterministic
// (paths sorted) and idempotent.
func GenerateIndexes(concepts []*okf.Concept, version okf.SpecVersion, opts IndexOptions) map[string][]byte {
	// Directories that must have an index: every ancestor of every concept, plus
	// the root (""). Track direct concept files and immediate child dirs.
	dirConcepts := map[string][]*okf.Concept{}
	childDirs := map[string]map[string]bool{}
	dirs := map[string]bool{"": true}

	ensureDir := func(d string) {
		if childDirs[d] == nil {
			childDirs[d] = map[string]bool{}
		}
		dirs[d] = true
	}
	ensureDir("")

	for _, c := range concepts {
		d := path.Dir(c.RelPath)
		if d == "." {
			d = ""
		}
		ensureDir(d)
		dirConcepts[d] = append(dirConcepts[d], c)
		// register d and all its ancestors, wiring each as a child of its parent.
		for cur := d; cur != ""; {
			parent := path.Dir(cur)
			if parent == "." {
				parent = ""
			}
			ensureDir(parent)
			childDirs[parent][baseName(cur)] = true
			cur = parent
		}
	}

	out := make(map[string][]byte, len(dirs))
	for d := range dirs {
		content := renderIndex(d, dirConcepts[d], sortedChildren(childDirs[d]), version)
		out[indexPath(d)] = content
	}

	// Additive catalog (issue #9): appended to the ROOT index.md only, over the
	// full corpus. Non-root indexes and the existing root nav are untouched, so
	// the default (no-flag) output is byte-identical to before.
	if opts.GroupByType {
		root := indexPath("")
		out[root] = append(out[root], renderCatalog(concepts, opts)...)
	}
	return out
}

// renderCatalog builds the additive "# Catalog" block for the root index: every
// concept grouped under "## <type>" subheaders (types sorted alphabetically,
// empty/unknown type under a final "## (untyped)" group), concepts sorted by
// RelPath within a group, each linked by its bundle-relative-absolute path. When
// IncludeBacklinks/IncludeGraph are set, each entry gets a bounded, sorted
// sub-list of inbound/outbound resolved edges. Pure function of its inputs, so
// it is deterministic and idempotent.
func renderCatalog(concepts []*okf.Concept, opts IndexOptions) []byte {
	byType := map[string][]*okf.Concept{}
	for _, c := range concepts {
		byType[c.Type] = append(byType[c.Type], c) // type used verbatim, no humanization
	}
	types := make([]string, 0, len(byType))
	untyped := false
	for t := range byType {
		if t == "" {
			untyped = true
			continue
		}
		types = append(types, t)
	}
	sort.Strings(types)

	// Edge lookups shared with `binder graph` via graph.EdgesFromConcepts, so the
	// annotated edges are provably the same resolved-edge set (parity invariant).
	var inbound, outbound map[string][]graph.Edge
	var byID map[string]*okf.Concept
	if opts.IncludeBacklinks || opts.IncludeGraph {
		inbound = map[string][]graph.Edge{}
		outbound = map[string][]graph.Edge{}
		for _, e := range graph.EdgesFromConcepts(concepts) {
			outbound[e.From] = append(outbound[e.From], e)
			inbound[e.To] = append(inbound[e.To], e)
		}
		byID = make(map[string]*okf.Concept, len(concepts))
		for _, c := range concepts {
			byID[c.ID] = c
		}
	}

	var b strings.Builder
	b.WriteString("\n# Catalog\n")
	writeGroup := func(header string, group []*okf.Concept) {
		ordered := append([]*okf.Concept(nil), group...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].RelPath < ordered[j].RelPath })
		b.WriteString("\n## " + header + "\n\n")
		for _, c := range ordered {
			b.WriteString("* [" + conceptTitle(c) + "](/" + c.RelPath + ")\n")
			if opts.IncludeBacklinks {
				writeCatalogEdges(&b, "backlink", inbound[c.ID], true, byID)
			}
			if opts.IncludeGraph {
				writeCatalogEdges(&b, "link", outbound[c.ID], false, byID)
			}
		}
	}
	for _, t := range types {
		writeGroup(t, byType[t])
	}
	if untyped {
		writeGroup("(untyped)", byType[""])
	}
	return []byte(b.String())
}

// writeCatalogEdges renders a bounded, sorted sub-list of edges under a catalog
// entry. For inbound edges the source concept (From) is shown; for outbound, the
// target (To). An empty set renders nothing. Order follows graph.EdgesFromConcepts.
func writeCatalogEdges(b *strings.Builder, label string, edges []graph.Edge, inbound bool, byID map[string]*okf.Concept) {
	shown := len(edges)
	if shown > maxCatalogEdges {
		shown = maxCatalogEdges
	}
	for i := 0; i < shown; i++ {
		e := edges[i]
		id := e.To
		if inbound {
			id = e.From
		}
		title, rel := id, id+".md"
		if c, ok := byID[id]; ok {
			title, rel = conceptTitle(c), c.RelPath
		}
		b.WriteString("  * " + label + ": [" + title + "](/" + rel + ")")
		if e.Text != "" {
			b.WriteString(" (" + e.Text + ")")
		}
		b.WriteString("\n")
	}
	if len(edges) > maxCatalogEdges {
		fmt.Fprintf(b, "  * … and %d more\n", len(edges)-maxCatalogEdges)
	}
}

func renderIndex(dir string, concepts []*okf.Concept, subdirs []string, version okf.SpecVersion) []byte {
	ordered := append([]*okf.Concept(nil), concepts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].RelPath < ordered[j].RelPath })

	var b strings.Builder
	if dir == "" {
		// Root index: the only legal location for okf_version (spec §12) and the
		// only index that carries frontmatter (spec §8).
		b.WriteString("---\n")
		b.WriteString("okf_version: \"" + string(version) + "\"\n")
		b.WriteString("---\n\n")
	}

	if len(ordered) > 0 {
		b.WriteString("# Concepts\n\n")
		for _, c := range ordered {
			name := baseName(c.RelPath)
			line := "* [" + conceptTitle(c) + "](" + name + ")"
			if desc := conceptDescription(c); desc != "" {
				line += " - " + desc
			}
			b.WriteString(line + "\n")
		}
	}

	if len(subdirs) > 0 {
		if len(ordered) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("# Subdirectories\n\n")
		for _, sd := range subdirs {
			b.WriteString("* [" + sd + "](" + sd + "/index.md)\n")
		}
	}

	return []byte(b.String())
}

func sortedChildren(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func indexPath(dir string) string {
	if dir == "" {
		return "index.md"
	}
	return dir + "/index.md"
}

func baseName(p string) string { return path.Base(p) }

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
