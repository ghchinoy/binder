package infer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/okf/native"
)

type mockGemini struct {
	response map[string]string
	err      error
}

func (m *mockGemini) InferDirectoryTypes(ctx context.Context, dirs map[string][]string, sampleTitles map[string][]string) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func setupTestCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// 1. subsystems/ (Folder signal)
	subDir := filepath.Join(dir, "subsystems")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "audio.md"), []byte("# Audio Subsystem\n\nDetails..."), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. runbooks/ with pattern (Pattern signal)
	rbDir := filepath.Join(dir, "ops")
	if err := os.MkdirAll(rbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rbDir, "deploy-runbook.md"), []byte("# Deploy Runbook\n\nSteps..."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rbDir, "troubleshooting.md"), []byte("# Troubleshooting\n\nSteps..."), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. custom/ with frontmatter goal: (Frontmatter hint)
	custDir := filepath.Join(dir, "custom")
	if err := os.MkdirAll(custDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custDir, "p1.md"), []byte("---\ngoal: new feature\n---\n# P1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. root file (should not create root directory mapping)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestInferDeterministic(t *testing.T) {
	dir := setupTestCorpus(t)
	codec := native.New()

	rep, err := Infer(context.Background(), dir, codec, Options{DefaultType: "Note"})
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}

	if rep.DefaultType != "Note" {
		t.Errorf("DefaultType = %q, want Note", rep.DefaultType)
	}

	mappingMap := make(map[string]Mapping)
	for _, m := range rep.Mappings {
		mappingMap[m.Dir] = m
	}

	// Check subsystems -> Subsystem (folder)
	if m, ok := mappingMap["subsystems"]; !ok || m.SuggestedType != "Subsystem" || m.Source != SourceFolder {
		t.Errorf("subsystems mapping = %+v, want Subsystem (folder)", m)
	}

	// Check ops -> Runbook (pattern)
	if m, ok := mappingMap["ops"]; !ok || m.SuggestedType != "Runbook" || m.Source != SourcePattern {
		t.Errorf("ops mapping = %+v, want Runbook (pattern)", m)
	}

	// Check custom -> Proposal (frontmatter)
	if m, ok := mappingMap["custom"]; !ok || m.SuggestedType != "Proposal" || m.Source != SourceFrontmatter {
		t.Errorf("custom mapping = %+v, want Proposal (frontmatter)", m)
	}

	wantTypeMap := "custom=Proposal,ops=Runbook,subsystems=Subsystem"
	if rep.TypeMap != wantTypeMap {
		t.Errorf("TypeMap = %q, want %q", rep.TypeMap, wantTypeMap)
	}
}

func TestInferWithMockGemini(t *testing.T) {
	dir := setupTestCorpus(t)
	codec := native.New()

	mock := &mockGemini{
		response: map[string]string{
			"custom": "ArchitectureProposal",
		},
	}

	rep, err := Infer(context.Background(), dir, codec, Options{
		DefaultType:  "Note",
		UseGemini:    true,
		GeminiClient: mock,
		GeminiModel:  "gemini-3.5-flash-lite",
	})
	if err != nil {
		t.Fatalf("Infer with Gemini failed: %v", err)
	}

	mappingMap := make(map[string]Mapping)
	for _, m := range rep.Mappings {
		mappingMap[m.Dir] = m
	}

	// Gemini should override custom -> ArchitectureProposal
	if m, ok := mappingMap["custom"]; !ok || m.SuggestedType != "ArchitectureProposal" || m.Source != SourceGemini {
		t.Errorf("custom mapping with Gemini = %+v, want ArchitectureProposal (gemini)", m)
	}
}
