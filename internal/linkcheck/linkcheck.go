// Package linkcheck holds the two broken-reference predicates shared by every
// read surface that reports link health over a resolved corpus/bundle:
// `binder review` (over an emitted bundle) and `binder lint` (over a source
// corpus, issue #8). Factoring them here means the two commands can never drift
// on what counts as a "broken concept reference" or a "residual wikilink" — the
// same drift the #9 EdgesFromConcepts parity work eliminated for edges. It adds
// no new link-resolution logic; it only classifies already-extracted raw targets
// and scans a persisted body for un-rewritten wikilinks.
package linkcheck

import (
	"regexp"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// wikilinkRE matches a residual wiki-style link left in a body: [[Target]] or
// [[Target|alias]]. `binder convert` rewrites RESOLVED wikilinks to standard
// markdown links, so any [[...]] surviving in a persisted body is by construction
// an UNRESOLVED reference the read side must report (design-v2 §4.2).
var wikilinkRE = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// IsBrokenConceptRef reports whether a raw, unresolved link target is an internal
// CONCEPT reference — i.e. a bundle-relative .md target that names no concept.
// External URLs, same-document anchors, and links to non-concept files (assets,
// scripts, directories) are not concept references and so are never "broken"
// edges, matching what `binder convert` tracks.
func IsBrokenConceptRef(raw string) bool {
	t := strings.TrimSpace(raw)
	if t == "" || strings.HasPrefix(t, "#") {
		return false
	}
	if strings.Contains(t, "://") {
		return false
	}
	for _, p := range []string{"mailto:", "tel:", "ftp:"} {
		if strings.HasPrefix(t, p) {
			return false
		}
	}
	// Only a .md target (ignoring any #fragment) is a concept reference.
	if i := strings.IndexByte(t, '#'); i >= 0 {
		t = t[:i]
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(t)), ".md")
}

// ResidualWikilinks returns the targets of any [[...]] / [[...|alias]] wikilinks
// left in body, excluding those inside code spans/blocks (matching the
// converter's code-aware handling). By construction these are unresolved
// references.
func ResidualWikilinks(body string) []string {
	if !strings.Contains(body, "[[") {
		return nil
	}
	code := okf.CodeRegions(body)
	var out []string
	for _, idx := range wikilinkRE.FindAllStringSubmatchIndex(body, -1) {
		if okf.InCodeRegion(idx[0], code) {
			continue
		}
		inner := body[idx[2]:idx[3]]
		target := inner
		if i := strings.IndexByte(inner, '|'); i >= 0 {
			target = inner[:i]
		}
		if target = strings.TrimSpace(target); target != "" {
			out = append(out, target)
		}
	}
	return out
}
