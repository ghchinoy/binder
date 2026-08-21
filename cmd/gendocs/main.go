// Command gendocs regenerates the binder CLI command reference into
// docs/commands/ by walking binder's own live Cobra command tree
// (github.com/spf13/cobra/doc). It is a developer/CI tool, not part of the
// shipped binder binary; run it via `make docs`.
//
// Output is deterministic (no timestamps, no host paths, sorted), so running
// it twice yields no diff. A drift test in internal/gendocs asserts the
// committed docs/commands/ match this generator's output, so an unregenerated
// flag or command fails CI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ghchinoy/binder/internal/gendocs"
)

func main() {
	out := flag.String("out", "docs/commands", "output directory for the generated command reference")
	flag.Parse()

	if err := gendocs.Generate(*out); err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "gendocs: wrote command reference to %s\n", *out)
}
