#!/usr/bin/env bash
# Fixture harness for scripts/check-shipped-version-literals.py (issue #60).
#
# The shipped-output gate's own regression lock. A guard never shown to fail is
# not known to be a guard — and #60 is a case where exactly that happened twice:
# the issue was filed to prevent stale exemplars and they still shipped in v0.4.0
# and v0.5.3, because nothing mechanical ever failed on them.
#
# Each case builds a REAL binder from a throwaway copy of the tree, patched to
# reintroduce one specific defect, and asserts the gate's exit code and the kind
# of finding it reports. Patching the source and rebuilding (rather than feeding
# the checker canned text) is the point: it exercises the same path CI takes, so
# a gate that stops reaching the binary's output fails here too.
#
# Case 4 is the vacuous-pass lock: it does not plant drift at all, it makes the
# must-reach surfaces go DARK. A gate that scanned nothing and exited 0 would be
# indistinguishable from a clean run, which is the silent-permissive failure the
# minimum-coverage inventory exists to remove.
#
# Cases 5 and 6 lock the stamped-build PRECONDITION: the gate must refuse to run
# against a binary whose version it cannot trust, rather than degrade to pinning
# literals against binder/dev or a commit hash. Case 6 was added after case 5
# passed locally and failed on CI — see its comment; the two are not redundant.
#
# Case 7 proves the harness is immune to line drift (it locates targets by
# CONTENT, never by line number), and case 8 proves the coverage inventory uses
# minimum COUNTS rather than mere presence — it plants no drift at all and reds
# purely because a surface lost one of the two exemplars it must carry.
#
# Cases 9 and 10 cover the documented-transcript rule: 9 puts a stale literal
# back into docs/tutorial.md and requires the gate to name the file, 10 makes the
# transcripts unfindable WITHOUT introducing drift, so findings stay 0 and the
# red can only come from the coverage floor. Both point the checker at a patched
# copy via its repo-root argument, since the defect is in docs, not in Go.
#
# Case 11 is the certifier's truth table, asserted in BOTH prerelease policies.
# Certification was moved out of the individual gates into
# scripts/lib/stamped_version.py so the helper that BUILDS the binary is also the
# one that decides it is fit to gate against; unifying two predicates is exactly
# the change that can quietly relax one, so the table is run rather than trusted.
#
# Case 12 attacks the DISCOVERER rather than a document, and it exists because
# every other case here attacks a document. Round-1 review found that the breadth
# sweep — the one scanner with no floor — could be collapsed to zero commands by
# rewording Cobra's "Available Commands:" header, after which the gate scanned 4
# surfaces instead of 19 and EXITED 0. The failure mode was reporting a SMALLER
# UNIVERSE rather than reporting a finding, which no document-level case can see.
#
# Case 14 is case 12's harder half and comes from round-2 review. Case 12 kills
# discovery outright, which a cardinality floor can catch. Case 14 kills only the
# RECURSION: the root block still parses, all 11 top-level commands are still
# found, and the total looks entirely plausible — while every `config`
# subcommand has gone dark. The previous floor named three TOP-LEVEL commands and
# required 8, so it exited 0 on this with a quarter of the tree unscanned, while
# the comment above the constant claimed it covered exactly this case. A
# cardinality floor cannot catch a collapse that stays above it, so depth is now
# asserted by IDENTITY on nested paths.
#
# Case 13 locks `--fix`, which rewrites the documented transcripts to the
# binary's own derived output so a release does not need a hand edit (#60 AC3).
# It asserts the postcondition — red, then repair, then a CLEAN re-run against
# the new version — not merely that the command exited.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CHECKER="$SCRIPT_DIR/check-shipped-version-literals.py"
cd "$REPO_ROOT"

# shellcheck source=scripts/lib/stamped-binder.sh
source "$REPO_ROOT/scripts/lib/stamped-binder.sh"

# Resolve the tag ONCE, here, where .git exists. The throwaway copies below have
# no .git, so `git describe` cannot run in them; exporting the resolved value
# makes every case stamp identically to CI.
BINDER_STAMP_VERSION="$(stamp_version)"
export BINDER_STAMP_VERSION

PASS=0
FAIL=0

