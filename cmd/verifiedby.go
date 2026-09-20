package cmd

import (
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/okf"
)

// resolveVerifiedBy resolves the verified_by actor and classifies its origin for the
// service's never-fabricate-trust routing (binder.ResolveTrust owns the Source
// vocabulary and the refused-verifier Note text). It keeps at the CLI edge only what
// is genuinely adapter-specific: reading the config-resolved actor, VALIDATING it
// (invalid ⇒ config.InvalidActorError, a usage error → exit 2) even when it will not
// be honored, and mapping config's finer VerifiedByOrigin onto binder.TrustOrigin.
//
// The user-set stamping exception is still decided in exactly one place —
// config.PermitsStampWithoutFlag — consulted here to collapse a permitting config
// origin onto binder.TrustConfig; the service never re-encodes that ruling. The
// --verified-by flag must already be bound to cfg.
func resolveVerifiedBy(cfg *config.Config) (actor string, origin binder.TrustOrigin, err error) {
	actor = cfg.GetString(config.KeyVerifiedBy)
	if actor != "" && !okf.IsValidActor(actor) {
		return "", binder.TrustNone, config.InvalidActorError(actor)
	}
	return actor, mapTrustOrigin(cfg.VerifiedByOrigin()), nil
}

// mapTrustOrigin projects config's VerifiedByOrigin onto the service's TrustOrigin,
// applying the owner ruling at the one place it lives (config.PermitsStampWithoutFlag)
// so the service receives an already-decided category:
//   - OriginFlag                     → TrustFlag  (explicit; stamps, may co-sign)
//   - PermitsStampWithoutFlag (global) → TrustConfig (stamps, never co-signs)
//   - OriginRepoConfig               → TrustRepoConfig (refused, disclosed)
//   - OriginEnv                      → TrustEnv        (refused, disclosed)
//   - otherwise (OriginNone)         → TrustNone
func mapTrustOrigin(origin config.VerifiedByOrigin) binder.TrustOrigin {
	switch {
	case origin == config.OriginFlag:
		return binder.TrustFlag
	case config.PermitsStampWithoutFlag(origin):
		return binder.TrustConfig
	case origin == config.OriginRepoConfig:
		return binder.TrustRepoConfig
	case origin == config.OriginEnv:
		return binder.TrustEnv
	default:
		return binder.TrustNone
	}
}
