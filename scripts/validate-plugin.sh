#!/usr/bin/env bash
# Agent Plugin and Skill Validator (binder)
#
# Validates conformance with the Agent Plugins Specification v1.0.0 and the
# Agent Skills spec for the plugin(s) that ship inside this repo. Vendored and
# adapted from ghchinoy/agent-skills scripts/validate-plugins.sh so binder runs
# the same 1.0.0 checks (see .design / issue #14). Exits non-zero on any error.

set -euo pipefail

# Directory this script ships in. Captured BEFORE any cd so helper scripts
# (frontmatter_parse.py) are found in the real scripts/ dir even when the
# fixture harness points VALIDATE_PLUGIN_ROOT at a throwaway tree that has no
# copy of them (issue #89).
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Frontmatter parser (issue #89), extracted so the #171 conformance check can
# exercise the exact same code this gate runs instead of a second copy.
FRONTMATTER_PARSER="$SELF_DIR/frontmatter_parse.py"

# Root of the tree to validate. Defaults to the repo this script ships in;
# VALIDATE_PLUGIN_ROOT lets the fixture harness (scripts/validate-plugin-fixtures.sh)
# point the same checks at a throwaway tree without copying the script (issue #89).
REPO_ROOT="${VALIDATE_PLUGIN_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$REPO_ROOT"

RED='\033[1;31m'
GREEN='\033[1;32m'
YELLOW='\033[1;33m'
BLUE='\033[1;34m'
NC='\033[0m'

ERRORS=0
WARNINGS=0

info()  { echo -e "${BLUE}==>${NC} $1"; }
ok()    { echo -e "  ${GREEN}✓${NC} $1"; }
warn()  { echo -e "  ${YELLOW}⚠${NC} $1"; WARNINGS=$((WARNINGS + 1)); }
err()   { echo -e "  ${RED}✖${NC} $1"; ERRORS=$((ERRORS + 1)); }

info "1. Validating repository layout"
if [ ! -d "plugins" ]; then
  err "Missing top-level 'plugins/' directory."
else
  ok "Found 'plugins/' directory."
fi

info "2. Validating Plugin Package Manifests (plugin.json)"
for plugin_dir in plugins/*; do
  [ -d "$plugin_dir" ] || continue
  plugin_name="$(basename "$plugin_dir")"
  manifest="$plugin_dir/plugin.json"

  if [ ! -f "$manifest" ]; then
    err "Plugin '$plugin_name' missing plugin.json"
    continue
  fi

  # Basic JSON syntax check
  if ! python3 -m json.tool "$manifest" >/dev/null 2>&1; then
    err "Plugin '$plugin_name' plugin.json is invalid JSON"
    continue
  fi

  # Required fields check
  schema_val="$(python3 -c "import json; print(json.load(open('$manifest')).get('\$schema',''))" 2>/dev/null || true)"
  name_val="$(python3 -c "import json; print(json.load(open('$manifest')).get('name',''))" 2>/dev/null || true)"

  if [ "$schema_val" != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" ]; then
    err "Plugin '$plugin_name' $manifest has invalid or missing \$schema: '$schema_val'"
  else
    ok "Plugin '$plugin_name' \$schema matches Agent Plugins 1.0.0"
  fi

  if [ "$name_val" != "$plugin_name" ]; then
    err "Plugin '$plugin_name' name in plugin.json ('$name_val') does not match folder name '$plugin_name'"
  else
    ok "Plugin '$plugin_name' manifest name matches directory name"
  fi
done

info "3. Validating Agent Skills (SKILL.md)"
for skill_md in plugins/*/skills/*/SKILL.md; do
  [ -f "$skill_md" ] || continue
  skill_dir="$(dirname "$skill_md")"
  expected_name="$(basename "$skill_dir")"

  # Parse the frontmatter with a REAL YAML parser and fail closed on anything
  # a YAML parser rejects (issue #89). The prior implementation split on the
  # '---' substring and scraped name/description/license with per-key regexes,
  # so it could not tell valid YAML from invalid YAML and waved malformed
  # frontmatter through with a clean exit 0 (this is the gate #88 was about).
  #
  # The parsing logic lives in scripts/frontmatter_parse.py; its fence/mapping
  # semantics mirror the Go codec's splitFrontmatter + parseFrontmatterNode
  # (internal/okf/native/native.go) so binder and this gate agree on what "valid
  # frontmatter" means and use the same wording. That agreement is enforced —
  # not merely intended — by the #171 conformance check
  # (scripts/conformance/cross-language-conformance.sh), which imports the very
  # same frontmatter_parse.py and diffs it against error text derived live from
  # the Go codec. Editing a Go string turns that suite red for this copy.
  #
  # Only stdout is captured here, and only stdout carries the JSON contract the
  # shell parses below. Python's stderr is routed to a temp file (NOT merged into
  # stdout, which would corrupt that JSON stream) and surfaced on failure, so a
  # future crash shows its traceback instead of a bare "python execution failed"
  # (issue #89, round 3).
  py_stderr="$(mktemp)"
  read_res="$(SKILL_MD="$skill_md" python3 "$FRONTMATTER_PARSER" "$skill_md" 2>"$py_stderr" || echo '{"error": "python execution failed"}')"

  has_err="$(echo "$read_res" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error',''))" 2>/dev/null || true)"
  if [ -n "$has_err" ]; then
    err "Skill '$expected_name' at '$skill_md': $has_err"
    # Surface any interpreter diagnostics (e.g. a traceback) captured on the
    # python process's stderr, so a crash is debuggable instead of opaque. Goes
    # to stderr to keep it clear of anything parsed as data.
    if [ -s "$py_stderr" ]; then
      echo "      python stderr (diagnostics):" >&2
      sed 's/^/        /' "$py_stderr" >&2
    fi
    rm -f "$py_stderr"
    continue
  fi
  rm -f "$py_stderr"

  name_in_fm="$(echo "$read_res" | python3 -c "import json,sys; print(json.load(sys.stdin).get('name',''))" 2>/dev/null || true)"
  desc_len="$(echo "$read_res" | python3 -c "import json,sys; print(json.load(sys.stdin).get('desc_len',0))" 2>/dev/null || echo 0)"
  lic_in_fm="$(echo "$read_res" | python3 -c "import json,sys; print(json.load(sys.stdin).get('license',''))" 2>/dev/null || true)"

  if [ "$name_in_fm" != "$expected_name" ]; then
    err "Skill '$expected_name' frontmatter name ('$name_in_fm') does not match directory '$expected_name'"
  else
    ok "Skill '$expected_name' frontmatter name matches directory"
  fi

  if [ "$desc_len" -eq 0 ]; then
    err "Skill '$expected_name' missing 'description' in frontmatter"
  elif [ "$desc_len" -gt 1024 ]; then
    err "Skill '$expected_name' description exceeds 1024 characters ($desc_len chars)"
  else
    ok "Skill '$expected_name' description length OK ($desc_len chars)"
  fi

  if [ -z "$lic_in_fm" ]; then
    err "Skill '$expected_name' missing 'license' in frontmatter"
  else
    ok "Skill '$expected_name' has a license"
  fi

  # Executable scripts check
  if [ -d "$skill_dir/scripts" ]; then
    for script in "$skill_dir/scripts"/*; do
      [ -f "$script" ] || continue
      if [ ! -x "$script" ]; then
        err "Script '$script' in skill '$expected_name' is NOT executable (+x)"
      else
        ok "Script '$(basename "$script")' in '$expected_name' is executable"
      fi
    done
  fi
done

info "4. Validating Claude Plugin Marketplace manifest"
if [ -f ".claude-plugin/marketplace.json" ]; then
  if python3 -m json.tool ".claude-plugin/marketplace.json" >/dev/null 2>&1; then
    ok ".claude-plugin/marketplace.json is valid JSON"
  else
    err ".claude-plugin/marketplace.json is invalid JSON"
  fi
else
  warn "Missing .claude-plugin/marketplace.json"
fi

echo
if [ "$ERRORS" -eq 0 ]; then
  echo -e "${GREEN}✓ All plugin and skill validations passed successfully!${NC}"
  [ "$WARNINGS" -gt 0 ] && echo -e "${YELLOW}  ($WARNINGS warning(s) logged)${NC}"
  exit 0
else
  echo -e "${RED}✖ Validation failed with $ERRORS error(s) and $WARNINGS warning(s).${NC}"
  exit 1
fi
