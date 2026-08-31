#!/usr/bin/env bash
# Fixture harness for scripts/check-transcript-versions.py (issue #169).
#
# The version-literal gate's own regression lock. Round-2 review found the gate
# could pass VACUOUSLY — break discovery (empty dir, wrong docroot, renamed fence
# tag) and it exited 0 even with genuine drift present, the exact
# silent-permissive failure #169 exists to remove. These cases assemble throwaway
# copies of plugins/ and assert the gate's exit code, with the broken-discovery
# cases locking in the minimum-coverage assertion so it cannot regress.
#
# Each case runs against a STAMPED binder (the checker refuses an unstamped
# build), so the harness builds one first, exactly as check-transcript-versions.sh
# does in CI.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CHECKER="$SCRIPT_DIR/check-transcript-versions.py"
cd "$REPO_ROOT"

SKILL_REL="okf-convert/skills/okf-convert/SKILL.md"
CONTRACT_REL="okf-convert/skills/okf-convert/references/binder-json-contract.md"

STAMP_VERSION="${BINDER_STAMP_VERSION:-$(git describe --tags --abbrev=0)}"
BIN="$(mktemp -d)/binder"
echo "==> building stamped binder (cmd.Version=${STAMP_VERSION})"
go build -ldflags "-X github.com/ghchinoy/binder/cmd.Version=${STAMP_VERSION}" -o "$BIN" .
echo "==> stamped binder --version: $("$BIN" --version)"
echo

PASS=0
FAIL=0

# assert_exit <label> <docroot> <expected-exit> [want-substring]
assert_exit() {
  local label="$1" docroot="$2" expected="$3" want="${4:-}"
  local out actual
  out="$(python3 "$CHECKER" "$docroot" "$BIN" 2>&1)"
  actual=$?
  if [ "$actual" -eq "$expected" ] && { [ -z "$want" ] || echo "$out" | grep -qF "$want"; }; then
    echo "  PASS  $label (exit $actual)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $label: expected exit $expected${want:+ + substring '$want'}, got exit $actual"
    echo "$out" | sed 's/^/        | /'
    FAIL=$((FAIL + 1))
  fi
}

# fresh_copy — copy plugins/ into a new throwaway dir and echo its path.
fresh_copy() {
  local tmp
  tmp="$(mktemp -d)"
  cp -r plugins "$tmp/plugins"
  echo "$tmp/plugins"
}

echo "==> check-transcript-versions fixture harness (issue #169)"

# [1] clean copy -> GREEN
CLEAN="$(fresh_copy)"
assert_exit "clean copy -> exit 0" "$CLEAN" 0 "0 coverage failure(s)"
rm -rf "$(dirname "$CLEAN")"

# [2] drifted JSON envelope literal -> RED (drift finding)
DRIFT_JSON="$(fresh_copy)"
sed -i '125s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$DRIFT_JSON/$SKILL_REL"
assert_exit "drifted JSON envelope literal -> exit 1" "$DRIFT_JSON" 1 "[JSON-ENVELOPE]"
rm -rf "$(dirname "$DRIFT_JSON")"

# [3] drifted prose provenance sentence -> RED (drift finding)
DRIFT_PROSE="$(fresh_copy)"
sed -i '5s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$DRIFT_PROSE/$CONTRACT_REL"
assert_exit "drifted prose provenance -> exit 1" "$DRIFT_PROSE" 1 "[PROSE-PROVENANCE]"
rm -rf "$(dirname "$DRIFT_PROSE")"

# [4] broken discovery: renamed fence tag hides a genuinely-drifted literal.
#     This is the regression lock for the round-2 Critical — pre-fix the gate
#     exited 0 here; it must now fail via the minimum-coverage assertion.
BROKEN="$(fresh_copy)"
sed -i '125s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$BROKEN/$SKILL_REL"   # plant real drift
sed -i '124s/^```json$/```jsonX/' "$BROKEN/$SKILL_REL"                  # then hide the block
assert_exit "broken discovery (renamed fence) -> exit 1" "$BROKEN" 1 "MISSING-COVERAGE"
rm -rf "$(dirname "$BROKEN")"

# [5] A1: empty docroot -> coverage failure
EMPTY="$(mktemp -d)"
assert_exit "empty docroot (A1) -> exit 1" "$EMPTY" 1 "MISSING-COVERAGE"
rm -rf "$EMPTY"

# [6] A2: non-existent docroot -> coverage failure
assert_exit "non-existent docroot (A2) -> exit 1" "/tmp/check-tv-does-not-exist-$$" 1 "MISSING-COVERAGE"

echo
if [ "$FAIL" -eq 0 ]; then
  echo "OK: $PASS fixture case(s) passed."
  exit 0
else
  echo "FAILED: $FAIL of $((PASS + FAIL)) fixture case(s) did not match."
  exit 1
fi
