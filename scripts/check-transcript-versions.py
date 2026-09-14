#!/usr/bin/env python3
"""Version-literal drift gate for the documented JSON transcripts (issue #169).

Covers `plugins/`, `docs/`, and `README.md` (issue #185 widened the scan from
`plugins/` only — the same transcript envelopes live in the user guide and the
README, and until #185 nothing pinned them; they had drifted two minor versions).

The in-process `internal/plugindocs` gate enforces key-set equality for every
JSON transcript under `plugins/**/*.md` but deliberately does NOT check the
`binder/<version>` value: a `go test` build is unstamped and reports
`binder/dev`, so inside the unit gate there is no trustworthy current version to
compare against (see the "KNOWN LIMIT" note in internal/plugindocs/drift_test.go).

This script closes that gap from OUTSIDE the unit gate. It requires a STAMPED
binder (real release tag injected via goreleaser ldflags or `git describe`),
captures `binder --version`, and asserts that the documented version literals
track it. It is wired into CI as a separate step, not bolted onto `go test`.

# Why this is JSON-scoped (the false-positive trap this avoids)

A blanket "every `binder/X.Y.Z` literal must equal current" check over the docs
is WRONG: the docs legitimately retain older-version references that must NOT
track the release — minimum-version floors, historical "as of binder/0.3.1"
notes, and a "measured-with" label (six such references live today, all
`binder/0.3.1`). Telling a stale capture-provenance claim (must track) from a
min-version floor (must not) is a semantic judgment, not a mechanical one.

So this gate is scoped so it STRUCTURALLY cannot touch the prose references:

  1. Envelope literals: only `binder/X.Y.Z` literals that appear INSIDE a fenced
     JSON block (```json / ```jsonc / ```json5) are pinned. Every one of these is
     a transcript envelope's `"binder"` field and MUST equal the producing
     binary. The six prose references live in prose or in a ```bash fence and are
     never scanned.
  2. Prose provenance: exactly ONE prose sentence is a capture-provenance claim
     ("... was captured from real `binder/X.Y.Z` output ..."). It is pinned by a
     narrow, targeted pattern that matches that sentence and nothing else — not
     the floor/historical/measured-with phrasings.

# Minimum-coverage assertion (why this gate cannot pass vacuously)

# "0 findings" is only trustworthy if the gate actually reached the literals it
# is supposed to pin. A version gate that silently inspects nothing and exits 0
# is the exact silent-permissive failure #169 exists to remove. So the gate
# carries an EXPLICIT inventory of the must-track locations and asserts each was
# visited and checked; a broken discovery path (empty/wrong base, moved file,
# renamed fence tag, changed glob) fails LOUD instead of passing green.

Exit non-zero if any pinned literal drifts, if any must-track location does not
yield exactly the declared number of literals (vacuous-pass guard), or if any
allowlist entry exempted nothing (reachability guard); exit 0 only when every
must-track location was visited in full, every checked literal matches the
stamped binary, and every declared exemption was actually used.

Usage: check-transcript-versions.py <base-dir> <stamped-binder-binary>

<base-dir> is the repo root (or a throwaway copy of it, as the fixture harness
builds). The directories and files actually scanned are SCAN_ROOTS below, not
whatever *.md happens to live under the base: scanning the whole tree would drag
in testdata/ and .design/ notes, whose version literals are historical and must
not track the release.
"""
import collections
import re
import subprocess
import sys
import pathlib

# Fenced JSON blocks. Language tag is one of json / jsonc / json5; fences may be
# indented inside list items, so do not anchor the backticks at column 0. The
# closing fence must have the same-or-compatible indentation but we accept any
# ```-only line as the terminator (matches the in-process gate's tolerance).
#
# FENCE MODEL (FYI, round-2 review #2): this is "JSON-fenced vs everything-else",
# NOT "fenced vs prose". Only json/jsonc/json5 flip in_json; a ```bash fence is
# treated as not-in_json. That is deliberate and load-bearing — it is *why* the
# bash-fenced min-version-floor line `binder --version # need binder/0.3.1` is
# correctly excluded from the JSON-envelope check.
FENCE_OPEN = re.compile(r"^[ \t]*```(json[c5]?)[ \t]*$")
FENCE_CLOSE = re.compile(r"^[ \t]*```[ \t]*$")

