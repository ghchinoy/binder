---
type: Note
title: "Multi-View: Tabs and Windows"
note: 'single: quoted is fine too'
homepage: https://example.com/a:b
standup: 12:30
ratio: 16:9
summary: |
  Note: a colon-space inside a block scalar is ordinary text, not a mapping.
  Warning: so is this line.
folded: >
  Heads up: folded block scalars are text too.
tags: [alpha, beta]
---

# A

Negative controls for the issue-#93 colon-space advisory. Every value above is
either quoted, colon-space-free, or block-scalar text, so the advisory must stay
silent on this file and the whole corpus must lint clean.

Links to [b](b.md).
