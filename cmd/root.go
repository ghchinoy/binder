// Package cmd wires binder's Cobra command tree. cmd/root.go is the composition
// root: the ONE place a concrete codec is selected and injected as an
// okf.Codec/okf.LinkGraph. Every other command (and all of internal/convert and
// internal/validate) depends only on the binder-owned okf interfaces, never on
// factile or a concrete codec (dependency rule, design-v2 §2.2).
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// Version is the binder version, stamped into generated.by ("binder/<version>").
const Version = "0.1.0"

// NewRootCmd builds the root command with the default (native) codec.
func NewRootCmd() *cobra.Command {
	var codec okf.Codec = native.New()

	root := &cobra.Command{
		Use:           "binder",
		Short:         "Convert a plain-markdown corpus into a conformant OKF v0.2 bundle",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Print `binder/<version>` for --version so the discovery surface matches the
	// JSON envelope's "binder" field exactly (#13 §4.5).
	root.SetVersionTemplate("binder/{{.Version}}\n")
	// Flag-parse errors (unknown flag, bad value) are usage errors → exit 2.
	// Cobra propagates a flag error func to subcommands, so setting it on the
	// root covers every command.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clijson.Usage(err)
	})
	root.AddCommand(newConvertCmd(codec))
	root.AddCommand(newValidateCmd(codec))
	root.AddCommand(newIndexCmd(codec))
	root.AddCommand(newReviewCmd(codec))
	root.AddCommand(newGraphCmd(codec))
	return root
}

// exactArgs wraps a positional-args validator so an arg-count violation is a
// usage error (exit 2) rather than an IO/internal error (exit 3).
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		return clijson.Usage(cobra.ExactArgs(n)(cmd, args))
	}
}

// Execute runs the binder CLI.
func Execute() error {
	return NewRootCmd().Execute()
}
