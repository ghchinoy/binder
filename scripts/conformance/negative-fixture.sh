#!/usr/bin/env bash
# negative-fixture.sh — the permanent, automated negative test for the #171
# conformance check.
#
# A conformance check is only worth anything if it actually goes RED when a copy
# diverges. This script proves that end to end and is itself an assertion: it
# mutates the Go source of truth, runs the conformance check, and FAILS unless
# the check went red once per copy (TS, the Python gate, and BOTH prose
# fixtures); then it reverts and requires green again; then it does the same for
# a trust.ts actor-rule divergence.
#
# It restores every file it touches (via backups + an EXIT trap) and rebuilds
# the astro-okf dist on the way out, so it leaves the tree exactly as it found
# it even if it fails midway. Safe to run in CI, where the checkout is
# disposable anyway.
#
# Requires: go, node, python3 (with PyYAML), a built astro-okf dist.
set -uo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SELF_DIR/../.." && pwd)"
cd "$REPO_ROOT"

CONF="$SELF_DIR/cross-language-conformance.sh"
NATIVE="internal/okf/native/native.go"
TRUST_TS="packages/astro-okf/src/trust.ts"
CORPUS="scripts/conformance/corpus.json"

WORK="$(mktemp -d)"
NATIVE_BAK="$WORK/native.go.bak"
TRUST_BAK="$WORK/trust.ts.bak"
CORPUS_BAK="$WORK/corpus.json.bak"
cp "$NATIVE" "$NATIVE_BAK"
cp "$TRUST_TS" "$TRUST_BAK"
cp "$CORPUS" "$CORPUS_BAK"

restore() {
  cp "$NATIVE_BAK" "$NATIVE"
  cp "$TRUST_BAK" "$TRUST_TS"
  cp "$CORPUS_BAK" "$CORPUS"
  npm run build --workspace astro-okf >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap restore EXIT

PROBLEMS=0
note() { echo; echo "######## $1"; }

# expect_pass <label> — conformance must exit 0.
expect_pass() {
  local label="$1"
  if "$CONF"; then
    echo ">>> ASSERT OK: $label — conformance GREEN as expected."
  else
    echo ">>> ASSERT FAILED: $label — expected GREEN, got RED."
    PROBLEMS=$((PROBLEMS + 1))
  fi
}

# expect_red_naming <label> <output-file> <pattern>...
# conformance must exit non-zero AND its output must contain every pattern.
expect_red_naming() {
  local label="$1" out="$2"
  shift 2
  local rc=0
  "$CONF" >"$out" 2>&1 || rc=$?
  cat "$out"
  if [ "$rc" -eq 0 ]; then
    echo ">>> ASSERT FAILED: $label — expected RED, got GREEN."
    PROBLEMS=$((PROBLEMS + 1))
    return
  fi
  local ok=1 pat
  for pat in "$@"; do
    if grep -qF "$pat" "$out"; then
      echo ">>> named as diverged: $pat"
    else
      echo ">>> ASSERT FAILED: $label — expected the check to name: $pat"
      ok=0
    fi
  done
  if [ "$ok" -eq 1 ]; then
    echo ">>> ASSERT OK: $label — RED, and named every diverged copy."
  else
    PROBLEMS=$((PROBLEMS + 1))
  fi
}

echo "=================================================================="
echo " #171 NEGATIVE FIXTURE — proving the conformance check goes RED"
echo " per copy when the Go source of truth changes, and GREEN on revert"
echo " commit: $(git rev-parse HEAD 2>/dev/null || echo '(no git)')"
echo "=================================================================="

# Ensure dist reflects committed TS before we start.
npm run build --workspace astro-okf >/dev/null 2>&1

note "[1] BASELINE — unmodified tree must be GREEN"
expect_pass "baseline"

note "[2] MUTATE the Go error strings (source of truth) — expect RED per copy"
# Keep the 'unterminated' substring (the full literal still changes, so every
# hand-copy must be named). This mirrors the pre-existing-bug demonstration,
# where retaining that substring keeps the Go suite itself green — the point of
# #171 being that the Go suite does NOT catch this drift.
sed -i \
  -e "s/unterminated '---' block/unterminated '---' fence/" \
  -e "s/expected a mapping at the top level/expected a mapping at the document root/" \
  "$NATIVE"
echo "Applied Go mutation:"
git diff --no-color "$NATIVE" | grep -E '^[+-].*frontmatter:' || true
expect_red_naming "go-error-string divergence" "$WORK/red1.txt" \
  "FAIL TypeScript parse.ts DIVERGED from Go on frontmatter[unterminated-fence]" \
  "FAIL TypeScript parse.ts DIVERGED from Go on frontmatter[nonmapping-sequence]" \
  "FAIL Python (validate-plugin.sh copy) DIVERGED from Go on frontmatter[unterminated-fence]" \
  "FAIL Python (validate-plugin.sh copy) DIVERGED from Go on frontmatter[nonmapping-sequence]" \
  "FAIL prose fixture scripts/testdata/plugin-validate/unterminated/SKILL.md" \
  "FAIL prose fixture scripts/testdata/plugin-validate/nonmapping/SKILL.md"

note "[3] REVERT the Go strings — must be GREEN again"
cp "$NATIVE_BAK" "$NATIVE"
expect_pass "after reverting the Go strings"

note "[4] MUTATE trust.ts actor rule (isHumanActor) — expect RED naming trust.ts"
# Make TS isHumanActor case-insensitive; Go's okf.IsHumanActor is not, so
# "Human:bob" now diverges.
sed -i 's/actor.startsWith("human:")/actor.toLowerCase().startsWith("human:")/' "$TRUST_TS"
echo "Applied trust.ts mutation:"
git diff --no-color "$TRUST_TS" | grep -E '^[+-].*startsWith' || true
npm run build --workspace astro-okf >/dev/null 2>&1
expect_red_naming "trust.ts actor divergence" "$WORK/red2.txt" \
  "FAIL TypeScript trust.ts DIVERGED from Go on actor \"Human:bob\""

note "[5] REVERT trust.ts — must be GREEN again"
cp "$TRUST_BAK" "$TRUST_TS"
npm run build --workspace astro-okf >/dev/null 2>&1
expect_pass "after reverting trust.ts"

note "[6] EMPTY the corpus — the check must FAIL CLOSED, not pass vacuously"
# A check that reports success because it examined nothing is worthless. Stub the
# corpus to zero cases and require the positive check to refuse it.
printf '{"frontmatter": [], "actors": []}\n' >"$CORPUS"
expect_red_naming "vacuous (empty) corpus" "$WORK/red3.txt" \
  "FATAL: conformance corpus is vacuous"

note "[7] RESTORE the corpus — must be GREEN again"
cp "$CORPUS_BAK" "$CORPUS"
expect_pass "after restoring the corpus"

echo
echo "=================================================================="
if [ "$PROBLEMS" -eq 0 ]; then
  echo "NEGATIVE FIXTURE PASSED: the conformance check goes RED once per copy on"
  echo "divergence and GREEN on revert. #171 is enforced."
  exit 0
else
  echo "NEGATIVE FIXTURE FAILED: $PROBLEMS assertion(s) did not hold (see above)."
  exit 1
fi
