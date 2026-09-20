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
// value is an UNQUOTED plain scalar containing a colon followed by a space or a
// TAB (both end a mapping key in YAML, so both are the same defect).
//
// That construct is the single most common OKF authoring defect in the wild
// (issue #93): in `title: Multi-View: Tabs and Windows` a YAML parser reads the
// second colon as a nested mapping indicator, so the value is not the string the
// author meant and the frontmatter block stops parsing. The defect is NOT
// key-specific — the measured wild instance was on `title:`, not `description:`
// — so every key is scanned. Quoting the value is the fix.
//
// PRECONDITION — call this ONLY for a file whose frontmatter the codec FAILED to
// parse (convert.Analyze's `recovered`). It is not a general-purpose scanner and
// it is not safe on a clean file: on its own it can name a phantom key, and the
// single caller's `if recovered` is what makes that unreachable.
//
// That gate is the whole of the never-gates guarantee, and deliberately lives in
// the WIRING rather than in an argument about YAML. `recovered` is the same flag
// lint derives the invalid-frontmatter SchemaViolation from, so an advisory
// cannot outlive the violation that subsumes it — a data dependency, not two
// parsers agreeing. Two earlier revisions tried to establish that here instead,
// and both were wrong in the same direction:
//
//   - The first scanned line by line with no whole-block check, so a multi-line
//     quoted scalar (`description: "line one` / `  note: a: b"`) — valid YAML —
//     was reported against the phantom key `note`.
//   - The second re-parsed the block with yaml.Unmarshal into an `any`. The codec
//     unmarshals into a yaml.Node, and those are NOT the same acceptance
//     predicate: a value decode also rejects duplicate keys, unresolvable tags
//     and recursive anchors, none of which stop a node decode. Its door therefore
//     opened on blocks convert parses happily, and the line scan below could not
//     carry that alone — a sequence-item (`- |`), anchored (`key: &a |`) or
//     tagged (`key: !!str |`) block scalar leaves blockIndent at -1 and its
//     literal interior gets read as mapping entries.
//
// Both were the same class of bug — a phantom key on a cleanly-parsing file —
// reached through a different door. Gating at the caller closes every such door
// at once, including ones not yet found, which is why it is preferred over
// hardening the scan further. See TestColonSpaceNeverDerivedForCleanFile.
//
// What remains here is a single stage: WHICH key is it? The block is scanned line
// by line with the kvLine pre-filter, each candidate confirmed by a real parse of
// the re-formed entry (#93 calls for a parser, not a regex alone; the regex is
// only ever a pre-filter). The caller already knows the file is broken; this
// names the key to quote. Its skips keep the classic false positives clean even
// on a file that is broken for some OTHER reason:
//   - `url: https://example.com` and `standup: 12:30` — the trigger is a colon
//     followed by a space or a tab, and neither of these has one.
//   - `title: "Multi-View: Tabs"` — quoted, so it is not a plain scalar (and it
//     is the very form the advisory asks for).
//   - the continuation lines of a MULTI-LINE quoted scalar, which are string
//     content rather than mapping entries. Without this, a value written as
//     `description: "line one` / `  note: a: b"` — perfectly valid YAML — was
//     reported against the phantom key `note`.
//   - the interior of a `|` / `>` block scalar, where `Note: like this` is
//     ordinary text rather than a mapping — skipped by indentation.
//   - a flow collection (`[...]` / `{...}`), which is not a plain scalar.
//   - a QUOTED key, which kvLine cannot split reliably (it stops the key at the
//     first colon, so `"a: b: c"` in a flow sequence would otherwise be read as
//     the key `"a` with the value `b: c",`). Deliberately a miss rather than a
//     false positive: for an advisory, the wrong finding costs more than the
//     absent one.
//
// norm is NormalizeInput's output (BOM-stripped, lone CRs translated), the same
// bytes the codec parses. A file with no opening "---" fence yields nothing, and
// so does one whose fence is never closed — see frontmatterRegion below for why
// that case is deliberately left alone rather than scanned to EOF.
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
	// openQuote is the quote character of a multi-line quoted scalar still
	// looking for its terminator, or 0 when not inside one. Every line until that
	// terminator is scalar CONTENT, however much it may look like a mapping.
	var openQuote byte

	for _, raw := range strings.Split(fmText, "\n") {
		line := strings.TrimRight(raw, " \t")
		if openQuote != 0 {
			if closesQuote(line, openQuote) {
				openQuote = 0
			}
			continue
		}
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
		if key[0] == '"' || key[0] == '\'' {
			continue // quoted key: kvLine cannot split it reliably, so do not guess
		}
		switch value[0] {
		case '|', '>':
			blockIndent = leadingIndent(line) // interior is text, not mapping entries
			continue
		case '"', '\'':
			// Quoted: the correct form, and what the advisory asks for. A quoted
			// scalar may also SPAN LINES, so note an unterminated one and skip its
			// continuation lines rather than reading them as mapping entries.
			if !closesQuote(value[1:], value[0]) {
				openQuote = value[0]
			}
			continue
		case '{':
			// A single-line flow MAPPING is scanned for the same defect one level in
			// (a MULTI-line one needs no help here: its entries sit on their own lines
			// and the ordinary line scan above already sees them):
			// issue #93's fourth measured instance is exactly this shape
			// (`meta: {name: Multi-View: Tabs, x: 1}`), so treating every flow
			// collection as a false-positive class would miss a case the issue cites
			// as real. The entries are confirmed by the same parser probe as plain
			// scalars, so nothing is named on a guess.
			keys = append(keys, flowMappingKeys(value)...)
			continue
		case '[':
			continue // flow sequence: no key to name, so nothing to advise
		case '&', '*', '!':
			continue // anchor / alias / tag, not a plain scalar
		}
		// A plain scalar cannot contain " #": that begins a trailing comment. This
		// runs AFTER the quote check, so a quoted value carrying a '#' keeps the
		// closing quote the scan above needs to see.
		if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimRight(value[:i], " \t")
			if value == "" {
				continue
			}
		}
		if !containsColonBreak(value) {
			continue
		}
		// Stage-2 parser confirmation. The entry is re-formed at column 0 so nesting
		// depth cannot colour the result: what is being asked is only whether THIS
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
// document does not open a fence, and ALSO when it never closes one.
//
// Requiring the closing fence is deliberate. A file whose first line is "---"
// with no terminator is indistinguishable from ordinary markdown opening on a
// thematic break, and treating the remainder as frontmatter meant scanning BODY
// PROSE: a line like "Note: prose: with a colon-space." was reported as the key
// "Note". The cost of that miss is low — such a file is still reported as
// "invalid frontmatter: unterminated '---' block", so nothing goes unreported,
// and only the key name is lost — while the cost of the false positive is a
// finding against a word that is not a key at all. That is the same trade the
// quoted-key skip above makes: for an advisory, the wrong finding costs more
// than the absent one.
//
// The fence rules here intentionally mirror native.splitFrontmatter, which is
// unexported in another package; TestFrontmatterRegionAgreesWithCodec pins the
// two together so they cannot drift apart unnoticed.
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
	return "", false
}

