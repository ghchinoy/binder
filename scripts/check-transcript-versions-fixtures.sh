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

# The stamped version as the docs spell it (no leading "v"), for the cases that
# must construct a literal carrying the CURRENT version.
CURRENT="${STAMP_VERSION#v}"

PASS=0
FAIL=0

# The number of cases this harness MUST run. Asserted at the end against
# PASS+FAIL, so the harness cannot itself pass vacuously (round-4 review): if a
# case is commented out, reordered away, or conditionally guarded off, PASS+FAIL
# drops below this and the harness fails loud naming the shortfall. A bare
# "ran at least one" floor is weaker — it would not catch losing three of seven —
# exactly the inventory-over-floor reasoning from round 2. Update this when you
# add or remove a case.
EXPECTED_CASES=18

# assert_exit <label> <docroot> <expected-exit> [want] [checker]
# [want] is a substring of the output, or several joined by " && " when a case
# needs to assert more than one thing about the same run (e.g. WHICH finding
# fired and HOW MANY did). [checker] defaults to the real checker; the allowlist
# cases pass a patched copy, since the allowlist lives in the script rather than
# in the doc tree.
assert_exit() {
  local label="$1" docroot="$2" expected="$3" want="${4:-}" checker="${5:-$CHECKER}"
  local out actual all_found=1 part
  out="$(python3 "$checker" "$docroot" "$BIN" 2>&1)"
  actual=$?
  if [ -n "$want" ]; then
    local rest="$want"
    while [ -n "$rest" ]; do
      case "$rest" in
        *" && "*) part="${rest%%" && "*}"; rest="${rest#*" && "}" ;;
        *)        part="$rest"; rest="" ;;
      esac
      echo "$out" | grep -qF "$part" || all_found=0
    done
  fi
  if [ "$actual" -eq "$expected" ] && [ "$all_found" -eq 1 ]; then
    echo "  PASS  $label (exit $actual)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $label: expected exit $expected${want:+ + '$want'}, got exit $actual"
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

# CONTENT ADDRESSING, NOT LINE ADDRESSING (#185 round 2). The mutation cases
# below must plant drift in a SPECIFIC transcript of the REAL committed docs.
# They used to do that by line number (SKILL.md:125, the fence at :124, ...).
# Those addresses went stale three times in this one change: +4 lines from #192
# in README, +6 from a new bullet in this PR, +6 from #189 in the user guide.
# The third one was clean to git and green in PR CI — the coupling is semantic,
# not textual — and would have reddened the gate on main after merge. Any prose
# landing anywhere above a transcript moves it, so staleness is the steady state
# of a line address, not an accident.
#
# So nothing here is addressed by line. A case names a stable ANCHOR — a heading
# or a sentence that identifies WHICH transcript it means — and the target is
# then found relative to that anchor by content: the first version literal at or
# below it, and, where a case needs to hide a block, the nearest opening fence
# above that literal. An edit above the target changes the line number of
# everything and the behaviour of nothing. Cases [13] and [14] prove exactly
# that property rather than asserting it.
#
# Every lookup is fail-loud. An anchor must match exactly ONE line; zero matches
# (the anchor's wording changed) and two matches (it stopped being specific) both
# abort the harness naming the anchor, and every mutation asserts it actually
# changed the file. Without that, a lookup that silently found nothing would be
# fail-safe only for a case expecting exit 1 — the unplanted tree exits 0 and the
# case fails loud. A case expecting exit 0, like [11]'s escape hatch, would pass
# VACUOUSLY: green for the wrong reason, the precise failure this harness exists
# to prevent. The asymmetry is closed here, once, for all cases.

# The anchors. Each identifies one transcript and must match exactly one line.
A_SKILL_CONTRACT='^## The binder JSON contract \(what you parse\)$'
A_CONTRACT_PROSE='^was captured from real `binder/'
A_README_ENVELOPE='^binder validate path/to/bundle --json$'
A_README_PROSE='prints `binder/<version>`, no leading'
A_GUIDE_CONFIG='^\*\*`binder config --json`'
A_GUIDE_ENVELOPE='^### The envelope \(schema `binder\.report/v1`\)$'

# A version literal as it appears in a transcript, and the drift to plant.
LITERAL_RE='binder/0\.5\.[0-9]+'
DRIFT_SED='s#binder/0\.5\.[0-9]\+#binder/9.9.9#'

