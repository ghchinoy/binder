package convert

import (
	"bytes"
	"path"
	"strings"
	"time"

	"github.com/ghchinoy/binder/internal/okf"
)

// Normalization notes emitted by NormalizeInput. They are machine-readable and
// stable so a headless pipeline can key on them; they are the vocabulary the
// convert/enrich reports disclose in the per-file `normalized` signal (#124).
const (
	// NoteStrippedUTF8BOM is emitted when a single leading UTF-8 BOM was removed.
	NoteStrippedUTF8BOM = "stripped-utf8-bom"
	// NoteTranslatedLoneCR is emitted when one or more lone CRs (a "\r" that is
	// NOT part of a "\r\n") were translated to "\n".
	NoteTranslatedLoneCR = "translated-lone-cr"
)

// utf8BOM is the three-byte UTF-8 byte-order mark (U+FEFF encoded as UTF-8).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// NormalizeInput is the SINGLE normalization point at the convert/enrich read
// boundary (#124, design §4.4). It strips a single leading UTF-8 BOM and
// translates lone CRs ("\r" not part of a "\r\n") to "\n", returning the
// normalized bytes and machine-readable notes describing what changed (empty
// when nothing changed). Both the fence detectors below and the codec parser
// receive its output, so recognition and parsing see the same bytes.
//
// It deliberately does NOT touch "\r\n": CRLF is already preserved by the codec,
// and rewriting it here would change output for ordinary CRLF files. The BOM
// strip mirrors internal/infer's precedent; lone-CR→LF mirrors
// the existing "\r\n"→"\n" step. Normalization is not applied inside the codec
// (design §4.4), so the codec still receives a clean, recognised fence and the
// existing parse/skip contract is unchanged.
func NormalizeInput(raw []byte) (norm []byte, notes []string) {
	out := raw
	if bytes.HasPrefix(out, utf8BOM) {
		out = out[len(utf8BOM):]
		notes = append(notes, NoteStrippedUTF8BOM)
	}
	if translated, changed := translateLoneCR(out); changed {
		out = translated
		notes = append(notes, NoteTranslatedLoneCR)
	}
	return out, notes
}

// translateLoneCR replaces every lone CR (a "\r" NOT immediately followed by a
// "\n") with "\n", leaving "\r\n" pairs untouched. It reports whether any lone
// CR was found so the caller can emit the disclosure note only when bytes
// actually changed.
func translateLoneCR(b []byte) (out []byte, changed bool) {
	if bytes.IndexByte(b, '\r') < 0 {
		return b, false
	}
	res := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == '\r' && (i+1 >= len(b) || b[i+1] != '\n') {
			res = append(res, '\n')
			changed = true
			continue
		}
		res = append(res, b[i])
	}
	return res, changed
}

// hasFrontmatter reports whether raw begins with a YAML frontmatter block
// (a "---" line at the very start). Plain-markdown corpus files usually do not.
// It operates on NormalizeInput's output so a leading BOM or a lone-CR-delimited
// fence is recognised the same as any other (#124).
func hasFrontmatter(raw []byte) bool {
	norm, _ := NormalizeInput(raw)
	s := strings.ReplaceAll(string(norm), "\r\n", "\n")
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
//
// It operates on NormalizeInput's output so a BOM-prefixed or lone-CR-delimited
// fence opens the same as a plain one (#124); before this, such a fence never
// opened and the file was silently demoted to body text.
func opensFrontmatterFence(raw []byte) bool {
	norm, _ := NormalizeInput(raw)
	s := string(norm)
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
	if mapped := lookupPrefixMap(typeMap, relPath); mapped != "" {
		typ = mapped
	}
	fm.Set("type", typ)
	return typ
}

// lookupPrefixMap matches relPath against directory-prefix keys in m. A key
// matches when it equals the file's directory or is an ancestor directory of it.
// The longest (most specific) matching key wins; ties break lexicographically
// for determinism. Keys are trimmed of surrounding "/". Empty keys are ignored.
// It is the shared matcher behind --type-map, --status-map, and --stale-after-map.
func lookupPrefixMap(m map[string]string, relPath string) string {
	if len(m) == 0 {
		return ""
	}
	dir := path.Dir(relPath)
	best, bestKey := "", ""
	for key, val := range m {
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
// preserved losslessly (design-v2 §3.2); binder never clobbers it.
func stampGenerated(fm *okf.OrderedMap, version string, now time.Time) {
	if fm.Has("generated") {
		return
	}
	fm.Set("generated", map[string]any{
		"by": "binder/" + version,
		"at": now.UTC().Format(time.RFC3339),
	})
}
