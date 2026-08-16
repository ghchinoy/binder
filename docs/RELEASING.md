# Releasing binder

binder is released with two interlocking pieces of automation:

- **[release-please](https://github.com/googleapis/release-please)** owns
  *versioning*: it reads [Conventional Commits](https://www.conventionalcommits.org/)
  on `main`, maintains a "release PR" that bumps the version and updates
  `CHANGELOG.md`, and — when that PR is merged — creates the `vX.Y.Z` git tag and
  a GitHub Release shell.
- **[goreleaser](https://goreleaser.com) (v2)** owns *building & publishing*:
  triggered by the `vX.Y.Z` tag, it builds reproducible cross-platform binaries
  and publishes them to the GitHub Release, the Homebrew tap, and winget.

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
| goreleaser winget publish | `WINGET_GITHUB_TOKEN` | `${{ secrets.RELEASE_TOKEN }}` |
| GitHub Release upload | `GITHUB_TOKEN` | built-in `${{ secrets.GITHUB_TOKEN }}` (not `RELEASE_TOKEN`) |

**Why a PAT and not the default `GITHUB_TOKEN` for release-please?** A tag pushed
using the default `GITHUB_TOKEN` does **not** trigger another workflow. If
release-please used it, the tag would land but `release.yml` (goreleaser) would
never fire. The PAT breaks that loop. (This is the classic release-please +
goreleaser gotcha.)

The token needs write access to:

- **this repo** (`ghchinoy/binder`) — create branches, PRs, tags, releases;
- **`ghchinoy/homebrew-tap`** — commit the updated formula;
- **`ghchinoy/winget-pkgs`** (the fork of `microsoft/winget-pkgs`) — push a
  branch and open the upstream PR.

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
   - updates the Homebrew formula in `ghchinoy/homebrew-tap`;
   - opens a winget PR to `microsoft/winget-pkgs` (id `ghchinoy.binder`).
5. (Optional) After the winget PR is accepted, run the **Verify winget install**
   workflow (`workflow_dispatch`) as a smoke test.

## Versioning posture (pre-1.0)

- SemVer with `v`-prefixed tags. First release is **`v0.1.0`**.
- While `0.x`: `feat:` → **minor**, `fix:` → **patch**, breaking changes →
  **minor** (not major). Configured via `bump-minor-pre-major: true` and
  `bump-patch-for-minor-pre-major: false` in `release-please-config.json`.
  `v1.0.0` is reserved for the first stability commitment (frozen converter
  output + trust-stamp format).
- The OKF spec level binder targets is a **separate axis** — advertise it in the
  README/CHANGELOG, never encode it in binder's SemVer.

## How the version reaches the binary (single-source: the tag)

`cmd/root.go` declares `var Version = "dev"`. goreleaser injects the tag at build
time:

```
-ldflags "-X github.com/ghchinoy/binder/cmd.Version={{ .Version }}"
```

For `go install github.com/ghchinoy/binder@vX.Y.Z` builds (no ldflags), an
`init()` fallback recovers the module version from `debug.ReadBuildInfo()`. This
matters beyond `--version`: the version is stamped into every converted concept's
trust provenance as `generated.by: "binder/<version>"`, so a release binary must
carry the real tag.

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
