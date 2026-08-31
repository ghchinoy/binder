#!/usr/bin/env python3
"""Version-literal drift gate for plugin JSON transcripts (issue #169).

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
# carries an EXPLICIT inventory of the four must-track locations and asserts each
# was visited and checked; a broken discovery path (empty/wrong docroot, moved
# file, renamed fence tag, changed glob) fails LOUD instead of passing green.

Exit non-zero if any pinned literal drifts OR if any must-track location was not
reached (vacuous-pass guard); exit 0 only when every must-track location was
visited and every checked literal matches the stamped binary.

Usage: check-transcript-versions.py <docroot> <stamped-binder-binary>
"""
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

# MINIMUM-COVERAGE INVENTORY (#169 Critical, round-2 review). The four must-track
# locations, each as (docroot-relative path, coverage-key, human label). The gate
# asserts every one of these was actually visited and its literal checked, so it
# can tell "0 findings because everything is correct" apart from "0 findings
# because I parsed nothing". This is an explicit per-location inventory, NOT a
# bare aggregate floor: a floor like `blocks >= 10` can be satisfied
# coincidentally while one specific file's discovery is broken (one file loses
# blocks as another gains them). The inventory catches that; a floor does not.
#
# Keys are docroot-RELATIVE PATHS, not basenames (Optional-1, round-3 review): two
# files sharing a basename in different directories must not let one satisfy the
# other's coverage entry — a filename collision defeating the inventory would be
# the same vacuous-pass class, reachable by a different route.
_SKILL = "okf-convert/skills/okf-convert/SKILL.md"
_CONTRACT = "okf-convert/skills/okf-convert/references/binder-json-contract.md"
EXPECTED_COVERAGE = [
    (_SKILL, "envelope:binder.report/v1", "convert report envelope literal"),
    (_CONTRACT, "envelope:binder.report/v1", "report envelope literal"),
    (_CONTRACT, "envelope:binder.config/v1", "config envelope literal"),
    (_CONTRACT, "prose-provenance", "prose provenance sentence"),
]


def main() -> int:
    if len(sys.argv) != 3:
        sys.stderr.write(__doc__.strip().splitlines()[-1] + "\n")
        return 2
    docroot = pathlib.Path(sys.argv[1])
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
    visited = set()  # (docroot-relative path, coverage-key) reached and checked

    def relpath(f):
        # docroot-relative, forward-slashed, so coverage keys are stable across
        # OS and match EXPECTED_COVERAGE regardless of how docroot was spelled.
        return f.relative_to(docroot).as_posix()

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
                visited.add((relpath(f), key))
                if ("binder/" + m.group(1)) != expected:
                    findings.append(
                        (str(f), lineno, "JSON-ENVELOPE",
                         f"transcript literal binder/{m.group(1)} != "
                         f"stamped binary {expected}")
                    )

    md_files = sorted(docroot.rglob("*.md"))
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
                    visited.add((relpath(f), "prose-provenance"))
                    if ("binder/" + m.group(1)) != expected:
                        findings.append(
                            (str(f), lineno, "PROSE-PROVENANCE",
                             f"provenance sentence says binder/{m.group(1)}, "
                             f"stamped binary emits {expected}")
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

    # Vacuous-pass guard: assert every must-track location was actually reached.
    coverage_fail = [
        (path, key, label)
        for path, key, label in EXPECTED_COVERAGE
        if (path, key) not in visited
    ]

    print(f"# version-literal gate: {len(md_files)} md files, {blocks} json "
          f"blocks, {literals_checked} version literal(s) checked, "
          f"stamped binary = {expected}")
    for fpath, lineno, kind, detail in findings:
        print(f"{fpath}:{lineno}: [{kind}] {detail}")
    if coverage_fail:
        print("# COVERAGE FAILURE: expected must-track location(s) were never "
              "reached, so a green result would be VACUOUS. Likely cause: a "
              "moved file, a renamed/removed fence tag, or a changed path glob.")
        for path, key, label in coverage_fail:
            print(f"# MISSING-COVERAGE: {path} [{key}] ({label}) "
                  f"not found under {docroot}")
    print(f"# {len(findings)} drift finding(s), "
          f"{len(coverage_fail)} coverage failure(s)")
    return 1 if (findings or coverage_fail) else 0


if __name__ == "__main__":
    sys.exit(main())
