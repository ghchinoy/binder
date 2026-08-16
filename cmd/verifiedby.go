package cmd

import (
	"fmt"
	"strings"

	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/okf"
)

// envVerifiedByName is the environment variable that supplies verified_by, derived
// from the config env prefix so the disclosure note never drifts from the name
// viper actually resolves (BINDER_VERIFIED_BY).
var envVerifiedByName = config.EnvPrefix + "_" + strings.ToUpper(config.KeyVerifiedBy)

// verifiedByDecision is the resolved trust-stamp decision for a stamping verb
// (convert/enrich) under the owner's never-fabricate-trust ruling: the actor to
// stamp (empty ⇒ write no stamp), whether it came from an EXPLICIT per-invocation
// --verified-by (which alone may co-sign another identity — Residual A), the
// disclosure source token ("flag" or "config" — a refused env or repo-local value
// is not a source and rides in Note instead), and an optional Note disclosing that
// resolved-but-unhonored verifier.
type verifiedByDecision struct {
	Actor    string
	Explicit bool
	Source   string
	Note     string
}

// resolveVerifiedBy applies the owner's ruling: a `verified` stamp is written
// only for an explicit --verified-by, or when config.PermitsStampWithoutFlag says
// the resolved origin satisfies the user-set exception (global home config ONLY;
// NOT BINDER_VERIFIED_BY and NOT a repo-local .binder.yaml). The --verified-by flag must already be
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
		// User-set exception (global home config only): stamps, but never co-signs.
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
	case origin == config.OriginEnv:
		// Owner ruling: an inherited BINDER_VERIFIED_BY export is not a
		// per-invocation decision to attest, so it does not authorize a stamp. It is
		// disclosed with a note PARALLEL to the repo-local one — env is the MORE
		// surprising refusal (the value is visibly set in the environment and worked
		// before this ruling), so silently ignoring it would be a trust-surface
		// regression. Because env outranks repo-local in resolution, this note also
		// covers the both-present case, ensuring env never SUPPRESSES a disclosure.
		return verifiedByDecision{
			Note: fmt.Sprintf("ignored %s %q: an environment default does not authorize "+
				"stamping (pass --verified-by to stamp)", envVerifiedByName, actor),
		}, nil
	default:
		// OriginNone: no verifier was resolved, so there is nothing to stamp and
		// nothing to disclose.
		return verifiedByDecision{}, nil
	}
}
