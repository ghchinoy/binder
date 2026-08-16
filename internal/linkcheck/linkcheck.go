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

// rootEntrypointNames are the conventional corpus-root entrypoint documents: a
// document that links OUTWARD into the rest of the corpus rather than being linked
// into. They are auto-recognized as entrypoints (not orphans) by `binder review`
// and `binder lint` even when they have no inbound edges (issue #24). Kept here,
// in the review/lint-shared package, so the two surfaces can never drift on what
// counts as a root entrypoint — the same anti-drift rationale as the predicates
// above.
var rootEntrypointNames = map[string]bool{"readme.md": true, "index.md": true}

// IsRootEntrypoint reports whether relPath is a conventional corpus-root
// entrypoint document (README.md or index.md at the corpus root). Matching is
// case-insensitive on the file name and requires the file to sit at the root (no
// directory component), so a nested docs/README.md is NOT auto-recognized — only
// the corpus root README/index serve as the primary index (issue #24).
func IsRootEntrypoint(relPath string) bool {
	rel := strings.TrimSpace(relPath)
	if rel == "" || strings.ContainsAny(rel, "/\\") {
		return false
	}
	return rootEntrypointNames[strings.ToLower(rel)]
}

// EntrypointSet normalizes user-designated entrypoint identifiers into a set keyed
// by concept id. Each entry may be given as a concept id ("docs/intro") or as a
// bundle-relative path ("docs/intro.md"); a trailing ".md" (any case) and
// surrounding whitespace are stripped so either form matches a concept's ID. Empty
// entries are ignored. It is advisory input only — it never mints an identity,
// merely reclassifies an existing concept from orphan to entrypoint.
func EntrypointSet(designations []string) map[string]bool {
	set := make(map[string]bool, len(designations))
	for _, d := range designations {
		id := strings.TrimSpace(d)
		if len(id) >= 3 && strings.EqualFold(id[len(id)-3:], ".md") {
			id = id[:len(id)-3]
		}
		if id != "" {
			set[id] = true
		}
	}
	return set
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
