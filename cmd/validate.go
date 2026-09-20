package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/binder/render"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
)

func newValidateCmd(codec okf.Codec) *cobra.Command {
	var (
		jsonOut bool
		strict  bool
	)
	// Construct the shared service ONCE with the composition root's codec (it is
	// stateless and safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
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
			// One code path: the service owns validate.Bundle and the spec default.
			// The adapter only resolves inputs, renders, and maps the gate to an exit.
			res, err := svc.Validate(cmd.Context(), binder.ValidateRequest{
				Bundle:  args[0],
				Spec:    okf.DefaultSpecVersion,
				Version: binder.Version,
			})
			if err != nil {
				return err
			}

			// Report is ALWAYS emitted before the gate signals, so the gate never
			// suppresses output. The gate decision (hard §11 non-conformance always
			// gates; trust advisories gate only under --strict) is the Result's,
			// defined once in the service.
			out := cmd.OutOrStdout()
			if jsonOut {
				if encErr := res.EncodeJSON(out); encErr != nil {
					return fmt.Errorf("encoding json report: %w", encErr)
				}
				return res.Gate(strict)
			}
			fmt.Fprint(out, render.Validate(res))
			return res.Gate(strict)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the validation result as deterministic JSON (schema "+clijson.SchemaVersion+") instead of prose")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) on trust well-formedness advisories, not just hard non-conformance")
	return cmd
}