# A binder version literal: binder/<major>.<minor>.<patch>.
VERSION_LITERAL = re.compile(r"binder/(\d+\.\d+\.\d+)")

# The single capture-provenance prose sentence. This deliberately matches ONLY
# the "captured from real `binder/X.Y.Z` output" claim — a stale provenance
# statement that MUST track the release — and structurally cannot match the
# minimum-version-floor / historical / measured-with phrasings, which never use
# this wording.
#
# KNOWN LIMIT (#176): this narrow pattern covers only the one known provenance
# wording. A NEW capture-provenance sentence phrased differently (e.g. "sampled
# from binder/X.Y.Z") would be a must-track claim this gate would NOT catch. That
# is a deliberate, documented limit of the JSON-scoped / narrow-prose approach
# #169 prescribes (broadening it risks false-positiving the six historical prose
# refs), tracked in #176 rather than left as tribal knowledge.
PROSE_PROVENANCE = re.compile(r"captured from real `binder/(\d+\.\d+\.\d+)` output")

# PROSE RULE ORDER — THERE ISN'T ONE, DELIBERATELY (#185 round 2, O1).
#
# Two rules read the same prose line: PROSE_PROVENANCE (pins the one
# capture-provenance sentence) and NO_UNPINNED_PROSE (reports any other literal
# in a listed file). They used to be chained — a provenance match `continue`d,
# SHADOWING the second rule for the rest of that line, so a line carrying a
# correct provenance sentence AND a stale literal produced zero findings. The
# result depended on which rule ran first, which is a property of the code's
# shape rather than of the rules.
#
# So each literal is now classified exactly once, by WHERE IT IS rather than by
# WHICH RULE RAN FIRST: the provenance pass records the character span of every
# literal it pins, and the no-unpinned-prose pass skips literals inside those
# spans and reports the rest. Adding a third prose rule means adding another
# pass that contributes spans; it cannot silence an existing rule by running
# before it, and no rule needs to know the others exist. Fixture case [15] is
# the shadowing line, and expects both the finding and the count.
#
# RESIDUAL, narrower than the hole above (#185 round 4, O6): ordering is gone,
# but a pass can still silence a finding by CLAIMING. Appending to pinned_spans
# and comparing the literal against the stamped binary are two statements tied
# together only by convention, so a future pass that claims a span for a literal
# it does not actually check reproduces the silent hole by a different door. Two
# passes do not justify the machinery to prevent it; when a third arrives, have
# each pass return (span, finding_or_None) so claiming and checking are one
# statement and the convention becomes a signature.
#
# Schema literal inside a JSON envelope, used to classify which must-track
# envelope a discovered version literal belongs to (report vs config).
#
# ONE-SCHEMA-PER-BLOCK ASSUMPTION (Optional-2, round-3 review): .search() takes
# the FIRST schema in a block. That is correct because each envelope is its own
# fenced block — no block contains two envelopes today. If that ever changes,
# only the first schema would classify the block and only its coverage key would
# be credited; split such a block into one envelope per fence rather than relaxing
# this.
SCHEMA_RE = re.compile(r'"schema"\s*:\s*"([\w.]+/v\d+)"')

