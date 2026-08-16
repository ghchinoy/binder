package convert

import (
	"fmt"
	"time"

	"github.com/ghchinoy/binder/internal/okf"
)

// VerifiedResult is the outcome of applying a verified actorstamp to one concept,
// so callers can disclose it (Residual B). At most one of Stamped/Skipped is true;
// Advisory (else "") is the preserve-or-advise carry-forward note. A run where
// nothing applied (empty actor, or a byte-identical re-stamp) is the zero value.
type VerifiedResult struct {
	Stamped       bool   // a fresh {by,at} stamp was appended
	Skipped       bool   // Residual A: declined to co-sign a different identity's attestation
	ExistingActor string // the different identity, when Skipped
	Advisory      string // preserve-or-advise note for a spec-invalid scalar verified value
}

// applyVerifiedBy appends a verified actorstamp {by: opts.VerifiedBy, at:
// opts.Now (RFC3339 UTC)} to a concept's verified list (issue #7). It is
// deduplicated by (by, at) so a re-run with a fixed clock is idempotent /
// byte-identical. With opts.VerifiedBy empty (no flag, and no user-set config/env
// exception) NO stamp is written — binder never auto-stamps (design §3.1,
// never-fabricate). The actor is assumed valid: the CLI validates it (and the
// config default) with okf.IsValidActor before Convert runs (option (a)).
//
// Residual A (never co-sign): when the actor did NOT come from an explicit
// per-invocation act (opts.VerifiedByExplicit is false — i.e. it came from the
// user-set config/env exception) and the concept ALREADY carries a `verified`
// attestation by a DIFFERENT identity, the stamp is SKIPPED rather than appended.
// The user-set default is consent to attest the user's OWN work, not to co-sign
// somebody else's. This is a skip, never a reject: nothing errors, nothing is
// dropped, the concept is otherwise processed normally. An EXPLICIT --verified-by
// may co-sign, so it is exempt from this guard.
//
// It returns a VerifiedResult.Advisory (else "") when the existing verified value
// is a spec-invalid SCALAR (spec §5.2 wants a {by,at} stamp or a list of them). In
// that case binder PRESERVES the authored scalar unchanged and does NOT append —
// it never silently drops or reshapes authored data — and the caller surfaces the
// advisory as a finding (convert: a warning; enrich: an advisory that --strict
// gates on). This is the shared preserve-or-advise carry-forward fix for #5/#7.
func applyVerifiedBy(c *okf.Concept, opts Options) VerifiedResult {
	if opts.VerifiedBy == "" {
		return VerifiedResult{}
	}
	at := opts.Now.UTC().Format(time.RFC3339)

	// Normalize the existing verified value to a list. The spec (§5.2) treats a
	// bare {by,at} mapping as a one-element list, so honor both shapes.
	var list []any
	switch v := mapGetFM(c.Frontmatter, "verified").(type) {
	case nil:
		// Absent (or explicit null): a fresh one-element list is created below.
	case []any:
		list = v
	case map[string]any:
		list = []any{v}
	default:
		// A spec-invalid scalar verified value. Preserve it byte-faithfully and do
		// NOT append (appending would reshape or discard authored data); advise the
		// caller so it is reported as a finding instead of silently dropped.
		return VerifiedResult{Advisory: fmt.Sprintf("verified: value %q is not a {by,at} stamp or list of them (spec §5.2); preserved unchanged, no verified stamp appended", okf.AsString(v))}
	}

	// Residual A: a config/env (non-explicit) actor must not co-sign a concept a
	// DIFFERENT identity has already attested. An explicit --verified-by is exempt.
	if !opts.VerifiedByExplicit {
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				if by := okf.AsString(m["by"]); by != "" && by != opts.VerifiedBy {
					return VerifiedResult{Skipped: true, ExistingActor: by}
				}
			}
		}
	}

	// Dedup by (by, at): skip if an equivalent stamp is already present.
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			if okf.AsString(m["by"]) == opts.VerifiedBy && okf.AsString(m["at"]) == at {
				return VerifiedResult{}
			}
		}
	}

	list = append(list, map[string]any{"by": opts.VerifiedBy, "at": at})
	c.Frontmatter.Set("verified", list)
	return VerifiedResult{Stamped: true}
}

// mapGetFM reads a frontmatter key, returning nil when the map or key is absent.
func mapGetFM(fm *okf.OrderedMap, key string) any {
	if fm == nil {
		return nil
	}
	v, _ := fm.Get(key)
	return v
}
