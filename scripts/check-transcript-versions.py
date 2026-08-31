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

Exit 1 (with file:line findings) if any pinned literal drifts; exit 0 otherwise.

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
FENCE_OPEN = re.compile(r"^[ \t]*```(json[c5]?)[ \t]*$")
FENCE_CLOSE = re.compile(r"^[ \t]*```[ \t]*$")

# A binder version literal: binder/<major>.<minor>.<patch>.
VERSION_LITERAL = re.compile(r"binder/(\d+\.\d+\.\d+)")

# The single capture-provenance prose sentence. This deliberately matches ONLY
# the "captured from real `binder/X.Y.Z` output" claim — a stale provenance
# statement that MUST track the release — and structurally cannot match the
# minimum-version-floor / historical / measured-with phrasings, which never use
# this wording.
PROSE_PROVENANCE = re.compile(r"captured from real `binder/(\d+\.\d+\.\d+)` output")


def main() -> int:
    if len(sys.argv) != 3:
        sys.stderr.write(__doc__.strip().splitlines()[-1] + "\n")
        return 2
    docroot = pathlib.Path(sys.argv[1])
    binder = sys.argv[2]

    expected = subprocess.run(
        [binder, "--version"], capture_output=True, text=True
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
    md_files = sorted(docroot.rglob("*.md"))
    for f in md_files:
        lines = f.read_text().splitlines()
        in_json = False
        for i, line in enumerate(lines):
            lineno = i + 1
            if not in_json:
                if FENCE_OPEN.match(line):
                    in_json = True
                    blocks += 1
                # PROSE provenance check runs on non-fenced text only.
                m = PROSE_PROVENANCE.search(line)
                if m and ("binder/" + m.group(1)) != expected:
                    findings.append(
                        (str(f), lineno, "PROSE-PROVENANCE",
                         f"provenance sentence says binder/{m.group(1)}, "
                         f"stamped binary emits {expected}")
                    )
                continue
            # inside a fenced JSON block
            if FENCE_CLOSE.match(line):
                in_json = False
                continue
            for m in VERSION_LITERAL.finditer(line):
                if ("binder/" + m.group(1)) != expected:
                    findings.append(
                        (str(f), lineno, "JSON-ENVELOPE",
                         f"transcript literal binder/{m.group(1)} != "
                         f"stamped binary {expected}")
                    )

    print(f"# version-literal gate: {len(md_files)} md files, "
          f"{blocks} json blocks, stamped binary = {expected}")
    for fpath, lineno, kind, detail in findings:
        print(f"{fpath}:{lineno}: [{kind}] {detail}")
    print(f"# {len(findings)} finding(s)")
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
