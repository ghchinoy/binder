// Package cmd wires binder's Cobra command tree. cmd/root.go is the composition
// root: the ONE place a concrete codec is selected and injected as an
// okf.Codec/okf.LinkGraph. Every other command (and all of internal/convert and
// internal/validate) depends only on the binder-owned okf interfaces, never on
// factile or a concrete codec (dependency rule, design-v2 §2.2).
package cmd

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// Version is the binder version, stamped into generated.by ("binder/<version>")
// and printed by `binder --version`. It is single-sourced from the git tag: at
// release time goreleaser injects the tag via
// -ldflags "-X github.com/ghchinoy/binder/cmd.Version=<version>" (see
// .goreleaser.yaml). It MUST stay a var (not a const) so the linker's -X can
// override it — a const cannot be overridden. The literal default "dev" is a
// constant string expression, which is what makes -X effective.
//
// The trust-provenance stamp is load-bearing (design-v2 §2.3): every converted
// concept records generated.by = "binder/<Version>", so a release binary must
// carry the real tag or it corrupts trust metadata forever.
//
// The canonical form is NO leading "v": "binder/<X.Y.Z>". That form matches the
// shipped release artifacts (the majority install path), the baked exemplars,
// and common semver-string usage. Two different sources feed this var and they
// disagree on the "v": goreleaser's -X injects the v-STRIPPED tag, while the
// debug.ReadBuildInfo() fallback below returns the v-PREFIXED module version.
// Left unchecked they write two different trust stamps for one release. init()
// therefore routes BOTH sources through normalizeVersion — the single funnel —
// so the stamped and fallback paths can never diverge again.
var Version = "dev"

// init recovers the version from Go's build metadata for builds that were not
// stamped by goreleaser's ldflags — most importantly `go install
// github.com/ghchinoy/binder@vX.Y.Z`, which embeds the module version. The
// build-info recovery only runs when Version is still the "dev" default, so it
// never clobbers an ldflags-injected value (nor does it alter test builds, whose
// module version is "(devel)"). Whichever source wins, the final assignment
// passes it through normalizeVersion so both paths converge on the canonical
// no-leading-v form.
func init() {
	if Version == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if v := bi.Main.Version; v != "" && v != "(devel)" {
				Version = v
			}
		}
	}
	Version = normalizeVersion(Version)
}

// normalizeVersion canonicalizes a binder version string to the no-leading-v
// form. It strips exactly one leading "v" when the remainder looks like a
// version (i.e. "v" immediately followed by a digit, which is the shape of every
// v-prefixed semver: v0.3.0, v1.2.3-rc1, …) and leaves everything else exactly
// as it is: the "dev" default, "(devel)", the empty string, and any value that
// merely starts with "v" but is not a semver (e.g. "version-x"). It never
// fabricates a version — it only removes a prefix from a value already present.
func normalizeVersion(v string) string {
	if len(v) >= 2 && v[0] == 'v' && v[1] >= '0' && v[1] <= '9' {
		return v[1:]
	}
	return v
}

// NewRootCmd builds the root command with the default (native) codec.
func NewRootCmd() *cobra.Command {
	var codec okf.Codec = native.New()

	// cfg is the shared, viper-backed configuration substrate (#10). It is
	// resolved once in PersistentPreRunE so every subcommand observes the same
	// precedence (flag > env > file > default) and the same fail-fast validation.
	cfg := &config.Config{}

	root := &cobra.Command{
		Use:           "binder",
		Short:         "Convert a plain-markdown corpus into a conformant OKF v0.2 bundle",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Resolve configuration before any command runs. A missing config file is
		// normal (defaults apply); a malformed config `verified_by` fails fast here
		// as a usage error (exit 2), not at first use (design §3.1 / option (a)).
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// The bare `binder` invocation only prints help and never touches
			// config, so it must not fail on a malformed config file. The root is
			// runnable (see RunE below) purely so an unknown subcommand can be
			// classified as a usage error; skip the config load for it to preserve
			// the pre-existing help behavior. Every real command is a subcommand
			// (has a parent) and still loads config.
			if !cmd.HasParent() {
				return nil
			}
			return cfg.Load()
		},
		// The root has no default action; running it prints help (exit 0). It is
		// declared runnable ONLY so cobra invokes the Args validator below for an
		// unknown subcommand — a non-runnable root short-circuits to help before
		// arg validation, which would let `binder bogus` exit 0. rootUnknownCommand
		// rejects any positional arg, so RunE is reached only with no args.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
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
	// An unknown subcommand (e.g. `binder bogus`) is a usage error → exit 2.
	// cobra's Find() validates root args with the default legacyArgs when
	// Command.Args is nil, and returns that unknown-command error bare — which
	// clijson.ExitCode would misclassify as ExitIO (exit 3). Setting a custom
	// Args both suppresses that bare path and, paired with the runnable root
	// above, wraps the error in clijson.Usage at ValidateArgs time. It is set on
	// the root only: cobra emits the "unknown command" error solely at the root
	// level, and every subcommand that itself has children (e.g. `config`)
	// already guards its args with exactArgs.
	root.Args = rootUnknownCommand
	root.AddCommand(newConvertCmd(codec, cfg))
	root.AddCommand(newEnrichCmd(codec, cfg))
	root.AddCommand(newValidateCmd(codec))
	root.AddCommand(newIndexCmd(codec))
	root.AddCommand(newReviewCmd(codec))
	root.AddCommand(newLintCmd(codec))
	root.AddCommand(newGraphCmd(codec))
	root.AddCommand(newMCPCmd(codec))
	root.AddCommand(newInferCmd(codec, cfg))
	root.AddCommand(newConfigCmd(cfg))
	return root
}

// rootUnknownCommand mirrors cobra's default root-level argument validator
// (legacyArgs) but marks an unknown subcommand as a usage error (exit 2)
// instead of leaving it bare (which clijson.ExitCode maps to ExitIO / exit 3).
// The message — including cobra's "Did you mean this?" suggestions — is
// reproduced verbatim so the text users see is unchanged. With no positional
// args it returns nil, so the root's RunE runs and prints help as before.
func rootUnknownCommand(cmd *cobra.Command, args []string) error {
	if !cmd.HasSubCommands() || cmd.HasParent() || len(args) == 0 {
		return nil
	}
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if !cmd.DisableSuggestions {
		if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
			msg += "\n\nDid you mean this?\n"
			for _, s := range suggestions {
				msg += fmt.Sprintf("\t%v\n", s)
			}
		}
	}
	return clijson.Usage(errors.New(msg))
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
