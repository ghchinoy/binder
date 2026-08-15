# Scratch Notes

A quick note with **no YAML frontmatter at all** — no `type`, no `title:` key.

`binder lint` reports it under `schema_violations` (missing type); `binder
convert` applies `--default-type` (default `Note`) so it still becomes a
conformant concept, and `binder enrich` can inject `type`/`title`/`generated`
at the source. This file exercises the no-frontmatter triage case.