# SCAN ROOTS (#185). Base-relative directories (scanned recursively for *.md) and
# individual files. `plugins/` was the original and only root; `docs/` and
# `README.md` were added by #185 after nine envelopes there drifted to
# binder/0.3.0 unnoticed. Everything else is deliberately OUT: testdata/ golden
# files and docs/examples/ YAML stamps carry frozen historical versions, and
# CHANGELOG.md is history by definition. A missing root is a coverage failure,
# not a silent skip.
#
# GENERATED COMMAND DOCS ARE NOT THIS GATE'S (#185 round 2, from the #196 read).
# `docs/commands/*.md` are produced from the binary by internal/gendocs, so the
# version literals in them — `--verified-by ... e.g. binder/0.3.0` in
# binder_convert.md and binder_enrich.md today — are the BINARY'S OUTPUT, quoted.
# They belong to the shipped-output gate (#60), which runs the binary and checks
# what it prints, and regenerating those files is what updates them. This gate
# cannot see them today only because they are prose rather than JSON-fenced —
# true by accident, not by decision. Writing the decision down: do not add a
# prose rule, a NO_UNPINNED_PROSE entry, or a coverage entry for docs/commands/
# here. Two gates with an opinion about the same bytes is a contested-ownership
# argument later, and the accident that currently prevents it is not load-bearing.
SCAN_ROOTS = ["plugins", "docs", "README.md"]

# NO-UNPINNED-PROSE FILES (#185 follow-on). Base-relative files in which a
# `binder/X.Y.Z` literal OUTSIDE a JSON fence is itself a finding.
#
# This is the blanket "every literal must equal current" check #169 rejected —
# but applied to ONE file where the objection does not hold. `plugins/` and
# `docs/` legitimately carry version literals that must not track the release
# (minimum-version floors, historical notes, a measured-with label, actor-grammar
# examples like `--verified-by binder/0.3.0`), which is why they are not listed
# here. README.md carries none of those: the two prose literals it did carry
# ("go install … prints binder/0.3.0" and "the complete v0.3.0 surface") were
# simply stale claims, found by review after #185 was filed, and both were fixed
# by REMOVING the literal — `binder/<version>` and "the complete surface" cannot
# go stale. This entry keeps that true. The fix for a hit is the same: write the
# placeholder form, or make it a real JSON envelope so the pinned check applies.
NO_UNPINNED_PROSE = {"README.md"}

# THE ESCAPE HATCH — READ THIS BEFORE DELETING THE RULE ABOVE.
#
# The justification for NO_UNPINNED_PROSE is that README carries no literal that
# legitimately must NOT track the release. That is true today; nothing makes it
# true forever. The day someone writes a genuine historical reference into README
# prose ("binder/0.2.1 was the first release to ...", a minimum-version floor, an
# actor-grammar example), this rule goes RED on correct content. That is the
# rule's KNOWN false-positive shape, and it should be expected eventually.
#
# When it happens the remedy is an entry HERE — (base-relative path, version) ->
# a reason — NOT deletion of the rule. Deletion is the cheap move and it silently
# takes two things with it: the drift protection on every other README prose
# literal, and the mechanical enforcement of the ruling that #192's `(v0.3.0)`
# validator pin be dropped rather than tracked. An allowlist entry costs one line
# and leaves a reviewable record of WHY one literal is exempt; removing the rule
# leaves no record of anything.
#
# An entry is a claim that this specific literal must not track the release, and
# it should say why in the same terms #169 uses: floor, historical note, or
# measured-with label. Keep it narrow — the version is part of the key, so
# exempting 0.2.1 does not exempt the next stale 0.5.x someone pastes in.
#
# Intended shape (illustrative, not a live entry):
#     ("README.md", "0.2.1"): "historical: first release to ship checksums.txt",
#
# EVERY ENTRY MUST BE REACHED (#185 round 4, R6). An entry is checked two ways,
# and the second one exists because the first has a blind side:
#
#   - forward, at startup: an entry for the CURRENT stamped version is refused
#     (exit 2), because such an entry is not an exemption for one historical
#     literal, it is a permanent one that switches on the day this release stops
#     being current.
#   - backward, after the scan: an entry that exempted NOTHING is a finding. A
#     version already below the stamp can never become current, so the startup
#     guard can never fire on it — and an unused entry is not inert. It sits
#     there pre-approving a literal that does not exist yet, so the day someone
#     pastes `binder/0.2.1` into README prose as a fresh false claim, the rule is
#     already switched off over it. A silent permanent exemption, displaced in
#     time. Reachability removes the dormancy: the entry cannot survive the
#     interval between being written and being used, because with nothing to
#     exempt it is red from the moment it lands.
#
# What reachability does NOT close, stated so it is not mistaken for more: an
# entry added in the SAME commit as the literal it exempts is reached, and passes.
# That is not a hole a checker can close — the entry states a reason and the diff
# shows both halves, which makes it exactly the reviewable claim the allowlist is
# for. The hole was the dormant entry, and that one is closed.
#
# This is the same assertion EXPECTED_COVERAGE makes (O3): a declaration that can
# drift out of correspondence with the tree without saying so is not a
# declaration. The allowlist had no correspondence assertion at all.
NO_UNPINNED_PROSE_ALLOW = {}

