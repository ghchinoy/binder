package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/validate"
)

func newValidateCmd(codec okf.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <bundle>",
		Short: "Check a bundle for OKF v0.2 conformance (spec §11)",
		Long: "Validate checks the hard conformance rules: every non-reserved .md has a\n" +
			"parseable frontmatter block with a non-empty type. It reports trust\n" +
			"well-formedness as advisories and NEVER rejects a bundle for missing\n" +
			"optional fields, unknown keys, unknown type values, broken links, or\n" +
			"absent trust families.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := validate.Bundle(args[0], codec, okf.DefaultSpecVersion)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "bundle: %s\n", result.Root)
			fmt.Fprintf(out, "concepts: %d, reserved files: %d\n", result.NumConcepts, result.NumReserved)

			for _, f := range result.Advisories() {
				fmt.Fprintf(out, "%s\n", f)
			}
			errs := result.Errors()
			for _, f := range errs {
				fmt.Fprintf(out, "%s\n", f)
			}

			if result.Conformant() {
				fmt.Fprintf(out, "RESULT: conformant (OKF %s)\n", okf.DefaultSpecVersion)
				return nil
			}
			fmt.Fprintf(out, "RESULT: NOT conformant (%d violation(s))\n", len(errs))
			return fmt.Errorf("bundle is not conformant")
		},
	}
	return cmd
}
