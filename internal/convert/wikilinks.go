package convert

import (
	"regexp"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// wikilinkRE matches a wiki-style link: [[Target]] or [[Target|alias]].
var wikilinkRE = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// rewriteWikilinks resolves [[Target]] / [[Target|alias]] against the corpus
// index (rel-path → filename → title-slug, §4.2) and rewrites each resolved one
// to a standard bundle-relative-absolute markdown link [display](/out/rel.md).
// Unresolved wikilinks are LEFT IN PLACE and reported (spec §6: consumers MUST
// tolerate broken links; design-v2 §4.2: never dropped). Wikilinks inside code
// (fenced/indented blocks or inline spans) are ignored — the same markdown-aware
// code detection the standard-link rewriter uses. Returns the rewritten body and
// the discovered edges in source order.
func rewriteWikilinks(body, fromDir string, ix *corpusIndex) (string, []okf.Link) {
	if !strings.Contains(body, "[[") {
		return body, nil
	}
	code := okf.CodeRegions(body)

	var links []okf.Link
	var out strings.Builder
	last := 0
	for _, idx := range wikilinkRE.FindAllStringSubmatchIndex(body, -1) {
		matchStart, matchEnd := idx[0], idx[1]
		if okf.InCodeRegion(matchStart, code) {
			continue // link-like text inside code: leave untouched
		}
		inner := body[idx[2]:idx[3]]
		target, alias := splitWikilink(inner)
		if target == "" {
			continue
		}

		outRel, resolved, _ := ix.resolve(fromDir, target)
		display := alias
		if display == "" {
			display = target
		}

		link := okf.Link{RawTarget: target, Text: display, Resolved: resolved}
		if resolved {
			link.TargetID = strings.TrimSuffix(outRel, ".md")
		}
		links = append(links, link)

		if !resolved {
			continue // leave [[...]] untouched
		}
		out.WriteString(body[last:matchStart])
		out.WriteString("[" + display + "](/" + outRel + ")")
		last = matchEnd
	}
	out.WriteString(body[last:])
	return out.String(), links
}

// splitWikilink splits the inside of a [[...]] into target and optional alias.
func splitWikilink(inner string) (target, alias string) {
	if i := strings.IndexByte(inner, '|'); i >= 0 {
		return strings.TrimSpace(inner[:i]), strings.TrimSpace(inner[i+1:])
	}
	return strings.TrimSpace(inner), ""
}
