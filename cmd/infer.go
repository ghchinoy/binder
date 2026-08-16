package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/infer"
	"github.com/ghchinoy/binder/internal/okf"
)

func newInferCmd(codec okf.Codec, cfg *config.Config) *cobra.Command {
	var (
		defaultType    string
		useGemini      bool
		geminiModel    string
		geminiLocation string
		geminiProject  string
		geminiBackend  string
		geminiRequired bool
		jsonOut        bool
		strict         bool
	)

	cmd := &cobra.Command{
		Use:   "infer <corpus>",
		Short: "Inspect a source markdown corpus and propose a --type-map",
		Long: "Infer inspects a source markdown corpus and proposes a directory-to-type\n" +
			"mapping string (e.g. \"docs=Guide,subsystems=Subsystem\") and structured report.\n\n" +
			"It evaluates a tiered signal ladder: deterministic offline signals by default\n" +
			"(folder structure, filename patterns, frontmatter hints), plus an optional\n" +
			"opt-in Gemini semantic tier (--gemini) supporting API keys and Google Cloud\n" +
			"Vertex AI with Application Default Credentials.\n\n" +
			"Infer is proposal-only: it never writes to disk. Review the proposal, then\n" +
			"pass it to `binder convert --type-map` or `binder enrich --type-map`.",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]

			// A missing/non-directory corpus path is a usage error (exit 2).
			if info, err := os.Stat(src); err != nil || !info.IsDir() {
				return clijson.Usage(fmt.Errorf("corpus %q is not a readable directory", src))
			}

			// Bind flags to config keys
			cfg.BindFlag(config.KeyDefaultType, cmd.Flags().Lookup("default-type"))
			cfg.BindFlag(config.KeyGeminiModel, cmd.Flags().Lookup("gemini-model"))
			cfg.BindFlag(config.KeyGeminiLocation, cmd.Flags().Lookup("location"))
			cfg.BindFlag(config.KeyGeminiProject, cmd.Flags().Lookup("project"))
			cfg.BindFlag(config.KeyGeminiBackend, cmd.Flags().Lookup("backend"))

			defaultType = cfg.GetString(config.KeyDefaultType)
			geminiModel = cfg.GetString(config.KeyGeminiModel)
			geminiLocation = cfg.GetString(config.KeyGeminiLocation)
			geminiProject = cfg.GetString(config.KeyGeminiProject)
			geminiBackend = cfg.GetString(config.KeyGeminiBackend)

			opts := infer.Options{
				DefaultType:    defaultType,
				UseGemini:      useGemini,
				GeminiModel:    geminiModel,
				GeminiLocation: geminiLocation,
				GeminiProject:  geminiProject,
				GeminiBackend:  geminiBackend,
				GeminiRequired: geminiRequired,
			}

			rep, err := infer.Infer(cmd.Context(), src, codec, opts)
			if err != nil {
				return err
			}

			if jsonOut {
				if err := clijson.Encode(cmd.OutOrStdout(), Version, "infer", rep); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
			} else if len(rep.Mappings) == 0 {
				// Zero mappings: the human-readable diagnostic goes to stderr so
				// stdout stays machine-consumable and empty. This keeps the
				// documented `--type-map "$(binder infer SRC)"` idiom working —
				// the substitution yields "" and enrich/convert treat it as a
				// no-op. Exit stays 0: this is not a failure condition.
				fmt.Fprint(cmd.ErrOrStderr(), rep.String())
			} else {
				fmt.Fprint(cmd.OutOrStdout(), rep.String())
			}

			hasWarnings := len(rep.Warnings) > 0
			return clijson.Gate(strict, false, hasWarnings,
				fmt.Sprintf("infer encountered %d warning(s) (--strict)", len(rep.Warnings)))
		},
	}

	cmd.Flags().StringVar(&defaultType, "default-type", "Note", "fallback concept type")
	cmd.Flags().BoolVar(&useGemini, "gemini", false, "enable Gemini semantic inference tier (requires API key or Google Cloud ADC)")
	cmd.Flags().StringVar(&geminiModel, "gemini-model", config.DefaultGeminiModel, "Gemini model for semantic inference")
	cmd.Flags().StringVar(&geminiLocation, "location", config.DefaultGeminiLocation, "Google Cloud location for Vertex AI")
	cmd.Flags().StringVar(&geminiProject, "project", "", "Google Cloud project for Vertex AI (defaults to ADC / GOOGLE_CLOUD_PROJECT)")
	cmd.Flags().StringVar(&geminiBackend, "backend", config.DefaultGeminiBackend, "Gemini auth backend: auto, api, or vertex")
	cmd.Flags().BoolVar(&geminiRequired, "gemini-required", false, "fail on Gemini inference error instead of degrading to deterministic tiers")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the inference report as deterministic JSON (schema "+clijson.SchemaVersion+")")
	cmd.Flags().BoolVar(&strict, "strict", false, "gate (exit 1) if any warning or failure occurs")

	return cmd
}
