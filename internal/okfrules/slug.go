package okfrules

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// htmlTagRE matches an HTML tag so it can be stripped before slugging (a heading
// like "# <code>API</code>" slugs to "api", not "codeapicode").
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// HeadingSlugs returns the GitHub-style anchor slug for every ATX heading
// (levels 1–6) in body, in document order, skipping headings that fall inside a
// code region (fenced/indented blocks, code spans) via MaskCode — so a "# foo"
// line inside a fenced or indented block is never a heading. It is the single,
// unit-tested home of binder's heading-slug convention (a design commitment,
// issue #8), so any anchor consumer agrees on what "#bar" resolves to.
//
// Slug algorithm (pinned, GitHub-style, matching github-slugger, issues #76/#85):
// lowercase; strip HTML tags; keep Unicode letters, digits, and combining marks
// (\p{L}\p{N}\p{M}) plus underscore and hyphen; convert spaces to hyphens WITHOUT
// collapsing consecutive hyphens; and give duplicate slugs the suffixes -1, -2, …
// in document order.
func HeadingSlugs(body string) []string {
	masked := MaskCode(body)
	var slugs []string
	seen := map[string]int{}

	for _, line := range strings.SplitAfter(masked, "\n") {
		t := strings.TrimSpace(line)
		hashes := 0
		for hashes < len(t) && t[hashes] == '#' {
			hashes++
		}
		// A valid ATX heading is 1–6 '#'s followed by a space and some text.
		if hashes == 0 || hashes > 6 || hashes >= len(t) || t[hashes] != ' ' {
			continue
		}

		slug := Slugify(strings.TrimSpace(t[hashes+1:]))
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

// Slugify applies the pinned GitHub-style slug algorithm to a single heading or
// title text (issues #76, #85, #86).
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = htmlTagRE.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '_', r == '-':
			// Unicode word characters (\p{L}\p{N}\p{M}, underscore) and hyphens
			// are kept verbatim, matching github-slugger (#85).
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
