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

Exit non-zero if any pinned literal drifts OR if any must-track location was not
reached (vacuous-pass guard); exit 0 only when every must-track location was
visited and every checked literal matches the stamped binary.

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
# them lost their fence tag. It is a minimum, so ADDING a transcript needs no
# change here; removing or hiding one fails loud and the message says by how
# much. Update the count when a transcript is legitimately removed.
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

    findings = []
    blocks = 0
    literals_checked = 0
    # (base-relative path, coverage-key) -> number of literals checked there.
    visited = collections.Counter()

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
                # PROSE provenance check runs on non-fenced text only.
                m = PROSE_PROVENANCE.search(line)
                if m:
                    visited[(relpath(f), "prose-provenance")] += 1
                    if ("binder/" + m.group(1)) != expected:
                        findings.append(
                            (str(f), lineno, "PROSE-PROVENANCE",
                             f"provenance sentence says binder/{m.group(1)}, "
                             f"stamped binary emits {expected}")
                        )
                    continue
                # No-unpinned-prose files: outside a JSON fence, a version
                # literal here is unpinnable by construction, so report it
                # rather than let it rot. Already-pinned provenance sentences
                # are handled above and never reach this.
                if relpath(f) in NO_UNPINNED_PROSE:
                    for m in VERSION_LITERAL.finditer(line):
                        if (relpath(f), m.group(1)) in NO_UNPINNED_PROSE_ALLOW:
                            continue  # documented historical reference
                        findings.append(
                            (str(f), lineno, "PROSE-UNPINNED",
                             f"unpinned version literal binder/{m.group(1)} "
                             f"outside a JSON fence; no gate can track it — use "
                             f"the `binder/<version>` placeholder instead")
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
        if visited[(path, key)] < want
    ]

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
            print(f"# MISSING-COVERAGE: {path} [{key}] ({label}): expected "
                  f">= {want} literal(s) under {base}, checked {got}")
    print(f"# {len(findings)} drift finding(s), "
          f"{len(coverage_fail)} coverage failure(s), "
          f"{len(missing_roots)} missing scan root(s)")
    return 1 if (findings or coverage_fail or missing_roots) else 0


if __name__ == "__main__":
    sys.exit(main())
