package convert

import (
	"path"
	"strings"
	"time"

	"github.com/ghchinoy/binder/internal/okf"
)

// hasFrontmatter reports whether raw begins with a YAML frontmatter block
// (a "---" line at the very start). Plain-markdown corpus files usually do not.
func hasFrontmatter(raw []byte) bool {
	s := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") && s != "---" {
		return false
	}
	// Require a closing delimiter on its own line.
	rest := strings.TrimPrefix(s, "---\n")
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimRight(line, "\r") == "---" {
			return true
		}
	}
	return false
}

// opensFrontmatterFence reports whether raw begins with a "---" fence line, i.e.
// the file INTENDS to carry frontmatter — regardless of whether that fence is
// ever closed or the YAML between fences parses. It is deliberately more lenient
// than hasFrontmatter: an opened-but-unterminated fence and an opened-but-invalid
// fence both qualify, so the converter routes both to the codec parser and lets
// its error drive the never-reject recover-as-body path (design-v2 §4 robustness).
func opensFrontmatterFence(raw []byte) bool {
	s := string(raw)
	if strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n") {
		return true
	}
	return strings.TrimRight(s, "\r\n") == "---"
}

// ensureType applies the type precedence: existing (non-empty) → per-directory
// --type-map → --default-type. It sets frontmatter["type"] and returns the value.
func ensureType(fm *okf.OrderedMap, relPath string, typeMap map[string]string, defaultType string) string {
	if v, ok := fm.Get("type"); ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	typ := defaultType
	if mapped := lookupTypeMap(typeMap, relPath); mapped != "" {
		typ = mapped
	}
	fm.Set("type", typ)
	return typ
}

// lookupTypeMap matches relPath against directory keys in typeMap. A key matches
// when it equals any ancestor directory segment path of the file. The
// longest (most specific) matching key wins; ties break lexicographically for
// determinism.
func lookupTypeMap(typeMap map[string]string, relPath string) string {
	if len(typeMap) == 0 {
		return ""
	}
	dir := path.Dir(relPath)
	best, bestKey := "", ""
	for key, val := range typeMap {
		k := strings.Trim(key, "/")
		if k == "" {
			continue
		}
		if dir == k || strings.HasPrefix(dir+"/", k+"/") {
			if len(k) > len(bestKey) || (len(k) == len(bestKey) && k < bestKey) {
				best, bestKey = val, k
			}
		}
	}
	return best
}

// ensureTitle applies the title precedence: existing → first H1 → humanized filename.
func ensureTitle(fm *okf.OrderedMap, relPath, body string) {
	if v, ok := fm.Get("title"); ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return
		}
	}
	if h1 := firstH1(body); h1 != "" {
		fm.Set("title", h1)
		return
	}
	fm.Set("title", humanize(path.Base(relPath)))
}

// firstH1 returns the text of the first level-1 ATX heading in body, or "".
// Trailing "#tag" hashtag tokens are stripped: a heading like "# Introduction
// #overview" yields the title "Introduction" while `#overview` is still merged
// into tags (spec §4). The hashtag is a tag marker, not part of the title.
func firstH1(body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return stripTrailingHashtags(strings.TrimSpace(strings.TrimPrefix(t, "# ")))
		}
	}
	return ""
}

// stripTrailingHashtags removes trailing "#tag" tokens from a derived title.
// Only trailing tokens are removed, and only well-formed hashtags (a leading '#'
// followed by tag characters), so a mid-title token or a word like "C#" is kept.
func stripTrailingHashtags(s string) string {
	fields := strings.Fields(s)
	end := len(fields)
	for end > 0 && isHashtagToken(fields[end-1]) {
		end--
	}
	return strings.Join(fields[:end], " ")
}

// isHashtagToken reports whether f is a well-formed "#tag" token.
func isHashtagToken(f string) bool {
	if len(f) < 2 || f[0] != '#' {
		return false
	}
	for _, r := range f[1:] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// humanize turns a filename like "getting-started.md" into "Getting Started".
func humanize(name string) string {
	base := strings.TrimSuffix(name, path.Ext(name))
	base = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	fields := strings.Fields(base)
	for i, f := range fields {
		r := []rune(f)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		fields[i] = string(r)
	}
	return strings.Join(fields, " ")
}

// stampGenerated records the provenance of THIS conversion as
// generated: { by: "binder/<ver>", at: <ISO8601> } — but only if the concept
// does not already carry a generated stamp. Existing trust frontmatter is
// preserved byte-faithfully (design-v2 §3.2); binder never clobbers it.
func stampGenerated(fm *okf.OrderedMap, version string, now time.Time) {
	if fm.Has("generated") {
		return
	}
	fm.Set("generated", map[string]any{
		"by": "binder/" + version,
		"at": now.UTC().Format(time.RFC3339),
	})
}
