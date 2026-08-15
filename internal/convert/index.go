package convert

import (
	"path"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

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
func GenerateIndexes(concepts []*okf.Concept, version okf.SpecVersion) map[string][]byte {
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
	return out
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
