#!/usr/bin/env bash
# Fixture harness for scripts/check-transcript-versions.py (issues #169, #185).
#
# The version-literal gate's own regression lock. Round-2 review found the gate
# could pass VACUOUSLY — break discovery (empty dir, wrong docroot, renamed fence
# tag) and it exited 0 even with genuine drift present, the exact
# silent-permissive failure #169 exists to remove. These cases assemble throwaway
# copies of every scan root (plugins/, docs/, README.md) and assert the gate's
# exit code, with the broken-discovery cases locking in the minimum-coverage
# assertion so it cannot regress.
#
# Each case runs against a STAMPED binder (the checker refuses an unstamped
# build), so the harness builds one first, exactly as check-transcript-versions.sh
# does in CI.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CHECKER="$SCRIPT_DIR/check-transcript-versions.py"
cd "$REPO_ROOT"

# Base-relative, matching the checker's coverage keys: the fixtures now assemble
# a throwaway copy of every scan root (plugins/, docs/, README.md — issue #185),
# not of plugins/ alone.
SKILL_REL="plugins/okf-convert/skills/okf-convert/SKILL.md"
CONTRACT_REL="plugins/okf-convert/skills/okf-convert/references/binder-json-contract.md"
GUIDE_REL="docs/user_guide.md"
README_REL="README.md"

STAMP_VERSION="${BINDER_STAMP_VERSION:-$(git describe --tags --abbrev=0)}"
BIN="$(mktemp -d)/binder"
echo "==> building stamped binder (cmd.Version=${STAMP_VERSION})"
go build -ldflags "-X github.com/ghchinoy/binder/cmd.Version=${STAMP_VERSION}" -o "$BIN" .
echo "==> stamped binder --version: $("$BIN" --version)"
echo

PASS=0
FAIL=0

# The number of cases this harness MUST run. Asserted at the end against
# PASS+FAIL, so the harness cannot itself pass vacuously (round-4 review): if a
# case is commented out, reordered away, or conditionally guarded off, PASS+FAIL
# drops below this and the harness fails loud naming the shortfall. A bare
# "ran at least one" floor is weaker — it would not catch losing three of seven —
# exactly the inventory-over-floor reasoning from round 2. Update this when you
# add or remove a case.
EXPECTED_CASES=13

