#!/usr/bin/env bash
# Cross-language error-string + trust conformance check (issue #171).
#
# The two structural frontmatter error strings and the actor-derivation rules
# exist as hand-copies in THREE implementations plus TWO prose fixtures, with
# nothing binding them to the Go source of truth:
#
#   Go  (SOURCE OF TRUTH)  internal/okf/native/native.go, internal/okf/trust.go
#   TS                     packages/astro-okf/src/parse.ts, trust.ts
#   Python (the GATE)      scripts/frontmatter_parse.py (run by validate-plugin.sh)
#   prose                  scripts/testdata/plugin-validate/{unterminated,nonmapping}/SKILL.md
#
# This check derives the expected values LIVE from the Go code
# (scripts/conformance/gogolden runs the real ParseConcept and the real
# okf.IsHumanActor / okf.IsValidActor), then diffs every other copy against that
# golden. Editing a Go error string or actor rule changes the golden and turns
# this suite RED once per copy that failed to follow — the silent divergence #171
# describes becomes loud.
#
# It runs ALL sub-checks (it does not stop at the first failure) so a single Go
# change surfaces every copy that diverged in one run.
#
# Requires: go, node, python3 (with PyYAML), and a built astro-okf dist.
set -uo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SELF_DIR/../.." && pwd)"
cd "$REPO_ROOT"

CORPUS="$SELF_DIR/corpus.json"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
GOLDEN="$TMP/go-golden.json"

FAILS=0
section() { echo; echo "== $1 =="; }

echo "#171 cross-language conformance — binding TS, Python, and prose fixtures to the Go source of truth"

# 1. Derive the golden from the Go source of truth.
section "Deriving conformance golden from Go (internal/okf/native + internal/okf/trust)"
if ! go run ./internal/okf/conformance/gogolden "$CORPUS" >"$GOLDEN" 2>"$TMP/go.err"; then
  echo "FATAL: could not derive the Go golden:"
  cat "$TMP/go.err"
  exit 2
fi

# Fail CLOSED on a vacuous corpus. If corpus.json is empty (or its arrays are),
# every sub-check below iterates zero times and the suite passes having examined
# nothing — a check that reports success because it looked at nothing. Assert at
# least one frontmatter case AND one actor case actually ran.
FM_COUNT=$(grep -c '"error"' "$GOLDEN" || true)
ACTOR_COUNT=$(grep -c '"actor"' "$GOLDEN" || true)
if [ "${FM_COUNT:-0}" -lt 1 ] || [ "${ACTOR_COUNT:-0}" -lt 1 ]; then
  echo "FATAL: conformance corpus is vacuous — derived ${FM_COUNT:-0} frontmatter case(s) and ${ACTOR_COUNT:-0} actor case(s)." >&2
  echo "       Refusing to pass on an empty corpus (need >=1 of each). Check $CORPUS." >&2
  exit 2
fi
echo "  ok   golden derived by executing the real Go code ($FM_COUNT frontmatter case(s), $ACTOR_COUNT actor case(s))"

# 1b. Assert NON-VACUITY locally (issue #171 round 3). The count guard above only
# proves the corpus is non-empty; it does not prove the corpus still EXERCISES
# each thing the suite claims to bind. Without this, non-vacuity for the error
# strings was emergent — it held only because the prose check happened to require
# the two structural cases. coverage-check.py asserts directly, from the
# Go-derived golden, that both structural error strings were produced and that
# each trust predicate yielded both true and false. It fails closed (naming what
# was missing) the moment either stops being true, so the guarantee is local
# rather than a coincidence of other checks lining up.
section "Corpus coverage (non-vacuity asserted directly, not left emergent)"
if python3 "$SELF_DIR/coverage-check.py" "$GOLDEN"; then
  echo "  PASS corpus is non-vacuous for every property this suite binds."
else
  FAILS=$((FAILS + 1))
fi

# 2. The TS check runs against the shipped dist.
if [ ! -f packages/astro-okf/dist/index.js ]; then
  echo
  echo "FATAL: packages/astro-okf/dist/index.js missing." >&2
  echo "       Build it first: npm run build --workspace astro-okf" >&2
  exit 2
fi

# 3. TypeScript copy (parse.ts error strings + trust.ts actor derivations).
section "TypeScript (packages/astro-okf/src/parse.ts + trust.ts) vs Go"
if node "$SELF_DIR/ts-conformance.mjs" "$CORPUS" "$GOLDEN"; then
  echo "  PASS TypeScript agrees with Go."
else
  FAILS=$((FAILS + 1))
fi

# 4. Python copy — the validation gate itself.
section "Python (scripts/frontmatter_parse.py, run by validate-plugin.sh) vs Go"
if python3 "$SELF_DIR/py-conformance.py" "$CORPUS" "$GOLDEN"; then
  echo "  PASS Python agrees with Go."
else
  FAILS=$((FAILS + 1))
fi

# 5. Prose fixtures.
section "Prose fixtures (scripts/testdata/plugin-validate/*/SKILL.md) vs Go"
if python3 "$SELF_DIR/prose-conformance.py" "$GOLDEN"; then
  echo "  PASS Prose fixtures restate the current Go wording."
else
  FAILS=$((FAILS + 1))
fi

echo
if [ "$FAILS" -eq 0 ]; then
  echo "OK: all copies agree with the Go source of truth."
  exit 0
else
  echo "FAILED: $FAILS implementation group(s) diverged from the Go source of truth (see above)."
  echo "        The Go strings/rules in internal/okf are authoritative; update the diverged copy to match."
  exit 1
fi
