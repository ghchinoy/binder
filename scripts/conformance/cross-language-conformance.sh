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
echo "  ok   golden derived by executing the real Go code ($(grep -c '"error"' "$GOLDEN") frontmatter case(s), $(grep -c '"actor"' "$GOLDEN") actor case(s))"

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
