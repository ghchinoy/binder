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
// srcRel is the current file's SOURCE-relative path; srcToOut maps every source
// concept path to its output path (they differ only for renamed reserved files).
func rewriteLinks(body, srcRel string, srcToOut map[string]string) (string, []okf.Link) {
	var links []okf.Link
	fromDir := path.Dir(srcRel)

	out := mdLinkRE.ReplaceAllStringFunc(body, func(match string) string {
		m := mdLinkRE.FindStringSubmatch(match)
		bang, text, target := m[1], m[2], strings.TrimSpace(m[3])

		if isExternal(target) || target == "" || strings.HasPrefix(target, "#") {
			return match
		}
		targetPath, frag := splitFragment(target)
		if !strings.EqualFold(path.Ext(targetPath), ".md") {
			return match
		}

		srcTarget := resolveSourcePath(fromDir, targetPath)
		outRel, ok := srcToOut[srcTarget]

		link := okf.Link{RawTarget: target, Text: text, Resolved: ok}
		if ok {
			link.TargetID = strings.TrimSuffix(outRel, ".md")
		}
		links = append(links, link)

		if !ok {
			return match // unresolved: leave untouched
		}
		newTarget := "/" + outRel
		if frag != "" {
			newTarget += "#" + frag
		}
		return bang + "[" + text + "](" + newTarget + ")"
	})
	return out, links
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
