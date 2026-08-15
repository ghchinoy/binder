package mcp

import (
	"strings"
	"testing"
)

const brokenLinksCorpus = "../../testdata/corpus-lint-links"

// TestNeverReject: a tool run that produces findings returns them IN the payload
// with IsError=false — findings are never surfaced as MCP tool errors.
func TestNeverReject(t *testing.T) {
	res := callTool(t, "lint", map[string]any{"src": brokenLinksCorpus, "today": fixedToday})
	if res.IsError {
		t.Fatalf("lint with findings must not be a tool error: %s", toolText(t, res))
	}
	payload := toolText(t, res)
	if !strings.Contains(payload, `"broken_links"`) {
		t.Fatalf("expected findings in the payload, got:\n%s", payload)
	}
	// Byte-identical to the CLI, which also returns the report (exit is about the
	// run, not the payload).
	if want := cliJSON(t, "lint", brokenLinksCorpus, "--today", fixedToday, "--json"); payload != want {
		t.Fatalf("never-reject payload not byte-identical to CLI\n--- MCP ---\n%s\n--- CLI ---\n%s", payload, want)
	}
}

// TestNeverFabricateTrust_InvalidActor: an invalid verified_by is a usage-class
// tool error (IsError=true), not a crash and not a silent no-op.
func TestNeverFabricateTrust_InvalidActor(t *testing.T) {
	res := callTool(t, "convert", map[string]any{
		"src":         richCorpus,
		"dry_run":     true,
		"verified_by": "not a valid actor!!",
	})
	if !res.IsError {
		t.Fatalf("invalid verified_by must be a tool error, got success: %s", toolText(t, res))
	}
	if txt := toolText(t, res); !strings.Contains(txt, "invalid actor") {
		t.Fatalf("expected an invalid-actor message, got: %s", txt)
	}
}

// TestNeverFabricateTrust_NoAutoStamp: the server never auto-applies verified_by.
// Convert WITHOUT verified_by is byte-identical to the CLI without --verified-by;
// no stamp is invented. (A stamp only ever appears when explicitly passed.)
func TestNeverFabricateTrust_NoAutoStamp(t *testing.T) {
	got := toolText(t, callTool(t, "convert", map[string]any{"src": richCorpus, "dry_run": true}))
	want := cliJSON(t, "convert", richCorpus, "--dry-run", "--json")
	if got != want {
		t.Fatalf("convert without verified_by diverged from CLI (possible fabricated trust)\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestDeterminism_SourceDateEpoch: with SOURCE_DATE_EPOCH set, the default
// `today` is derived identically by the tool and the CLI, so the payloads are
// byte-identical without any explicit today param.
func TestDeterminism_SourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000") // 2023-11-14T22:13:20Z
	got := toolText(t, callTool(t, "review", map[string]any{"bundle": goldenBundle}))
	want := cliJSON(t, "review", goldenBundle, "--json")
	if got != want {
		t.Fatalf("review payload not deterministic under SOURCE_DATE_EPOCH\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestUsageError_MissingRequiredParam: a missing required param yields a tool
// error, not a crash.
func TestUsageError_MissingRequiredParam(t *testing.T) {
	res := callTool(t, "validate", map[string]any{}) // no bundle
	if !res.IsError {
		t.Fatalf("missing required param must be a tool error, got success: %s", toolText(t, res))
	}
}
