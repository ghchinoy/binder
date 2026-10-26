package convert

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// sourceFile is one markdown file discovered in the source corpus.
type sourceFile struct {
	rel string // slash-separated path relative to the source root
	abs string // absolute filesystem path
}

// walkCorpus returns every .md file under root, in deterministic (sorted) order.
// Hidden dot-directories (e.g. .git, .github, .claude, .obsidian) under root are
// skipped so agent harness and repo metadata are not ingested as OKF concepts
// (issue #252).
func walkCorpus(root string) ([]sourceFile, error) {
	var files []sourceFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, sourceFile{rel: filepath.ToSlash(rel), abs: path})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, nil
}
