package mcp

import (
	"os/exec"
	"strconv"
	"testing"
)

// The Phase-2 scope update added two NET-NEW CLI commands — `binder graph query`
// and `binder graph schema-describe` — as thin adapters over the SAME
// binder.Service operations the MCP query_graph / list_graphs tools drive. These
// commands have no pre-existing golden to match, so the contract we assert is
// PARITY: for the same inputs, the CLI command output must be BYTE-IDENTICAL to the
// MCP tool payload (which is itself golden-backed and envelope-parity-tested). One
// service, one serialization path, two transports.

// qgCLIArgs translates a query_graph tool argument map into the equivalent
// `binder graph query <bundle> ...` CLI flags, so a single source of truth
// (goldenCases) drives both transports.
func qgCLIArgs(bundle string, args map[string]any) []string {
	out := []string{"graph", "query", bundle}
	str := func(flag, key string) {
		if v, ok := args[key].(string); ok && v != "" {
			out = append(out, flag, v)
		}
	}
	str("--op", "op")
	str("--id", "id")
	str("--label", "label")
	str("--direction", "direction")
	str("--rel", "rel")
	str("--to-label", "to_label")
	str("--from", "from")
	str("--to", "to")
	str("--id-key", "id_key")
	str("--today", "today")
	if v, ok := args["depth"].(int); ok {
		out = append(out, "--depth", strconv.Itoa(v))
	}
	if v, ok := args["max_depth"].(int); ok {
		out = append(out, "--max-depth", strconv.Itoa(v))
	}
	if w, ok := args["where"].(map[string]any); ok {
		if p, ok := w["prop"].(string); ok {
			out = append(out, "--where-prop", p)
		}
		if e, ok := w["eq"].(string); ok {
			out = append(out, "--where-eq", e)
		}
	}
	return out
}

// TestGraphQueryCLIParity: for every op, `binder graph query` is BYTE-IDENTICAL to
// the query_graph MCP tool for the same inputs. This is the parity contract that
// stands in for a golden on the net-new CLI surface (Phase-2 scope update).
func TestGraphQueryCLIParity(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			args := map[string]any{}
			for k, v := range c.args {
				args[k] = v
			}
			args["bundle"] = goldenBundle

			got := cliJSON(t, qgCLIArgs(goldenBundle, args)...)
			want := toolText(t, callTool(t, "query_graph", args))
			if got != want {
				t.Fatalf("`graph query` not byte-identical to query_graph tool\nargs: %v\n--- CLI ---\n%s\n--- MCP ---\n%s",
					qgCLIArgs(goldenBundle, args), got, want)
			}
		})
	}
}

// TestGraphSchemaDescribeCLIParity: `binder graph schema-describe` is
// BYTE-IDENTICAL to the list_graphs MCP tool for the same inputs (with and without
// an id_key).
func TestGraphSchemaDescribeCLIParity(t *testing.T) {
	cases := []struct {
		name  string
		idKey string
	}{
		{"default_path_identity", ""},
		{"with_id_key", "concept-id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			toolArgs := map[string]any{"bundle": goldenBundle, "today": fixedToday}
			cliArgs := []string{"graph", "schema-describe", goldenBundle, "--today", fixedToday}
			if c.idKey != "" {
				toolArgs["id_key"] = c.idKey
				cliArgs = append(cliArgs, "--id-key", c.idKey)
			}
			got := cliJSON(t, cliArgs...)
			want := toolText(t, callTool(t, "list_graphs", toolArgs))
			if got != want {
				t.Fatalf("`graph schema-describe` not byte-identical to list_graphs tool\n--- CLI ---\n%s\n--- MCP ---\n%s", got, want)
			}
		})
	}
}

// TestGraphQueryCLIUsageError: a malformed CLI query (unknown op) is a usage error
// the CLI maps to exit 2 — the SAME semantic validation that makes the MCP tool
// return IsError=true — proving the check lives in the shared service, not
// per-transport.
func TestGraphQueryCLIUsageError(t *testing.T) {
	// Shell out directly to observe the exit code (cliJSON swallows *exec.ExitError).
	err := exec.Command(binderBin, "graph", "query", goldenBundle, "--op", "traverse").Run()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unknown op must fail with a usage exit, got err=%v", err)
	}
	if code := ee.ExitCode(); code != 2 {
		t.Errorf("unknown op exit = %d, want 2 (usage)", code)
	}

	// And the MCP transport treats the identical input as a tool error.
	if res := callTool(t, "query_graph", map[string]any{"op": "traverse", "bundle": goldenBundle}); !res.IsError {
		t.Errorf("query_graph unknown op must be a tool error over MCP too")
	}
}