# The number of cases this harness MUST run, asserted at the end against
# PASS+FAIL so the harness cannot itself pass vacuously — same reasoning as
# scripts/check-transcript-versions-fixtures.sh. Update when adding a case.
EXPECTED_CASES=15

# copy_tree — copy the working tree (minus VCS and node_modules) into a fresh
# temp dir and echo its path. The copy is what gets patched, so the real tree is
# never modified and an interrupted run cannot leave it dirty.
copy_tree() {
  local tmp
  tmp="$(scratch_dir)"
  tar -c --exclude=./.git --exclude=./node_modules --exclude=./bin -C "$REPO_ROOT" . \
    | tar -x -C "$tmp"
  echo "$tmp"
}

# patch — substitute every occurrence of a CONTENT anchor, and FAIL LOUD unless
# the file actually changed as a result.
#
# ASSERT THE POSTCONDITION, NOT THE CALL. Checking that the anchor was *found*
# tests grep; checking that the bytes on disk *differ afterwards* tests the
# thing we actually depend on. Those come apart in a way that bites: a
# substitution whose `from` and `to` coincide, or a replace that silently does
# nothing, leaves a case testing a pristine tree while reporting PASS. That is
# fail-SAFE for a case expecting a RED (it just goes green and we notice) but
# fail-SILENT for a case expecting a GREEN, which would then pass vacuously.
#
# Anchors are content, never line numbers, so inserting lines above a target
# changes nothing — case [7] proves it rather than asserting it.
patch() {
  local file="$1" from="$2" to="$3"
  if ! grep -qF -- "$from" "$file"; then
    echo "  SETUP-FAIL  anchor not found in ${file#"$REPO_ROOT"/}: $from"
    echo "              (the code moved; update this harness to match)"
    FAIL=$((FAIL + 1))
    return 1
  fi
  if ! python3 - "$file" "$from" "$to" <<'PY'
import sys
path, frm, to = sys.argv[1], sys.argv[2], sys.argv[3]
before = open(path).read()
after = before.replace(frm, to)
if after == before:
    sys.stderr.write("mutation was a no-op\n")
    sys.exit(1)
open(path, "w").write(after)
PY
  then
    echo "  SETUP-FAIL  patch did not change ${file#"$REPO_ROOT"/}"
    echo "              (anchor matched but the mutation was a no-op; this case"
    echo "               would have tested a pristine tree and reported PASS)"
    FAIL=$((FAIL + 1))
    return 1
  fi
}

# patch_unique — patch, but the anchor must match EXACTLY ONE line. Use where a
# case depends on hitting one specific occurrence and would be meaningless if a
# refactor introduced a second: "replace them all" and "replace the one I meant"
# are different intents, and only one of them should survive duplication.
patch_unique() {
  local file="$1" from="$2" to="$3" n
  n="$(grep -cF -- "$from" "$file" || true)"
  if [ "$n" -ne 1 ]; then
    echo "  SETUP-FAIL  anchor matched $n line(s), expected exactly 1, in" \
         "${file#"$REPO_ROOT"/}: $from"
    FAIL=$((FAIL + 1))
    return 1
  fi
  patch "$file" "$from" "$to"
}