# abort <line...> — a harness bug (stale anchor, no-op mutation), not a case
# failure. Stop immediately rather than reporting a result for a case whose
# precondition was never established.
abort() {
  echo "FAILED: fixture precondition not met —" "$@" >&2
  echo "        See CONTENT ADDRESSING above; re-point the anchor at the current" >&2
  echo "        doc wording." >&2
  exit 1
}

# uniq_line <file> <ERE> — echo the number of the ONE line matching <ERE>.
uniq_line() {
  local file="$1" ere="$2" hits n
  hits="$(grep -nE "$ere" "$file" | cut -d: -f1)"
  n="$(printf '%s\n' "$hits" | grep -c '[0-9]')"
  if [ "$n" -ne 1 ]; then
    abort "anchor '$ere' matched $n lines in ${file##*/}; it must match exactly 1."
  fi
  printf '%s\n' "$hits"
}

# literal_below <file> <line> — first version-literal line at or below <line>.
literal_below() {
  local file="$1" from="$2" hit
  hit="$(awk -v from="$from" -v re="$LITERAL_RE" \
    'NR >= from && $0 ~ re { print NR; exit }' "$file")"
  [ -n "$hit" ] || abort "no version literal at or below line $from of ${file##*/}."
  printf '%s\n' "$hit"
}

# fence_above <file> <line> — nearest ```json opening fence at or above <line>.
fence_above() {
  local file="$1" from="$2" hit
  hit="$(awk -v from="$from" \
    'NR <= from && /^```json$/ { last = NR } END { if (last) print last }' "$file")"
  [ -n "$hit" ] || abort "no opening json fence above line $from of ${file##*/}."
  printf '%s\n' "$hit"
}

# edit_line <file> <line> <sed-subst> — apply the substitution to that one line
# and assert the file actually changed.
edit_line() {
  local file="$1" line="$2" subst="$3" before
  before="$(cat "$file")"
  sed -i "${line}${subst}" "$file"
  [ "$before" != "$(cat "$file")" ] || \
    abort "'$subst' changed nothing at ${file##*/}:$line."
}

# plant_drift <file> <anchor> — drift the first version literal below <anchor>.
plant_drift() {
  local file="$1" anchor="$2" line
  line="$(uniq_line "$file" "$anchor")" || exit 1
  line="$(literal_below "$file" "$line")" || exit 1
  edit_line "$file" "$line" "$DRIFT_SED"
}

# hide_fence <file> <anchor> — rename the opening fence of <anchor>'s transcript,
# so the checker no longer sees the block at all.
hide_fence() {
  local file="$1" anchor="$2" line
  line="$(uniq_line "$file" "$anchor")" || exit 1
  line="$(literal_below "$file" "$line")" || exit 1
  line="$(fence_above "$file" "$line")" || exit 1
  edit_line "$file" "$line" 's/^```json$/```jsonX/'
}

# plant_prose <file> <anchor> <sed-subst> — mutate the anchor line itself, for
# the README prose cases, where the anchor IS the target.
plant_prose() {
  local file="$1" anchor="$2" subst="$3" line
  line="$(uniq_line "$file" "$anchor")" || exit 1
  edit_line "$file" "$line" "$subst"
}

# reported_line <file> <anchor> — the line the checker will name for <anchor>'s
# literal. Used only to compute a case's EXPECTED output.
reported_line() {
  local file="$1" anchor="$2" line
  line="$(uniq_line "$file" "$anchor")" || exit 1
  literal_below "$file" "$line"
}

# shift_down <file> <n> — insert n inert lines at the top of <file>, moving every
# target in it down by n. Markdown comments: no fences, no version literals.
shift_down() {
  local file="$1" n="$2" i tmp
  tmp="$(mktemp)"
  for ((i = 1; i <= n; i++)); do
    echo "<!-- filler line $i: inserted above every target in this file -->" >> "$tmp"
  done
  cat "$file" >> "$tmp"
  mv "$tmp" "$file"
}

# [2] drifted JSON envelope literal -> RED (drift finding)
DRIFT_JSON="$(fresh_copy)"
plant_drift "$DRIFT_JSON/$SKILL_REL" "$A_SKILL_CONTRACT"
assert_exit "drifted JSON envelope literal -> exit 1" "$DRIFT_JSON" 1 "[JSON-ENVELOPE]"
rm -rf "$DRIFT_JSON"

