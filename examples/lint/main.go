// Command lint-example is an in-repo external-consumer example (design §6 Phase 1
// point 4). It imports the shared service the way an external Go caller does and
// drives Service.Lint over a corpus, producing the same lint result as `binder lint`.
//
// It imports the published pkg/binder surface (relocated out of internal/ in
// Phase 6); the multi-capability companion examples/library showcases validate,
// index, and project against the same surface.
//
// Usage:
//
//	go run ./examples/lint [corpus-path] [today-YYYY-MM-DD]
//
// It honors SOURCE_DATE_EPOCH via the shared ResolveNow rule and BINDER_VERSION for
// the envelope's `binder` field (default "example"), so its output can be compared to
// the CLI's after normalizing that one version field.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/binder/render"
	"github.com/ghchinoy/binder/pkg/okf/native"
)

func main() {
	src := "testdata/corpus-basic"
	if len(os.Args) > 1 {
		src = os.Args[1]
	}
	today := ""
	if len(os.Args) > 2 {
		today = os.Args[2]
	}

	version := os.Getenv("BINDER_VERSION")
	if version == "" {
		version = "example"
	}

	// Same determinism rule the CLI and MCP adapters use; an external caller could
	// instead call binder.ResolveNowFromEnv().
	now, err := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "lint-example:", err)
		os.Exit(1)
	}

	// The composition root's codec choice, injected into the service — exactly as
	// cmd/root.go does for the CLI.
	svc := binder.New(native.New())

	res, err := svc.Lint(context.Background(), binder.LintRequest{
		Src:     src,
		Now:     now,
		Today:   today,
		Version: version,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "lint-example:", err)
		os.Exit(1)
	}

	// Both seams an external caller gets: the canonical prose (render) and the
	// machine envelope (Result.EncodeJSON), identical to the CLI's.
	fmt.Fprint(os.Stderr, render.Lint(res))
	if err := res.EncodeJSON(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "lint-example:", err)
		os.Exit(1)
	}

	// Gate is the service's one definition; surface it as an exit-ish signal without
	// os.Exit-ing on advisories (bare lint never gates).
	if gerr := res.Gate(false); gerr != nil {
		fmt.Fprintln(os.Stderr, "lint-example: gated:", gerr)
	}
}
