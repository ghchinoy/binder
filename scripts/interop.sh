#!/usr/bin/env bash
#
# Differential-validation interop gate (design-v2 §9 exit gate).
#
# Cross-checks binder's own OKF verdict against the external, vendor-neutral
# okfcli/okf validator, in BOTH directions, and captures every disagreement.
# Also produces the "opens with edges visible" evidence via `okf graph`.
#
# Requires: the `okf` binary on PATH (or in $GOBIN / $(go env GOPATH)/bin).
#   go install github.com/okfcli/okf/cmd/okf@v0.3.0
#
# Exit 0 only if every EXPECTED agreement holds. Documented, spec-grounded
# disagreements (see FINDING below) are asserted to be exactly what we expect;
# an unexpected change in them fails the gate too.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ---- locate tools --------------------------------------------------------
BINDER="${BINDER:-$ROOT/bin/binder}"
if [[ ! -x "$BINDER" ]]; then
  echo ">> building binder"
  go build -o "$BINDER" .
fi

OKF="$(command -v okf || true)"
if [[ -z "$OKF" ]]; then
  for cand in "${GOBIN:-}/okf" "$(go env GOPATH)/bin/okf"; do
    [[ -x "$cand" ]] && OKF="$cand" && break
  done
fi
if [[ -z "$OKF" ]]; then
  echo "FAIL: external validator 'okf' not found."
  echo "      install with: go install github.com/okfcli/okf/cmd/okf@v0.3.0"
  exit 127
fi

echo ">> binder: $BINDER"
echo ">> okf:    $OKF"
"$OKF" version || true
echo

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
fail=0
note() { printf '   %s\n' "$*"; }

# okf_valid <bundle> -> 0 if okf reports valid (exit 0), 1 otherwise
okf_valid() { "$OKF" validate "$1" >/dev/null 2>&1; }
# binder_valid <bundle> -> 0 if binder reports conformant
binder_valid() { "$BINDER" validate "$1" >/dev/null 2>&1; }

expect_agree_valid() { # <label> <bundle>
  local label="$1" b="$2"
  local bv=0 ov=0
  binder_valid "$b" || bv=1
  okf_valid "$b" || ov=1
  if [[ $bv -eq 0 && $ov -eq 0 ]]; then
    echo "PASS  [$label] binder=conformant  okf=valid  (agree)"
  else
    echo "FAIL  [$label] binder_valid=$([[ $bv -eq 0 ]] && echo yes || echo no)  okf_valid=$([[ $ov -eq 0 ]] && echo yes || echo no)  (expected both valid)"
    fail=1
  fi
}

expect_agree_invalid() { # <label> <bundle>
  local label="$1" b="$2"
  local bv=0 ov=0
  binder_valid "$b" || bv=1
  okf_valid "$b" || ov=1
  if [[ $bv -ne 0 && $ov -ne 0 ]]; then
    echo "PASS  [$label] binder=NOT-conformant  okf=invalid  (agree on hard error)"
  else
    echo "FAIL  [$label] binder_valid=$([[ $bv -eq 0 ]] && echo yes || echo no)  okf_valid=$([[ $ov -eq 0 ]] && echo yes || echo no)  (expected both invalid)"
    fail=1
  fi
}

echo "== 1. golden bundles: both validators must accept (bidirectional agreement) =="
for b in testdata/okf-bundles/*/; do
  [[ -d "$b" ]] || continue
  expect_agree_valid "golden:$(basename "$b")" "$b"
done
echo

echo "== 2. binder-converted CLEAN corpus: both validators must accept =="
CLEAN="$WORK/clean"
SOURCE_DATE_EPOCH=1700000000 "$BINDER" convert testdata/corpus-clean -o "$CLEAN" >/dev/null
expect_agree_valid "converted:corpus-clean" "$CLEAN"
echo "   -- okf graph (edges must be visible) --"
"$OKF" graph "$CLEAN" > "$WORK/graph.json" 2>/dev/null || true
edges="$(grep -oE '"edge_count":[[:space:]]*[0-9]+' "$WORK/graph.json" | grep -oE '[0-9]+' || echo 0)"
nodes="$(grep -oE '"node_count":[[:space:]]*[0-9]+' "$WORK/graph.json" | grep -oE '[0-9]+' || echo 0)"
if [[ "${edges:-0}" -ge 1 && "${nodes:-0}" -ge 1 ]]; then
  echo "PASS  [graph:corpus-clean] okf graph sees nodes=$nodes edges=$edges (bundle-absolute links resolved)"
else
  echo "FAIL  [graph:corpus-clean] okf graph saw nodes=$nodes edges=$edges"
  fail=1
fi
echo

echo "== 3. hard error (missing type): both validators must flag =="
expect_agree_invalid "malformed:notype" "testdata/malformed"
echo

echo "== 4. FINDING — broken cross-link: spec-grounded disagreement (captured, not papered over) =="
# OKF v0.2 §11: consumers MUST NOT reject a bundle for broken cross-links.
# binder honours this (conformant); okfcli/okf v0.3.0 rejects (okf/links/broken
# = ERROR). binder is spec-correct. We ASSERT this exact disagreement so a change
# in either tool's behaviour re-surfaces here.
BASIC="$WORK/basic"
SOURCE_DATE_EPOCH=1700000000 "$BINDER" convert testdata/corpus-basic -o "$BASIC" >/dev/null
bv=0; binder_valid "$BASIC" || bv=1
ov=0; "$OKF" validate "$BASIC" > "$WORK/basic.json" 2>/dev/null || ov=1
# Which ERROR-severity rules did okf report? (grep may match nothing -> guard set -e)
err_block="$(grep -B2 '"severity": *"ERROR"' "$WORK/basic.json" 2>/dev/null || true)"
broken_present=0;  echo "$err_block" | grep -q 'okf/links/broken' && broken_present=1
other_errs="$(echo "$err_block" | grep -oE 'okf/[a-z/-]+' | grep -vc 'okf/links/broken' || true)"
if [[ $bv -eq 0 && $ov -ne 0 && $broken_present -eq 1 && "${other_errs:-0}" -eq 0 ]]; then
  echo "PASS  [finding:broken-link] binder=conformant (§11: MUST NOT reject broken links); okf=invalid (okf/links/broken ERROR)"
  note "DISAGREEMENT is expected and spec-grounded: binder follows OKF v0.2 §11, okfcli/okf v0.3.0 is stricter."
else
  echo "FAIL  [finding:broken-link] unexpected: binder_valid=$([[ $bv -eq 0 ]] && echo yes || echo no) okf_valid=$([[ $ov -eq 0 ]] && echo yes || echo no) broken_rule_present=$broken_present other_error_rules=$other_errs"
  fail=1
fi
echo

if [[ $fail -eq 0 ]]; then
  echo "INTEROP GATE: PASS"
else
  echo "INTEROP GATE: FAIL"
fi
exit $fail
