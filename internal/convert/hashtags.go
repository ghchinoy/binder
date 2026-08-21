package convert

import (
	"regexp"

	"github.com/ghchinoy/binder/internal/okf"
)

// hashtagRE matches a #tag: a "#" preceded by start-of-string or whitespace,
// beginning with a letter, followed by tag characters (letters, digits, "_",
// "-", "/"). Requiring a leading letter avoids matching ATX headings ("# H",
// which have a space) and bare "#123" / hex-colour-like tokens.
var hashtagRE = regexp.MustCompile(`(^|\s)#([A-Za-z][A-Za-z0-9_/-]*)`)

// extractHashtags returns the #tags found in body, in first-appearance order,
// de-duplicated, skipping any inside code (fenced/indented blocks or inline
// spans). The tags are returned WITHOUT the leading "#".
func extractHashtags(body string) []string {
	code := okf.CodeRegions(body)
	var tags []string
	seen := map[string]bool{}
	for _, m := range hashtagRE.FindAllStringSubmatchIndex(body, -1) {
		// m[4]:m[5] is the tag text (group 2); the "#" is just before it.
		hashPos := m[4] - 1
		if okf.InCodeRegion(hashPos, code) {
			continue
		}
		tag := body[m[4]:m[5]]
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// mergeTags merges body hashtags into the frontmatter "tags" list (spec §4),
// preserving any pre-existing tags and their order, appending only new tags in
// first-appearance order, de-duplicated. Frontmatter is only mutated when a new
// tag is actually added, so a concept with no hashtags round-trips unchanged.
func mergeTags(fm *okf.OrderedMap, bodyTags []string) {
	existing := existingTags(fm)
	present := map[string]bool{}
	for _, t := range existing {
		present[t] = true
	}
	added := false
	merged := append([]string(nil), existing...)
	for _, t := range bodyTags {
		if !present[t] {
			present[t] = true
			merged = append(merged, t)
			added = true
		}
	}
	if !added {
		return // nothing new: leave frontmatter untouched
	}
	list := make([]any, len(merged))
	for i, t := range merged {
		list[i] = t
	}
	fm.Set("tags", list)
}

// existingTags reads the current "tags" value, tolerating both a YAML list and
// a bare scalar tag.
func existingTags(fm *okf.OrderedMap) []string {
	v, ok := fm.Get("tags")
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := okf.AsString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t != "" {
			return []string{t}
		}
	}
	return nil
}
