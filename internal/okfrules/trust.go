// Package okfrules holds binder's trust/spec derivation logic and markdown
// helpers over the okf domain model. These are the DEFER/KEEP-INTERNAL
// identifiers from the Phase-2 export trace (packaging/phase2/okf-export-trace.md):
// used across binder's capability packages but deliberately NOT part of the
// narrow published okf surface (design Decision 2 / OQ2). Keeping them here lets
// convert/graph/lint/review/enrich/validate and the native codec share one
// implementation without widening the public okf vocabulary beyond its confirmed
// MUST list.
package okfrules

import (
	"fmt"
	"strings"
	"time"

	"github.com/ghchinoy/binder/pkg/okf"
)

// Trust logic is binder-owned (factile has none). These functions are pure
// projections/derivations over Frontmatter and are identical regardless of which
// Codec parsed the concept (design-v2 §2.3/§3).

// Tier is a derived trust level (spec §5.3). It is never stored, only computed.
type Tier string

const (
	TierUnverified       Tier = "unverified"
	TierMachineConfirmed Tier = "machine-confirmed"
	TierHumanReviewed    Tier = "human-reviewed"
)

// Severity classifies a validation Finding.
type Severity string

const (
	// SeverityError marks a hard conformance violation (spec §11 items 1-2).
	SeverityError Severity = "error"
	// SeverityAdvisory marks trust/lifecycle well-formedness guidance that MUST
	// NOT reject a bundle (spec §11).
	SeverityAdvisory Severity = "advisory"
)

// Finding is one validation result.
type Finding struct {
	ConceptID string   `json:"concept_id"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"`
}

func (f Finding) String() string {
	id := f.ConceptID
	if id == "" {
		id = "<bundle>"
	}
	return fmt.Sprintf("[%s] %s: %s", f.Severity, id, f.Message)
}

// AttestedComputationType is the concept type carrying a sanctioned computation (spec §10).
const AttestedComputationType = "Attested Computation"

// IsProtectedTrustKey reports whether key is a trust/attestation-carrying key
// that --overwrite-keys must refuse (see okf.ProtectedTrustKeys).
func IsProtectedTrustKey(key string) bool {
	for _, k := range okf.ProtectedTrustKeys() {
		if k == key {
			return true
		}
	}
	return false
}

// ProjectTrust derives a typed TrustSignals view from frontmatter. It never
// mutates fm and never fails: malformed families project to whatever can be read
// and are reported (advisory) by ValidateTrust, never rejected.
func ProjectTrust(fm *okf.OrderedMap, conceptType string) okf.TrustSignals {
	ts := okf.TrustSignals{
		Status:     asString(mapGet(fm, "status")),
		StaleAfter: asString(mapGet(fm, "stale_after")),
		Attested:   conceptType == AttestedComputationType,
	}
	if g := asStringMap(mapGet(fm, "generated")); g != nil {
		ts.Generated = &okf.Actorstamp{By: asString(g["by"]), At: asString(g["at"])}
	}
	ts.Verified = projectActorstamps(mapGet(fm, "verified"))
	ts.Sources = projectSources(mapGet(fm, "sources"))
	if w := asStringMap(mapGet(fm, "usage_window")); w != nil {
		ts.UsageWindow = &okf.DateRange{From: asString(w["from"]), To: asString(w["to"])}
	}
	return ts
}

// TrustTier derives the trust tier from a concept's verified events (spec §5.3).
func TrustTier(c *okf.Concept) Tier {
	if len(c.Trust.Verified) == 0 {
		return TierUnverified
	}
	for _, v := range c.Trust.Verified {
		if hasHumanPrefix(v.By) {
			return TierHumanReviewed
		}
	}
	return TierMachineConfirmed
}

// IsStale reports whether the concept is stale as of today (YYYY-MM-DD), i.e.
// today >= stale_after (spec §5.5). A concept without stale_after is never stale.
func IsStale(c *okf.Concept, today string) bool {
	if c.Trust.StaleAfter == "" {
		return false
	}
	return today >= c.Trust.StaleAfter
}

