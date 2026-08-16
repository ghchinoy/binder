package okf

import (
	"regexp"
	"strconv"
	"strings"
)

// htmlTagRE matches an HTML tag so it can be stripped before slugging (a heading
// like "# <code>API</code>" slugs to "api", not "codeapicode").
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// HeadingSlugs returns the GitHub-style anchor slug for every ATX heading
// (levels 1–6) in body, in document order, skipping headings that fall inside a
// code region (fenced/indented blocks, code spans) via CodeRegions — so a "# foo"
// line inside a fenced block is never a heading. It is the single, unit-tested
// home of binder's heading-slug convention (a design commitment, issue #8), so
// any anchor consumer agrees on what "#bar" resolves to.
//
// Slug algorithm (pinned, GitHub-style, matching github-slugger): lowercase;
// strip HTML tags; drop every character that is not a word character
// ([a-z0-9_] — underscore is kept), a space, or a hyphen; convert spaces to
// hyphens WITHOUT collapsing consecutive hyphens; and give duplicate slugs the
// suffixes -1, -2, … in document order.
//
// Deliberate divergence from github-slugger: the kept letter/digit set is ASCII
// [a-z0-9] rather than the full Unicode word-character class (\p{L}\p{N}\p{M}),
// so non-ASCII heading text slugs differently than on GitHub. This preserves
// binder's long-standing ASCII behaviour; #76 was scoped to the two polarity
// bugs (hyphen collapsing and dropped underscores), not to Unicode support.
func HeadingSlugs(body string) []string {
	code := CodeRegions(body)
	var slugs []string
	seen := map[string]int{}

	offset := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		lineStart := offset
		offset += len(line)

		t := strings.TrimSpace(line)
		hashes := 0
		for hashes < len(t) && t[hashes] == '#' {
			hashes++
		}
		// A valid ATX heading is 1–6 '#'s followed by a space and some text.
		if hashes == 0 || hashes > 6 || hashes >= len(t) || t[hashes] != ' ' {
			continue
		}
		if InCodeRegion(lineStart, code) {
			continue
		}

		slug := slugify(strings.TrimSpace(t[hashes+1:]))
		base := slug
		if n, ok := seen[base]; ok {
			seen[base] = n + 1
			slug = base + "-" + strconv.Itoa(n)
		} else {
			seen[base] = 1
		}
		slugs = append(slugs, slug)
	}
	return slugs
}

// slugify applies the pinned slug algorithm to a single heading's text.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = htmlTagRE.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			// Word characters ([a-z0-9_]) and hyphens are kept verbatim;
			// underscore is a word character on GitHub and must round-trip.
			b.WriteRune(r)
		case r == ' ':
			// Spaces become hyphens. Runs are NOT collapsed: two adjacent
			// spaces (e.g. from a dropped "/") yield "--", matching GitHub.
			b.WriteByte('-')
		default:
			// dropped
		}
	}
	return b.String()
}
