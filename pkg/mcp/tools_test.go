package mcp

import (
	"strings"
	"testing"
)

// Shared source-corpus + bundle fixtures for the parity tests.
const (
	richCorpus = "../../testdata/corpus-rich"
	// fixedToday pins staleness-dependent payloads so the in-process tool and
	// the CLI subprocess agree regardless of wall clock.
	fixedToday = "2026-08-15"
)

// TestConvertDryRunParity: the convert tool with dry_run:true is byte-identical
// to `binder convert <src> --dry-run --json` (the preview; writes nothing).
func TestConvertDryRunParity(t *testing.T) {
	got := toolText(t, callTool(t, "convert", map[string]any{
		"src":     richCorpus,
		"dry_run": true,
	}))
	want := cliJSON(t, "convert", richCorpus, "--dry-run", "--json")
	if got != want {
		t.Fatalf("convert dry_run payload not byte-identical to `convert --dry-run --json`\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestConvertWriteParity: the convert tool write path is byte-identical to
// `binder convert <src> -o <out> --json`. Both write to the SAME out dir
// (convert is deterministic/idempotent), so the report's src/out match.
func TestConvertWriteParity(t *testing.T) {
	out := t.TempDir()
	want := cliJSON(t, "convert", richCorpus, "-o", out, "--json")
	got := toolText(t, callTool(t, "convert", map[string]any{
		"src": richCorpus,
		"out": out,
	}))
	if got != want {
		t.Fatalf("convert write payload not byte-identical to `convert -o <out> --json`\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestReviewParity: review tool == `binder review <bundle> --today <d> --json`.
func TestReviewParity(t *testing.T) {
	got := toolText(t, callTool(t, "review", map[string]any{
		"bundle": goldenBundle,
		"today":  fixedToday,
	}))
	want := cliJSON(t, "review", goldenBundle, "--today", fixedToday, "--json")
	if got != want {
		t.Fatalf("review payload not byte-identical\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestLintParity: lint tool == `binder lint <src> --today <d> --json`.
func TestLintParity(t *testing.T) {
	got := toolText(t, callTool(t, "lint", map[string]any{
		"src":   richCorpus,
		"today": fixedToday,
	}))
	want := cliJSON(t, "lint", richCorpus, "--today", fixedToday, "--json")
	if got != want {
		t.Fatalf("lint payload not byte-identical\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestGraphParity asserts every graph format is byte-identical to the
// corresponding `binder graph --format <fmt>`, and that format:json returns the
// raw {nodes,edges} export (NOT the report envelope).
func TestGraphParity(t *testing.T) {
	for _, format := range []string{"json", "dot", "graphml", "html"} {
		t.Run(format, func(t *testing.T) {
			got := toolText(t, callTool(t, "graph", map[string]any{
				"bundle": goldenBundle,
				"format": format,
				"today":  fixedToday,
			}))
			want := cliJSON(t, "graph", goldenBundle, "--format", format, "--today", fixedToday)
			if got != want {
				t.Fatalf("graph %s payload not byte-identical\n--- MCP ---\n%s\n--- CLI ---\n%s", format, got, want)
			}
			// format:json must be the raw export object, never the clijson envelope.
			if format == "json" {
				if len(got) == 0 || got[0] != '{' {
					t.Fatalf("graph json must be a raw object, got: %.60s", got)
				}
				if wantsEnvelope(got) {
					t.Fatalf("graph json must NOT be the report envelope; got:\n%s", got)
				}
			}
		})
	}
}

// TestGraphDefaultFormatJSON: omitting format defaults to json (the tool
// default), matching `binder graph --json`.
func TestGraphDefaultFormatJSON(t *testing.T) {
	got := toolText(t, callTool(t, "graph", map[string]any{
		"bundle": goldenBundle,
		"today":  fixedToday,
	}))
	want := cliJSON(t, "graph", goldenBundle, "--json", "--today", fixedToday)
	if got != want {
		t.Fatalf("graph default-format payload not byte-identical to `graph --json`\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// wantsEnvelope reports whether s looks like a clijson report envelope (carries
// the schema marker) rather than a raw graph export.
func wantsEnvelope(s string) bool {
	return strings.Contains(s, `"schema"`) && strings.Contains(s, "binder.report/v1")
}
