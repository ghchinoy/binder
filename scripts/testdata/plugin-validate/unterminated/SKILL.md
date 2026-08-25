---
name: okf-fixture
description: A description whose frontmatter fence is never closed.
license: Apache-2.0

# Fixture skill

Negative control (b): the opening `---` fence is never closed. The old
substring split treated the missing fence as "no frontmatter, nothing to
check"; the Go codec and this gate both call it "unterminated '---' block".
