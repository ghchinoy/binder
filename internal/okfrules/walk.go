package okfrules

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// WalkMarkdownFiles walks root and returns the slash-separated paths (relative
// to root) of every .md file discovered, sorted lexically for determinism.
// Hidden dot-directories (e.g. .git, .github, .claude, .obsidian) under root
// are skipped so agent harness and repository metadata files are never ingested
// as OKF concepts (issue #252).
func WalkMarkdownFiles(root string) ([]string, error) {
	var rels []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(rels)
	return rels, nil
}