# [3] drifted prose provenance sentence -> RED (drift finding)
DRIFT_PROSE="$(fresh_copy)"
plant_prose "$DRIFT_PROSE/$CONTRACT_REL" "$A_CONTRACT_PROSE" "$DRIFT_SED"
assert_exit "drifted prose provenance -> exit 1" "$DRIFT_PROSE" 1 "[PROSE-PROVENANCE]"
rm -rf "$DRIFT_PROSE"

# [4] broken discovery: renamed fence tag hides a genuinely-drifted literal.
#     This is the regression lock for the round-2 Critical — pre-fix the gate
#     exited 0 here; it must now fail via the minimum-coverage assertion.
BROKEN="$(fresh_copy)"
# Hide the fence FIRST: both lookups find the block by its still-current version
# literal, and drifting it to 9.9.9 first would make the fence lookup walk past
# this block to the next real literal.
hide_fence  "$BROKEN/$SKILL_REL" "$A_SKILL_CONTRACT"   # hide the block
plant_drift "$BROKEN/$SKILL_REL" "$A_SKILL_CONTRACT"   # then plant real drift
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
hide_fence "$COL_SKILL" "$A_SKILL_CONTRACT"   # hide the real report envelope
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

# [7] drifted docs/user_guide.md envelope literal -> RED. The target is the
#     `binder config --json` transcript's version literal.
DRIFT_GUIDE="$(fresh_copy)"
plant_drift "$DRIFT_GUIDE/$GUIDE_REL" "$A_GUIDE_CONFIG"
assert_exit "drifted user-guide envelope literal -> exit 1" "$DRIFT_GUIDE" 1 \
  "docs/user_guide.md:1457"
rm -rf "$DRIFT_GUIDE"

# [8] drifted README.md envelope literal -> RED. The target is the
#     `binder validate --json` transcript's version literal.
DRIFT_README="$(fresh_copy)"
plant_drift "$DRIFT_README/$README_REL" "$A_README_ENVELOPE"
assert_exit "drifted README envelope literal -> exit 1" "$DRIFT_README" 1 \
  "README.md:287"
rm -rf "$DRIFT_README"

# [9] broken discovery inside a MULTI-envelope file: plant drift in the user
#     guide's envelope-shape transcript, then hide that block's opening fence.
#     The guide holds four report envelopes, so bare per-location
#     presence would still be satisfied by the other three and the gate would
#     pass green over real drift. The inventory's per-location COUNT is what
#     catches this — 3 found where 4 are declared.
GUIDE_HIDDEN="$(fresh_copy)"
hide_fence  "$GUIDE_HIDDEN/$GUIDE_REL" "$A_GUIDE_ENVELOPE"   # fence first, as in [4]
plant_drift "$GUIDE_HIDDEN/$GUIDE_REL" "$A_GUIDE_ENVELOPE"
assert_exit "hidden fence in multi-envelope file -> exit 1" "$GUIDE_HIDDEN" 1 \
  "MISSING-COVERAGE: docs/user_guide.md [envelope:binder.report/v1]"
rm -rf "$GUIDE_HIDDEN"

# [10] an unpinned version literal reintroduced into README prose -> RED. The
#      target is the `go install` note, which said "prints binder/0.3.0" until
#      this change replaced the literal with the `binder/<version>` placeholder. No JSON-fenced gate can track a prose literal, so the rule
#      is that README carries none.
DRIFT_README_PROSE="$(fresh_copy)"
plant_prose "$DRIFT_README_PROSE/$README_REL" "$A_README_PROSE" \
  's#binder/<version>#binder/0.3.0#'
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
plant_prose "$ALLOW_TREE/$README_REL" "$A_README_PROSE" \
  's#binder/<version>#binder/0.2.1#'
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

# --- #185 round 2: the line-shift immunity proof ---------------------------
# The acceptance test for content addressing: inserting arbitrary lines ABOVE a
# fixture's target must not change that fixture's behaviour. [13] proves it for
# the green path (discovery and the coverage inventory), [14] for the red path
# (the mutation still lands on the right transcript, and the checker names its
# new line). Under the old line addressing both would have failed — which is
# what #189 did to this harness in review.

