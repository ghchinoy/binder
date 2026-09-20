package okf

import (
	"sort"
	"strings"
	"time"
)

// This file holds the confirmed-public trust/spec vocabulary that binder's
// service and adapters reference directly (the Phase-2 MUST seam): the
// overwrite-protection key derivation and the actor/date validators. The
// derivation helpers (trust projection, tier, recovery, validate-trust) that
// only binder's own capability packages use live in the internal okfrules
// package so the published okf surface stays narrow (design Decision 2).

// refreshableLifecycleKeys are the trust-family keys that carry NO human
// attestation or provenance lineage and MAY be safely refreshed in place: the
// lifecycle stamps status and stale_after (spec §5.4/§5.5). They are the
// intended targets of `binder enrich --overwrite-keys` (issue #22). Every OTHER
// key in the trust vocabulary (spec.go specRules.TrustFields) is attestation- or
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
	r, _ := rules(DefaultSpecVersion)
	set := map[string]bool{"verified_by": true}
	for _, k := range r.TrustFields {
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

// isoDateLayouts accept the ISO 8601 date shape the spec uses. Validation is a
// shape check only; it never rejects a bundle.
var isoDateLayouts = []string{"2006-01-02"}

// IsValidISODate reports whether s is an absolute YYYY-MM-DD date (spec §5.5/§5.1).
func IsValidISODate(s string) bool {
	return parsesAny(strings.TrimSpace(s), isoDateLayouts)
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
