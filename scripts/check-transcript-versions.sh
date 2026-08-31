#!/usr/bin/env bash
# Version-literal drift gate for plugin JSON transcripts (issue #169).
#
# Builds a STAMPED binder (the real release tag injected via the same ldflag
# goreleaser uses), then runs scripts/check-transcript-versions.py against it.
# The stamped build is what makes this gate possible OUTSIDE the in-process unit
# gate: a `go test` build is unstamped (binder/dev), so the unit gate cannot pin
# version literals — see internal/plugindocs/drift_test.go "KNOWN LIMIT".
#
# This is a distinct CI step, not part of `make check`. Exits non-zero on drift.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# The current release version: the most recent tag reachable from HEAD. This is
# the same value goreleaser injects at release time (v-prefixed; cmd.init strips
# the leading v via normalizeVersion). Overridable via BINDER_STAMP_VERSION so a
# release pipeline can pass the exact tag being built.
STAMP_VERSION="${BINDER_STAMP_VERSION:-$(git describe --tags --abbrev=0)}"

BIN="$(mktemp -d)/binder"
echo "==> building stamped binder (cmd.Version=${STAMP_VERSION})"
go build -ldflags "-X github.com/ghchinoy/binder/cmd.Version=${STAMP_VERSION}" -o "$BIN" .

echo "==> stamped binder --version: $("$BIN" --version)"
exec python3 scripts/check-transcript-versions.py plugins "$BIN"