# assert_exit <label> <docroot> <expected-exit> [want-substring] [checker]
# [checker] defaults to the real checker; the allowlist case passes a patched
# copy, since the allowlist lives in the script rather than in the doc tree.
assert_exit() {
  local label="$1" docroot="$2" expected="$3" want="${4:-}" checker="${5:-$CHECKER}"
  local out actual
  out="$(python3 "$checker" "$docroot" "$BIN" 2>&1)"
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

# fresh_copy — copy every scan root into a new throwaway BASE dir and echo it.
fresh_copy() {
  local tmp
  tmp="$(mktemp -d)"
  cp -r plugins "$tmp/plugins"
  cp -r docs "$tmp/docs"
  cp README.md "$tmp/README.md"
  echo "$tmp"
}

echo "==> check-transcript-versions fixture harness (issues #169, #185)"

# [1] clean copy -> GREEN
CLEAN="$(fresh_copy)"
assert_exit "clean copy -> exit 0" "$CLEAN" 0 "0 coverage failure(s)"
rm -rf "$CLEAN"

# LINE-ADDRESS COUPLING (FYI-2, delta review): the mutation cases below sed
# specific line numbers of the REAL committed docs (e.g. the version literal at
# SKILL.md:125, the opening fence at :124) to plant drift or hide a block. Those
# addresses are coupled to the current doc layout, so a doc re-flow that shifts
# them means a sed edits the wrong line — the case's precondition (planted drift /
# hidden fence) is not set up, so the checker's exit code and expected substring
# no longer match and assert_exit FAILS. This is fail-safe: a re-flow breaks the
# harness LOUD rather than silently skipping the case or passing green. If you
# re-flow these docs, update the line numbers here to match.

# [2] drifted JSON envelope literal -> RED (drift finding)
DRIFT_JSON="$(fresh_copy)"
sed -i '125s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$DRIFT_JSON/$SKILL_REL"
assert_exit "drifted JSON envelope literal -> exit 1" "$DRIFT_JSON" 1 "[JSON-ENVELOPE]"
rm -rf "$DRIFT_JSON"

# [3] drifted prose provenance sentence -> RED (drift finding)
DRIFT_PROSE="$(fresh_copy)"
sed -i '5s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$DRIFT_PROSE/$CONTRACT_REL"
assert_exit "drifted prose provenance -> exit 1" "$DRIFT_PROSE" 1 "[PROSE-PROVENANCE]"
rm -rf "$DRIFT_PROSE"

# [4] broken discovery: renamed fence tag hides a genuinely-drifted literal.
#     This is the regression lock for the round-2 Critical — pre-fix the gate
#     exited 0 here; it must now fail via the minimum-coverage assertion.
BROKEN="$(fresh_copy)"
sed -i '125s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$BROKEN/$SKILL_REL"   # plant real drift
sed -i '124s/^```json$/```jsonX/' "$BROKEN/$SKILL_REL"                  # then hide the block
assert_exit "broken discovery (renamed fence) -> exit 1" "$BROKEN" 1 "MISSING-COVERAGE"
rm -rf "$BROKEN"

# [4b] basename collision: a same-basename decoy in a different directory must
#      NOT satisfy the real location's coverage entry (Optional-1, round-3). Break
#      discovery of the real convert report envelope, then plant a decoy SKILL.md
#      elsewhere carrying a valid report envelope. Under basename keying the decoy
#      would vacuously satisfy coverage (exit 0); under path keying it must not.
COLLISION="$(fresh_copy)"
COL_SKILL="$COLLISION/$SKILL_REL"
COL_REFDIR="$COLLISION/plugins/okf-convert/skills/okf-convert/references"
sed -i '124s/^```json$/```jsonX/' "$COL_SKILL"        # hide the real report envelope
cat > "$COL_REFDIR/SKILL.md" <<'DECOY'
# decoy SKILL.md (different directory, same basename)

```json
{ "binder": "binder/0.5.2", "command": "convert",
  "schema": "binder.report/v1", "result": { } }
```
DECOY
assert_exit "basename collision (decoy must not cover real) -> exit 1" \
  "$COLLISION" 1 "MISSING-COVERAGE: plugins/okf-convert/skills/okf-convert/SKILL.md"
rm -rf "$COLLISION"

# [5] A1: empty base -> coverage failure
EMPTY="$(mktemp -d)"
assert_exit "empty base (A1) -> exit 1" "$EMPTY" 1 "MISSING-COVERAGE"
rm -rf "$EMPTY"

# [6] A2: non-existent base -> coverage failure
assert_exit "non-existent base (A2) -> exit 1" "/tmp/check-tv-does-not-exist-$$" 1 "MISSING-COVERAGE"

# --- #185 cases: the docs/ and README.md scan roots ------------------------
# Before #185 the checker's docroot was `plugins` alone, so every case below
# exited 0 on a tree carrying genuine drift. They are the regression lock for
# that coverage gap.

# [7] drifted docs/user_guide.md envelope literal -> RED. Line 1451 is the
#     `binder config --json` transcript's version literal.
DRIFT_GUIDE="$(fresh_copy)"
sed -i '1451s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$DRIFT_GUIDE/$GUIDE_REL"
assert_exit "drifted user-guide envelope literal -> exit 1" "$DRIFT_GUIDE" 1 \
  "docs/user_guide.md:1451"
rm -rf "$DRIFT_GUIDE"

# [8] drifted README.md envelope literal -> RED. Line 287 is the
#     `binder validate --json` transcript's version literal.
DRIFT_README="$(fresh_copy)"
sed -i '287s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$DRIFT_README/$README_REL"
assert_exit "drifted README envelope literal -> exit 1" "$DRIFT_README" 1 \
  "README.md:287"
rm -rf "$DRIFT_README"

# [9] broken discovery inside a MULTI-envelope file: plant drift in the user
#     guide's envelope-shape transcript (literal :1586), then hide its fence
#     (:1584). The guide holds four report envelopes, so bare per-location
#     presence would still be satisfied by the other three and the gate would
#     pass green over real drift. The inventory's per-location COUNT is what
#     catches this — 3 found where 4 are declared.
GUIDE_HIDDEN="$(fresh_copy)"
sed -i '1586s#binder/0\.5\.[0-9]\+#binder/9.9.9#' "$GUIDE_HIDDEN/$GUIDE_REL"
sed -i '1584s/^```json$/```jsonX/' "$GUIDE_HIDDEN/$GUIDE_REL"
assert_exit "hidden fence in multi-envelope file -> exit 1" "$GUIDE_HIDDEN" 1 \
  "MISSING-COVERAGE: docs/user_guide.md [envelope:binder.report/v1]"
rm -rf "$GUIDE_HIDDEN"

# [10] an unpinned version literal reintroduced into README prose -> RED. Line
#      185 is the `go install` note, which said "prints binder/0.3.0" until the
#      #185 follow-on replaced the literal with the `binder/<version>`
#      placeholder. No JSON-fenced gate can track a prose literal, so the rule
#      is that README carries none.
DRIFT_README_PROSE="$(fresh_copy)"
sed -i '185s#binder/<version>#binder/0.3.0#' "$DRIFT_README_PROSE/$README_REL"
assert_exit "unpinned README prose literal -> exit 1" "$DRIFT_README_PROSE" 1 \
  "[PROSE-UNPINNED]"
rm -rf "$DRIFT_README_PROSE"

# [11] THE ESCAPE HATCH WORKS. Same planted literal as [10], but with an
#      allowlist entry for it -> GREEN. Paired with [10], this is the whole
#      claim the rule's comment makes: a legitimate historical README literal is
#      resolved by one allowlist line, so nobody hitting the known false positive
#      needs to delete the rule. The allowlist lives in the script, so the case
#      runs a patched COPY of the checker; the patch is applied by pattern, and
#      if the marker line ever changes the sed no-ops, the copy behaves like the
#      real checker, and this case fails LOUD rather than passing vacuously.
ALLOW_TREE="$(fresh_copy)"
sed -i '185s#binder/<version>#binder/0.2.1#' "$ALLOW_TREE/$README_REL"
ALLOW_CHECKER="$(mktemp -d)/check-transcript-versions.py"
sed 's#^NO_UNPINNED_PROSE_ALLOW = {}$#NO_UNPINNED_PROSE_ALLOW = {("README.md", "0.2.1"): "fixture: historical reference"}#' \
  "$CHECKER" > "$ALLOW_CHECKER"
assert_exit "allowlisted historical README literal -> exit 0" "$ALLOW_TREE" 0 \
  "0 drift finding(s)" "$ALLOW_CHECKER"
rm -rf "$ALLOW_TREE" "$(dirname "$ALLOW_CHECKER")"

# [12] a scan root that does not exist -> RED. A moved or renamed root must fail
#      loud, not shrink the scan silently.
NOROOT="$(fresh_copy)"
rm -rf "$NOROOT/docs"
assert_exit "missing scan root (docs/ removed) -> exit 1" "$NOROOT" 1 \
  "MISSING-ROOT: scan root docs"
rm -rf "$NOROOT"

echo
# Vacuous-pass guard for the harness itself: a harness whose whole job is locking
# in the "examines-nothing" fix must not report success while examining nothing.
# With no cases run (PASS=FAIL=0) the checks below would print "OK: 0 ... passed"
# and exit 0, so assert the expected number of cases actually ran FIRST.
TOTAL=$((PASS + FAIL))
if [ "$TOTAL" -ne "$EXPECTED_CASES" ]; then
  echo "FAILED: ran $TOTAL case(s) but expected $EXPECTED_CASES — cases were" \
       "skipped, reordered, or guarded out. A harness that runs nothing must" \
       "not report success."
  exit 1
fi

if [ "$FAIL" -eq 0 ]; then
  echo "OK: all $PASS of $EXPECTED_CASES fixture case(s) passed."
  exit 0
else
  echo "FAILED: $FAIL of $TOTAL fixture case(s) did not match."
  exit 1
fi
