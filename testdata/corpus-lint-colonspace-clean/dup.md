---
type: Note
title: Duplicate keys beside a sequence-item block scalar
dup: 1
dup: 2
steps:
  - |
    Note: this is: literal text, not a mapping entry.
---

# Duplicate keys beside a sequence-item block scalar

A node decode accepts the repeated `dup:` key, so binder parses this file and
`binder validate` calls it conformant. The `Note:` line is the literal interior
of a block scalar. Nothing here is a colon-space defect, so the advisory must
stay silent — see [tagged](tagged.md).
