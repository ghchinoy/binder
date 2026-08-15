package okf

import "fmt"

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
	ConceptID string
	Severity  Severity
	Message   string
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

// ProjectTrust derives a typed TrustSignals view from frontmatter. It never
// mutates fm and never fails: malformed families project to whatever can be read
// and are reported (advisory) by ValidateTrust, never rejected.
func ProjectTrust(fm *OrderedMap, conceptType string) TrustSignals {
	ts := TrustSignals{
		Status:     asString(mapGet(fm, "status")),
		StaleAfter: asString(mapGet(fm, "stale_after")),
		Attested:   conceptType == AttestedComputationType,
	}
	if g := asStringMap(mapGet(fm, "generated")); g != nil {
		ts.Generated = &Actorstamp{By: asString(g["by"]), At: asString(g["at"])}
	}
	ts.Verified = projectActorstamps(mapGet(fm, "verified"))
	ts.Sources = projectSources(mapGet(fm, "sources"))
	if w := asStringMap(mapGet(fm, "usage_window")); w != nil {
		ts.UsageWindow = &DateRange{From: asString(w["from"]), To: asString(w["to"])}
	}
	return ts
}

// TrustTier derives the trust tier from a concept's verified events (spec §5.3).
func TrustTier(c *Concept) Tier {
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
func IsStale(c *Concept, today string) bool {
	if c.Trust.StaleAfter == "" {
		return false
	}
	return today >= c.Trust.StaleAfter
}

// ValidateTrust returns advisory findings about trust-signal well-formedness. It
// NEVER returns an error and NEVER emits SeverityError: absence of any optional
// family is not a violation (spec §11). Full well-formedness validation is
// Phase 2; Phase 1 keeps a minimal, non-rejecting set.
func ValidateTrust(c *Concept, v SpecVersion) []Finding {
	var out []Finding
	add := func(msg string) {
		out = append(out, Finding{ConceptID: c.ID, Severity: SeverityAdvisory, Message: msg})
	}
	if c.Trust.Generated != nil && c.Trust.Generated.By == "" {
		add("generated is present but generated.by is empty (spec §5.2 requires by)")
	}
	for i, ver := range c.Trust.Verified {
		if ver.By == "" {
			add(fmt.Sprintf("verified[%d] is missing 'by' (spec §5.2)", i))
		}
	}
	if c.Trust.Attested && !c.Frontmatter.Has("runtime") {
		add("Attested Computation is missing required 'runtime' (spec §10.2)")
	}
	return out
}

func hasHumanPrefix(actor string) bool {
	const p = "human:"
	return len(actor) >= len(p) && actor[:len(p)] == p
}

func mapGet(fm *OrderedMap, key string) any {
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
func projectActorstamps(v any) []Actorstamp {
	switch t := v.(type) {
	case map[string]any:
		return []Actorstamp{{By: asString(t["by"]), At: asString(t["at"])}}
	case []any:
		out := make([]Actorstamp, 0, len(t))
		for _, item := range t {
			if m := asStringMap(item); m != nil {
				out = append(out, Actorstamp{By: asString(m["by"]), At: asString(m["at"])})
			}
		}
		return out
	default:
		return nil
	}
}

func projectSources(v any) []Source {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]Source, 0, len(list))
	for _, item := range list {
		m := asStringMap(item)
		if m == nil {
			continue
		}
		out = append(out, Source{
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
