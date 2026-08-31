#!/usr/bin/env python3
"""prose-conformance.py — binds the two testdata prose fixtures to the Go source
of truth for issue #171.

scripts/testdata/plugin-validate/unterminated/SKILL.md and .../nonmapping/SKILL.md
each restate one of the two structural error phrases IN PROSE ("the Go codec and
this gate both call it ..."). Those restatements are unbound: editing the Go
string leaves the prose asserting old wording. This check derives the current
phrase from the golden (scripts/conformance/gogolden, live from the Go codec)
and asserts each fixture still contains it.

The phrase checked is taken from the golden, never hard-coded here, so this adds
no copy of the literal. Only the mapping of "which case is restated in which
fixture" is configuration. A missing phrase is reported loudly and exits
non-zero, naming the fixture.

Usage: python3 prose-conformance.py <go-golden.json>
"""

import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))

# Which frontmatter case's wording each prose fixture restates. Configuration,
# not a copy of the error text — the text itself is derived from the golden.
FIXTURES = {
    "unterminated-fence": "scripts/testdata/plugin-validate/unterminated/SKILL.md",
    "nonmapping-sequence": "scripts/testdata/plugin-validate/nonmapping/SKILL.md",
}

# The prose quotes the phrase WITHOUT the "invalid frontmatter: " prefix; strip
# it from the golden error to get the core phrase the fixture should contain.
PREFIX = "invalid frontmatter: "


def main(argv):
    if len(argv) < 2:
        print("usage: python3 prose-conformance.py <go-golden.json>", file=sys.stderr)
        return 2

    with open(argv[1], "r", encoding="utf-8") as fh:
        golden = json.load(fh)
    go_error = {c["name"]: c["error"] for c in golden["frontmatter"]}

    fails = 0
    for case, rel in FIXTURES.items():
        full = go_error.get(case, "")
        core = full[len(PREFIX):] if full.startswith(PREFIX) else full
        with open(os.path.join(REPO_ROOT, rel), "r", encoding="utf-8") as fh:
            text = fh.read()
        if core and core in text:
            print(f'  ok   {rel} restates the current Go wording "{core}"')
        else:
            print(
                f"  FAIL prose fixture {rel} does NOT restate the current Go wording "
                f'"{core}" (case {case}) — the Go string changed but the prose was not '
                f"updated to match"
            )
            fails += 1

    if fails:
        print(f"  -> Prose fixtures: {fails} divergence(s) from the Go source of truth.")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
