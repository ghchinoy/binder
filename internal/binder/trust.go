package binder

import "fmt"

// envVerifiedByName and repoLocalConfigName are the two ambient trust sources the
// never-fabricate-trust ruling refuses. They are RESTATED here rather than imported
// from internal/config so the service core stays free of the viper-backed config
// package (design §3.3: L3 carries no process/CWD/env coupling). config.EnvPrefix +
// "_" + config.KeyVerifiedBy and config.LocalConfigName are the source of truth for
// the CLI wiring; these literals must track them (a divergence would only change the
// disclosure text, never the ruling). They previously lived in cmd/verifiedby.go.
const (
	envVerifiedByName   = "BINDER_VERIFIED_BY"
	repoLocalConfigName = ".binder.yaml"
)

// TrustOrigin classifies where a resolved verified_by actor came from, at the
// granularity the never-fabricate-trust routing needs. It mirrors
// config.VerifiedByOrigin (plus an Input origin for the MCP surface) but is
// service-owned so the core does not depend on the config package. The adapter maps
// its own origin onto this enum:
//
//   - CLI: config.OriginFlag→TrustFlag; config.PermitsStampWithoutFlag→TrustConfig;
//     config.OriginRepoConfig→TrustRepoConfig; config.OriginEnv→TrustEnv; else
//     TrustNone. The user-set stamping exception itself is still decided in exactly
//     one place (config.PermitsStampWithoutFlag); this enum only carries the
//     already-decided category.
//   - MCP: a set verified_by is TrustInput (an explicit per-invocation input);
//     unset is TrustNone. MCP never loads config.
type TrustOrigin int

const (
	// TrustNone: no verifier was resolved — nothing to stamp, nothing to disclose.
	TrustNone TrustOrigin = iota
	// TrustFlag: an explicit CLI --verified-by on this invocation. Stamps; may co-sign.
	TrustFlag
	// TrustInput: an explicit MCP tool input. Stamps; may co-sign (like a flag).
	TrustInput
	// TrustConfig: a config default the user set that authorizes stamping without a
	// flag (global home config; the CLI decides this via config.PermitsStampWithoutFlag).
	// Stamps, but never co-signs (Residual A).
	TrustConfig
	// TrustRepoConfig: a repo-local .binder.yaml. Refused (it cannot evidence THIS
	// user's decision — Option A) and disclosed in Note rather than acted on.
	TrustRepoConfig
	// TrustEnv: an inherited BINDER_VERIFIED_BY export. Refused (owner ruling: not a
	// per-invocation decision to attest) and disclosed in Note.
	TrustEnv
)

// TrustDecision is the resolved trust-stamp decision for a stamping verb
// (convert/enrich) under the never-fabricate-trust ruling. It is the single home for
// the disclosure Source vocabulary and the refused-verifier Note text — the two used
// to be split across cmd/verifiedby.go (Source via origin.String(), the Note
// literals) and internal/mcp/convert.go (mcpVerifiedBySource). Both adapters now
// route through ResolveTrust so the wording cannot drift between the CLI and MCP.
type TrustDecision struct {
	// Actor is the verifier to stamp (empty ⇒ write no stamp).
	Actor string
	// Explicit records that Actor came from an EXPLICIT per-invocation act (a
	// --verified-by flag or an MCP tool input), which alone may co-sign another
	// identity (Residual A). A config default is not explicit.
	Explicit bool
	// Source is the disclosure token for a WRITTEN stamp: "flag" | "input" |
	// "config" | "none". A refused env/repo-local verifier is never a write source,
	// so it carries "none" and rides in Note instead.
	Source string
	// Note discloses a resolved-but-unhonored verifier (a refused env or repo-local
	// value), so the decision is observable rather than silently dropped (Residual B).
	// Empty when there is nothing to disclose.
	Note string
}

// ResolveTrust is the ONE definition of the never-fabricate-trust routing (design
// §3.3): given a resolved (already validated) actor and its origin, it returns the
// stamp decision — whether to stamp, whether the actor may co-sign, the Source
// disclosure token, and any refused-verifier Note. It reads no ambient state; the
// adapter classifies the origin and passes it in.
//
// An empty actor is always TrustNone-equivalent: nothing to stamp, source "none".
func ResolveTrust(actor string, origin TrustOrigin) TrustDecision {
	if actor == "" {
		return TrustDecision{Source: "none"}
	}
	switch origin {
	case TrustFlag:
		// Explicit per-invocation act: always stamps, may co-sign.
		return TrustDecision{Actor: actor, Explicit: true, Source: "flag"}
	case TrustInput:
		// MCP resolves verified_by from tool input ONLY; a set value is an EXPLICIT
		// per-invocation act, like a --verified-by flag (stamps, may co-sign).
		return TrustDecision{Actor: actor, Explicit: true, Source: "input"}
	case TrustConfig:
		// User-set exception (global home config only): stamps, but never co-signs.
		return TrustDecision{Actor: actor, Explicit: false, Source: "config"}
	case TrustRepoConfig:
		// Option A: a repo-local config does not evidence THIS user's decision, so it
		// does not authorize a stamp. Disclose the ignored value rather than acting on
		// it silently or dropping it.
		return TrustDecision{Source: "none", Note: fmt.Sprintf(
			"ignored repo-local %s verified_by %q: a repo-local config does not "+
				"authorize stamping (pass --verified-by to stamp)",
			repoLocalConfigName, actor)}
	case TrustEnv:
		// Owner ruling: an inherited BINDER_VERIFIED_BY export is not a per-invocation
		// decision to attest, so it does not authorize a stamp. It is disclosed with a
		// note PARALLEL to the repo-local one — env is the MORE surprising refusal (the
		// value is visibly set and worked before this ruling), so silently ignoring it
		// would be a trust-surface regression. Because env outranks repo-local in
		// resolution, this note also covers the both-present case.
		return TrustDecision{Source: "none", Note: fmt.Sprintf(
			"ignored %s %q: an environment default does not authorize stamping "+
				"(pass --verified-by to stamp)", envVerifiedByName, actor)}
	default:
		// TrustNone: nothing to stamp, nothing to disclose.
		return TrustDecision{Source: "none"}
	}
}
