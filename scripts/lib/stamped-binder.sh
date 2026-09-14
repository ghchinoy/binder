# shellcheck shell=bash
# Shared stamped-build helper for the version-literal gates (issues #169, #60).
#
# A `go build`/`go test` binary is UNSTAMPED and reports binder/dev, so no
# in-process gate can pin version literals to the real release — see the
# "KNOWN LIMIT" note in internal/plugindocs/drift_test.go. Every gate that needs
# to compare shipped text against the current version therefore has to build a
# binary the way goreleaser does, with the tag injected via ldflags.
#
# This file exists so that build is written ONCE. It was extracted from
# scripts/check-transcript-versions.sh when #60 added a second gate with the same
# precondition: two hand-copied build blocks would be two places to update when
# the ldflags path or the tag-resolution rule changes, and a gate that silently
# builds the wrong thing is worse than no gate.
#
# Source it, then call build_stamped_binder; it echoes the binary's path.

# Resolve this file's directory AT SOURCE TIME, not inside the function.
# ${BASH_SOURCE[0]} names the defining file only while the file is being sourced;
# read from inside a function it can come back EMPTY depending on how the caller
# was invoked, and `dirname ""` is `.`, which silently resolves the certifier
# against the CALLER'S CWD instead of against this script. Capturing it here is
# the only point where the value is guaranteed correct.
_STAMPED_BINDER_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# stamp_version echoes the version to inject: the most recent tag reachable from
# HEAD, which is the same value goreleaser injects at release time (v-prefixed;
# cmd.init strips the leading v via normalizeVersion). BINDER_STAMP_VERSION
# overrides it so a release pipeline can pass the exact tag being built.
#
# `git describe --tags` needs full history — CI checkouts must use fetch-depth: 0.
stamp_version() {
  echo "${BINDER_STAMP_VERSION:-$(git describe --tags --abbrev=0)}"
}

# build_stamped_binder [--no-prerelease] builds binder with the release tag
# injected, CERTIFIES the result, and echoes the path to the binary. Progress
# goes to stderr so the path is the only thing on stdout and the caller can
# capture it directly.
#
# THE THING THAT BUILDS IT IS THE THING THAT CERTIFIES IT. Sharing only the build
# left each caller to decide separately whether to trust the result, and the two
# callers had silently diverged — see the header of scripts/lib/stamped_version.py
# for the divergence and for why the two predicates are decomposed rather than
# merged. In short: "is it stamped at all" is absolute and applied here to every
# caller, while "are prereleases acceptable" is this project's release policy and
# so is a per-caller argument.
#
# --no-prerelease is #169's policy (plugin transcripts are only ever regenerated
# from a full release). #60's shipped-output gate omits it, because goreleaser
# can ship v1.2.3-rc1 and that binary's help text is still worth pinning.
#
# A failed build OR a failed certification returns non-zero and echoes nothing,
# so a caller that is not running under `set -e` (the fixture harness, which must
# survive a failing case to report on the rest) cannot mistake an untrustworthy
# binary for a good one.
build_stamped_binder() {
  local version bin reported certifier
  certifier="$_STAMPED_BINDER_LIB/stamped_version.py"
  # A MISSING CERTIFIER IS NOT A FAILED CERTIFICATION. Both return non-zero, so
  # both fail closed and neither can wave a bad binary through — but they have
  # different causes and different fixes, and reporting "REFUSING to gate against
  # this build" when the truth is "I could not find the checker" sends whoever
  # reads it to debug the binary instead of the path. Distinguish them.
  if [ ! -f "$certifier" ]; then
    echo "ERROR: certifier not found at $certifier" >&2
    echo "       This is a BROKEN GATE, not an untrustworthy build: the check" >&2
    echo "       never ran. Fix the path; do not interpret this as a version" >&2
    echo "       problem with the binary." >&2
    return 1
  fi
  version="$(stamp_version)"
  bin="$(mktemp -d)/binder"
  echo "==> building stamped binder (cmd.Version=${version})" >&2
  if ! go build -ldflags "-X github.com/ghchinoy/binder/cmd.Version=${version}" -o "$bin" . >&2; then
    return 1
  fi
  reported="$("$bin" --version 2>&1)"
  if ! python3 "$certifier" --certify "$reported" "$@" >&2; then
    echo "==> REFUSING to gate against this build. Certification is not optional:" >&2
    echo "    a gate run against an unstamped binary pins literals to whatever" >&2
    echo "    the binary happens to report and still exits 0." >&2
    return 1
  fi
  echo "==> stamped binder --version: ${reported} (certified)" >&2
  echo "$bin"
}
