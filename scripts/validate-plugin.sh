#!/usr/bin/env bash
# Agent Plugin and Skill Validator (binder)
#
# Validates conformance with the Agent Plugins Specification v1.0.0 and the
# Agent Skills spec for the plugin(s) that ship inside this repo. Vendored and
# adapted from ghchinoy/agent-skills scripts/validate-plugins.sh so binder runs
# the same 1.0.0 checks (see .design / issue #14). Exits non-zero on any error.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
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

  # Single Python script to safely parse frontmatter from the file
  read_res="$(python3 -c "
import json, sys, os

filepath = '$skill_md'
content = open(filepath, 'r', encoding='utf-8').read()

if not content.startswith('---'):
    print(json.dumps({'error': 'No YAML frontmatter delimiter at start'}))
    sys.exit(0)

parts = content.split('---', 2)
if len(parts) < 3:
    print(json.dumps({'error': 'Malformed YAML frontmatter delimiters'}))
    sys.exit(0)

fm_text = parts[1]

# Fallback simple parser for name, description, license
import re
def get_val(key, text):
    m = re.search(r'^' + key + r':\s*(.*)$', text, re.MULTILINE)
    if not m:
        return ''
    val = m.group(1).strip()
    if (val.startswith('\"') and val.endswith('\"')) or (val.startswith(\"'\") and val.endswith(\"'\")):
        val = val[1:-1]
    return val

name = get_val('name', fm_text)
desc = get_val('description', fm_text)
lic = get_val('license', fm_text)

print(json.dumps({'name': name, 'desc': desc, 'desc_len': len(desc), 'license': lic}))
" 2>/dev/null || echo '{"error": "python execution failed"}')"

  has_err="$(echo "$read_res" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error',''))" 2>/dev/null || true)"
  if [ -n "$has_err" ]; then
    err "Skill '$expected_name' at '$skill_md': $has_err"
    continue
  fi

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
