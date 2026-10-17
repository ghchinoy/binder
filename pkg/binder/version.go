package binder

import (
	"runtime/debug"

	"github.com/ghchinoy/binder/internal/version"
)

// Version is the canonical binder version. It is stamped into generated.by
// ("binder/<version>") and printed by `binder --version`. It is single-sourced
// from the git tag: at release time goreleaser injects the tag via
// -ldflags "-X github.com/ghchinoy/binder/pkg/binder.Version=<version>"
// (see .goreleaser.yaml). It MUST stay a var (not a const) so the linker's -X
// can override it — a const cannot be overridden. The literal default "dev" is a
// constant string expression, which is what makes -X effective.
//
// The var lives in the core service package (not in the cmd/ adapter) so that a
// library consumer can learn binder's version without importing the CLI, and so
// the mcp.Serve(version) seam is fed from the same home the CLI reads. This is
// the final published home: pkg/binder.Version (design §3.6, Decision 3.6).
//
// The trust-provenance stamp is load-bearing (design-v2 §2.3): every converted
// concept records generated.by = "binder/<Version>", so a release binary must
// carry the real tag or it corrupts trust metadata forever.
//
// init() resolves the build-info fallback for `go install ...@vX.Y.Z`, applies
// the single NormalizeVersion funnel that strips a leading "v", and publishes
// the canonical value to internal/version so both CLI and Go SDK consumers
// observe the same resolved version without needing to import cmd/.
var Version = "dev"

func init() {
	if Version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if v := bi.Main.Version; v != "" && v != "(devel)" {
				Version = v
			}
		}
	}
	Version = normalizeVersion(Version)
	version.Set(Version)
}

// normalizeVersion canonicalizes a binder version string to the no-leading-v
// form. It strips exactly one leading "v" when the remainder looks like a
// version (i.e. "v" immediately followed by a digit, which is the shape of every
// v-prefixed semver: v0.3.0, v1.2.3-rc1, …) and leaves everything else exactly
// as it is: the "dev" default, "(devel)", the empty string, and any value that
// merely starts with "v" but is not a semver (e.g. "version-x"). It never
// fabricates a version — it only removes a prefix from a value already present.
func normalizeVersion(v string) string {
	if len(v) >= 2 && v[0] == 'v' && v[1] >= '0' && v[1] <= '9' {
		return v[1:]
	}
	return v
}
