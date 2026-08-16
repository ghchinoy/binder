package cmd

import (
	"fmt"

	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/okf"
)

// verifiedByDecision is the resolved trust-stamp decision for a stamping verb
// (convert/enrich) under the owner's never-fabricate-trust ruling: the actor to
// stamp (empty ⇒ write no stamp), whether it came from an EXPLICIT per-invocation
// --verified-by (which alone may co-sign another identity — Residual A), the
// disclosure source token ("flag"|"env"|"config"|"none"), and an optional Note
// disclosing a resolved-but-unhonored verifier.
type verifiedByDecision struct {
	Actor    string
	Explicit bool
	Source   string
	Note     string
}

// resolveVerifiedBy applies the owner's ruling: a `verified` stamp is written
// only for an explicit --verified-by, or when config.PermitsStampWithoutFlag says
// the resolved origin satisfies the user-set exception (global config or env; NOT
// a repo-local .binder.yaml — Option A). The --verified-by flag must already be
// bound to cfg. The resolved actor is validated (invalid ⇒ usage error, exit 2)
// regardless of whether it will be honored, so a malformed value never passes
// silently.
//
// The exception itself is decided in exactly one place — config.PermitsStampWithoutFlag
// — so this function only routes; it never re-encodes the ruling.
func resolveVerifiedBy(cfg *config.Config) (verifiedByDecision, error) {
	actor := cfg.GetString(config.KeyVerifiedBy)
	if actor != "" && !okf.IsValidActor(actor) {
		return verifiedByDecision{}, config.InvalidActorError(actor)
	}
	origin := cfg.VerifiedByOrigin()
	switch {
	case origin == config.OriginFlag:
		// Explicit per-invocation act: always stamps, may co-sign.
		return verifiedByDecision{Actor: actor, Explicit: true, Source: origin.String()}, nil
	case config.PermitsStampWithoutFlag(origin):
		// User-set exception (global config / env): stamps, but never co-signs.
		return verifiedByDecision{Actor: actor, Explicit: false, Source: origin.String()}, nil
	case origin == config.OriginRepoConfig:
		// Option A: a repo-local config does not evidence THIS user's decision, so it
		// does not authorize a stamp. Disclose the ignored value rather than acting
		// on it silently or dropping it.
		return verifiedByDecision{
			Note: fmt.Sprintf("ignored repo-local %s verified_by %q: a repo-local config "+
				"does not authorize stamping (pass --verified-by to stamp)",
				config.LocalConfigName, actor),
		}, nil
	default:
		// OriginNone: no verifier determined → no stamp, nothing to disclose.
		return verifiedByDecision{}, nil
	}
}