# MINIMUM-COVERAGE INVENTORY (#169 Critical, round-2 review). The must-track
# locations, each as (base-relative path, coverage-key, minimum literal count,
# human label). The gate asserts every one of these was actually visited and at
# least that many literals checked there, so it can tell "0 findings because
# everything is correct" apart from "0 findings because I parsed nothing". This
# is an explicit per-location inventory, NOT a bare aggregate floor: a floor like
# `blocks >= 10` can be satisfied coincidentally while one specific file's
# discovery is broken (one file loses blocks as another gains them). The
# inventory catches that; a floor does not.
#
# Keys are base-RELATIVE PATHS, not basenames (Optional-1, round-3 review): two
# files sharing a basename in different directories must not let one satisfy the
# other's coverage entry — a filename collision defeating the inventory would be
# the same vacuous-pass class, reachable by a different route.
#
# The per-location COUNT (added with the #185 roots) is what makes presence
# meaningful in a file holding several envelopes: `docs/user_guide.md` carries
# four report envelopes, so bare presence would still be satisfied after three of
# them lost their fence tag.
#
# The count is EXACT, not a floor (#185 round 2, O3). A floor is silent about
# ADDING a transcript, which sounds harmless and is not: the inventory then
# drifts away from the tree it is supposed to describe, unreviewed, and the
# declared numbers stop meaning anything. Exact makes adding a transcript a loud,
# one-line event — update the number in the same commit that adds the block, and
# the reviewer sees the count change.
#
# KNOWN LIMIT, stated rather than papered over: this is CARDINALITY, not
# identity. Adding one transcript while hiding another in the same file and the
# same commit nets zero and passes, under an exact count exactly as under a
# floor — cardinality cannot distinguish four literals from a different four.
# Exact narrows it (the add alone is now loud, so the pair has to be
# simultaneous), but only per-literal identity would close it. Tracked in #197
# rather than left as tribal knowledge — a KNOWN LIMIT with no number attached is
# a deferral with no expiry, which is #197's own argument and applies to the
# comment that filed it. Documented here so nobody mistakes the guard for more
# than it is; see #197 for the cost analysis and the candidate keying.
_SKILL = "plugins/okf-convert/skills/okf-convert/SKILL.md"
_CONTRACT = "plugins/okf-convert/skills/okf-convert/references/binder-json-contract.md"
_GUIDE = "docs/user_guide.md"
_README = "README.md"
EXPECTED_COVERAGE = [
    (_SKILL, "envelope:binder.report/v1", 1, "convert report envelope literal"),
    (_CONTRACT, "envelope:binder.report/v1", 1, "report envelope literal"),
    (_CONTRACT, "envelope:binder.config/v1", 1, "config envelope literal"),
    (_CONTRACT, "prose-provenance", 1, "prose provenance sentence"),
    (_GUIDE, "envelope:binder.report/v1", 4,
     "user guide report envelopes (envelope shape, enrich, list_graphs, query_graph)"),
    (_GUIDE, "envelope:binder.config/v1", 4,
     "user guide config envelopes (config list/get/set/unset)"),
    (_README, "envelope:binder.report/v1", 1, "README validate envelope literal"),
]


