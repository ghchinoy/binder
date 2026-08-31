#!/usr/bin/env python3
"""Frontmatter parser shared by the plugin/skill validator and the #171
cross-language conformance check.

This is the SINGLE Python implementation of binder's frontmatter fence/mapping
semantics. scripts/validate-plugin.sh runs it as a subprocess (the validation
GATE itself); scripts/conformance/py-conformance.py imports parse_frontmatter()
directly. Neither re-copies the logic, so the gate and the conformance check
exercise the exact same code — a divergence cannot hide in a second copy.

The fence/mapping semantics and the error wording mirror the Go codec
(internal/okf/native/native.go: splitFrontmatter + parseFrontmatterNode) so
binder and this gate agree on what "valid frontmatter" means and phrase the two
structural errors identically. The Go codec is the source of truth for that
wording; the #171 conformance check binds this copy to the Go strings, so
editing a Go string turns that suite red for this copy instead of letting the
two drift apart in silence.

Only stdout carries the JSON contract the shell parses; diagnostics belong on
stderr (issue #89).
"""

import json
import os
import re
import sys


def parse_frontmatter(content):
    """Parse SKILL.md-style frontmatter, returning a dict.

    On any structural problem the dict has an ``error`` key whose value is the
    message (mirroring the Go codec's wording for the two structural cases). On
    success it carries ``name``/``desc``/``desc_len``/``license``.
    """
    # Split on the YAML 1.1 line-break set (\r\n, lone \r, \n), matching the Go
    # codec's splitLinesKeepEnds so a fence is recognised the same way here.
    lines = re.split(r"\r\n|\r|\n", content)

    # Opening fence: first line must be exactly '---' (after trimming line ends).
    if not lines or lines[0].strip() != "---":
        return {"error": "missing frontmatter: document does not start with '---'"}

    # Closing fence: a subsequent line that is exactly '---'.
    end = None
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            end = i
            break
    if end is None:
        return {"error": "invalid frontmatter: unterminated '---' block"}

    fm_text = "\n".join(lines[1:end])

    try:
        import yaml
    except ImportError:
        return {"error": "PyYAML is required to validate frontmatter (pip install pyyaml)"}

    try:
        data = yaml.safe_load(fm_text)
    except yaml.YAMLError as e:
        return {"error": "invalid frontmatter: " + " ".join(str(e).split())}

    if data is None:
        data = {}
    if not isinstance(data, dict):
        return {"error": "invalid frontmatter: expected a mapping at the top level"}

    def as_str(v):
        return "" if v is None else str(v)

    name = as_str(data.get("name", ""))
    desc = as_str(data.get("description", ""))
    lic = as_str(data.get("license", ""))

    return {"name": name, "desc": desc, "desc_len": len(desc), "license": lic}


def main(argv):
    # Accept the file path as argv[1], falling back to the SKILL_MD env var that
    # scripts/validate-plugin.sh sets, so both call styles work.
    path = argv[1] if len(argv) > 1 else os.environ.get("SKILL_MD", "")
    if not path:
        print(json.dumps({"error": "python execution failed: no file path given"}))
        return 0
    with open(path, "r", encoding="utf-8") as fh:
        content = fh.read()
    print(json.dumps(parse_frontmatter(content)))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
