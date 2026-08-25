package infer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestInferDoesNotCiteUnparsedFrontmatter is the negative fixture for issue
// #162: a file whose frontmatter does not parse must not be cited in
// sample_files as evidence for source:frontmatter, must not count toward the
// authored-type majority, and must be disclosed in warnings. Before the fix,
// parseConceptSafe swallowed the parse error and returned empty frontmatter, so
// guides/a.md landed in sample_files with no warning emitted — this test failed.
func TestInferDoesNotCiteUnparsedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	guides := filepath.Join(dir, "guides")
	if err := os.MkdirAll(guides, 0o755); err != nil {
		t.Fatal(err)
	}
	// a.md: `title: A: colon breaks this` is invalid YAML — frontmatter does not parse.
	if err := os.WriteFile(filepath.Join(guides, "a.md"),
		[]byte("---\ntype: Guide\ntitle: A: colon breaks this\n---\n\n# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// b.md: clean frontmatter with an authored type.
	if err := os.WriteFile(filepath.Join(guides, "b.md"),
		[]byte("---\ntype: Guide\ntitle: B\n---\n\n# B\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Infer(context.Background(), dir, native.New(), Options{DefaultType: "Note"})
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}

	var guidesMapping *Mapping
	for i := range rep.Mappings {
		if rep.Mappings[i].Dir == "guides" {
			guidesMapping = &rep.Mappings[i]
		}
	}
	if guidesMapping == nil {
		t.Fatalf("expected a mapping for guides/, got mappings: %+v", rep.Mappings)
	}

	// guides/a.md was never parsed, so it must not appear as evidence.
	for _, s := range guidesMapping.SampleFiles {
		if s == "guides/a.md" {
			t.Errorf("guides/a.md cited in sample_files %v, but its frontmatter never parsed", guidesMapping.SampleFiles)
		}
	}

	// The unparsed file must be disclosed in warnings.
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "guides/a.md") && strings.Contains(w, "did not parse") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning disclosing guides/a.md's parse failure, got warnings: %v", rep.Warnings)
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
