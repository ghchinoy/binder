#!/usr/bin/env bash
# Fixture harness for scripts/validate-plugin.sh (issue #89).
#
# Proves the plugin/skill validator fails CLOSED on malformed frontmatter and
# still passes on well-formed frontmatter. Each case assembles a throwaway repo
# tree (a valid plugin.json + one fixture SKILL.md) and runs the real validator
# against it via VALIDATE_PLUGIN_ROOT, asserting the expected exit code.
#
# The uniform failure mode these guard against: the happy path being the only
# path ever tested. badyaml/ is the exact #88 shape; before the #89 fix the
# validator scraped its fields with regexes and exited 0.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VALIDATOR="$SCRIPT_DIR/validate-plugin.sh"
FIXROOT="$SCRIPT_DIR/testdata/plugin-validate"

PASS=0
FAIL=0

# run_case <label> <fixture-dir> <expected-exit>
run_case() {
  local label="$1" fixture="$2" expected="$3"
  local tmp skill_dir actual
  tmp="$(mktemp -d)"
  skill_dir="$tmp/plugins/okf-fixture/skills/okf-fixture"
  mkdir -p "$skill_dir" "$tmp/.claude-plugin"
  cp "$FIXROOT/plugin.json" "$tmp/plugins/okf-fixture/plugin.json"
  cp "$FIXROOT/marketplace.json" "$tmp/.claude-plugin/marketplace.json"
  cp "$FIXROOT/$fixture/SKILL.md" "$skill_dir/SKILL.md"

  VALIDATE_PLUGIN_ROOT="$tmp" "$VALIDATOR" >"$tmp/out.txt" 2>&1
  actual=$?

  if [ "$actual" -eq "$expected" ]; then
    echo "  PASS  $label (exit $actual)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $label: expected exit $expected, got $actual"
    sed 's/^/        | /' "$tmp/out.txt"
    FAIL=$((FAIL + 1))
  fi
  rm -rf "$tmp"
}

echo "==> plugin-validate fixture harness (issue #89)"
run_case "positive: well-formed frontmatter"              valid        0
run_case "negative (a): #88 unquoted colon-space desc"    badyaml      1
run_case "negative (b): unterminated '---' fence"         unterminated 1
run_case "negative (c): non-mapping top-level node"       nonmapping   1

echo
if [ "$FAIL" -eq 0 ]; then
  echo "OK: $PASS fixture case(s) passed."
  exit 0
else
  echo "FAILED: $FAIL of $((PASS + FAIL)) fixture case(s) did not match."
  exit 1
fi
