#!/usr/bin/env python3
"""py-conformance.py — binds the validate-plugin.sh Python copy to the Go source
of truth for issue #171.

It imports scripts/frontmatter_parse.py — the EXACT module scripts/validate-plugin.sh
runs as the validation gate — and runs it over the shared input corpus, diffing
its error text against the golden that scripts/conformance/gogolden derived live
from the Go codec. Importing the real module (rather than re-copying the parsing
logic) is the whole point: the gate and this check exercise the same code, so a
drift cannot hide.

Nothing here restates a Go literal; the expected values come only from the
golden. A mismatch is reported loudly and exits non-zero.

Usage: python3 py-conformance.py <corpus.json> <go-golden.json>
"""

import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
# Import the SAME module the gate runs.
sys.path.insert(0, os.path.join(REPO_ROOT, "scripts"))
import frontmatter_parse  # noqa: E402


def main(argv):
    if len(argv) < 3:
        print("usage: python3 py-conformance.py <corpus.json> <go-golden.json>", file=sys.stderr)
        return 2

    with open(argv[1], "r", encoding="utf-8") as fh:
        corpus = json.load(fh)
    with open(argv[2], "r", encoding="utf-8") as fh:
        golden = json.load(fh)

    go_error = {c["name"]: c["error"] for c in golden["frontmatter"]}

    fails = 0
    for case in corpus["frontmatter"]:
        result = frontmatter_parse.parse_frontmatter(case["input"])
        py_err = result.get("error", "")
        want = go_error.get(case["name"], "")
        if py_err == want:
            print(f"  ok   frontmatter_parse.py frontmatter[{case['name']}] matches Go")
        else:
            print(
                f"  FAIL Python (validate-plugin.sh copy) DIVERGED from Go on "
                f"frontmatter[{case['name']}]:\n"
                f"         Go:     {json.dumps(want)}\n"
                f"         Python: {json.dumps(py_err)}"
            )
            fails += 1

    if fails:
        print(f"  -> Python: {fails} divergence(s) from the Go source of truth.")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