# assert_case <label> <tree> <expected-exit> [want-substring]
# Builds a stamped binder from <tree> and runs the checker against it.
assert_case() {
  local label="$1" tree="$2" expected="$3" want="${4:-}"
  local bin out actual
  if ! bin="$(cd "$tree" && build_stamped_binder 2>/dev/null)"; then
    echo "  FAIL  $label: stamped build failed"
    FAIL=$((FAIL + 1))
    return
  fi
  out="$(python3 "$CHECKER" "$bin" 2>&1)"
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

echo "==> check-shipped-version-literals fixture harness (issue #60)"
echo "    stamping every case as ${BINDER_STAMP_VERSION}"

# Anchors in internal/config/config.go, which is where all four shipped surfaces
# get their exemplar from. Patching one place reaches help text and both error
# paths, which is itself the property #60's fix established.
EXEMPLAR_CALL='version.ActorExemplar()'
VERSION_IMPORT='"github.com/ghchinoy/binder/internal/version"'

# blank_version_import — Go rejects an unused import, so a case that removes
# every version.ActorExemplar() call from config.go must also blank the import
# or the build fails and the case proves nothing. Blanking (rather than deleting)
# keeps the patch a single content-anchored substitution.
blank_version_import() {
  patch "$1" "	$VERSION_IMPORT" "	_ $VERSION_IMPORT"
}

# prepend_filler <file> <n> — insert n comment lines directly after the package
# clause, shifting every subsequent line down by n. Used by case [7] to prove
# the harness locates its targets by CONTENT and is immune to line drift.
prepend_filler() {
  local file="$1" n="$2"
  python3 - "$file" "$n" <<'PY'
import re, sys
path, n = sys.argv[1], int(sys.argv[2])
s = open(path).read()
m = re.search(r"^package \w+$", s, re.M)
if not m:
    sys.stderr.write(f"SETUP-FAIL: no package clause in {path}\n")
    sys.exit(1)
filler = "\n" + "".join(f"// line-shift filler {i}\n" for i in range(n))
open(path, "w").write(s[:m.end()] + filler + s[m.end():])
PY
}

# [1] unpatched tree -> GREEN. Establishes that a correct tree passes, so the
#     RED cases below are attributable to the planted defect and not to noise.
CLEAN="$(copy_tree)"
assert_case "clean tree -> exit 0" "$CLEAN" 0 "0 drift finding(s)"
rm -rf "$CLEAN"

# [2] the #60 defect verbatim: a hand-maintained literal back in the shared hint.
#     This is the exact state of main at v0.5.3 — the gate must red on it.
STALE="$(copy_tree)"
if patch "$STALE/internal/config/config.go" "$EXEMPLAR_CALL" '"binder/0.3.0"' \
   && blank_version_import "$STALE/internal/config/config.go"; then
  assert_case "stale binder/0.3.0 literal -> exit 1" "$STALE" 1 "[DRIFT]"
fi
rm -rf "$STALE"

# [3] a literal that is CURRENT but v-prefixed. PR #52 banned the leading v on
#     the producer string, so this is wrong at every release, not merely stale —
#     and a drift-only comparison would miss it, since the digits match.
VPREFIX="$(copy_tree)"
if patch_unique "$VPREFIX/internal/version/version.go" \
         'return Producer + "/" + current' 'return Producer + "/v" + current'; then
  assert_case "v-prefixed producer version -> exit 1" "$VPREFIX" 1 "[V-PREFIXED]"
fi
rm -rf "$VPREFIX"

# [4] VACUOUS-PASS LOCK: no drift planted; the must-reach surfaces simply stop
#     emitting an exemplar. Findings will be zero. The gate must still red, via
#     the minimum-coverage inventory, because zero findings over zero inspected
#     text is not a pass.
DARK="$(copy_tree)"
if patch "$DARK/internal/config/config.go" "$EXEMPLAR_CALL" '""' \
   && blank_version_import "$DARK/internal/config/config.go"; then
  assert_case "must-reach surfaces go dark -> exit 1" "$DARK" 1 "MISSING-COVERAGE"
fi
rm -rf "$DARK"

# [5] an UNSTAMPED binary must abort with exit 2, not silently compare literals
#     against binder/dev. Degrading to a no-op when the precondition is unmet is
#     the failure mode #169 documented and this gate inherits the refusal.
UNSTAMPED="$(scratch_dir)/binder"
go build -o "$UNSTAMPED" . 2>/dev/null
UNSTAMPED_OUT="$(python3 "$CHECKER" "$UNSTAMPED" 2>&1)"
UNSTAMPED_EXIT=$?
if [ "$UNSTAMPED_EXIT" -eq 2 ] && echo "$UNSTAMPED_OUT" | grep -qF "did not certify"; then
  echo "  PASS  unstamped binary -> exit 2 (refuses to run)"
  PASS=$((PASS + 1))
else
  echo "  FAIL  unstamped binary: expected exit 2 + 'did not certify', got exit $UNSTAMPED_EXIT"
  echo "$UNSTAMPED_OUT" | sed 's/^/        | /'
  FAIL=$((FAIL + 1))
fi

# [6] a PSEUDO-VERSION stamp must also abort with exit 2. This case exists
#     because case 5 did NOT catch the hole on its own: `go build` reports a
#     pseudo-version only inside a VCS checkout, and its exact shape depends on
#     whether the tree is dirty. Locally the `+dirty` suffix made the
#     precondition refuse, so case 5 passed; on CI the tree was clean, the
#     suffix was absent, and the same binary sailed through — the gate pinned
#     literals to `0.5.4-0.20260914164646-656b05e16068` and reported green.
#
#     Case 5 is therefore environment-dependent by construction. This one stamps
#     the pseudo-version EXPLICITLY, so the refusal is locked regardless of how
#     the tree or the checkout happens to look.
PSEUDO="$(scratch_dir)/binder"
go build -ldflags \
  "-X github.com/ghchinoy/binder/cmd.Version=0.5.4-0.20260914164646-656b05e16068" \
  -o "$PSEUDO" . 2>/dev/null
PSEUDO_OUT="$(python3 "$CHECKER" "$PSEUDO" 2>&1)"
PSEUDO_EXIT=$?
if [ "$PSEUDO_EXIT" -eq 2 ] && echo "$PSEUDO_OUT" | grep -qF "did not certify"; then
  echo "  PASS  pseudo-version stamp -> exit 2 (refuses to run)"
  PASS=$((PASS + 1))
else
  echo "  FAIL  pseudo-version stamp: expected exit 2 + 'did not certify', got exit $PSEUDO_EXIT"
  echo "$PSEUDO_OUT" | sed 's/^/        | /'
  FAIL=$((FAIL + 1))
fi

# [7] LINE-SHIFT IMMUNITY. Case [2] again, with 40 lines inserted ABOVE the
#     anchor first. Same planted defect, same expected finding.
#
#     Required by the EM after #185's harness had its hand-maintained fixture
#     line addresses go stale THREE times — most recently against #142's +6
#     lines in user_guide.md, which produced a textually clean but semantically
#     broken harness that still passed CI because it tested an older base. That
#     is the worst shape a gate can fail in: green while measuring nothing.
#
#     This harness never had line addresses — patch() matches a content anchor
#     and hard-FAILs when it is absent, and the checker reads the binary's
#     OUTPUT, where source line numbers do not exist as a concept. This case
#     exists so that property is DEMONSTRATED rather than merely asserted, and
#     so that anyone who later reaches for a line number here breaks a test.
SHIFTED="$(copy_tree)"
if prepend_filler "$SHIFTED/internal/config/config.go" 40 \
   && patch "$SHIFTED/internal/config/config.go" "$EXEMPLAR_CALL" '"binder/0.3.0"' \
   && blank_version_import "$SHIFTED/internal/config/config.go"; then
  assert_case "stale literal, 40 lines inserted above -> exit 1 (unchanged)" \
    "$SHIFTED" 1 "[DRIFT]"
fi
rm -rf "$SHIFTED"

# [8] PARTIAL-LOSS LOCK — the case that justifies minimum COUNTS over mere
#     presence, which is the inventory shape #185 landed and this gate adopted.
#
#     Each help surface carries TWO exemplars: the --verified-by usage string's
#     own example, and the one inside the shared ActorFormsHint() appended to
#     it. Here the first is removed and the second left intact. Every must-reach
#     surface therefore still emits at least one literal, so a PRESENCE-ONLY
#     inventory would report full coverage and exit 0 over a real regression.
#     The count catches it: the two help surfaces emit 1 where 2 is declared.
#
#     Note this plants no drift at all — findings stay 0. The red comes purely
#     from the coverage floor, which is the whole point.
PARTIAL="$(copy_tree)"
if patch_unique "$PARTIAL/internal/config/config.go" \
     'version.ActorExemplar() + "\"; a stamp is written ONLY' \
     '"" + "\"; a stamp is written ONLY'; then
  assert_case "one of two exemplars lost -> exit 1 (count, not presence)" \
    "$PARTIAL" 1 "expected at least 2"
fi
rm -rf "$PARTIAL"

# [9] DOC-TRANSCRIPT DRIFT. The docs quote the invalid-actor error verbatim.
#     Part 1 makes that error version-derived, so the transcripts can now go
#     stale — exactly #60's disease relocated into prose that no JSON-fence gate
#     can reach. Here the OLD literal is put back in docs/tutorial.md while the
#     binary stays correct, and the gate must name the file and say so.
#
#     Unlike every case above, this one patches DOCS, not Go, so the binary is
#     clean and the checker is pointed at the patched tree via its repo-root
#     argument.
DOCDRIFT="$(copy_tree)"
if patch_unique "$DOCDRIFT/docs/tutorial.md" \
     '(e.g. binder/'"${BINDER_STAMP_VERSION#v}"')' '(e.g. binder/0.3.0)'; then
  if bin="$(cd "$DOCDRIFT" && build_stamped_binder 2>/dev/null)"; then
    out="$(python3 "$CHECKER" "$bin" "$DOCDRIFT" 2>&1)"; rc=$?
    if [ "$rc" -eq 1 ] && echo "$out" | grep -qF "[DOC-DRIFT]" \
       && echo "$out" | grep -qF "docs/tutorial.md"; then
      echo "  PASS  stale doc transcript -> exit 1 (named by file) (exit $rc)"
      PASS=$((PASS + 1))
    else
      echo "  FAIL  stale doc transcript: expected exit 1 + [DOC-DRIFT] naming" \
           "docs/tutorial.md, got exit $rc"
      echo "$out" | sed 's/^/        | /'
      FAIL=$((FAIL + 1))
    fi
  else
    echo "  FAIL  stale doc transcript: stamped build failed"
    FAIL=$((FAIL + 1))
  fi
fi
rm -rf "$DOCDRIFT"

# [10] DOC-TRANSCRIPT COVERAGE FLOOR. "No mismatches" is worth nothing if the
#      scan found nothing to compare. Here the transcripts are made unfindable
#      without introducing any drift at all, so findings stay 0 — the red must
#      come from the floor. This is the doc rule's vacuous-pass lock, and it is
#      the case that would fail if someone later narrowed the scan or the
#      pattern and assumed green meant clean.
DOCDARK="$(copy_tree)"
python3 - "$DOCDARK" <<'PY'
import sys, os, re

# Mutate every doc transcript the CHECKER would find, then assert the state this
# case actually depends on: that none remain findable. The previous version
# counted the files it edited (`assert n == 2`) against a hard-coded pair — a
# tautology over a 2-element literal, and a CARDINALITY claim standing in for the
# state claim. If a third doc ever grows a transcript, a count says "2, as
# expected" and the case silently stops establishing its premise; the postcondition
# below fails loudly instead. Files are discovered, not listed.
root = sys.argv[1]
DOC_ACTOR = re.compile(r'^binder: invalid actor "(?P<actor>.*?)"; valid forms:')

def transcripts():
    hits = []
    for dirpath, _, names in os.walk(os.path.join(root, "docs")):
        for name in names:
            if not name.endswith(".md"):
                continue
            p = os.path.join(dirpath, name)
            with open(p, encoding="utf-8") as fh:
                for line in fh:
                    if DOC_ACTOR.match(line.strip()):
                        hits.append(p)
                        break
    return hits

before = transcripts()
assert before, "SETUP: no doc transcripts found to darken -- case is vacuous"
for p in before:
    s = open(p, encoding="utf-8").read()
    t = s.replace("binder: invalid actor ", "binder: rejected actor ")
    assert t != s, f"{p}: mutation was a no-op"
    open(p, "w", encoding="utf-8").write(t)

after = transcripts()
assert not after, f"SETUP: transcripts still findable after mutation: {after}"
PY
if [ $? -eq 0 ]; then
  if bin="$(cd "$DOCDARK" && build_stamped_binder 2>/dev/null)"; then
    out="$(python3 "$CHECKER" "$bin" "$DOCDARK" 2>&1)"; rc=$?
    if [ "$rc" -eq 1 ] && echo "$out" | grep -qF "MISSING-COVERAGE" \
       && echo "$out" | grep -qF "0 drift finding(s)"; then
      echo "  PASS  doc transcripts unfindable -> exit 1 (floor, no drift) (exit $rc)"
      PASS=$((PASS + 1))
    else
      echo "  FAIL  doc transcripts unfindable: expected exit 1 +" \
           "MISSING-COVERAGE with 0 drift findings, got exit $rc"
      echo "$out" | sed 's/^/        | /'
      FAIL=$((FAIL + 1))
    fi
  else
    echo "  FAIL  doc transcripts unfindable: stamped build failed"
    FAIL=$((FAIL + 1))
  fi
else
  echo "  SETUP-FAIL  could not blind the doc transcripts"
  FAIL=$((FAIL + 1))
fi
rm -rf "$DOCDARK"

# [12] THE DISCOVERER GOES DARK (round-1 review, R1). Every case above attacks a
#      DOCUMENT; this one attacks the thing that decides what to look at. The
#      breadth sweep finds commands by parsing Cobra's English "Available
#      Commands:" header, so a Cobra upgrade or a reworded help template
#      collapses discovery to zero — and before the floor was added, the gate
#      scanned 4 surfaces instead of 19, found nothing to disagree with, and
#      EXITED 0. A vacuous pass inside the gate built to remove vacuous passes.
#
#      The stand-in is the real stamped binary with ONLY that header reworded, so
#      every must-reach surface still emits its literals and the must-reach floor
#      stays satisfied. The red must therefore come from the discovery floor
#      specifically, which is what makes this case a lock on R1 rather than a
#      restatement of case 4.
DARKDISC="$(scratch_dir)"
if bin="$(build_stamped_binder)"; then
  cat > "$DARKDISC/binder" <<PY
#!/usr/bin/env python3
import subprocess, sys
p = subprocess.run(["$bin"] + sys.argv[1:], capture_output=True, text=True)
sys.stdout.write(p.stdout.replace("Available Commands:", "Subcommands:"))
sys.stderr.write(p.stderr)
sys.exit(p.returncode)
PY
  chmod +x "$DARKDISC/binder"
  # The stand-in must still be a CERTIFIABLE binder, or this case would red for
  # the wrong reason and prove nothing about discovery.
  if [ "$("$DARKDISC/binder" --version)" != "$("$bin" --version)" ]; then
    echo "  SETUP-FAIL  stand-in binary does not report the real version"
    FAIL=$((FAIL + 1))
  elif "$DARKDISC/binder" --help 2>&1 | grep -qF "Available Commands:"; then
    echo "  SETUP-FAIL  stand-in still emits the discovery header; mutation was a no-op"
    FAIL=$((FAIL + 1))
  else
    out="$(python3 "$CHECKER" "$DARKDISC/binder" 2>&1)"; rc=$?
    # Require the DISCOVERY floor by name, and require that no drift finding is
    # what produced the red: discovery went dark without any document changing.
    if [ "$rc" -eq 1 ] && echo "$out" | grep -qF "sweep:discovery" \
       && echo "$out" | grep -qF "0 drift finding(s)"; then
      echo "  PASS  command discovery goes dark -> exit 1 (discovery floor) (exit $rc)"
      PASS=$((PASS + 1))
    else
      echo "  FAIL  command discovery goes dark: expected exit 1 +" \
           "sweep:discovery coverage failure with 0 drift findings, got exit $rc"
      echo "$out" | sed 's/^/        | /'
      FAIL=$((FAIL + 1))
    fi
  fi
else
  echo "  FAIL  command discovery goes dark: stamped build failed"
  FAIL=$((FAIL + 1))
fi
rm -rf "$DARKDISC"

# [13] --fix REPAIRS THE TRANSCRIPTS FROM THE BINARY (round-1 review, R2).
#      #60's AC3 says tutorial.md must stop needing a per-release edit. The gate
#      derives the correct line in order to compare it, so --fix writes that same
#      derived line back. Asserted as a POSTCONDITION: after --fix the tree is
#      clean against the NEW version, and the repair equals the binary's own
#      output rather than anything this harness typed.
FIXTREE="$(copy_tree)"
if bin999="$(cd "$FIXTREE" && BINDER_STAMP_VERSION=v9.9.9 build_stamped_binder 2>/dev/null)"; then
  before="$(python3 "$CHECKER" "$bin999" "$FIXTREE" 2>&1)"; rc_before=$?
  python3 "$CHECKER" "$bin999" "$FIXTREE" --fix >/dev/null 2>&1; rc_fix=$?
  after="$(python3 "$CHECKER" "$bin999" "$FIXTREE" 2>&1)"; rc_after=$?
  # The repaired text must be the binary's, and must mention the new version.
  if [ "$rc_before" -eq 1 ] && echo "$before" | grep -qF "[DOC-DRIFT]" \
     && [ "$rc_fix" -eq 1 ] \
     && [ "$rc_after" -eq 0 ] \
     && grep -qF "binder/9.9.9" "$FIXTREE/docs/tutorial.md"; then
    echo "  PASS  --fix repairs transcripts from the binary -> clean re-run (exit $rc_after)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  --fix: expected red(1) then fix(1) then clean(0) with 9.9.9" \
         "written into tutorial.md; got $rc_before/$rc_fix/$rc_after"
    echo "$after" | sed 's/^/        | /'
    FAIL=$((FAIL + 1))
  fi
else
  echo "  FAIL  --fix: stamped build failed"
  FAIL=$((FAIL + 1))
fi
rm -rf "$FIXTREE"

# [14] THE DISCOVERER GOES HALF-DARK (round-2 review, R4). Case 12 kills
#      discovery outright and the count floor catches it. This kills only the
#      RECURSION: the root block still parses, so the sweep still finds all 11
#      top-level commands and reports a perfectly plausible total — while every
#      `config` subcommand has gone dark.
#
#      That is the case the previous floor claimed to cover and did not. It named
#      three TOP-LEVEL commands and required 8 discovered; a depth collapse
#      leaves 11 of 15, still includes top-level `config`, and clears 8. THE
#      GATE EXITED 0 WITH A QUARTER OF THE COMMAND TREE UNSCANNED, and the
#      comment above the constant asserted the opposite. A cardinality floor
#      cannot catch a collapse that stays above it.
#
#      The fix is identity at DEPTH: `config set` is reachable only by recursing,
#      so requiring it is a direct claim that the recursion ran. This case locks
#      that, and the ablation below proves it reds against the PREVIOUS checker.
HALFDARK="$(scratch_dir)"
if bin="$(build_stamped_binder)"; then
  # Reword the header ONLY for subcommand help, never for the root. Root
  # discovery therefore succeeds completely and the collapse is depth-only.
  cat > "$HALFDARK/binder" <<PY
#!/usr/bin/env python3
import subprocess, sys
p = subprocess.run(["$bin"] + sys.argv[1:], capture_output=True, text=True)
out = p.stdout
if sys.argv[1:2] not in ([], ["--help"], ["-h"]):
    out = out.replace("Available Commands:", "Subcommands:")
sys.stdout.write(out)
sys.stderr.write(p.stderr)
sys.exit(p.returncode)
PY
  chmod +x "$HALFDARK/binder"
  # Three setup guards. Without all three this case can pass for a reason that
  # has nothing to do with depth.
  if [ "$("$HALFDARK/binder" --version)" != "$("$bin" --version)" ]; then
    echo "  SETUP-FAIL  half-dark stand-in does not report the real version"
    FAIL=$((FAIL + 1))
  elif ! "$HALFDARK/binder" --help 2>&1 | grep -qF "Available Commands:"; then
    echo "  SETUP-FAIL  root help lost its header; this is case 12, not a DEPTH collapse"
    FAIL=$((FAIL + 1))
  elif "$HALFDARK/binder" config --help 2>&1 | grep -qF "Available Commands:"; then
    echo "  SETUP-FAIL  subcommand help unchanged; the mutation was a NO-OP"
    FAIL=$((FAIL + 1))
  else
    out="$(python3 "$CHECKER" "$HALFDARK/binder" 2>&1)"; rc=$?
    # Require the NESTED path by name. Asserting only "sweep:discovery" would
    # also pass on a total collapse, which case 12 already covers.
    if [ "$rc" -eq 1 ] && echo "$out" | grep -qF "sweep:discovery" \
       && echo "$out" | grep -qF "config set" \
       && echo "$out" | grep -qF "0 drift finding(s)"; then
      echo "  PASS  recursion collapses, breadth intact -> exit 1 naming a nested path (exit $rc)"
      PASS=$((PASS + 1))
    else
      echo "  FAIL  half-dark discovery: expected exit 1 + sweep:discovery naming" \
           "\`config set\` with 0 drift findings, got exit $rc"
      echo "$out" | sed 's/^/        | /'
      FAIL=$((FAIL + 1))
    fi
  fi
else
  echo "  FAIL  half-dark discovery: stamped build failed"
  FAIL=$((FAIL + 1))
fi
rm -rf "$HALFDARK"

# [11] THE CERTIFIER'S TRUTH TABLE, in both policy modes.
#      Certification moved out of the two gates and into
#      scripts/lib/stamped_version.py, shared with the build helper. Unifying two
#      predicates is exactly the change that can quietly RELAX one of them, so
#      the table asserts every row under BOTH prerelease policies and is run
#      here, in CI, rather than trusted.
#
#      The rows that matter most: a pseudo-version and `+dirty` metadata must be
#      rejected under BOTH modes. #169's old predicate rejected pseudo-versions
#      only as a side effect of banning all suffixes, and #60's must permit
#      prerelease — so this is the "reds before AND after" property, in table
#      form, and it is what makes the shared precondition safe to adopt.
if python3 "$REPO_ROOT/scripts/lib/stamped_version.py" --self-test; then
  echo "  PASS  certifier truth table (both policy modes)"
  PASS=$((PASS + 1))
else
  echo "  FAIL  certifier truth table"
  FAIL=$((FAIL + 1))
fi

# [15] THE BUILD HELPER LEAVES NOTHING BEHIND.
#      Every stamped build is a ~28 MB binary in a temp directory. The helper
#      created one per call and removed none, which is invisible until it isn't:
#      a local acceptance run of the two harnesses reported "stamped build
#      failed" six times, reading exactly like a gate regression, and the actual
#      cause was `no space left on device` from 399 leaked directories. A gate
#      that reds for a reason unrelated to the code under test is worse than one
#      that stays quiet, because it spends the reader's trust.
#
#      ASSERT THE POSTCONDITION, NOT THE CALL. Grepping the helper for `trap`
#      would pass on a trap that is installed and never fires — which is the
#      precise bug, since bash resets traps inside the command-substitution
#      subshell the helper is always called from. So: run a CHILD shell that
#      sources the helper, builds, and prints its temp root; let the child EXIT;
#      then assert from out here that the root is gone and the binary with it.
#      Nothing but real cleanup satisfies that.
CHILD="$(scratch_file)"
cat > "$CHILD" <<'CHILDEOF'
set -euo pipefail
cd "$1"
. scripts/lib/stamped-binder.sh
bin="$(build_stamped_binder 2>/dev/null)"
[ -x "$bin" ] || { echo "CHILD-BUILD-FAILED"; exit 3; }
printf '%s\n%s\n' "$_STAMPED_BINDER_TMPROOT" "$bin"
CHILDEOF
child_out="$(bash "$CHILD" "$REPO_ROOT" 2>/dev/null)"; child_rc=$?
child_root="$(echo "$child_out" | sed -n 1p)"
child_bin="$(echo "$child_out" | sed -n 2p)"
if [ "$child_rc" -ne 0 ] || [ -z "$child_root" ]; then
  echo "  FAIL  temp-root cleanup: child shell did not build (rc $child_rc)"
  echo "$child_out" | sed 's/^/        | /'
  FAIL=$((FAIL + 1))
elif [ -z "$child_bin" ] || case "$child_bin" in "$child_root"/*) false ;; *) true ;; esac then
  # If the binary were built OUTSIDE the root, the root could vanish while the
  # leak continued, and this case would certify the opposite of the truth.
  echo "  SETUP-FAIL  temp-root cleanup: binary '$child_bin' is not under root '$child_root'"
  FAIL=$((FAIL + 1))
elif [ -e "$child_root" ]; then
  echo "  FAIL  temp-root cleanup: '$child_root' still exists after the sourcing shell exited"
  FAIL=$((FAIL + 1))
else
  echo "  PASS  stamped-build temp root removed when the sourcing shell exits"
  PASS=$((PASS + 1))
fi
rm -f "$CHILD"

echo
# Vacuous-pass guard for the harness itself: assert the expected number of cases
# actually ran BEFORE reporting success, so a harness that skipped everything
# cannot print "OK: 0 passed" and exit 0.
TOTAL=$((PASS + FAIL))
if [ "$TOTAL" -ne "$EXPECTED_CASES" ]; then
  echo "FAILED: ran $TOTAL case(s) but expected $EXPECTED_CASES — cases were" \
       "skipped, reordered, or their setup anchors moved. A harness that runs" \
       "nothing must not report success."
  exit 1
fi

if [ "$FAIL" -eq 0 ]; then
  echo "OK: all $PASS of $EXPECTED_CASES fixture case(s) passed."
  exit 0
else
  echo "FAILED: $FAIL of $TOTAL fixture case(s) did not match."
  exit 1
fi
