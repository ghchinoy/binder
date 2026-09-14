#!/usr/bin/env bash
# Shipped-output version-literal gate (issue #60).
#
# Builds a STAMPED binder (the real release tag injected via the same ldflag
# goreleaser uses), then runs scripts/check-shipped-version-literals.py against
# it: every `binder/<X.Y.Z>` the binary PRINTS must equal the version the binary
# IS. #60 escaped through two releases because nothing ever made that comparison.
#
# The stamped build is shared with the #169 transcript gate via
# scripts/lib/stamped-binder.sh — the two gates have the identical precondition
# and differ only in the surface they inspect (#169: plugin markdown transcripts;
# #60: the binary's own help and error text).
#
# This is a distinct CI step, not part of `make check`, for the same reason #169
# is: a `go test` build is unstamped, so no in-process gate can pin version
# VALUES. The authoring-time half that `make check` CAN run is
# internal/version's TestNoHardCodedVersionLiteralInShippedGo.
#
# Exits non-zero on drift.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# shellcheck source=scripts/lib/stamped-binder.sh
source "$REPO_ROOT/scripts/lib/stamped-binder.sh"

BIN="$(build_stamped_binder)"

# Arguments are forwarded so the release runbook can call `--fix` (see
# docs/RELEASING.md step 3), which rewrites the documented invalid-actor
# transcripts to this binary's own output instead of merely reporting the drift.
# Pass no arguments for the CI behaviour: report only, change nothing.
exec python3 scripts/check-shipped-version-literals.py "$BIN" "$@"
