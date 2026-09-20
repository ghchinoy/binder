package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/binder/render"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/gemini"
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

	// Construct the shared service ONCE with the composition root's codec (it is
	// stateless and safe for concurrent use); the RunE closure reuses it.
	svc := binder.New(codec)
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

			// Bind flags to config keys — the viper/pflag substrate stays at the
			// adapter edge; only resolved values cross the service seam.
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

			// One code path: the service owns the infer orchestration and the
			// gating-finding definition. The concrete Gemini client (which imports
			// google.golang.org/genai and reads GEMINI_API_KEY / GOOGLE_CLOUD_PROJECT)
			// is injected here, at the adapter edge, via gemini.New — so the SDK
			// never reaches the service seam.
			res, err := svc.Infer(cmd.Context(), binder.InferRequest{
				Src:                 src,
				DefaultType:         defaultType,
				UseGemini:           useGemini,
				GeminiModel:         geminiModel,
				GeminiLocation:      geminiLocation,
				GeminiProject:       geminiProject,
				GeminiBackend:       geminiBackend,
				GeminiRequired:      geminiRequired,
				GeminiClientFactory: gemini.New,
				Version:             binder.Version,
			})
			if err != nil {
				return err
			}

			if jsonOut {
				if err := res.EncodeJSON(cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("encoding json report: %w", err)
				}
			} else if res.Empty() {
				// Zero mappings: the human-readable diagnostic goes to stderr so
				// stdout stays machine-consumable and empty. This keeps the
				// documented `--type-map "$(binder infer SRC)"` idiom working —
				// the substitution yields "", which enrich/convert accept as a
				// --type-map that maps nothing (they still do the rest of their
				// work). Exit stays 0: this is not a failure condition. The routing
				// signal (Empty) is defined once in the service.
				fmt.Fprint(cmd.ErrOrStderr(), render.Infer(res))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), render.Infer(res))
			}

			// The gate decision (bare infer never gates; --strict gates on any
			// warning) is the Result's, defined once in the service.
			return res.Gate(strict)
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