// ValidateTrust returns advisory findings about trust-signal well-formedness. It
// NEVER returns an error and NEVER emits SeverityError: absence of any optional
// family is not a violation (spec §11). Every check here is a fidelity/shape
// advisory over already-present values; a missing family is silent.
func ValidateTrust(c *okf.Concept, v okf.SpecVersion) []Finding {
	var out []Finding
	add := func(msg string) {
		out = append(out, Finding{ConceptID: c.ID, Severity: SeverityAdvisory, Message: msg})
	}

	// generated: by REQUIRED (§5.2); by is an actor (§7); at is an ISO datetime.
	if c.Trust.Generated != nil {
		if c.Trust.Generated.By == "" {
			add("generated is present but generated.by is empty (spec §5.2 requires by)")
		} else if !okf.IsValidActor(c.Trust.Generated.By) {
			add(fmt.Sprintf("generated.by %q does not follow the actor convention (spec §7)", c.Trust.Generated.By))
		}
		if c.Trust.Generated.At != "" && !isValidISODateTime(c.Trust.Generated.At) {
			add(fmt.Sprintf("generated.at %q is not an ISO 8601 datetime (spec §5.2)", c.Trust.Generated.At))
		}
	}

	// verified[]: by REQUIRED and an actor; at an ISO datetime (§5.2/§5.3/§7).
	for i, ver := range c.Trust.Verified {
		switch {
		case ver.By == "":
			add(fmt.Sprintf("verified[%d] is missing 'by' (spec §5.2)", i))
		case !okf.IsValidActor(ver.By):
			add(fmt.Sprintf("verified[%d].by %q does not follow the actor convention (spec §7)", i, ver.By))
		}
		if ver.At != "" && !isValidISODateTime(ver.At) {
			add(fmt.Sprintf("verified[%d].at %q is not an ISO 8601 datetime (spec §5.2)", i, ver.At))
		}
	}

	// status: enum draft|stable|deprecated, absent ⇒ stable (§5.4).
	if s := c.Trust.Status; s != "" && s != "draft" && s != "stable" && s != "deprecated" {
		add(fmt.Sprintf("status %q is not one of draft|stable|deprecated (spec §5.4)", s))
	}

	// stale_after: absolute date YYYY-MM-DD (§5.5).
	if sa := c.Trust.StaleAfter; sa != "" && !okf.IsValidISODate(sa) {
		add(fmt.Sprintf("stale_after %q is not an absolute YYYY-MM-DD date (spec §5.5)", sa))
	}

	// sources[]: resource REQUIRED within an entry (§5.1); author is an actor;
	// last_modified is a date.
	for i, s := range c.Trust.Sources {
		if s.Resource == "" {
			add(fmt.Sprintf("sources[%d] is missing required 'resource' (spec §5.1)", i))
		}
		if s.Author != "" && !okf.IsValidActor(s.Author) {
			add(fmt.Sprintf("sources[%d].author %q does not follow the actor convention (spec §7)", i, s.Author))
		}
		if s.LastModified != "" && !okf.IsValidISODate(s.LastModified) {
			add(fmt.Sprintf("sources[%d].last_modified %q is not an absolute YYYY-MM-DD date (spec §5.1)", i, s.LastModified))
		}
	}

	// usage_window: a { from, to } date range (§5.1).
	if w := c.Trust.UsageWindow; w != nil {
		if w.From != "" && !okf.IsValidISODate(w.From) {
			add(fmt.Sprintf("usage_window.from %q is not an absolute YYYY-MM-DD date (spec §5.1)", w.From))
		}
		if w.To != "" && !okf.IsValidISODate(w.To) {
			add(fmt.Sprintf("usage_window.to %q is not an absolute YYYY-MM-DD date (spec §5.1)", w.To))
		}
	}

	// Attested Computation: runtime REQUIRED for this type (§10.2).
	if c.Trust.Attested && !c.Frontmatter.Has("runtime") {
		add("Attested Computation is missing required 'runtime' (spec §10.2)")
	}

	return out
}

// isoDateTimeLayouts accept the ISO 8601 datetime shapes the spec uses.
// Validation is a shape check only; it never rejects a bundle. (The date-only
// counterpart okf.IsValidISODate stays on the public okf surface.)
var isoDateTimeLayouts = []string{
	time.RFC3339, time.RFC3339Nano,
	"2006-01-02T15:04:05", "2006-01-02T15:04",
	"2006-01-02", // a date-only content stamp is tolerated
}

// isValidISODateTime reports whether s is an ISO 8601 datetime (spec §5.2).
func isValidISODateTime(s string) bool {
	return parsesAny(strings.TrimSpace(s), isoDateTimeLayouts)
}

func parsesAny(s string, layouts []string) bool {
	if s == "" {
		return false
	}
	for _, l := range layouts {
		if _, err := time.Parse(l, s); err == nil {
			return true
		}
	}
	return false
}

// IsHumanActor reports whether actor carries the "human:" prefix that promotes a
// verified stamp to the human-reviewed tier (spec §5.3). It is the SINGLE
// predicate TrustTier and any downstream projection (e.g. the property-graph
// NodeVerified.is_human column) share, so the frozen tier and the emitted
// attestations can never disagree about what counts as human review.
func IsHumanActor(actor string) bool {
	const p = "human:"
	return len(actor) >= len(p) && actor[:len(p)] == p
}

func hasHumanPrefix(actor string) bool {
	return IsHumanActor(actor)
}

func mapGet(fm *okf.OrderedMap, key string) any {
	if fm == nil {
		return nil
	}
	v, _ := fm.Get(key)
	return v
}

// AsString renders a frontmatter scalar value as a string, the same way trust
// projection reads it. Codecs use it to read simple fields (e.g. type) without
// re-implementing the conversion.
func AsString(v any) string { return asString(v) }

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// asStringMap normalizes a frontmatter mapping value to map[string]any.
func asStringMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// projectActorstamps handles both a list of {by,at} and a bare {by,at} mapping,
// which the spec mandates be treated as a one-element list (spec §5.2).
func projectActorstamps(v any) []okf.Actorstamp {
	switch t := v.(type) {
	case map[string]any:
		return []okf.Actorstamp{{By: asString(t["by"]), At: asString(t["at"])}}
	case []any:
		out := make([]okf.Actorstamp, 0, len(t))
		for _, item := range t {
			if m := asStringMap(item); m != nil {
				out = append(out, okf.Actorstamp{By: asString(m["by"]), At: asString(m["at"])})
			}
		}
		return out
	default:
		return nil
	}
}

func projectSources(v any) []okf.Source {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]okf.Source, 0, len(list))
	for _, item := range list {
		m := asStringMap(item)
		if m == nil {
			continue
		}
		out = append(out, okf.Source{
			ID:           asString(m["id"]),
			Resource:     asString(m["resource"]),
			Title:        asString(m["title"]),
			Author:       asString(m["author"]),
			UsageCount:   asString(m["usage_count"]),
			LastModified: asString(m["last_modified"]),
		})
	}
	return out
}