// closesQuote reports whether a quoted scalar ALREADY OPEN with quote character
// q (a '"' or '\”) finds its terminator somewhere in s. YAML's two escape
// conventions are honoured: inside a double-quoted scalar a backslash escapes
// the next byte, and inside a single-quoted one ” is a literal quote rather
// than the terminator.
func closesQuote(s string, q byte) bool {
	for i := 0; i < len(s); i++ {
		switch {
		case q == '"' && s[i] == '\\':
			i++ // the escaped byte cannot terminate the scalar
		case s[i] != q:
			// ordinary content
		case q == '\'' && i+1 < len(s) && s[i+1] == '\'':
			i++ // '' is one literal quote
		default:
			return true
		}
	}
	return false
}

// flowMappingKeys names the keys of a SINGLE-LINE flow mapping ("{a: b, c: d}")
// whose value is a plain scalar carrying a colon break — issue #93's fourth
// measured instance, `meta: {name: Multi-View: Tabs, x: 1}`, which YAML rejects
// with "did not find expected ',' or '}'".
//
// Scope is deliberately one line and one level: a flow mapping that spans lines,
// or nests another collection, yields nothing rather than a guess. Every
// candidate is confirmed by the same yaml.v3 probe the plain-scalar path uses.
func flowMappingKeys(value string) []string {
	inner, ok := flowMappingInner(value)
	if !ok {
		return nil
	}
	var keys []string
	for _, entry := range splitFlowEntries(inner) {
		entry = strings.TrimSpace(entry)
		i := strings.Index(entry, ":")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(entry[:i])
		val := strings.TrimSpace(entry[i+1:])
		if key == "" || val == "" {
			continue
		}
		if key[0] == '"' || key[0] == '\'' {
			continue // quoted key: cannot be split reliably, so do not guess
		}
		switch val[0] {
		case '"', '\'', '{', '[', '&', '*', '!':
			continue // quoted, nested, or tagged — not a plain scalar
		}
		if !containsColonBreak(val) {
			continue
		}
		var probe any
		if yaml.Unmarshal([]byte(key+": "+val), &probe) == nil {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// flowMappingInner returns the text between the braces of a flow mapping that
// OPENS AND CLOSES on this line. ok is false for anything else — an unbalanced or
// multi-line flow is left alone rather than guessed at.
func flowMappingInner(value string) (string, bool) {
	if value == "" || value[0] != '{' {
		return "", false
	}
	depth := 0
	var quote byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return value[1:i], true
			}
		}
	}
	return "", false
}

