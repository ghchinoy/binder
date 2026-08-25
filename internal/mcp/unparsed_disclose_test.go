package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// badFM is a frontmatter block no YAML parser accepts (an unquoted plain scalar
// containing ": "), the #161 negative-fixture payload. mcpBadBundle writes a
// bundle whose only concept file carries it, plus a good concept that links to
// it (so a dropped node would also produce a dangling edge).
const badFM = "---\ntitle: thing: with an unquoted colon\ngoal: another: bad line\n---\n\n# Bad\n"

// mcpBadBundle writes a temp bundle: an index, a good concept linking to bad, and
// an unparseable bad concept. It returns the bundle root.
func mcpBadBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.md": "---\ntype: index\ntitle: Index\nokf_version: \"0.2\"\n---\n\n# Index\n",
		"good.md":  "---\ntype: guide\ntitle: Good Doc\n---\n\n# Good Doc\n\nLink to [bad](bad.md).\n",
		"bad.md":   badFM,
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// resultUnparsed unmarshals a binder.report/v1 envelope and returns
// result.unparsed (list_graphs / query_graph shape).
func resultUnparsed(t *testing.T, payload string) []string {
	t.Helper()
	var env struct {
		Result struct {
			Unparsed []string `json:"unparsed"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		t.Fatalf("envelope parse: %v\n%s", err, payload)
	}
	return env.Result.Unparsed
}

// TestMCPReviewSurfacesUnparsed is the I-2 MCP fixture: the review tool must
// disclose the recovered file in unparsed_frontmatter, exactly as `binder review
// --json` now does. A fix that stopped at the CLI and left MCP silent would fail
// here.
func TestMCPReviewSurfacesUnparsed(t *testing.T) {
	bundle := mcpBadBundle(t)
	res := callTool(t, "review", map[string]any{"bundle": bundle})
	if res.IsError {
		t.Fatalf("review must not be a tool error (never-reject): %s", toolText(t, res))
	}
	var env struct {
		Result struct {
			UnparsedFrontmatter []string `json:"unparsed_frontmatter"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(toolText(t, res)), &env); err != nil {
		t.Fatalf("review envelope parse: %v", err)
	}
	if len(env.Result.UnparsedFrontmatter) != 1 || env.Result.UnparsedFrontmatter[0] != "bad" {
		t.Fatalf("review MCP unparsed_frontmatter = %v, want [bad]", env.Result.UnparsedFrontmatter)
	}
}

// TestMCPGraphSurfacesUnparsed is the I-4 MCP fixture for the graph tool: the raw
// json export must carry the recovered node in its unparsed field (and declare
// the node, so the good->bad edge does not dangle).
func TestMCPGraphSurfacesUnparsed(t *testing.T) {
	bundle := mcpBadBundle(t)
	res := callTool(t, "graph", map[string]any{"bundle": bundle, "format": "json"})
	if res.IsError {
		t.Fatalf("graph must not be a tool error: %s", toolText(t, res))
	}
	var m struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
		Unparsed []string `json:"unparsed"`
	}
	if err := json.Unmarshal([]byte(toolText(t, res)), &m); err != nil {
		t.Fatalf("graph json parse: %v", err)
	}
	if len(m.Unparsed) != 1 || m.Unparsed[0] != "bad" {
		t.Fatalf("graph MCP unparsed = %v, want [bad]", m.Unparsed)
	}
	var declared bool
	for _, n := range m.Nodes {
		if n.ID == "bad" {
			declared = true
		}
	}
	if !declared {
		t.Fatalf("graph MCP must declare the recovered node \"bad\" (no dangling edge)")
	}
}

// TestMCPListGraphsSurfacesUnparsed is the I-4 MCP fixture for list_graphs: the
// schema descriptor must disclose the recovered node so an introspecting client
// is not silently handed a schema derived partly from unparseable input.
func TestMCPListGraphsSurfacesUnparsed(t *testing.T) {
	bundle := mcpBadBundle(t)
	res := callTool(t, "list_graphs", map[string]any{"bundle": bundle})
	if res.IsError {
		t.Fatalf("list_graphs must not be a tool error: %s", toolText(t, res))
	}
	got := resultUnparsed(t, toolText(t, res))
	if len(got) != 1 || got[0] != "bad" {
		t.Fatalf("list_graphs MCP unparsed = %v, want [bad]", got)
	}
}

// TestMCPQueryGraphSurfacesUnparsed is the I-4 MCP fixture for query_graph: every
// traversal result carries the disclosure, so a client reading the graph via a
// query cannot be silently handed results computed over a set that excludes the
// unparseable file.
func TestMCPQueryGraphSurfacesUnparsed(t *testing.T) {
	bundle := mcpBadBundle(t)
	res := callTool(t, "query_graph", map[string]any{
		"bundle": bundle,
		"op":     "lookup",
		"label":  "guide",
	})
	if res.IsError {
		t.Fatalf("query_graph must not be a tool error: %s", toolText(t, res))
	}
	got := resultUnparsed(t, toolText(t, res))
	if len(got) != 1 || got[0] != "bad" {
		t.Fatalf("query_graph MCP unparsed = %v, want [bad]", got)
	}
}

// TestMCPCleanBundleOmitsUnparsed guards the omitempty contract: a conformant
// bundle must NOT grow an unparsed field, keeping the payload byte-identical to
// the pre-fix output (no golden churn). It checks the graph json export and the
// list_graphs / query_graph envelopes on the shared golden bundle.
func TestMCPCleanBundleOmitsUnparsed(t *testing.T) {
	if got := toolText(t, callTool(t, "graph", map[string]any{"bundle": goldenBundle, "format": "json"})); strings.Contains(got, "\"unparsed\"") {
		t.Errorf("clean bundle graph json must omit unparsed:\n%s", got)
	}
	if got := toolText(t, callTool(t, "list_graphs", map[string]any{"bundle": goldenBundle})); strings.Contains(got, "\"unparsed\"") {
		t.Errorf("clean bundle list_graphs must omit unparsed:\n%s", got)
	}
	if got := toolText(t, callTool(t, "query_graph", map[string]any{"bundle": goldenBundle, "op": "lookup", "label": "concept"})); strings.Contains(got, "\"unparsed\"") {
		t.Errorf("clean bundle query_graph must omit unparsed:\n%s", got)
	}
}
