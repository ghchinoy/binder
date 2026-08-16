package infer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

// GeminiClient is the interface for semantic taxonomy inference.
type GeminiClient interface {
	InferDirectoryTypes(ctx context.Context, dirs map[string][]string, sampleTitles map[string][]string) (map[string]string, error)
}

type realGeminiClient struct {
	client *genai.Client
	model  string
}

// NewGeminiClient constructs a Gemini client using the requested options,
// auto-detecting API key vs Vertex AI with ADC. Returns (client, model, backendName, error).
func NewGeminiClient(ctx context.Context, opts Options) (GeminiClient, string, string, error) {
	model := strings.TrimSpace(opts.GeminiModel)
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	location := strings.TrimSpace(opts.GeminiLocation)
	if location == "" {
		location = "global"
	}
	project := strings.TrimSpace(opts.GeminiProject)
	apiKey := strings.TrimSpace(opts.GeminiAPIKey)
	backendOpt := strings.ToLower(strings.TrimSpace(opts.GeminiBackend))

	var cfg *genai.ClientConfig
	var backendName string

	// Auth decision hierarchy:
	// 1. Explicit project or backend == "vertex" -> Vertex AI
	// 2. Explicit API key or GEMINI_API_KEY env present -> Gemini Developer API
	// 3. Otherwise -> Vertex AI with ADC (discovering project from env/ADC)
	if project != "" || backendOpt == "vertex" {
		cfg = &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Project:  project,
			Location: location,
		}
		backendName = "vertex"
	} else if apiKey != "" {
		cfg = &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		}
		backendName = "api"
	} else if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" && backendOpt != "vertex" {
		cfg = &genai.ClientConfig{
			APIKey:  envKey,
			Backend: genai.BackendGeminiAPI,
		}
		backendName = "api"
	} else {
		proj := os.Getenv("GOOGLE_CLOUD_PROJECT")
		cfg = &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Project:  proj,
			Location: location,
		}
		backendName = "vertex"
	}

	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, "", "", fmt.Errorf("initializing Gemini client (%s): %w", backendName, err)
	}

	return &realGeminiClient{client: client, model: model}, model, backendName, nil
}

func (g *realGeminiClient) InferDirectoryTypes(ctx context.Context, dirs map[string][]string, sampleTitles map[string][]string) (map[string]string, error) {
	if len(dirs) == 0 {
		return nil, nil
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString("You are an expert on the Open Knowledge Format (OKF v0.2). ")
	promptBuilder.WriteString("Analyze the following directories and sample documents from a markdown corpus, ")
	promptBuilder.WriteString("and suggest a concise, canonical OKF concept type for each directory ")
	promptBuilder.WriteString("(e.g., Guide, Runbook, Proposal, Decision, Specification, Subsystem, Metric, Table, Reference, Architecture, Journal, Benchmark, Policy).\n\n")
	promptBuilder.WriteString("Directories and sample content:\n")

	for dir, files := range dirs {
		promptBuilder.WriteString(fmt.Sprintf("Directory: %q\n", dir))
		promptBuilder.WriteString(fmt.Sprintf("  Files count: %d\n", len(files)))
		if len(files) > 0 {
			sampleCount := len(files)
			if sampleCount > 5 {
				sampleCount = 5
			}
			promptBuilder.WriteString(fmt.Sprintf("  Sample files: %s\n", strings.Join(files[:sampleCount], ", ")))
		}
		if titles, ok := sampleTitles[dir]; ok && len(titles) > 0 {
			sampleCount := len(titles)
			if sampleCount > 5 {
				sampleCount = 5
			}
			promptBuilder.WriteString(fmt.Sprintf("  Sample titles: %s\n", strings.Join(titles[:sampleCount], " | ")))
		}
		promptBuilder.WriteString("\n")
	}

	promptBuilder.WriteString("Respond ONLY with a valid JSON object mapping each directory path to its suggested concept type. Example:\n")
	promptBuilder.WriteString("{\n  \"subsystems\": \"Subsystem\",\n  \"runbooks\": \"Runbook\"\n}\n")

	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(promptBuilder.String()), nil)
	if err != nil {
		return nil, fmt.Errorf("gemini generate content: %w", err)
	}

	rawText := strings.TrimSpace(resp.Text())
	// Strip markdown code fences if present (```json ... ```)
	if strings.HasPrefix(rawText, "```") {
		lines := strings.Split(rawText, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			rawText = strings.Join(lines, "\n")
		}
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(rawText), &result); err != nil {
		return nil, fmt.Errorf("parsing Gemini JSON response: %w (raw: %q)", err, rawText)
	}

	return result, nil
}
