package convert

import (
	"path"
	"regexp"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// mdLinkRE matches a standard markdown (or image) link: [text](target).
var mdLinkRE = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)]+)\)`)

// rewriteLinks rewrites standard markdown links that point at other corpus
// concepts into the recommended bundle-relative-absolute form (spec §6), e.g.
// [t](../a/b.md) → [t](/a/b.md). Only .md targets that resolve to a known
// output concept are rewritten; everything else (external URLs, anchors,
// non-.md, or not-in-corpus) is left exactly as written (spec §6: consumers
// MUST tolerate broken links). It returns the rewritten body and the extracted
// edges in source order.
//
// Matches that fall inside code (fenced/indented blocks or inline code spans)
// are ignored: link-like text there is not an edge. Code regions come from
// goldmark via okf.CodeRegions, the same markdown-aware code path the codec's
// LinkGraph uses, so the source and output sides agree on what a link is.
//
// srcRel is the current file's SOURCE-relative path; srcToOut maps every source
// concept path to its output path (they differ only for renamed reserved files).
func rewriteLinks(body, srcRel string, srcToOut map[string]string) (string, []okf.Link) {
	var links []okf.Link
	fromDir := path.Dir(srcRel)
	code := okf.CodeRegions(body)

	var out strings.Builder
	last := 0
	for _, idx := range mdLinkRE.FindAllStringSubmatchIndex(body, -1) {
		matchStart, matchEnd := idx[0], idx[1]
		if okf.InCodeRegion(matchStart, code) {
			continue // link-like text inside code: leave untouched
		}
		bang := body[idx[2]:idx[3]]
		text := body[idx[4]:idx[5]]
		target := strings.TrimSpace(body[idx[6]:idx[7]])

		if isExternal(target) || target == "" || strings.HasPrefix(target, "#") {
			continue
		}
		targetPath, frag := splitFragment(target)
		if !strings.EqualFold(path.Ext(targetPath), ".md") {
			continue
		}

		srcTarget := resolveSourcePath(fromDir, targetPath)
		outRel, ok := srcToOut[srcTarget]

		link := okf.Link{RawTarget: target, Text: text, Resolved: ok}
		if ok {
			link.TargetID = strings.TrimSuffix(outRel, ".md")
		}
		links = append(links, link)

		if !ok {
			continue // unresolved: leave untouched
		}
		newTarget := "/" + outRel
		if frag != "" {
			newTarget += "#" + frag
		}
		out.WriteString(body[last:matchStart])
		out.WriteString(bang + "[" + text + "](" + newTarget + ")")
		last = matchEnd
	}
	out.WriteString(body[last:])
	return out.String(), links
}

func isExternal(target string) bool {
	for _, p := range []string{"http://", "https://", "mailto:", "tel:", "ftp://"} {
		if strings.HasPrefix(target, p) {
			return true
		}
	}
	// A scheme-qualified target like "foo://" is external too.
	if i := strings.Index(target, "://"); i > 0 {
		return true
	}
	return false
}

func splitFragment(target string) (pathPart, frag string) {
	if i := strings.Index(target, "#"); i >= 0 {
		return target[:i], target[i+1:]
	}
	return target, ""
}

// resolveSourcePath resolves a link target (relative or bundle-absolute) against
// the linking file's directory, yielding a clean source-relative path.
func resolveSourcePath(fromDir, target string) string {
	if strings.HasPrefix(target, "/") {
		return path.Clean(strings.TrimPrefix(target, "/"))
	}
	return path.Clean(path.Join(fromDir, target))
}