# [13] every target shifted down, nothing mutated -> still GREEN. Discovery and
#      the per-location coverage counts are position-independent.
SHIFT_CLEAN="$(fresh_copy)"
shift_down "$SHIFT_CLEAN/$GUIDE_REL" 37
shift_down "$SHIFT_CLEAN/$README_REL" 37
shift_down "$SHIFT_CLEAN/$SKILL_REL" 37
assert_exit "37 lines inserted above every target, clean -> exit 0" \
  "$SHIFT_CLEAN" 0 "0 coverage failure(s)"
rm -rf "$SHIFT_CLEAN"

# [14] the same shift, then the [7] mutation planted BY CONTENT -> same RED, at
#      the shifted line. The expected line number is derived from the UNSHIFTED
#      tree plus the known offset, not from the shifted lookup under test, so
#      the case cannot agree with itself by construction.
SHIFT_DRIFT="$(fresh_copy)"
SHIFT_BASE="$(reported_line "$SHIFT_DRIFT/$GUIDE_REL" "$A_GUIDE_CONFIG")" || exit 1
shift_down "$SHIFT_DRIFT/$GUIDE_REL" 37
plant_drift "$SHIFT_DRIFT/$GUIDE_REL" "$A_GUIDE_CONFIG"
assert_exit "37 lines inserted above the target, drifted -> exit 1 at the shifted line" \
  "$SHIFT_DRIFT" 1 "docs/user_guide.md:$((SHIFT_BASE + 37))"
rm -rf "$SHIFT_DRIFT"

# --- #185 round 2: rule interaction, allowlist abuse, inventory drift -------

# [15] PROSE RULE SHADOWING. One README line carrying BOTH a correct provenance
#      sentence and a stale literal. The provenance rule used to `continue` on a
#      match, which silenced the no-unpinned-prose rule for the rest of that line
#      — a crafted line like this produced ZERO findings. The rules are now
#      span-based rather than chained, so each literal is classified once by
#      where it is, not by which rule ran first. Two assertions: the stale
#      literal IS reported, and the provenance literal is NOT double-reported
#      (exactly one finding, not two).
SHADOW="$(fresh_copy)"
plant_prose "$SHADOW/$README_REL" "$A_README_PROSE" \
  's#$# It was captured from real `binder/'"$CURRENT"'` output, unlike binder/0.3.0.#'
assert_exit "provenance sentence must not shadow the unpinned-prose rule -> exit 1" \
  "$SHADOW" 1 "[PROSE-UNPINNED] && 1 drift finding(s)"
rm -rf "$SHADOW"

# [16] ALLOWLISTING THE CURRENT VERSION IS REFUSED. An entry for the version now
#      shipping is not an exemption for one historical literal, it is a permanent
#      silent exemption that switches the rule off over that literal the moment
#      this release stops being current. Exit 2 (a configuration error), not 1:
#      the tree is fine, the gate's own configuration is not.
ALLOW_CURRENT="$(fresh_copy)"
CUR_CHECKER="$(mktemp -d)/check-transcript-versions.py"
sed 's#^NO_UNPINNED_PROSE_ALLOW = {}$#NO_UNPINNED_PROSE_ALLOW = {("README.md", "'"$CURRENT"'"): "fixture: exempts the current version"}#' \
  "$CHECKER" > "$CUR_CHECKER"
assert_exit "allowlist entry for the CURRENT version -> exit 2" "$ALLOW_CURRENT" 2 \
  "exempts the CURRENT stamped version" "$CUR_CHECKER"
rm -rf "$ALLOW_CURRENT" "$(dirname "$CUR_CHECKER")"

# [17] INVENTORY DRIFT: a transcript ADDED without updating the count -> RED.
#      Under a floor this was silent, which let the declared numbers drift away
#      from the tree they describe. Exact counts make it a loud one-line event.
#      The appended block is a correct, current-version envelope: nothing here is
#      drift, which is the point — the failure is the inventory, and the message
#      says so.
ADDED="$(fresh_copy)"
cat >> "$ADDED/$GUIDE_REL" <<EOF

\`\`\`json
{
  "binder": "binder/$CURRENT",
  "command": "lint",
  "schema": "binder.report/v1",
  "result": { }
}
\`\`\`
EOF
assert_exit "transcript added without updating the inventory -> exit 1" "$ADDED" 1 \
  "expected exactly 4 literal(s) && checked 5 && a transcript was ADDED"
rm -rf "$ADDED"

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
