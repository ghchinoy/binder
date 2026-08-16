package convert

import (
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// mdLinkRE matches a standard markdown (or image) link: [text](target).
var mdLinkRE = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)]+)\)`)

// linkResolver rewrites markdown links for one corpus conversion. It carries the
// data the rewrite needs beyond the file being processed so file:// resolution
// can be evaluated against the workspace boundary without hand-threading many
// arguments.
//
//   - srcToOut maps every SOURCE-relative concept path to its OUTPUT path (they
//     differ only for renamed reserved files).
//   - srcRoot is the ABSOLUTE, cleaned corpus source root. file:// targets are
//     mapped to a corpus-source-relative path against it before the srcToOut
//     lookup. When empty, file:// resolution is disabled (targets stay external).
//   - wsRoot is the ABSOLUTE, cleaned workspace boundary within which a file://
//     absolute path is considered an internal candidate; it defaults to srcRoot.
//   - externalRoots are ABSOLUTE, cleaned sibling-workspace roots the author has
//     declared as known (issue #25). A file:// target that resolves outside
//     wsRoot but under one of these stays EXTERNAL exactly as before — it is
//     never internalized — but its "resolves outside the workspace root"
//     advisory is suppressed. Empty by default (every external link advises).
//   - warn receives non-fatal advisories (never-reject); it must never be nil.
type linkResolver struct {
	srcToOut      map[string]string
	srcRoot       string
	wsRoot        string
	externalRoots []string
	warn          func(format string, args ...any)
}

// rewriteLinks rewrites standard markdown links against a zero-value resolver
// (file:// resolution disabled). It preserves the historical helper signature
// used by callers and tests that do not exercise file:// URIs.
func rewriteLinks(body, srcRel string, srcToOut map[string]string) (string, []okf.Link) {
	r := &linkResolver{srcToOut: srcToOut, warn: func(string, ...any) {}}
	return r.rewrite(body, srcRel)
}

// rewrite rewrites standard markdown links that point at other corpus concepts
// into the recommended bundle-relative-absolute form (spec §6), e.g.
// [t](../a/b.md) → [t](/a/b.md). Only .md targets that resolve to a known
// output concept are rewritten; everything else (external URLs, anchors,
// non-.md, or not-in-corpus) is left exactly as written (spec §6: consumers
// MUST tolerate broken links). It returns the rewritten body and the extracted
// edges in source order.
//
// file:// URIs are handled specially (issue #6): a file:// target that resolves
// to an absolute path inside the workspace root is mapped to a corpus-source-
// relative path and run through the SAME resolution as a normal .md link, so
// IDE-generated file:///abs/path/doc.md links become internal edges rewritten to
// /<outRel>. file:// targets with a remote host, or that escape the workspace
// root (including via .. or symlinks), stay external and emit an advisory.
//
// Matches that fall inside code (fenced/indented blocks or inline code spans)
// are ignored: link-like text there is not an edge. Code regions come from
// goldmark via okf.CodeRegions, the same markdown-aware code path the codec's
// LinkGraph uses, so the source and output sides agree on what a link is.
//
// srcRel is the current file's SOURCE-relative path.
func (r *linkResolver) rewrite(body, srcRel string) (string, []okf.Link) {
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

		// Resolve the target to a corpus-source-relative path (srcTarget) plus an
		// optional fragment, or skip the link entirely (external / anchor / non-md).
		var srcTarget, frag string
		switch {
		case target == "" || strings.HasPrefix(target, "#"):
			continue
		case isFileURL(target):
			st, fr, ok := r.resolveFileURL(target)
			if !ok {
				continue // remote host / outside root / non-md / unparseable: external
			}
			srcTarget, frag = st, fr
		case isExternal(target):
			continue
		default:
			var tp string
			tp, frag = splitFragment(target)
			if !strings.EqualFold(path.Ext(tp), ".md") {
				continue
			}
			srcTarget = resolveSourcePath(fromDir, tp)
		}

		outRel, ok := r.srcToOut[srcTarget]

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

// isFileURL reports whether target carries the file: scheme (case-insensitive),
// e.g. file:///abs/doc.md or file://localhost/abs/doc.md.
func isFileURL(target string) bool {
	if i := strings.Index(target, ":"); i > 0 {
		return strings.EqualFold(target[:i], "file")
	}
	return false
}

// resolveFileURL maps a file:// target to a corpus-source-relative .md path when
// it points inside the workspace root. ok is false (leave the link external)
// when resolution is disabled, the URI is unparseable, the authority names a
// remote host, the target is not a .md file, or the path escapes the workspace
// root lexically or through a symlink. A target inside the root that is simply
// not a known concept still returns ok=true: it becomes a recorded-but-
// unresolved edge, tolerated like any other broken link (spec §6).
func (r *linkResolver) resolveFileURL(target string) (srcTarget, frag string, ok bool) {
	if r.srcRoot == "" {
		return "", "", false // file:// resolution disabled (no root configured)
	}
	u, err := url.Parse(target)
	if err != nil {
		r.warn("file:// link %q could not be parsed; left external", target)
		return "", "", false
	}
	// Authority/host: empty and "localhost" are LOCAL; any other host is remote.
	if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
		r.warn("file:// link %q names remote host %q; left external", target, u.Host)
		return "", "", false
	}
	// url.URL.Path is already percent-decoded ("%20" → space).
	decoded := u.Path
	if decoded == "" {
		return "", "", false
	}
	if !strings.EqualFold(path.Ext(decoded), ".md") {
		return "", "", false // .md-only rule (Windows drive paths land here too)
	}

	// Clean to an absolute OS path and enforce the workspace boundary. filepath
	// .Clean resolves any ".." lexically; filepath.Rel then reveals an escape.
	absPath := filepath.Clean(filepath.FromSlash(decoded))
	if escapesRoot(r.wsRoot, absPath) {
		// Outside the workspace: the link genuinely is external and is left
		// untouched. The advisory is suppressed only when the author has declared
		// this sibling root via --external-root (issue #25); the link still stays
		// external either way.
		if !r.underDeclaredRoot(absPath) {
			r.warn("file:// link %q resolves outside the workspace root; left external", target)
		}
		return "", "", false
	}
	// Symlink safety: if the target exists, its real path must also stay in-root.
	if real, err := filepath.EvalSymlinks(absPath); err == nil {
		root := r.wsRoot
		if rr, err := filepath.EvalSymlinks(r.wsRoot); err == nil {
			root = rr
		}
		if escapesRoot(root, real) {
			// Coherent with the lexical case above: a declared external root that
			// contains the symlink's real target suppresses this advisory too. The
			// link stays external; nothing is internalized.
			if !r.underDeclaredRoot(real) {
				r.warn("file:// link %q resolves through a symlink outside the workspace root; left external", target)
			}
			return "", "", false
		}
	}

	// Inside the boundary: map to a corpus-source-relative path for the srcToOut
	// lookup. When it lands outside the corpus source root, the lookup simply
	// misses and the edge is recorded unresolved (tolerated).
	rel, err := filepath.Rel(r.srcRoot, absPath)
	if err != nil {
		return "", "", false
	}
	return path.Clean(filepath.ToSlash(rel)), u.Fragment, true
}

// underDeclaredRoot reports whether absPath lies within any author-declared
// external root (issue #25). Matching reuses escapesRoot, so it is segment-safe:
// "/projects/jib" does not contain "/projects/jibo/x.md" (their filepath.Rel is
// "../jibo/x.md"), and a root equal to absPath's parent chain matches only at a
// path-segment boundary. Ordering of the declared roots does not affect the
// result — it is a pure any-match — preserving deterministic output. It only
// gates whether an advisory is emitted; it never changes the link bytes.
func (r *linkResolver) underDeclaredRoot(absPath string) bool {
	for _, root := range r.externalRoots {
		if root == "" {
			continue
		}
		if !escapesRoot(root, absPath) {
			return true
		}
	}
	return false
}

// escapesRoot reports whether absPath falls outside root (both absolute, cleaned
// OS paths). A relative that is ".." or begins "../" leaves the root.
func escapesRoot(root, absPath string) bool {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
