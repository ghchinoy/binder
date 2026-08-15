package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/validate"
)

func newValidateCmd(codec okf.Codec) *cobra.Command {
	var (
		jsonOut bool
		strict  bool
	)
	cmd := &cobra.Command{
		Use:   "validate <bundle>",
		Short: "Check a bundle for OKF v0.2 conformance (spec §11)",
		Long: "Validate checks the hard conformance rules: every non-reserved .md has a\n" +
			"parseable frontmatter block with a non-empty type. It reports trust\n" +
			"well-formedness as advisories and NEVER rejects a bundle for missing\n" +
			"optional fields, unknown keys, unknown type values, broken links, or\n" +
			"absent trust families.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := validate.Bundle(args[0], codec, okf.DefaultSpecVersion)
			if err != nil {
				return err
			}
			errs := result.Errors()
			// The exit code is about the run, not the output format: identical in
			// prose and --json. Non-conformance is a hard §11 violation (not an
			// advisory) and always gates (exit 1). Trust well-formedness advisories
			// gate only under --strict (#7): the flag flips the clijson.Gate seam.
			hard := !result.Conformant()
			adv := len(result.Advisories()) > 0
			msg := fmt.Sprintf("bundle is not conformant (%d violation(s))", len(errs))
			if !hard && strict && adv {
				msg = fmt.Sprintf("bundle has %d advisory finding(s) (--strict)", len(result.Advisories()))
			}
			gate := clijson.Gate(strict, hard, adv, msg)

			out := cmd.OutOrStdout()
			if jsonOut {
				if encErr := clijson.Encode(out, Version, "validate", result); encErr != nil {
					return fmt.Errorf("encoding json report: %w", encErr)
				}
				return gate
			}

			fmt.Fprintf(out, "bundle: %s\n", result.Root)
			fmt.Fprintf(out, "concepts: %d, reserved files: %d\n", result.NumConcepts, result.NumReserved)
			for _, f := range result.Advisories() {
				fmt.Fprintf(out, "%s\n", f)
			}
			for _, f := range errs {
				fmt.Fprintf(out, "%s\n", f)
			}
			if result.Conformant() {
				fmt.Fprintf(out, "RESULT: conformant (OKF %s)\n", okf.DefaultSpecVersion)
			} else {
				fmt.Fprintf(out, "RESULT: NOT conformant (%d violation(s))\n", len(errs))
			}
			return gate
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the validation result as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) on trust well-formedness advisories, not just hard non-conformance")
	return cmd
}
