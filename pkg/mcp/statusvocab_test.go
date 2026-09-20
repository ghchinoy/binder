package mcp

import (
	"strings"
	"testing"
)

// The status-vocabulary tests (issue #23) exercise the convert tool's §5.4
// handling over the SDK transport and pin it to the CLI. status_map values are
// validated independent of whether the mapped directory exists, so a
// "default=<value>" entry is enough to drive every path. dry_run keeps them
// write-free and lets the payload be compared byte-for-byte with
// `binder convert --dry-run --json`.

// TestConvertStatusConformantEmitsEmptyNotes: a conformant status_map value
// emits status_notes as [] — never null, never omitted (item 2 / PR #56's
// stable-empty-array contract) — and stays byte-identical to the CLI.
func TestConvertStatusConformantEmitsEmptyNotes(t *testing.T) {
	got := toolText(t, callTool(t, "convert", map[string]any{
		"src":        richCorpus,
		"dry_run":    true,
		"status_map": "default=stable",
	}))
	if !strings.Contains(got, `"status_notes": []`) {
		t.Fatalf("conformant status_map must emit status_notes: [] (never null/omitted); got:\n%s", got)
	}
	if strings.Contains(got, `"status_notes": null`) {
		t.Fatalf("status_notes must never marshal to null; got:\n%s", got)
	}
	want := cliJSON(t, "convert", richCorpus, "--dry-run", "--json", "--status-map", "default=stable")
	if got != want {
		t.Fatalf("MCP convert status payload not byte-identical to CLI\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestConvertStatusDefaultWarnsNoRewrite: on the DEFAULT path a non-conformant
// value warns and is written unchanged — never rewritten, even for a known
// alias — and the warning points at the opt-in flag. Byte-identical to CLI.
func TestConvertStatusDefaultWarnsNoRewrite(t *testing.T) {
	got := toolText(t, callTool(t, "convert", map[string]any{
		"src":        richCorpus,
		"dry_run":    true,
		"status_map": "default=active",
	}))
	if !strings.Contains(got, "wrote it unchanged") {
		t.Fatalf("default path must warn and pass the value through unchanged; got:\n%s", got)
	}
	if !strings.Contains(got, "pass --canonicalize-status") {
		t.Fatalf("default-path warning for a known alias must point at --canonicalize-status; got:\n%s", got)
	}
	if strings.Contains(got, "canonicalized") {
		t.Fatalf("default path must NEVER rewrite (no canonicalized note); got:\n%s", got)
	}
	want := cliJSON(t, "convert", richCorpus, "--dry-run", "--json", "--status-map", "default=active")
	if got != want {
		t.Fatalf("MCP convert status payload not byte-identical to CLI\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestConvertStatusCanonicalizeRewrites: with canonicalize_status:true the fixed
// alias is rewritten and the rewrite is reported (never a bare warning). Matches
// the CLI's --canonicalize-status byte-for-byte.
func TestConvertStatusCanonicalizeRewrites(t *testing.T) {
	got := toolText(t, callTool(t, "convert", map[string]any{
		"src":                 richCorpus,
		"dry_run":             true,
		"status_map":          "default=active",
		"canonicalize_status": true,
	}))
	// The payload is JSON, so the note's quotes are escaped (\"stable\").
	if !strings.Contains(got, `canonicalized to \"stable\"`) {
		t.Fatalf("canonicalize_status must rewrite the alias and report it; got:\n%s", got)
	}
	if strings.Contains(got, "wrote it unchanged") {
		t.Fatalf("a canonicalized value must not also warn as unchanged; got:\n%s", got)
	}
	want := cliJSON(t, "convert", richCorpus, "--dry-run", "--json", "--status-map", "default=active", "--canonicalize-status")
	if got != want {
		t.Fatalf("MCP convert status payload not byte-identical to CLI\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}
