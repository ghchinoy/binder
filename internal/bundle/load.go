// Package bundle loads an on-disk OKF bundle into the binder-owned okf model,
// using an injected okf.Codec/okf.LinkGraph. It is the read side shared by the
// index, review, and graph commands. Like every package above internal/okf, it
// depends only on the binder-owned interfaces, never on a concrete codec
// (dependency rule, design-v2 §2.2).
package bundle

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// Load parses every non-reserved .md under root into a concept, extracting each
// concept's edges with the codec's LinkGraph (the output-side graph surface, so
// edges match what validate/graph see). Reserved files (index.md/log.md) are not
// concepts and are skipped. Concepts are returned sorted by RelPath for
// determinism. A file with unparseable frontmatter is skipped, not fatal
// (never-reject, spec §11); use validate to surface such problems.
func Load(root string, codec okf.Codec) (*okf.Bundle, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "load", Path: root, Err: os.ErrInvalid}
	}

	var rels []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
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

	lg, _ := codec.(okf.LinkGraph)
	b := &okf.Bundle{Root: root, OKFVersion: okf.DefaultSpecVersion}
	for _, rel := range rels {
		if codec.IsReservedFile(rel) {
			if rel == "index.md" {
				b.OKFVersion = rootOKFVersion(filepath.Join(root, "index.md"), b.OKFVersion)
			}
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		c, err := codec.ParseConcept(rel, raw)
		if err != nil {
			continue // never-reject: unparseable concepts are validate's concern
		}
		if lg != nil {
			c.Links = lg.ExtractLinks(c.ID, c.Body)
		}
		b.Concepts = append(b.Concepts, c)
	}
	return b, nil
}

// rootOKFVersion reads okf_version from the bundle-root index.md frontmatter if
// present (spec §12), falling back to def.
func rootOKFVersion(path string, def okf.SpecVersion) okf.SpecVersion {
	raw, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "okf_version:") {
			v := strings.TrimSpace(strings.TrimPrefix(t, "okf_version:"))
			v = strings.Trim(v, `"'`)
			if v != "" {
				return okf.SpecVersion(v)
			}
		}
	}
	return def
}
