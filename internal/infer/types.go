package infer

import (
	"context"
	"strings"
)

// GeminiClient is the interface for semantic taxonomy inference. It is
// deliberately free of any google.golang.org/genai type, so packages that only
// need to offer or drive infer (the shared service, the CLI request) can name it
// without pulling the cloud SDK. The concrete implementation lives in
// internal/gemini (constructed by the adapter and injected via
// Options.NewGeminiClient); tests inject a fake.
type GeminiClient interface {
	InferDirectoryTypes(ctx context.Context, dirs map[string][]string, sampleTitles map[string][]string) (map[string]string, error)
}

// SignalSource identifies the tier/mechanism that produced a type suggestion.
const (
	SourceFolder      = "folder"
	SourcePattern     = "pattern"
	SourceFrontmatter = "frontmatter"
	SourceGemini      = "gemini"
)

// Mapping represents a proposed type mapping for a directory prefix.
type Mapping struct {
	Dir           string   `json:"dir"`
	SuggestedType string   `json:"suggested_type"`
	Source        string   `json:"source"`
	Rationale     string   `json:"rationale,omitempty"`
	SampleFiles   []string `json:"sample_files,omitempty"`
	Model         string   `json:"model,omitempty"`
	Backend       string   `json:"backend,omitempty"`
}

// Report holds the full inference proposal.
type Report struct {
	Src         string `json:"src"`
	TypeMap     string `json:"type_map"`
	DefaultType string `json:"default_type"`
	// Mappings and Warnings are always present in the JSON envelope: an empty run
	// marshals each as [] (not null and not an omitted key), so consumers see a
	// stable output shape. infer.Infer initializes both to non-nil slices to
	// guarantee this.
	Mappings []Mapping `json:"mappings"`
	Warnings []string  `json:"warnings"`
}

// String formats the report for human-readable CLI output.
func (r *Report) String() string {
	if len(r.Mappings) == 0 {
		return "No directory type mappings inferred (use --default-type: " + r.DefaultType + ")\n"
	}
	var b strings.Builder
	b.WriteString(r.TypeMap)
	b.WriteString("\n")
	return b.String()
}

// Options configures the infer engine.
type Options struct {
	DefaultType    string
	UseGemini      bool
	GeminiModel    string
	GeminiLocation string
	GeminiProject  string
	GeminiBackend  string // "auto" | "api" | "vertex"
	GeminiAPIKey   string
	GeminiRequired bool
	GeminiClient   GeminiClient // optional pre-built client (tests inject a mock)

	// NewGeminiClient constructs the concrete Gemini client when UseGemini is set
	// and no GeminiClient was pre-injected. It is a factory the ADAPTER supplies
	// (cmd/infer.go injects internal/gemini.New) so that google.golang.org/genai
	// and its GEMINI_API_KEY / GOOGLE_CLOUD_PROJECT env reads stay OUT of this
	// package — and therefore off the shared service seam (design §6 Phase 4 /
	// Residual Risk 7). When nil, the Gemini tier is simply unavailable rather
	// than reaching for a concrete SDK from the core.
	NewGeminiClient func(ctx context.Context, opts Options) (GeminiClient, string, string, error)
}
