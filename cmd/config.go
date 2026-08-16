package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
)

func newConfigCmd(cfg *config.Config) *cobra.Command {
	var jsonOut bool

	printConfig := func(cmd *cobra.Command) error {
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
	}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration (show, get, set, unset)",
		Long: "Config manages persistent settings and prints the resolved effective\n" +
			"configuration: each key's value and its source (flag, env, file, or default),\n" +
			"plus the config file that was read (if any). Precedence is flag > env >\n" +
			"config file > built-in default. Ships --json (schema " + config.SchemaVersion + ").",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printConfig(cmd)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the resolved config as deterministic JSON (schema "+config.SchemaVersion+")")

	// Subcommand: list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all resolved configuration values and their sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printConfig(cmd)
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit the resolved config as deterministic JSON (schema "+config.SchemaVersion+")")

	// Subcommand: get <key>
	getCmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get the resolved value of a configuration key",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			canonical, err := config.CanonicalKey(args[0])
			if err != nil {
				return err
			}
			val := cfg.GetString(canonical)
			source := cfg.Source(canonical)

			out := cmd.OutOrStdout()
			if jsonOut {
				result := map[string]any{
					"key":    canonical,
					"value":  val,
					"source": source,
				}
				if err := clijson.EncodeSchema(out, Version, "config get", config.SchemaVersion, result); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				return nil
			}

			fmt.Fprintf(out, "%s\n", val)
			return nil
		},
	}
	getCmd.Flags().BoolVar(&jsonOut, "json", false, "emit the result as JSON (schema "+config.SchemaVersion+")")

	// Subcommand: set <key> <value>
	var setGlobal bool
	setCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a persistent configuration value in .binder.yaml or user config",
		Long: "Set persists a configuration setting to ./.binder.yaml (default) or\n" +
			"~/.config/binder/config.yaml (with --global).\n\n" +
			"It performs isolated file mutation, modifying only the specified key\n" +
			"without altering other settings or dumping runtime defaults.",
		Args: exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, val := args[0], args[1]

			canonical, err := config.CanonicalKey(key)
			if err != nil {
				return err
			}
			if err := config.ValidateValue(canonical, val); err != nil {
				return err
			}

			targetFile, err := config.TargetFilePath(setGlobal)
			if err != nil {
				return err
			}

			if _, err := config.SetKeyInFile(targetFile, canonical, val); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if jsonOut {
				result := map[string]any{
					"key":    canonical,
					"value":  config.CoerceValue(val),
					"file":   targetFile,
					"status": "updated",
				}
				if err := clijson.EncodeSchema(out, Version, "config set", config.SchemaVersion, result); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				return nil
			}

			fmt.Fprintf(out, "Set %s = %q in %s\n", canonical, val, targetFile)
			return nil
		},
	}
	setCmd.Flags().BoolVarP(&setGlobal, "global", "g", false, "write setting to global user config (~/.config/binder/config.yaml) instead of ./.binder.yaml")
	setCmd.Flags().BoolVar(&jsonOut, "json", false, "emit the result as JSON (schema "+config.SchemaVersion+")")

	// Subcommand: unset <key>
	var unsetGlobal bool
	unsetCmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a persistent configuration value",
		Long: "Unset removes a key from ./.binder.yaml (default) or ~/.config/binder/config.yaml\n" +
			"(with --global), reverting the setting to its environment override or built-in default.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			canonical, err := config.CanonicalKey(key)
			if err != nil {
				return err
			}

			targetFile, err := config.TargetFilePath(unsetGlobal)
			if err != nil {
				return err
			}

			_, existed, err := config.UnsetKeyInFile(targetFile, canonical)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if jsonOut {
				status := "removed"
				if !existed {
					status = "noop"
				}
				result := map[string]any{
					"key":    canonical,
					"file":   targetFile,
					"status": status,
				}
				if err := clijson.EncodeSchema(out, Version, "config unset", config.SchemaVersion, result); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
				return nil
			}

			if existed {
				fmt.Fprintf(out, "Unset %s in %s (reverted to default)\n", canonical, targetFile)
			} else {
				fmt.Fprintf(out, "Key %s is not set in %s\n", canonical, targetFile)
			}
			return nil
		},
	}
	unsetCmd.Flags().BoolVarP(&unsetGlobal, "global", "g", false, "remove setting from global user config (~/.config/binder/config.yaml) instead of ./.binder.yaml")
	unsetCmd.Flags().BoolVar(&jsonOut, "json", false, "emit the result as JSON (schema "+config.SchemaVersion+")")

	cmd.AddCommand(listCmd)
	cmd.AddCommand(getCmd)
	cmd.AddCommand(setCmd)
	cmd.AddCommand(unsetCmd)

	return cmd
}
