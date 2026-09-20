// Command library-example is an in-repo external-consumer example: it imports
// binder's now-published service surface under pkg/ and drives THREE capabilities
// end to end — validate, index, and project — the way an external Go program
// would. It is the library-usage showcase for the read/derive seam (design §6),
// the companion to examples/lint which showcases a single read-side capability.
//
// Each section's output is byte-identical to the matching CLI command (modulo the
// version field, which the example fixes via BINDER_VERSION so a reader can diff a
// section against `binder validate|index|project` after normalizing that one
// field). The capability output goes to stdout; the "==> ..." section banners and
// the warnings-as-data disclosures go to stderr, so `go run ./examples/library
// <bundle> 2>/dev/null` yields exactly the three commands' stdout concatenated.
//
// Everything here uses only the published pkg/ surface: pkg/binder (the service,
// its Request/Result types, ResolveNow, UnparsedWarnings, Version),
// pkg/binder/render (the canonical prose), and pkg/okf/native (the codec injected
// at the composition root). No internal/ package is imported.
//
// Usage:
//
//	go run ./examples/library [bundle-path]
//
// It honors SOURCE_DATE_EPOCH via the shared ResolveNow rule (so project's frozen
// snapshot is reproducible) and BINDER_VERSION for the envelope's `binder` field
// (default "example").
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
	bundle := "testdata/okf-bundles/acme_retail"
	if len(os.Args) > 1 {
		bundle = os.Args[1]
	}

	version := os.Getenv("BINDER_VERSION")
	if version == "" {
		version = "example"
	}

	// The same determinism rule the CLI and MCP adapters use; an external caller
	// could instead call binder.ResolveNowFromEnv().
	now, err := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())
	if err != nil {
		fail(err)
	}

	// The composition root's codec choice, injected into the service — exactly as
	// cmd/root.go does for the CLI. This is the one wiring decision an external
	// caller makes; everything else is method calls on the returned service.
	svc := binder.New(native.New())
	ctx := context.Background()

	runValidate(ctx, svc, bundle, version)
	runIndex(ctx, svc, bundle)
	runProject(ctx, svc, bundle, now, version)
}

// runValidate mirrors `binder validate <bundle>`: it runs the capability and
// prints the canonical prose (render.Validate) to stdout. Gate is the service's
// one policy definition; a bare validate surfaces it without os.Exit-ing here.
func runValidate(ctx context.Context, svc *binder.Service, bundle, version string) {
	fmt.Fprintln(os.Stderr, "==> validate")
	res, err := svc.Validate(ctx, binder.ValidateRequest{
		Bundle:  bundle,
		Version: version,
	})
	if err != nil {
		fail(err)
	}
	fmt.Print(render.Validate(res))
	if gerr := res.Gate(false); gerr != nil {
		fmt.Fprintln(os.Stderr, "library-example: validate gated:", gerr)
	}
}

// runIndex mirrors `binder index <bundle> --dry-run`: the service owns the sorted
// write manifest and the write-vs-regenerate classification; the caller only
// renders the manifest to stdout and the unparsed advisories to stderr. DryRun
// keeps the example side-effect free.
func runIndex(ctx context.Context, svc *binder.Service, bundle string) {
	fmt.Fprintln(os.Stderr, "==> index --dry-run")
	res, err := svc.Index(ctx, binder.IndexRequest{
		Root:   bundle,
		DryRun: true,
	})
	if err != nil {
		fail(err)
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, w)
	}
	for _, e := range res.Entries {
		fmt.Printf("would %s %s\n", e.Action, e.Rel)
	}
}

// runProject mirrors `binder project <bundle> --out <dir>`: the service loads the
// bundle, builds the deterministic projection, and writes the six artifacts; the
// caller discloses unparsed files on stderr (warnings as data) and streams the
// JSON report envelope to stdout. A temp out dir keeps the example self-contained.
func runProject(ctx context.Context, svc *binder.Service, bundle string, now time.Time, version string) {
	fmt.Fprintln(os.Stderr, "==> project")
	out, err := os.MkdirTemp("", "binder-library-example-")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(out)

	res, err := svc.Project(ctx, binder.ProjectRequest{
		Bundle:  bundle,
		Out:     out,
		Now:     now,
		Version: version,
	})
	if err != nil {
		fail(err)
	}
	// The same unparsed-file disclosure the CLI renders on stderr, from the shared
	// binder.UnparsedWarnings text so example and CLI cannot drift.
	for _, w := range binder.UnparsedWarnings(res.Bundle) {
		fmt.Fprintln(os.Stderr, w)
	}
	if err := res.EncodeJSON(os.Stdout); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "library-example:", err)
	os.Exit(1)
}
