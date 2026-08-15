// Command binder converts a plain-markdown corpus into a conformant OKF v0.2
// bundle and validates OKF bundles. See github.com/ghchinoy/binder.
package main

import (
	"fmt"
	"os"

	"github.com/ghchinoy/binder/cmd"
	"github.com/ghchinoy/binder/internal/clijson"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "binder:", err)
		os.Exit(clijson.ExitCode(err))
	}
}
