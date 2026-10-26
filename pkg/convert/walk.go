package convert

import (
	"path/filepath"

	"github.com/ghchinoy/binder/internal/okfrules"
)

// sourceFile is one markdown file discovered in the source corpus.
type sourceFile struct {
	rel string // slash-separated path relative to the source root
	abs string // absolute filesystem path
}

// walkCorpus returns every .md file under root, in deterministic (sorted) order.
// Hidden dot-directories (e.g. .git, .github, .claude, .obsidian) under root are
// skipped via okfrules.WalkMarkdownFiles (issue #252).
func walkCorpus(root string) ([]sourceFile, error) {
	rels, err := okfrules.WalkMarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	files := make([]sourceFile, 0, len(rels))
	for _, rel := range rels {
		files = append(files, sourceFile{rel: rel, abs: filepath.Join(root, filepath.FromSlash(rel))})
	}
	return files, nil
}
