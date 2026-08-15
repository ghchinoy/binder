// Package cmd wires binder's Cobra command tree. cmd/root.go is the composition
// root: the ONE place a concrete codec is selected and injected as an
// okf.Codec/okf.LinkGraph. Every other command (and all of internal/convert and
// internal/validate) depends only on the binder-owned okf interfaces, never on
// factile or a concrete codec (dependency rule, design-v2 §2.2).
package cmd

import (
	"github.com/spf13/cobra"

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
	root.AddCommand(newConvertCmd(codec))
	root.AddCommand(newValidateCmd(codec))
	root.AddCommand(newIndexCmd(codec))
	root.AddCommand(newReviewCmd(codec))
	root.AddCommand(newGraphCmd(codec))
	return root
}

// Execute runs the binder CLI.
func Execute() error {
	return NewRootCmd().Execute()
}
