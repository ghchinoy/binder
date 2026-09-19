package binder

// Version is the canonical binder version. It is stamped into generated.by
// ("binder/<version>") and printed by `binder --version`. It is single-sourced
// from the git tag: at release time goreleaser injects the tag via
// -ldflags "-X github.com/ghchinoy/binder/internal/binder.Version=<version>"
// (see .goreleaser.yaml). It MUST stay a var (not a const) so the linker's -X
// can override it — a const cannot be overridden. The literal default "dev" is a
// constant string expression, which is what makes -X effective.
//
// The var lives in the core service package (not in the cmd/ adapter) so that a
// library consumer can learn binder's version without importing the CLI, and so
// the mcp.Serve(version) seam is fed from the same home the CLI reads. It is
// promoted to pkg/binder.Version at the final phase with zero code change — only
// the directory (and the goreleaser/CI -X target string) move (design §3.6,
// Decision 3.6).
//
// The trust-provenance stamp is load-bearing (design-v2 §2.3): every converted
// concept records generated.by = "binder/<Version>", so a release binary must
// carry the real tag or it corrupts trust metadata forever.
//
// Resolution and normalization stay in the cmd/ adapter (cmd/root.go's init):
// the build-info fallback for `go install ...@vX.Y.Z` and the single
// normalizeVersion funnel that strips a leading "v" are inherently about the
// running binary, so they remain adapter concerns that WRITE this canonical var.
// The canonical form carried here after normalization is NO leading "v":
// "binder/<X.Y.Z>".
var Version = "dev"
