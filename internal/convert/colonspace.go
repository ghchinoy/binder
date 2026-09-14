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
// Detection is in TWO stages, both anchored on a real yaml.v3 parse (#93 calls
// for a parser, not a regex alone; a regex is only ever a pre-filter here).
//
// Stage 1 — does this frontmatter block have the defect AT ALL? This is the
// caller's frontmatterFailed: the verdict of the codec parse convert ALREADY ran
// on this exact file, passed in rather than recomputed. A block convert parsed
// cleanly yields nothing, full stop. That is not a heuristic: YAML forbids ": "
// inside a plain scalar outright, so a genuine instance ALWAYS breaks the parse
// ("mapping values are not allowed in this context"). It is also what #93 asks
// for in so many words — a plain scalar containing ": " "that would fail a real
// YAML parse".
//
// Taking the verdict instead of re-deriving it is deliberate, and it is the
// whole of the guarantee. An earlier revision re-parsed the block here with
// yaml.Unmarshal into an `any`; the codec unmarshals into a yaml.Node, and the
// two are DIFFERENT acceptance predicates — decoding into `any` additionally
// rejects duplicate keys, unresolvable tags and recursive anchors, none of which
// stop a Node parse. A lookalike parse can therefore call a file broken that
// convert accepted, and open stage 2 on a document with no violation to subsume a
// finding. That was a LATENT hazard rather than a live bug — stage 2 is
// conservative enough to have absorbed it, since a line it would flag is itself
// invalid YAML and so breaks the codec too — but the guarantee should not rest on
// stage 2 staying conservative forever. There is now no second parser to
// disagree with the first. See TestColonSpaceStage1IsTheCallersVerdictAlone and
// TestCodecAndPlainParseDisagree.
//
// The consequence worth stating is that this advisory can only ever fire on a
// file convert recovered — and lint derives the invalid-frontmatter schema
// violation from that SAME SourceFacts.Recovered flag, so the advisory is
// subsumed by a counted violation as a matter of data dependency rather than of
// two parsers agreeing. See the never-gates note on lint.Report.NumFindings, and
// TestColonSpaceAlwaysAccompaniedByViolation, which holds that property shut.
//
// Stage 2 — WHICH key is it? Only now is the block scanned line by line with the
// kvLine pre-filter, each candidate confirmed by a second parse of the re-formed
// entry. Stage 2 exists purely for precision: stage 1 already knows the file is
// broken, stage 2 names the key to quote. Its skips keep the classic false
// positives clean even on a file that is broken for some OTHER reason:
//   - `url: https://example.com` and `standup: 12:30` — a colon NOT followed by
//     a space; the pre-filter's value never contains ": " at all.
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
// bytes the codec parses, and frontmatterFailed is that codec's verdict on them.
// A file with no opening "---" fence yields nothing; an unterminated fence is
// still scanned, because the trap is just as present in a block the author never
// closed and convert recovers that file rather than rejecting it.
//
// Like every other SourceFacts field this is purely descriptive: it never
// rejects, gates, or mutates anything (never-reject, spec §11).
func colonSpaceKeys(norm []byte, frontmatterFailed bool) []string {
	// Stage 1. A frontmatter block convert parsed is a block with no colon-space
	// trap in it, because YAML cannot parse one. Returning here is what makes
	// every clean-parsing document — including the multi-line quoted scalars and
	// quoted flow entries that once tripped the line scanner — structurally
	// incapable of producing a finding.
	if !frontmatterFailed {
		return nil
	}
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
		case '[', '{':
			continue // flow collection, not a plain scalar (out of scope for #93)
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
		if !strings.Contains(value, ": ") {
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

// leadingIndent is the number of leading space/tab bytes of line.
func leadingIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}
