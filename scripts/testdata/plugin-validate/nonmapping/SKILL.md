---
- name: okf-fixture
- description: valid YAML, but the top-level node is a sequence, not a mapping
- license: Apache-2.0
---

# Fixture skill

Negative control (c): the frontmatter is valid YAML but its top-level node is
a sequence, so there are no keys to read. The Go codec and this gate both
reject it with "expected a mapping at the top level".
