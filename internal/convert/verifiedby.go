package convert

import (
	"fmt"
	"time"

	"github.com/ghchinoy/binder/internal/okf"
)

// applyVerifiedBy appends a verified actorstamp {by: opts.VerifiedBy, at:
// opts.Now (RFC3339 UTC)} to a concept's verified list (issue #7). It is
// deduplicated by (by, at) so a re-run with a fixed clock is idempotent /
// byte-identical. With opts.VerifiedBy empty (no flag, no config default) NO
// stamp is written — binder never auto-stamps (design §3.1, never-fabricate).
// The actor is assumed valid: the CLI validates it (and the config default) with
// okf.IsValidActor before Convert runs (option (a)).
//
// It returns an advisory message (else "") when the existing verified value is a
// spec-invalid SCALAR (spec §5.2 wants a {by,at} stamp or a list of them). In
// that case binder PRESERVES the authored scalar unchanged and does NOT append —
// it never silently drops or reshapes authored data — and the caller surfaces the
// advisory as a finding (convert: a warning; enrich: an advisory that --strict
// gates on). This is the shared preserve-or-advise carry-forward fix for #5/#7.
func applyVerifiedBy(c *okf.Concept, opts Options) string {
	if opts.VerifiedBy == "" {
		return ""
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
		return fmt.Sprintf("verified: value %q is not a {by,at} stamp or list of them (spec §5.2); preserved unchanged, no verified stamp appended", okf.AsString(v))
	}

	// Dedup by (by, at): skip if an equivalent stamp is already present.
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			if okf.AsString(m["by"]) == opts.VerifiedBy && okf.AsString(m["at"]) == at {
				return ""
			}
		}
	}

	list = append(list, map[string]any{"by": opts.VerifiedBy, "at": at})
	c.Frontmatter.Set("verified", list)
	return ""
}

// mapGetFM reads a frontmatter key, returning nil when the map or key is absent.
func mapGetFM(fm *okf.OrderedMap, key string) any {
	if fm == nil {
		return nil
	}
	v, _ := fm.Get(key)
	return v
}
