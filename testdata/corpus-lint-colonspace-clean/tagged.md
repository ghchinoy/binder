---
type: Note
title: Tag coercion beside an anchored block scalar
n: !!int notanumber
body: &a |
  Note: this is: literal text too.
---

# Tag coercion beside an anchored block scalar

A node decode does not resolve the `!!int` tag, so this file parses and is
conformant. The anchored block scalar's interior is literal text. Back to
[dup](dup.md).
