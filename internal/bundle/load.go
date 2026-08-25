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
// determinism.
//
// A file whose frontmatter will not parse is never dropped (that silently
// excluded it from every read-side answer — #161): it is RECOVERED as a
// body-only concept with the recovery marker stamped (mirroring `binder convert`,
// design-v2 §4.6), so it still appears in Concepts — and the drop is disclosed in
// Bundle.Unparsed so review/index/graph/project (and their MCP tools) can report
// it. This honours never-reject (spec §11) on the read side while ensuring the
// unparseable file is disclosed, not hidden. Use `binder validate` for the hard,
// exit-nonzero verdict.
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
				b.OKFVersion = rootOKFVersion(codec, filepath.Join(root, "index.md"), b.OKFVersion, b)
			}
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		c, err := codec.ParseConcept(rel, raw)
		if err != nil {
			// Never-reject WITH disclosure (#161): recover the file as a body-only
			// concept and stamp the recovery marker (mirroring `binder convert`), so
			// it appears in the node set — index keeps it in nav, graph/project stop
			// dangling — and record the drop so review/index/graph/project surface it
			// instead of silently computing over a set that excludes it.
			c = recoveredConcept(codec, rel, raw)
			b.Unparsed = append(b.Unparsed, okf.UnparsedConcept{ID: c.ID, RelPath: rel, Err: err.Error()})
		}
		if lg != nil {
			c.Links = lg.ExtractLinks(c.ID, c.Body)
		}
		b.Concepts = append(b.Concepts, c)
	}
	return b, nil
}

// recoveredConcept builds a body-only concept for a file whose frontmatter would
// not parse: an empty frontmatter block, the raw text (fence and all) preserved
// verbatim as body, and the recovery marker stamped so the read side reports it
// the same way `binder convert` does (design-v2 §4.6). It mirrors
// convert.plainConcept + okf.MarkRecovered so the two surfaces cannot disagree.
func recoveredConcept(codec okf.Codec, rel string, raw []byte) *okf.Concept {
	id, _ := codec.ConceptIDFromRel(rel)
	fm := okf.NewOrderedMap()
	okf.MarkRecovered(fm, "unparseable-frontmatter")
	return &okf.Concept{
		ID:          id,
		RelPath:     rel,
		Frontmatter: fm,
		Body:        strings.ReplaceAll(string(raw), "\r\n", "\n"),
	}
}

// rootOKFVersion reads okf_version from the bundle-root index.md FRONTMATTER
// (spec §12), falling back to def. It parses the frontmatter with the codec
// rather than scraping lines: a version is adopted ONLY from a value that lives
// in a frontmatter block that actually parses as YAML. An index.md whose
// frontmatter is invalid YAML, or one where `okf_version:` appears only in the
// body (including inside a fenced code block), never contributes a version — the
// drop is disclosed on the bundle instead of silently adopting unchecked input
// (#163).
func rootOKFVersion(codec okf.Codec, path string, def okf.SpecVersion, b *okf.Bundle) okf.SpecVersion {
	raw, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	c, err := codec.ParseConcept("index.md", raw)
	if err != nil {
		// The root index.md frontmatter did not parse. Do NOT adopt a version from
		// unchecked bytes; disclose that the declared version could not be read.
		b.RootVersionUnparsed = &okf.UnparsedConcept{RelPath: "index.md", Err: err.Error()}
		return def
	}
	v, ok := c.Frontmatter.Get("okf_version")
	if !ok {
		return def
	}
	if s := strings.TrimSpace(okf.AsString(v)); s != "" {
		return okf.SpecVersion(s)
	}
	return def
}