def main() -> int:
    if len(sys.argv) != 3:
        sys.stderr.write(__doc__.strip().splitlines()[-1] + "\n")
        return 2
    base = pathlib.Path(sys.argv[1])
    binder = sys.argv[2]

    # check=True (FYI, round-2 review #3): if the binary runs but exits non-zero,
    # raise CalledProcessError rather than reading a partial/empty --version. A
    # bad path already aborts with FileNotFoundError, and an empty stdout is
    # rejected by the stamped-build guard below; check=True closes the last
    # quiet-failure path (non-zero exit with some stdout) so every failure mode
    # is loud.
    expected = subprocess.run(
        [binder, "--version"], capture_output=True, text=True, check=True
    ).stdout.strip()

    # Refuse to run against an unstamped build. An unstamped binder reports
    # `binder/dev`, and pinning literals to `binder/dev` would either misflag
    # every correct release literal or (if special-cased) silently degrade the
    # gate to a no-op. That degradation is exactly the failure mode #169 warns
    # against, so fail LOUD instead.
    if not re.fullmatch(r"binder/\d+\.\d+\.\d+", expected):
        sys.stderr.write(
            f"FATAL: `{binder} --version` returned {expected!r}, not a stamped "
            f"release version (binder/X.Y.Z). Build a stamped binary with\n"
            f'  go build -ldflags "-X github.com/ghchinoy/binder/cmd.Version='
            f'$(git describe --tags --abbrev=0)" -o <bin> .\n'
            f"before running this gate.\n"
        )
        return 2

    # O2 (#185 round 2): an allowlist entry for the CURRENT version is a
    # permanent silent exemption, not an exemption for one historical literal.
    # The entry survives the release it was written against, so the day 0.5.3
    # stops being current, every 0.5.3 literal README carries goes stale with
    # the rule switched off over it — protection that looks like protection and
    # is not. A historical reference is by definition NOT the version now
    # shipping, so this can never reject a legitimate entry.
    stamped_version = expected.split("/", 1)[1]
    current_entries = [
        (path, ver) for (path, ver) in NO_UNPINNED_PROSE_ALLOW
        if ver == stamped_version
    ]
    if current_entries:
        sys.stderr.write(
            f"FATAL: NO_UNPINNED_PROSE_ALLOW exempts the CURRENT stamped "
            f"version {expected}: "
            + ", ".join(f"({p!r}, {v!r})" for p, v in current_entries)
            + ".\nThe allowlist is for literals that must NOT track the "
            "release; the current version is precisely the one that must. "
            "Such an entry would silently exempt that literal forever, "
            "starting the moment this version stops being current. Remove "
            "the entry and fix the literal instead.\n"
        )
        return 2

    findings = []
    blocks = 0
    literals_checked = 0
    # (base-relative path, coverage-key) -> number of literals checked there.
    visited = collections.Counter()
    # (base-relative path, version) -> number of literals this allowlist entry
    # actually exempted. Keys with zero hits are reported (R6).
    allow_hits = collections.Counter()

    def relpath(f):
        # base-relative, forward-slashed, so coverage keys are stable across OS
        # and match EXPECTED_COVERAGE regardless of how base was spelled.
        return f.relative_to(base).as_posix()

    def check_json_block(f, block_lines):
        """Classify a completed JSON block by its schema and check every version
        literal in it. block_lines is a list of (lineno, text)."""
        nonlocal literals_checked
        block_text = "\n".join(t for _, t in block_lines)
        sm = SCHEMA_RE.search(block_text)
        key = f"envelope:{sm.group(1)}" if sm else "envelope:UNCLASSIFIED"
        for lineno, text in block_lines:
            for m in VERSION_LITERAL.finditer(text):
                literals_checked += 1
                visited[(relpath(f), key)] += 1
                if ("binder/" + m.group(1)) != expected:
                    findings.append(
                        (str(f), lineno, "JSON-ENVELOPE",
                         f"transcript literal binder/{m.group(1)} != "
                         f"stamped binary {expected}")
                    )

    # Collect the *.md files under every scan root. A root that does not exist is
    # reported (missing_roots) rather than skipped: a renamed or moved root is a
    # broken discovery path, and a gate that quietly scans one fewer root is the
    # vacuous pass this design exists to prevent.
    md_files = []
    missing_roots = []
    for root in SCAN_ROOTS:
        p = base / root
        if p.is_dir():
            md_files.extend(p.rglob("*.md"))
        elif p.is_file():
            md_files.append(p)
        else:
            missing_roots.append(root)
    md_files = sorted(set(md_files))

    for f in md_files:
        lines = f.read_text().splitlines()
        in_json = False
        block_lines = []
        for i, line in enumerate(lines):
            lineno = i + 1
            if not in_json:
                if FENCE_OPEN.match(line):
                    in_json = True
                    block_lines = []
                    blocks += 1
                    continue
                # PROSE provenance check runs on non-fenced text only. Every
                # match is recorded, and the character span of each literal it
                # pins is collected so the no-unpinned-prose rule below can skip
                # exactly those literals — see PROSE RULE ORDER above.
                pinned_spans = []
                for m in PROSE_PROVENANCE.finditer(line):
                    visited[(relpath(f), "prose-provenance")] += 1
                    pinned_spans.append(m.span(1))
                    if ("binder/" + m.group(1)) != expected:
                        findings.append(
                            (str(f), lineno, "PROSE-PROVENANCE",
                             f"provenance sentence says binder/{m.group(1)}, "
                             f"stamped binary emits {expected}")
                        )
                # NO-UNPINNED-PROSE. Runs on every prose line of a listed
                # file, INCLUDING a line that also carried a provenance
                # sentence: see PROSE RULE ORDER above. Literals already pinned
                # by a prose rule are skipped by span, not by short-circuit.
                if relpath(f) in NO_UNPINNED_PROSE:
                    for m in VERSION_LITERAL.finditer(line):
                        if any(a <= m.start(1) and m.end(1) <= b
                               for a, b in pinned_spans):
                            continue  # already pinned by a prose rule above
                        if (relpath(f), m.group(1)) in NO_UNPINNED_PROSE_ALLOW:
                            # Record the hit: an allowlist key that exempts
                            # nothing is reported below (R6). The counter is the
                            # allowlist's correspondence assertion, the same one
                            # EXPECTED_COVERAGE makes about the tree.
                            allow_hits[(relpath(f), m.group(1))] += 1
                            continue  # documented historical reference
                        # BOTH remedies, because this message is where the
                        # decision actually gets made (#185 round 2, R2). Naming
                        # only the placeholder tells a maintainer who wrote a
                        # legitimate historical reference to DESTROY CORRECT
                        # CONTENT, and the next cheapest move after that is
                        # deleting the rule — which is what the allowlist above
                        # exists to prevent. Whoever reads this is deciding
                        # between the two cases right now, and only one of them
                        # is a stale literal.
                        #
                        # THE THIRD CASE (#185 round 4, O5): the literal is
                        # QUOTED BINARY OUTPUT — a ```text block holding
                        # `binder convert --help`, whose actor-grammar example
                        # really does read `(e.g. binder/0.3.0)`. Neither of the
                        # first two remedies is correct there: the placeholder
                        # falsifies quoted output, and "historical reference" is
                        # a false reason, since that literal is live output owned
                        # by the shipped-output gate (#60) under the same
                        # boundary SCAN_ROOTS states for docs/commands/. Both
                        # prose passes are line-local and know only in_json, so
                        # "literals inside a quoted-output fence belong to #60"
                        # is a rule this model cannot express; the structural fix
                        # is a quoted_output fence state, tracked in #204. Until
                        # then the only available action IS an allowlist entry,
                        # so the message dictates the true reason to write in it
                        # — leaving the maintainer to invent one is how the #60
                        # boundary gets eroded from this side.
                        findings.append(
                            (str(f), lineno, "PROSE-UNPINNED",
                             f"unpinned version literal binder/{m.group(1)} "
                             f"outside a JSON fence; no gate can track it. "
                             f"If it should TRACK the release, write the "
                             f"`binder/<version>` placeholder or a real JSON "
                             f"envelope instead. If it is a LEGITIMATE "
                             f"HISTORICAL reference that must NOT track the "
                             f"release (a floor, a historical note, a "
                             f"measured-with label, an actor-grammar example), "
                             f"KEEP THE TEXT and add one line to "
                             f"NO_UNPINNED_PROSE_ALLOW in this script: "
                             f'("{relpath(f)}", "{m.group(1)}"): '
                             f'"historical: <why this must not track>" — do '
                             f"not delete the rule to silence this. THIRD CASE: "
                             f"if it is QUOTED BINARY OUTPUT (a --help block, a "
                             f"captured error), it is neither of the above — the "
                             f"binary really prints it, and it is owned by the "
                             f"shipped-output gate (#60), the same boundary "
                             f"SCAN_ROOTS states for docs/commands/. Allowlist "
                             f"it with THAT as the reason — "
                             f'("{relpath(f)}", "{m.group(1)}"): "quoted output: '
                             f'owned by the shipped-output gate (#60)" — not as '
                             f"a historical reference, which it is not (#204 "
                             f"tracks teaching this gate about quoted-output "
                             f"fences so the exemption stops being manual)")
                        )
                continue
            # inside a fenced JSON block
            if FENCE_CLOSE.match(line):
                in_json = False
                check_json_block(f, block_lines)
                continue
            block_lines.append((lineno, line))
        # flush an unterminated trailing block so its literals are still checked
        if in_json and block_lines:
            check_json_block(f, block_lines)

    # Vacuous-pass guard: assert every must-track location was reached and
    # yielded at least the number of literals the inventory declares.
    coverage_fail = [
        (path, key, want, label, visited[(path, key)])
        for path, key, want, label in EXPECTED_COVERAGE
        if visited[(path, key)] != want
    ]

    # Allowlist reachability (R6): an entry that exempted nothing. Reported only
    # when the roots it could have matched in were actually scanned — with a
    # missing root every entry would look unreached, and that failure already has
    # its own louder name (MISSING-ROOT).
    stale_allow = (
        [] if missing_roots
        else [key for key in NO_UNPINNED_PROSE_ALLOW if allow_hits[key] == 0]
    )

    print(f"# version-literal gate: {len(SCAN_ROOTS)} scan root(s) "
          f"({', '.join(SCAN_ROOTS)}), {len(md_files)} md files, {blocks} json "
          f"blocks, {literals_checked} version literal(s) checked, "
          f"stamped binary = {expected}")
    for fpath, lineno, kind, detail in findings:
        print(f"{fpath}:{lineno}: [{kind}] {detail}")
    for root in missing_roots:
        print(f"# MISSING-ROOT: scan root {root} does not exist under {base}")
    if coverage_fail:
        print("# COVERAGE FAILURE: expected must-track location(s) were not "
              "reached in full, so a green result would be VACUOUS. Likely "
              "cause: a moved file, a renamed/removed fence tag, or a changed "
              "path glob.")
        for path, key, want, label, got in coverage_fail:
            why = ("discovery is broken — a moved file, a renamed fence tag"
                   if got < want else
                   "a transcript was ADDED — update the count in "
                   "EXPECTED_COVERAGE in the same commit")
            print(f"# MISSING-COVERAGE: {path} [{key}] ({label}): expected "
                  f"exactly {want} literal(s) under {base}, checked {got} "
                  f"({why})")
    for path, ver in stale_allow:
        print(f"# STALE-ALLOWLIST: NO_UNPINNED_PROSE_ALLOW entry "
              f'("{path}", "{ver}") exempted nothing — {path} under {base} '
              f"carries no binder/{ver} literal outside a JSON fence. An unused "
              f"entry is not inert: it pre-approves that literal for whoever "
              f"pastes it next, with the rule already switched off over it. "
              f"Delete the entry (see NO_UNPINNED_PROSE_ALLOW in this script).")
    print(f"# {len(findings)} drift finding(s), "
          f"{len(coverage_fail)} coverage failure(s), "
          f"{len(missing_roots)} missing scan root(s), "
          f"{len(stale_allow)} stale allowlist entry(ies)")
    return 1 if (findings or coverage_fail or missing_roots or stale_allow) else 0


if __name__ == "__main__":
    sys.exit(main())
