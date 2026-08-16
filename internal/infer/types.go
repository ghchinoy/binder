package infer

import (
	"strings"
)

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
	Src         string    `json:"src"`
	TypeMap     string    `json:"type_map"`
	DefaultType string    `json:"default_type"`
	Mappings    []Mapping `json:"mappings"`
	// Warnings is always present in the JSON envelope: no warnings marshals as
	// [] (not null and not an omitted key), so consumers see a stable output
	// shape. infer.Infer initializes it to a non-nil slice to guarantee this.
	Warnings []string `json:"warnings"`
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
	GeminiClient   GeminiClient // optional mock for testing
}
