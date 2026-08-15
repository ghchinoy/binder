package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
)

func newConfigCmd(cfg *config.Config) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the resolved effective configuration and where each value came from",
		Long: "Config prints the resolved effective configuration: each key's value and\n" +
			"its source (flag, env, file, or default), plus the config file that was\n" +
			"read (if any). Precedence is flag > env > config file > built-in default.\n" +
			"A missing config file is normal — defaults apply. Ships --json (schema\n" +
			config.SchemaVersion + ").",
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved := cfg.Resolve()
			out := cmd.OutOrStdout()
			if jsonOut {
				if err := clijson.EncodeSchema(out, Version, "config", config.SchemaVersion, resolved); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				return nil
			}

			fmt.Fprintf(out, "binder config\n")
			if resolved.ConfigFile != "" {
				fmt.Fprintf(out, "  config file: %s\n", resolved.ConfigFile)
			} else {
				fmt.Fprintf(out, "  config file: (none; using defaults)\n")
			}
			for _, k := range config.Keys() {
				v := resolved.Values[k]
				fmt.Fprintf(out, "  %s: %q (source: %s)\n", k, v.Value, v.Source)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the resolved config as deterministic JSON (schema "+config.SchemaVersion+") instead of prose")
	return cmd
}
