package okf

import (
	"regexp"
	"strconv"
	"strings"
)

// htmlTagRE matches an HTML tag so it can be stripped before slugging (a heading
// like "# <code>API</code>" slugs to "api", not "codeapicode").
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// multiHyphenRE collapses runs of hyphens produced by adjacent
// spaces/punctuation into a single hyphen.
var multiHyphenRE = regexp.MustCompile(`-+`)

// HeadingSlugs returns the GitHub-style anchor slug for every ATX heading
// (levels 1–6) in body, in document order, skipping headings that fall inside a
// code region (fenced/indented blocks, code spans) via CodeRegions — so a "# foo"
// line inside a fenced block is never a heading. It is the single, unit-tested
// home of binder's heading-slug convention (a design commitment, issue #8), so
// any anchor consumer agrees on what "#bar" resolves to.
//
// Slug algorithm (pinned, GitHub-style): lowercase; strip HTML tags; drop every
// character except [a-z0-9], space, and hyphen; convert spaces to hyphens;
// collapse repeated hyphens; and give duplicate slugs the suffixes -1, -2, … in
// document order.
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
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		default:
			// dropped
		}
	}
	return multiHyphenRE.ReplaceAllString(b.String(), "-")
}
