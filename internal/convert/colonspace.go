package convert

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// kvLine matches one frontmatter line that opens a "key:" or "key: value"
// mapping entry, tolerating leading indentation and a single "- " block-sequence
// marker. The key class excludes ':' and '#', so on "title: thing: with a colon"
// the key stops at the FIRST colon and the value is everything after it —
// exactly the split the advisory needs. It is a cheap PRE-FILTER only; a match
// is never a finding on its own (see colonSpaceKeys).
var kvLine = regexp.MustCompile(`^([ \t]*)(?:-[ \t]+)?([^#:\s][^#:]*):(?:[ \t]+(\S.*))?$`)

// colonSpaceKeys reports, in document order, the frontmatter keys of norm whose
// value is an UNQUOTED plain scalar containing a colon-space (": ").
//
// That construct is the single most common OKF authoring defect in the wild
// (issue #93): in `title: Multi-View: Tabs and Windows` a YAML parser reads the
// second colon as a nested mapping indicator, so the value is not the string the
// author meant and the frontmatter block stops parsing. The defect is NOT
// key-specific — the measured wild instance was on `title:`, not `description:`
// — so every key is scanned. Quoting the value is the fix.
//
// Detection is the cheap kvLine pre-filter CONFIRMED by a real yaml.v3 parse of
// the candidate entry in isolation (#93 requires a parser, not a regex alone),
// which is what keeps the classic false positives clean:
//   - `url: https://example.com` and `standup: 12:30` — a colon NOT followed by
//     a space; the pre-filter's value never contains ": " at all.
//   - `title: "Multi-View: Tabs"` — quoted, so it is not a plain scalar (and it
//     is the very form the advisory asks for).
//   - the interior of a `|` / `>` block scalar, where `Note: like this` is
//     ordinary text rather than a mapping — skipped by indentation.
//   - a flow collection (`[...]` / `{...}`), which is not a plain scalar.
//
// norm is NormalizeInput's output (BOM-stripped, lone CRs translated), the same
// bytes the codec parses. A file with no opening "---" fence yields nothing; an
// unterminated fence is still scanned, because the trap is just as present in a
// block the author never closed and convert recovers that file rather than
// rejecting it.
//
// Like every other SourceFacts field this is purely descriptive: it never
// rejects, gates, or mutates anything (never-reject, spec §11).
func colonSpaceKeys(norm []byte) []string {
	fmText, ok := frontmatterRegion(strings.ReplaceAll(string(norm), "\r\n", "\n"))
	if !ok {
		return nil
	}

	var keys []string
	// blockIndent is the indent of the key that opened a |/> block scalar, or -1
	// when not inside one. Everything more-indented than it is literal text.
	blockIndent := -1

	for _, raw := range strings.Split(fmText, "\n") {
		line := strings.TrimRight(raw, " \t")
		if blockIndent >= 0 {
			if strings.TrimSpace(line) == "" || leadingIndent(line) > blockIndent {
				continue
			}
			blockIndent = -1
		}

		m := kvLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, value := strings.TrimSpace(m[2]), m[3]
		if value == "" {
			continue // a nested-mapping or empty-valued key has no scalar to judge
		}
		// A plain scalar cannot contain " #": that begins a trailing comment.
		if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimRight(value[:i], " \t")
			if value == "" {
				continue
			}
		}
		switch value[0] {
		case '|', '>':
			blockIndent = leadingIndent(line) // interior is text, not mapping entries
			continue
		case '"', '\'':
			continue // quoted: the correct form, and what the advisory asks for
		case '[', '{':
			continue // flow collection, not a plain scalar (out of scope for #93)
		case '&', '*', '!':
			continue // anchor / alias / tag, not a plain scalar
		}
		if !strings.Contains(value, ": ") {
			continue
		}
		// Parser confirmation. The entry is re-formed at column 0 so nesting depth
		// cannot colour the result: what is being asked is only whether THIS
		// key/value pair is a well-formed scalar entry. A plain scalar carrying a
		// colon-space never is — yaml.v3 rejects it with "mapping values are not
		// allowed in this context" — while every shape the pre-filter lets through
		// by accident parses cleanly and is dropped here.
		var probe any
		if yaml.Unmarshal([]byte(key+": "+value), &probe) == nil {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// frontmatterRegion returns the text between the opening and closing "---"
// fences of a document whose CRLFs are already collapsed. ok is false when the
// document does not open a fence. An UNTERMINATED fence yields everything after
// the opener: convert recovers such a file as body rather than rejecting it, and
// what the author wrote is still worth advising on.
func frontmatterRegion(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return "", false
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return strings.Join(lines[1:i], "\n"), true
		}
	}
	return strings.Join(lines[1:], "\n"), true
}

// leadingIndent is the number of leading space/tab bytes of line.
func leadingIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}
