package convert

import (
	"fmt"
	"sort"
	"strings"
)

// StatusVocabulary is the OKF v0.2 §5.4 status enum: the only values `binder
// validate` accepts for a concept's `status`. A --status-map value outside this
// set is what downstream validation flags as an advisory, so #23 surfaces it at
// the point of input instead.
var StatusVocabulary = []string{"draft", "stable", "deprecated"}

// statusVocabList is the human-readable "draft|stable|deprecated" rendering used
// verbatim in messages so they match `binder validate`'s §5.4 phrasing exactly.
const statusVocabList = "draft|stable|deprecated"

// statusAliases is the FIXED, non-extensible canonicalization table the owner
// specified for #23. It is consulted ONLY when canonicalization is explicitly
// opted in; on the default path a user's value is never rewritten. The table is
// deliberately closed in this slice — do not make it user-extensible here.
var statusAliases = map[string]string{
	"active":      "stable",
	"wip":         "draft",
	"in-progress": "draft",
	"archived":    "deprecated",
	"legacy":      "deprecated",
}

// StatusVocabResult carries the outcome of checking (and optionally
// canonicalizing) the --status-map values against the §5.4 vocabulary. Notes are
// deterministically sorted, self-describing lines destined for the run report
// (prose + --json). Warnings holds ONLY the non-conformance lines and drives the
// --strict gate; canonicalization rewrites are informational and never gate.
type StatusVocabResult struct {
	// Notes are all status-vocabulary messages (rewrites + non-conformance),
	// sorted, surfaced additively in the report. Empty on a fully conformant run
	// so output stays byte-identical.
	Notes []string
	// Warnings are the non-conformance messages only, sorted. Their presence is
	// what --strict escalates to a pre-write gate.
	Warnings []string
}

// NonConformant reports whether any --status-map value remains outside the §5.4
// vocabulary after (optional) canonicalization. It is the --strict gate signal.
func (r StatusVocabResult) NonConformant() bool { return len(r.Warnings) > 0 }

// GateMessage renders the --strict gate error naming every offending value.
func (r StatusVocabResult) GateMessage() string {
	return fmt.Sprintf("--status-map has non-conformant status value(s) and --strict is set: %s",
		strings.Join(r.Warnings, "; "))
}

// isConformantStatus reports whether v is one of the §5.4 vocabulary values.
func isConformantStatus(v string) bool {
	for _, s := range StatusVocabulary {
		if v == s {
			return true
		}
	}
	return false
}

// ResolveStatusVocabulary checks the parsed --status-map values (the per-prefix
// map plus the special default= value) against the OKF §5.4 vocabulary. When
// canonicalize is true it first rewrites the fixed alias set (issue #23),
// recording each rewrite; on the DEFAULT path (canonicalize=false) it NEVER
// rewrites — a non-conformant value is reported and passed through unchanged, so
// binder never silently decides the user's intent. It returns the possibly
// canonicalized prefix map and default plus a StatusVocabResult carrying the
// deterministic report notes and the non-conformance set that --strict gates on.
//
// The returned map is a fresh copy when canonicalization changes a value, so the
// caller's parsed map is never mutated in place; when nothing is rewritten the
// original map is returned so the default path allocates nothing new.
func ResolveStatusVocabulary(prefixes map[string]string, def string, canonicalize bool) (map[string]string, string, StatusVocabResult) {
	var res StatusVocabResult

	// Build the ordered key list (prefix keys sorted, then the synthetic
	// "default") so notes are deterministic regardless of Go map iteration order.
	keys := make([]string, 0, len(prefixes))
	for k := range prefixes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	outPrefixes := prefixes
	copied := false // copy-on-write guard: the caller's parsed map is never mutated
	// resolve applies the check/canonicalization to a single value, returning the
	// (possibly rewritten) value. key is the map key it came from ("default" for
	// the default= value) purely for message attribution.
	resolve := func(key, value string) string {
		if isConformantStatus(value) {
			return value
		}
		if canonicalize {
			if canon, ok := statusAliases[value]; ok {
				res.Notes = append(res.Notes, fmt.Sprintf(
					"status value %q (from --status-map key %q) canonicalized to %q (OKF §5.4)",
					value, key, canon))
				return canon
			}
		}
		// Still non-conformant: warn, do NOT rewrite. Point known aliases at the
		// opt-in flag so the user can see the fix without binder guessing intent.
		msg := fmt.Sprintf(
			"status value %q (from --status-map key %q) is not one of %s (OKF §5.4); wrote it unchanged",
			value, key, statusVocabList)
		if !canonicalize {
			if canon, ok := statusAliases[value]; ok {
				msg += fmt.Sprintf(" — pass --canonicalize-status to map it to %q", canon)
			}
		}
		res.Warnings = append(res.Warnings, msg)
		return value
	}

	for _, k := range keys {
		nv := resolve(k, prefixes[k])
		if nv != prefixes[k] {
			// Copy-on-write so the caller's parsed map is never mutated.
			if !copied {
				outPrefixes = copyMap(prefixes)
				copied = true
			}
			outPrefixes[k] = nv
		}
	}

	if def != "" {
		def = resolve(statusDefaultKey, def)
	}

	// Notes = rewrites ∪ warnings, deterministically ordered.
	res.Notes = append(res.Notes, res.Warnings...)
	sort.Strings(res.Notes)
	sort.Strings(res.Warnings)

	return outPrefixes, def, res
}

// copyMap returns a shallow copy of m (values are strings, so shallow is deep).
func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
