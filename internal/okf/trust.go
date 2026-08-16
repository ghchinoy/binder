package okf

import (
	"fmt"
	"sort"
	"strings"
	"time"
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

// refreshableLifecycleKeys are the trust-family keys that carry NO human
// attestation or provenance lineage and MAY be safely refreshed in place: the
// lifecycle stamps status and stale_after (spec §5.4/§5.5). They are the
// intended targets of `binder enrich --overwrite-keys` (issue #22). Every OTHER
// key in the trust vocabulary (spec.go SpecRules.TrustFields) is attestation- or
// provenance-carrying and is protected by ProtectedTrustKeys below.
var refreshableLifecycleKeys = map[string]bool{
	"status":      true,
	"stale_after": true,
}

// ProtectedTrustKeys returns the trust/attestation-carrying frontmatter keys
// that MUST NOT be overwritten by `binder enrich --overwrite-keys` (issue #22).
// Overwriting them could destroy human attestations or provenance lineage and
// would violate the never-fabricate-trust invariant (spec §5).
//
// The list is DERIVED from the authoritative trust vocabulary
// (SpecRules.TrustFields for the default spec version) minus the refreshable
// lifecycle stamps (status, stale_after), plus the "verified_by" alias — the
// config/flag name (config.KeyVerifiedBy) that writes into the `verified`
// attestation list. Deriving it from the spec means a new trust key added to the
// vocabulary is protected automatically. The result is sorted for determinism.
func ProtectedTrustKeys() []string {
	rules, _ := Rules(DefaultSpecVersion)
	set := map[string]bool{"verified_by": true}
	for _, k := range rules.TrustFields {
		if refreshableLifecycleKeys[k] {
			continue
		}
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsProtectedTrustKey reports whether key is a trust/attestation-carrying key
// that --overwrite-keys must refuse (see ProtectedTrustKeys).
func IsProtectedTrustKey(key string) bool {
	for _, k := range ProtectedTrustKeys() {
		if k == key {
			return true
		}
	}
	return false
}

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
// family is not a violation (spec §11). Every check here is a fidelity/shape
// advisory over already-present values; a missing family is silent.
func ValidateTrust(c *Concept, v SpecVersion) []Finding {
	var out []Finding
	add := func(msg string) {
		out = append(out, Finding{ConceptID: c.ID, Severity: SeverityAdvisory, Message: msg})
	}

	// generated: by REQUIRED (§5.2); by is an actor (§7); at is an ISO datetime.
	if c.Trust.Generated != nil {
		if c.Trust.Generated.By == "" {
			add("generated is present but generated.by is empty (spec §5.2 requires by)")
		} else if !IsValidActor(c.Trust.Generated.By) {
			add(fmt.Sprintf("generated.by %q does not follow the actor convention (spec §7)", c.Trust.Generated.By))
		}
		if c.Trust.Generated.At != "" && !IsValidISODateTime(c.Trust.Generated.At) {
			add(fmt.Sprintf("generated.at %q is not an ISO 8601 datetime (spec §5.2)", c.Trust.Generated.At))
		}
	}

	// verified[]: by REQUIRED and an actor; at an ISO datetime (§5.2/§5.3/§7).
	for i, ver := range c.Trust.Verified {
		switch {
		case ver.By == "":
			add(fmt.Sprintf("verified[%d] is missing 'by' (spec §5.2)", i))
		case !IsValidActor(ver.By):
			add(fmt.Sprintf("verified[%d].by %q does not follow the actor convention (spec §7)", i, ver.By))
		}
		if ver.At != "" && !IsValidISODateTime(ver.At) {
			add(fmt.Sprintf("verified[%d].at %q is not an ISO 8601 datetime (spec §5.2)", i, ver.At))
		}
	}

	// status: enum draft|stable|deprecated, absent ⇒ stable (§5.4).
	if s := c.Trust.Status; s != "" && s != "draft" && s != "stable" && s != "deprecated" {
		add(fmt.Sprintf("status %q is not one of draft|stable|deprecated (spec §5.4)", s))
	}

	// stale_after: absolute date YYYY-MM-DD (§5.5).
	if sa := c.Trust.StaleAfter; sa != "" && !IsValidISODate(sa) {
		add(fmt.Sprintf("stale_after %q is not an absolute YYYY-MM-DD date (spec §5.5)", sa))
	}

	// sources[]: resource REQUIRED within an entry (§5.1); author is an actor;
	// last_modified is a date.
	for i, s := range c.Trust.Sources {
		if s.Resource == "" {
			add(fmt.Sprintf("sources[%d] is missing required 'resource' (spec §5.1)", i))
		}
		if s.Author != "" && !IsValidActor(s.Author) {
			add(fmt.Sprintf("sources[%d].author %q does not follow the actor convention (spec §7)", i, s.Author))
		}
		if s.LastModified != "" && !IsValidISODate(s.LastModified) {
			add(fmt.Sprintf("sources[%d].last_modified %q is not an absolute YYYY-MM-DD date (spec §5.1)", i, s.LastModified))
		}
	}

	// usage_window: a { from, to } date range (§5.1).
	if w := c.Trust.UsageWindow; w != nil {
		if w.From != "" && !IsValidISODate(w.From) {
			add(fmt.Sprintf("usage_window.from %q is not an absolute YYYY-MM-DD date (spec §5.1)", w.From))
		}
		if w.To != "" && !IsValidISODate(w.To) {
			add(fmt.Sprintf("usage_window.to %q is not an absolute YYYY-MM-DD date (spec §5.1)", w.To))
		}
	}

	// Attested Computation: runtime REQUIRED for this type (§10.2).
	if c.Trust.Attested && !c.Frontmatter.Has("runtime") {
		add("Attested Computation is missing required 'runtime' (spec §10.2)")
	}

	return out
}

// IsValidActor reports whether actor follows the actor convention (spec §7):
// "<producer>/<version>" for tools/agents, or one of the "human:", "process:",
// "team:" prefixes for people, processes, and teams. Empty is not valid here;
// callers skip empty values before calling.
func IsValidActor(actor string) bool {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return false
	}
	for _, p := range []string{"human:", "process:", "team:"} {
		if strings.HasPrefix(actor, p) {
			return len(actor) > len(p)
		}
	}
	// "<producer>/<version>": a single slash with non-empty sides.
	if i := strings.IndexByte(actor, '/'); i > 0 && i < len(actor)-1 {
		return !strings.ContainsAny(actor, " \t")
	}
	return false
}

// isoDateLayouts and isoDateTimeLayouts accept the ISO 8601 shapes the spec
// uses. Validation is a shape check only; it never rejects a bundle.
var (
	isoDateLayouts     = []string{"2006-01-02"}
	isoDateTimeLayouts = []string{
		time.RFC3339, time.RFC3339Nano,
		"2006-01-02T15:04:05", "2006-01-02T15:04",
		"2006-01-02", // a date-only content stamp is tolerated
	}
)

// IsValidISODate reports whether s is an absolute YYYY-MM-DD date (spec §5.5/§5.1).
func IsValidISODate(s string) bool {
	return parsesAny(strings.TrimSpace(s), isoDateLayouts)
}

// IsValidISODateTime reports whether s is an ISO 8601 datetime (spec §5.2).
func IsValidISODateTime(s string) bool {
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
