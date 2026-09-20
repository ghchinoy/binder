package infersvc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/infer"
	"github.com/ghchinoy/binder/internal/infersvc"
	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/okf/native"
)

const inferCorpus = "../../testdata/corpus-rich"

// writeFile writes content to path, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// fakeGemini is a genai-free fake proving the service drives the Gemini tier
// through the interface — no cloud SDK, no env reads.
type fakeGemini struct{ resp map[string]string }

func (f *fakeGemini) InferDirectoryTypes(ctx context.Context, dirs, sampleTitles map[string][]string) (map[string]string, error) {
	return f.resp, nil
}

// TestInferResultComplete proves the service returns a complete report and that
// Empty()/Warnings() are the single definitions the adapter reads.
func TestInferResultComplete(t *testing.T) {
	svc := infersvc.New(native.New())
	res, err := svc.Infer(context.Background(), infersvc.InferRequest{
		Src:         inferCorpus,
		DefaultType: "Note",
		Version:     "test",
	})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if res.Report == nil {
		t.Fatal("Report is nil")
	}
	if res.Empty() {
		t.Errorf("corpus-rich should yield mappings; Empty()=true")
	}
	if res.Report.Src != inferCorpus {
		t.Errorf("Src = %q, want %q", res.Report.Src, inferCorpus)
	}
	// infersvc.Render is the canonical prose seam.
	if got := infersvc.Render(res); got != res.Report.String() {
		t.Errorf("infersvc.Render mismatch:\n%q\n%q", got, res.Report.String())
	}
}

// TestInferEmpty proves the zero-mappings routing signal.
func TestInferEmpty(t *testing.T) {
	dir := t.TempDir()
	// only a root file, no subdirectories => no mappings
	if err := writeFile(dir+"/README.md", "# Readme\n"); err != nil {
		t.Fatal(err)
	}
	svc := infersvc.New(native.New())
	res, err := svc.Infer(context.Background(), infersvc.InferRequest{Src: dir, DefaultType: "Note", Version: "test"})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if !res.Empty() {
		t.Errorf("Empty() = false, want true for a mapping-less corpus")
	}
	if res.Gate(true) != nil {
		t.Errorf("no warnings => --strict must not gate, got %v", res.Gate(true))
	}
}

// TestInferGate proves --strict gates only when warnings are present, and bare
// infer never gates.
func TestInferGate(t *testing.T) {
	svc := infersvc.New(native.New())
	// An unparseable-frontmatter file yields a disclosure warning.
	dir := t.TempDir()
	if err := writeFile(dir+"/guides/a.md", "---\ntype: Guide\ntitle: A: colon breaks this\n---\n\n# A\n"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Infer(context.Background(), infersvc.InferRequest{Src: dir, DefaultType: "Note", Version: "test"})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if res.Warnings() == 0 {
		t.Fatal("expected a disclosure warning for unparsed frontmatter")
	}
	if err := res.Gate(false); err != nil {
		t.Errorf("bare infer must never gate, got %v", err)
	}
	err = res.Gate(true)
	var fe *clijson.FindingsError
	if !errors.As(err, &fe) {
		t.Errorf("--strict with warnings must return *clijson.FindingsError, got %T (%v)", err, err)
	}
}

// TestInferGeminiViaInterface proves the service offers the Gemini tier through
// the genai-free interface (fake injected on the request), with no SDK on the seam.
func TestInferGeminiViaInterface(t *testing.T) {
	svc := infersvc.New(native.New())
	res, err := svc.Infer(context.Background(), infersvc.InferRequest{
		Src:          inferCorpus,
		DefaultType:  "Note",
		UseGemini:    true,
		GeminiModel:  "mock",
		GeminiClient: &fakeGemini{resp: map[string]string{"guides": "GeminiGuide"}},
		Version:      "test",
	})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	var found bool
	for _, m := range res.Report.Mappings {
		if m.Dir == "guides" && m.SuggestedType == "GeminiGuide" && m.Source == infer.SourceGemini {
			found = true
		}
	}
	if !found {
		t.Errorf("expected guides mapping from the fake Gemini client, got %+v", res.Report.Mappings)
	}
}

// TestInferEncodeJSON proves the envelope is the binder.report/v1 shape and is
// produced by the Result, not hand-built by an adapter.
func TestInferEncodeJSON(t *testing.T) {
	svc := infersvc.New(native.New())
	res, err := svc.Infer(context.Background(), infersvc.InferRequest{Src: inferCorpus, DefaultType: "Note", Version: "test"})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	var buf bytes.Buffer
	if err := res.EncodeJSON(&buf); err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var env clijson.Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	if env.Command != "infer" {
		t.Errorf("command = %q, want infer", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}
	if !strings.HasPrefix(env.Binder, "binder/test") {
		t.Errorf("binder = %q, want binder/test prefix", env.Binder)
	}
}
