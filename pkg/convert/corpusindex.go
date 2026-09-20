package convert

import (
	"path"
	"strings"
)

// corpusIndex resolves a link target (a wikilink target or a frontmatter-ref
// value) to a bundle output path. It is the resolver design-v2 §4.2 specifies
// for wikilinks: rel-path → filename → title-slug, in that precedence order.
// Filename and title-slug matches that are ambiguous (more than one concept)
// resolve to nothing and are reported, so resolution stays deterministic.
type corpusIndex struct {
	byRel      map[string]string // normalized rel path (no leading "/", no ".md") -> outRel
	byStem     map[string]string // lowercased filename stem -> outRel
	byTitle    map[string]string // slugified title -> outRel
	stemAmbig  map[string]bool
	titleAmbig map[string]bool
}

// indexEntry is one concept's identity for corpus indexing.
type indexEntry struct {
	srcRel string
	outRel string
	title  string
}

func buildCorpusIndex(entries []indexEntry) *corpusIndex {
	ix := &corpusIndex{
		byRel:      map[string]string{},
		byStem:     map[string]string{},
		byTitle:    map[string]string{},
		stemAmbig:  map[string]bool{},
		titleAmbig: map[string]bool{},
	}
	for _, e := range entries {
		// Both the source path and the (possibly renamed) output path resolve to
		// the output concept, so a wikilink written against either name resolves.
		ix.byRel[normRelKey(e.srcRel)] = e.outRel
		ix.byRel[normRelKey(e.outRel)] = e.outRel

		stem := strings.ToLower(strings.TrimSuffix(path.Base(e.outRel), ".md"))
		ix.addUnique(ix.byStem, ix.stemAmbig, stem, e.outRel)

		if slug := slugify(e.title); slug != "" {
			ix.addUnique(ix.byTitle, ix.titleAmbig, slug, e.outRel)
		}
	}
	return ix
}

func (ix *corpusIndex) addUnique(m map[string]string, ambig map[string]bool, key, outRel string) {
	if key == "" {
		return
	}
	if existing, ok := m[key]; ok {
		if existing != outRel {
			ambig[key] = true
		}
		return
	}
	m[key] = outRel
}

// resolve maps a target to an output concept path. fromDir is the linking
// concept's directory (bundle-relative), used for relative path targets.
// It returns the output rel path, whether it resolved, and whether it failed
// specifically because a filename/title match was ambiguous.
func (ix *corpusIndex) resolve(fromDir, target string) (outRel string, resolved, ambiguous bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false, false
	}

	// 1. rel-path: interpret the target as a path (relative to fromDir, or
	//    bundle-absolute when it begins with "/"), then as a bare bundle path.
	for _, cand := range pathCandidates(fromDir, target) {
		if out, ok := ix.byRel[cand]; ok {
			return out, true, false
		}
	}

	// 2. filename: the last path segment as a filename stem.
	stem := strings.ToLower(strings.TrimSuffix(path.Base(strings.TrimSuffix(target, ".md")), ".md"))
	if ix.stemAmbig[stem] {
		return "", false, true
	}
	if out, ok := ix.byStem[stem]; ok {
		return out, true, false
	}

	// 3. title-slug.
	slug := slugify(target)
	if ix.titleAmbig[slug] {
		return "", false, true
	}
	if out, ok := ix.byTitle[slug]; ok {
		return out, true, false
	}

	return "", false, false
}

// pathCandidates returns normalized rel keys to try for a path-shaped target.
func pathCandidates(fromDir, target string) []string {
	t := strings.ReplaceAll(target, "\\", "/")
	var cands []string
	add := func(p string) {
		if k := normRelKey(p); k != "" {
			cands = append(cands, k)
		}
	}
	if strings.HasPrefix(t, "/") {
		add(strings.TrimPrefix(t, "/"))
	} else if strings.Contains(t, "/") || strings.HasPrefix(t, ".") {
		add(path.Join(fromDir, t))
	}
	// Always try the target as a bare bundle-root path too.
	add(t)
	return cands
}

// normRelKey normalizes a path to the corpusIndex key form: forward slashes, no
// leading "/", cleaned, and without a trailing ".md".
func normRelKey(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, ".md")
	if p == "" {
		return ""
	}
	p = path.Clean(p)
	if p == "." || strings.HasPrefix(p, "..") {
		return ""
	}
	return p
}

// slugify lowercases text and turns runs of non-alphanumeric characters into
// single hyphens, trimming leading/trailing hyphens. It is the title-slug form
// used for wikilink title resolution.
func slugify(s string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
