# Releasing binder

binder is released with two interlocking pieces of automation:

- **[release-please](https://github.com/googleapis/release-please)** owns
  *versioning*: it reads [Conventional Commits](https://www.conventionalcommits.org/)
  on `main`, maintains a "release PR" that bumps the version and updates
  `CHANGELOG.md`, and — when that PR is merged — creates the `vX.Y.Z` git tag and
  a GitHub Release shell.
- **[goreleaser](https://goreleaser.com) (v2)** owns *building & publishing*:
  triggered by the `vX.Y.Z` tag, it builds reproducible cross-platform binaries
  and publishes them to the GitHub Release and the Homebrew tap.

The boundary between them is the **git tag**: release-please owns the tag,
the tag is the only trigger for goreleaser. Nobody hand-edits a version string.

## The one secret you must configure: `RELEASE_TOKEN`

Create a single **fine-grained Personal Access Token** and store it as the repo
Actions secret **`RELEASE_TOKEN`**. Every cross-repo/cross-workflow token is wired
to this one name:

| Consumer | Env / input | Value |
|---|---|---|
| `release-please` action | `token:` | `${{ secrets.RELEASE_TOKEN }}` |
| goreleaser Homebrew publish | `HOMEBREW_TAP_GITHUB_TOKEN` | `${{ secrets.RELEASE_TOKEN }}` |
| GitHub Release upload | `GITHUB_TOKEN` | built-in `${{ secrets.GITHUB_TOKEN }}` (not `RELEASE_TOKEN`) |

**Why a PAT and not the default `GITHUB_TOKEN` for release-please?** A tag pushed
using the default `GITHUB_TOKEN` does **not** trigger another workflow. If
release-please used it, the tag would land but `release.yml` (goreleaser) would
never fire. The PAT breaks that loop. (This is the classic release-please +
goreleaser gotcha.)

The token needs write access to:

- **this repo** (`ghchinoy/binder`) — create branches, PRs, tags, releases;
- **`ghchinoy/homebrew-tap`** — commit the updated formula.

## The release flow

1. Merge Conventional-Commit PRs to `main` (`feat:`, `fix:`, `feat!:`/
   `BREAKING CHANGE:`…).
2. `release-please.yml` opens/updates a **release PR** with the next version and
   `CHANGELOG.md`.
3. Merge the release PR. release-please tags `vX.Y.Z` and creates the GitHub
   Release shell.
4. The tag triggers `release.yml` → `goreleaser release --clean`, which:
   - builds `linux/darwin/windows × amd64/arm64` binaries, uploads archives +
     `checksums.txt` to the GitHub Release;
   - updates the Homebrew formula in `ghchinoy/homebrew-tap`.

### What a release publishes

Seven assets, from goreleaser's default name template with the version
**`v`-stripped**. The v0.2.1 release, verified with `gh release view v0.2.1`:

```text
binder_0.2.1_darwin_amd64.tar.gz  binder_0.2.1_linux_amd64.tar.gz   binder_0.2.1_windows_amd64.zip
binder_0.2.1_darwin_arm64.tar.gz  binder_0.2.1_linux_arm64.tar.gz   binder_0.2.1_windows_arm64.zip
checksums.txt
```

So the predictable pattern for an install script is
`binder_<version>_<os>_<arch>.tar.gz`, `.zip` on Windows. Each archive also
carries `LICENSE` and `README.md` (`archives.files` in `.goreleaser.yaml`).

The tap update lands as `Formula/binder.rb` in `ghchinoy/homebrew-tap`, which
means the user-facing install command is:

```sh
brew install ghchinoy/tap/binder
```

That is the one thing worth smoke-testing after a release — `ghchinoy/tap` is
Homebrew's shorthand for the `ghchinoy/homebrew-tap` repo, and the formula's own
`test` block runs `binder --version`.

## Versioning posture (pre-1.0)

- SemVer with `v`-prefixed tags. The series opened at **`v0.1.0`** (seeded by
  `"initial-version": "0.1.0"` in `release-please-config.json`); the current
  released version is always whatever `.release-please-manifest.json` holds —
  never read a version out of this document.
- While `0.x`: `feat:` → **minor**, `fix:` → **patch**, breaking changes →
  **minor** (not major). Configured via `bump-minor-pre-major: true` and
  `bump-patch-for-minor-pre-major: false` in `release-please-config.json`.
  `v1.0.0` is reserved for the first stability commitment (frozen converter
  output + trust-stamp format).
- The OKF spec level binder targets is a **separate axis** — advertise it in the
  README/CHANGELOG, never encode it in binder's SemVer.

## How the version reaches the binary (single-source: the tag)

`cmd/root.go` declares `var Version = "dev"`, and two different sources can fill
it in:

- **goreleaser** injects the tag at build time —
  `-ldflags "-X github.com/ghchinoy/binder/cmd.Version={{ .Version }}"`. Note
  that goreleaser's `.Version` is the tag with its `v` **stripped**.
- **`go install github.com/ghchinoy/binder@vX.Y.Z`** gets no ldflags, so an
  `init()` fallback recovers the module version from `debug.ReadBuildInfo()`.
  That value is `v`-**prefixed**.

Left alone those two disagree about the leading `v`, and the disagreement is not
cosmetic: the version is stamped into every converted concept's trust provenance
as `generated.by: "binder/<version>"`, so two install methods would write two
different trust stamps for one release.

They are therefore reconciled in code. `init()` routes **both** sources through a
single funnel, `normalizeVersion`, which strips exactly one leading `v` when what
follows is a digit and leaves everything else (`dev`, `(devel)`, the empty
string) untouched. It never fabricates a version — it only removes a prefix from
a value that is already there.

**The canonical form therefore has no leading `v`:** `binder/0.3.0`, not
`binder/v0.3.0`. That is what `binder --version` prints, what the `--json`
envelope's `binder` field contains, and what lands in `generated.by` — identical
across the goreleaser, `go install`, and `go build` paths. What normalization
does *not* do is invent a version: a plain `go build` in an untagged clone still
reports the Go module pseudo-version it was built from (e.g.
`binder/0.2.2-0.20260816073646-33fbd445d1c2`), which is why the release path must
inject the tag with `-ldflags`.

To rehearse the stamp locally, build the way the release builds:

```sh
go build -ldflags "-X github.com/ghchinoy/binder/cmd.Version=0.3.0" -o /tmp/binder .
/tmp/binder --version     # binder/0.3.0
```

## Reproducible build invariants

The release build preserves binder's core invariants (design §7):

- **Pinned deps:** modules are fetched from the Go module proxy at build time,
  pinned via `go.mod`/`go.sum` and verified against the `go.sum` hashes. There is
  **no** `go mod tidy`/`download` hook; the compile-sanity before-hook is a plain
  `go build -o /dev/null .`. The build needs network access to the proxy.
- **Reproducible:** `-trimpath`, `-s -w`, `mod_timestamp={{.CommitTimestamp}}`,
  and `release.yml` exports `SOURCE_DATE_EPOCH` = commit time so timestamps agree.

You can rehearse the release build locally without publishing:

```sh
goreleaser check                     # validate .goreleaser.yaml
goreleaser release --snapshot --clean   # build all targets, no publish
./dist/binder_linux_amd64_v1/binder --version
```

## Deferred (Phase 4)

cosign signatures and SBOMs are intentionally **not** configured yet (no
`signs:`/`sboms:` blocks). Checksums (`checksums.txt`) are emitted today.

**winget is temporarily unavailable.** The winget publisher was removed in
[#46](https://github.com/ghchinoy/binder/pull/46) until the cross-repo PAT can
open a PR to `microsoft/winget-pkgs` (the fine-grained token 403'd on the
upstream PR). There is no `winget:` block in `.goreleaser.yaml` today. Tracked in
[#40](https://github.com/ghchinoy/binder/issues/40); Windows users should take
the release `.zip` in the meantime.
