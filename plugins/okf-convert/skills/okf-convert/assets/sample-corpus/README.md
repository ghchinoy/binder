# Sample corpus

A tiny plain-markdown corpus for practicing the `okf-convert` workflow. It is an
ordinary directory of notes with YAML frontmatter and standard markdown links —
*not* an OKF bundle yet. Drive `binder` over it to produce one:

```bash
binder convert . --dry-run --json | jq '.result | {num_concepts, num_unresolved}'
binder convert . -o /tmp/sample-bundle --json | jq '.result.num_concepts'
binder validate /tmp/sample-bundle --json | jq '.result.findings'   # [] ⇒ conformant (exit 0)
```

This file (`README.md`) is a reserved-looking doc, not a concept target; the
concepts live under `topics/`.
