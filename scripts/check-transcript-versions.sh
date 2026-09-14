#!/usr/bin/env bash
# Version-literal drift gate for the documented JSON transcripts (issue #169;
# scan widened from plugins/ to docs/ and README.md by issue #185).
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

# The stamped build lives in scripts/lib/stamped-binder.sh, shared with the
# shipped-output gate (#60) which has the identical precondition. It resolves the
# release tag via `git describe --tags` (overridable with BINDER_STAMP_VERSION)
# and injects it through the same ldflag goreleaser uses.
# shellcheck source=scripts/lib/stamped-binder.sh
source "$REPO_ROOT/scripts/lib/stamped-binder.sh"

# The stamped build and its certification come from the shared helper: the thing
# that BUILDS the binary is the thing that must certify it is stamped at all.
# --no-prerelease keeps #169's stricter policy EXPLICIT here rather than hiding
# it as the helper's default -- #60's gate deliberately permits prereleases, and
# a shared default would silently pick one of the two policies for both.
BIN="$(build_stamped_binder --no-prerelease)"

# The repo root is the BASE; the scanned roots (plugins/, docs/, README.md) are
# the checker's own SCAN_ROOTS list, so they stay next to the coverage inventory
# that must move with them.
# NOT `exec` -- see the note in check-shipped-version-literals.sh. exec would
# discard the EXIT trap that removes the stamped-build temp root.
python3 scripts/check-transcript-versions.py . "$BIN"
