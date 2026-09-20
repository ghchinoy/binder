package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMCPExternalRootCorpus lays out a corpus whose intro.md links, via a
// file:// URI, to a doc under a SIBLING directory outside the corpus root. It
// returns the corpus root and the absolute sibling directory an author would
// declare with external_root. Mirrors the CLI-side fixture in
// internal/convert/external_root_convert_test.go.
func writeMCPExternalRootCorpus(t *testing.T) (root, sibling string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "corpus")
	sibling = filepath.Join(base, "sibling")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sibling, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	siblingURI := "file://" + filepath.ToSlash(filepath.Join(sibling, "docs", "a.md"))
	intro := "# Intro\n\nCross-repo [away](" + siblingURI + ") link.\n"
	if err := os.WriteFile(filepath.Join(root, "intro.md"), []byte(intro), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "docs", "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, sibling
}

const outsideRootAdvisory = "resolves outside the workspace root"

// TestConvertExternalRootParity mirrors the CLI's --external-root coverage over
// the MCP convert tool: passing external_root suppresses the outside-root
// advisory, and the resulting payload is BYTE-IDENTICAL to
// `binder convert <src> -o <out> --external-root <r> --json`. Both sides write
// to the same out dir (convert is deterministic/idempotent), matching the
// existing write-parity tests.
//
// Teeth: if the handler ignored external_root (fed nil to
// convert.Options.ExternalRoots), the with-flag payload would still carry the
// advisory, failing both the parity comparison and the suppression assertion.
func TestConvertExternalRootParity(t *testing.T) {
	root, sibling := writeMCPExternalRootCorpus(t)

	out := t.TempDir()
	want := cliJSON(t, "convert", root, "-o", out, "--external-root", sibling, "--json")
	got := toolText(t, callTool(t, "convert", map[string]any{
		"src":           root,
		"out":           out,
		"external_root": []string{sibling},
	}))
	if got != want {
		t.Fatalf("convert external_root payload not byte-identical to `convert --external-root --json`\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
	if strings.Contains(got, outsideRootAdvisory) {
		t.Fatalf("external_root must suppress the outside-root advisory, but it is present:\n%s", got)
	}
}

// TestConvertExternalRootAbsentStillAdvises pins the control: without
// external_root the same corpus still raises the outside-root advisory. This is
// what gives TestConvertExternalRootParity its teeth — it proves the advisory
// exists to be suppressed and that suppression is caused by the param.
func TestConvertExternalRootAbsentStillAdvises(t *testing.T) {
	root, _ := writeMCPExternalRootCorpus(t)

	got := toolText(t, callTool(t, "convert", map[string]any{
		"src": root,
		"out": t.TempDir(),
	}))
	if !strings.Contains(got, outsideRootAdvisory) {
		t.Fatalf("without external_root the outside-root advisory must be present:\n%s", got)
	}
}

// TestConvertExternalRootEmptyValueRejected mirrors the CLI's usage-class gate
// (cmd/convert.go: "--external-root value must not be empty") on the MCP side.
func TestConvertExternalRootEmptyValueRejected(t *testing.T) {
	root, _ := writeMCPExternalRootCorpus(t)

	res := callTool(t, "convert", map[string]any{
		"src":           root,
		"out":           t.TempDir(),
		"external_root": []string{"  "},
	})
	if !res.IsError {
		t.Fatalf("empty external_root value must be a usage-class tool error, got success:\n%s", toolText(t, res))
	}
	if msg := toolText(t, res); !strings.Contains(msg, "external_root value must not be empty") {
		t.Fatalf("expected empty-value error, got: %s", msg)
	}
}
