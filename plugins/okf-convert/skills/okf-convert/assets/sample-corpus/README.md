# Sample corpus

A tiny plain-markdown corpus for practicing the `okf-convert` workflow. It is an
ordinary directory of notes with YAML frontmatter and standard markdown links —
*not* an OKF bundle yet. Drive `binder` over it to produce one.

It deliberately contains three triage cases so you can exercise the full loop:

- **an unresolved link** — `topics/onboarding.md` links to `/topics/deploy.md`,
  which is not written (legal; shows up under `unresolved`);
- **a missing-title file** — `topics/glossary.md` has a `type` but no title
  (shows up under `missing_titles`);
- **a no-frontmatter file** — `notes/scratch.md` has no frontmatter at all
  (shows up under `schema_violations` as missing `type`).

```bash
# 3. Dry-run triage
binder convert . --dry-run --json | jq '.result | {num_concepts, num_unresolved, num_recovered}'
binder lint . --json | jq '.result | {broken_links, missing_titles, schema_violations}'

# 4. Remediate the source frontmatter (additive, byte-faithful)
binder enrich . --dry-run --json | jq '.result.files'
binder enrich . --json

# 5. Convert and validate
binder convert . -o /tmp/sample-bundle --json | jq '.result.num_concepts'
binder validate /tmp/sample-bundle --json | jq '.result.findings'   # [] ⇒ conformant (exit 0)
```

`binder convert` applies `--default-type` (default `Note`), so even the
no-frontmatter file becomes a conformant concept and `binder validate` reports
the bundle conformant **whether or not** you run `enrich` first — `enrich` is
about fixing the *source*, `validate` is about the emitted bundle.
