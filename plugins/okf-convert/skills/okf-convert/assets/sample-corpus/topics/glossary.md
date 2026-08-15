---
type: Reference
tags: [glossary]
---

- **Bundle** — a directory tree of OKF concepts.
- **Concept** — one non-reserved `.md` file.
- **Edge** — a resolved markdown link between concepts.

This file has frontmatter with a `type` but **no title** (no `# Heading` and no
`title:` key). `binder lint` reports it under `missing_titles`; `binder enrich`
can inject a `title`.
