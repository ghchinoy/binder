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

# run_noyaml_case <label> <fixture-dir> <expected-exit> <expected-message>
# Runs the validator with PyYAML made UNIMPORTABLE, so it exercises the
# missing-parser branch. A shadow `yaml.py` that raises ImportError is placed on
# PYTHONPATH; because PYTHONPATH precedes site-packages on sys.path it wins over
# the real module (verified, not assumed — see the sanity check below). This
# guards the property that a gate which cannot load a parser must NOT fall back
# to passing files it cannot read. The message is asserted too: the clean
# fail-closed path names the missing parser, so stubbing the ImportError handler
# to `pass` (which degrades to a generic "python execution failed") turns this
# case red even though the exit code stays non-zero.
run_noyaml_case() {
  local label="$1" fixture="$2" expected="$3" want_msg="$4"
  local tmp shadow skill_dir actual
  tmp="$(mktemp -d)"
  shadow="$(mktemp -d)"
  printf 'raise ImportError("yaml shadowed unimportable for fixture test")\n' > "$shadow/yaml.py"

  skill_dir="$tmp/plugins/okf-fixture/skills/okf-fixture"
  mkdir -p "$skill_dir" "$tmp/.claude-plugin"
  cp "$FIXROOT/plugin.json" "$tmp/plugins/okf-fixture/plugin.json"
  cp "$FIXROOT/marketplace.json" "$tmp/.claude-plugin/marketplace.json"
  cp "$FIXROOT/$fixture/SKILL.md" "$skill_dir/SKILL.md"

  # Sanity: confirm the shadow really does make `import yaml` fail, so a green
  # result means "the validator handled a missing parser", not "the shadow was
  # ignored and the real yaml loaded anyway".
  if PYTHONPATH="$shadow${PYTHONPATH:+:$PYTHONPATH}" python3 -c "import yaml" 2>/dev/null; then
    echo "  FAIL  $label: shadow yaml.py did not shadow the real module (test is not valid)"
    FAIL=$((FAIL + 1))
    rm -rf "$tmp" "$shadow"
    return
  fi

  PYTHONPATH="$shadow${PYTHONPATH:+:$PYTHONPATH}" \
    VALIDATE_PLUGIN_ROOT="$tmp" "$VALIDATOR" >"$tmp/out.txt" 2>&1
  actual=$?

  if [ "$actual" -eq "$expected" ] && grep -qF "$want_msg" "$tmp/out.txt"; then
    echo "  PASS  $label (exit $actual, message matched)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $label: expected exit $expected + message '$want_msg', got exit $actual"
    sed 's/^/        | /' "$tmp/out.txt"
    FAIL=$((FAIL + 1))
  fi
  rm -rf "$tmp" "$shadow"
}

echo "==> plugin-validate fixture harness (issue #89)"
run_case "positive: well-formed frontmatter"              valid        0
run_case "negative (a): #88 unquoted colon-space desc"    badyaml      1
run_case "negative (b): unterminated '---' fence"         unterminated 1
run_case "negative (c): non-mapping top-level node"       nonmapping   1
run_noyaml_case "negative (d): PyYAML unavailable (fail closed)" valid 1 "PyYAML is required"

echo
if [ "$FAIL" -eq 0 ]; then
  echo "OK: $PASS fixture case(s) passed."
  exit 0
else
  echo "FAILED: $FAIL of $((PASS + FAIL)) fixture case(s) did not match."
  exit 1
fi