// splitFlowEntries splits a flow mapping's interior on the commas that separate
// its entries, ignoring commas nested inside a quoted scalar or an inner
// collection.
func splitFlowEntries(inner string) []string {
	var entries []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				entries = append(entries, inner[start:i])
				start = i + 1
			}
		}
	}
	return append(entries, inner[start:])
}

// containsColonBreak reports whether s contains a colon followed by a space OR A
// TAB. That pair is the detector's ENTIRE TRIGGER — `desc: value:<TAB>more` is
// treated as the same defect as `desc: value: more`. Checking only for ": "
// missed the tab form entirely.
//
// This trigger is DELIBERATELY NARROWER THAN YAML'S OWN RULE. Other spellings end
// a key too: a colon at the end of the line, and, in flow context, one directly
// after a quoted key — `m: {"a":b}` parses as a nested mapping with no space, tab
// or line break anywhere near the colon.
//
// DO NOT restate that narrowing here as a rule about when a colon is inert. This
// function is reached from the FLOW path, which is where every such rule breaks.
// Three were written during review and executed against yaml.v3; all three were
// false, the last falsified by `m: {a:{b: 1}}`. Describe the trigger; claim
// nothing about what YAML requires.
//
// This advisory runs ONLY on a file whose frontmatter FAILED to parse (the
// recovered gate documented above), and that parse failure is itself reported as
// an invalid-frontmatter schema violation — one data dependency, not a claim
// about which spellings exist. It promises NOTHING about a file that parses
// cleanly. The advisory's job is to name the key in the form the issue measured
// in the wild, and widening a rule whose whole review history is about over-reach
// is the wrong instinct. An ACCEPTED FALSE NEGATIVE, recorded rather than
// overlooked.
func containsColonBreak(s string) bool {
	return strings.Contains(s, ": ") || strings.Contains(s, ":\t")
}

// leadingIndent is the number of leading space/tab bytes of line.
func leadingIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}
