---
name: okf-fixture
description: Drive the binder CLI to ingest a corpus. Note: this colon breaks YAML.
license: Apache-2.0
---

# Fixture skill

Negative control (a): the exact #88 shape — an unquoted plain scalar whose
"colon-space" makes the mapping value unparseable. The old regex scrape read
`description` straight past it; a real parser rejects it.
