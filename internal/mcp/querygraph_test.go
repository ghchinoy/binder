package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// queryGraphGoldenDir holds the committed golden payloads. Each golden carries
// {{BUNDLE_ROOT}} (substituted with the actual bundle path so it is portable) and
// {{VERSION}} (substituted with testVersion so no version literal is baked in,
// per design §14.3). Set UPDATE_GOLDEN=1 to regenerate them.
const queryGraphGoldenDir = "../../testdata/query_graph"

// qgCase is one golden-backed query_graph call on the acme_retail bundle.
type qgCase struct {
	name string
	args map[string]any
}

// goldenCases exercise every op with deterministic inputs (fixedToday) so the
// committed golden is byte-stable.
func goldenCases() []qgCase {
	return []qgCase{
		{"lookup_by_id", map[string]any{"op": "lookup", "id": "tables/orders", "today": fixedToday}},
		{"lookup_by_label", map[string]any{"op": "lookup", "label": "Metric", "today": fixedToday}},
		{"neighbors_out", map[string]any{"op": "neighbors", "id": "metrics/gross-margin", "direction": "out", "today": fixedToday}},
		{"neighbors_both", map[string]any{"op": "neighbors", "id": "metrics/gross-margin", "direction": "both", "today": fixedToday}},
		{"neighborhood_depth2", map[string]any{"op": "neighborhood", "id": "metrics/gross-margin", "depth": 2, "direction": "out", "today": fixedToday}},
		{"pattern_to_label", map[string]any{"op": "pattern", "label": "Policy", "to_label": "Metric", "today": fixedToday}},
		{"pattern_where", map[string]any{"op": "pattern", "label": "Policy", "where": map[string]any{"prop": "type", "eq": "BigQuery Table"}, "today": fixedToday}},
		{"path_shortest", map[string]any{"op": "path", "from": "metrics/gross-margin", "to": "computations/revenue-ytd", "max_depth": 3, "today": fixedToday}},
		{"path_unreachable", map[string]any{"op": "path", "from": "tables/orders", "to": "metrics/gross-margin", "max_depth": 5, "today": fixedToday}},
		{"lookup_id_key", map[string]any{"op": "lookup", "id": "tables/orders", "id_key": "concept-id", "today": fixedToday}},
	}
}

// TestQueryGraphGoldenFiles: every op's payload matches its committed golden
// byte-for-byte (design §12.4/§12.6). These goldens double as the determinism and
// verb-correctness record.
func TestQueryGraphGoldenFiles(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			c.args["bundle"] = goldenBundle
			got := toolText(t, callTool(t, "query_graph", c.args))
			path := filepath.Join(queryGraphGoldenDir, c.name+".json")

			if os.Getenv("UPDATE_GOLDEN") == "1" {
				norm := strings.ReplaceAll(got, goldenBundle, "{{BUNDLE_ROOT}}")
				norm = strings.ReplaceAll(norm, "binder/"+testVersion, "binder/{{VERSION}}")
				if err := os.MkdirAll(queryGraphGoldenDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(path, []byte(norm), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to generate)", path, err)
			}
			want := strings.ReplaceAll(string(raw), "{{BUNDLE_ROOT}}", goldenBundle)
			want = strings.ReplaceAll(want, "{{VERSION}}", testVersion)
			if got != want {
				t.Fatalf("payload does not match golden %s\n--- GOT ---\n%s\n--- WANT ---\n%s", path, got, want)
			}
		})
	}
}

// TestQueryGraphDeterminism: two identical calls are byte-identical, and a call
// under a set SOURCE_DATE_EPOCH (no explicit today) equals the direct
// encoder path — the same discipline as list_graphs (design §12.4).
func TestQueryGraphDeterminism(t *testing.T) {
	args := map[string]any{"op": "neighborhood", "id": "metrics/gross-margin", "depth": 2, "direction": "out", "today": fixedToday, "bundle": goldenBundle}
	a := toolText(t, callTool(t, "query_graph", args))
	b := toolText(t, callTool(t, "query_graph", args))
	if a != b {
		t.Fatalf("identical calls diverged\n--- A ---\n%s\n--- B ---\n%s", a, b)
	}

	t.Setenv("SOURCE_DATE_EPOCH", "1700000000") // 2023-11-14T22:13:20Z
	got := toolText(t, callTool(t, "query_graph", map[string]any{"op": "lookup", "label": "Metric", "bundle": goldenBundle}))
	want := queryGraphEnvelope(t, func(idx *graph.Index) any {
		return idx.Lookup("", "", "Metric")
	}, todayOrNow(""))
	if got != want {
		t.Fatalf("not deterministic under SOURCE_DATE_EPOCH\n--- MCP ---\n%s\n--- WANT ---\n%s", got, want)
	}
}

// queryGraphEnvelope builds the expected payload from the SAME entry points the
// tool uses (bundle.Load → graph.Build → graph.NewIndex → verb → clijson.Encode
// with command "query_graph"). There is no CLI surface for query_graph, so
// envelope parity is asserted against the existing encoder directly: the tool must
// add no second serialization path (design §12.3).
func queryGraphEnvelope(t *testing.T, verb func(*graph.Index) any, today string) string {
	t.Helper()
	b, err := bundle.Load(goldenBundle, native.New())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	idx := graph.NewIndex(graph.Build(b, today))
	var buf bytes.Buffer
	if err := clijson.Encode(&buf, testVersion, "query_graph", verb(idx)); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.String()
}

// TestQueryGraphEnvelopeParity: the tool payload is byte-identical to encoding the
// verb's result through the existing clijson encoder — no second serialization
// path (design §12.3).
func TestQueryGraphEnvelopeParity(t *testing.T) {
	got := toolText(t, callTool(t, "query_graph", map[string]any{
		"op": "neighbors", "id": "metrics/gross-margin", "direction": "out", "today": fixedToday, "bundle": goldenBundle,
	}))
	want := queryGraphEnvelope(t, func(idx *graph.Index) any {
		return idx.Neighbors("", "metrics/gross-margin", "out", "")
	}, fixedToday)
	if got != want {
		t.Fatalf("payload not byte-identical to clijson.Encode(verb(...))\n--- MCP ---\n%s\n--- WANT ---\n%s", got, want)
	}
}

// TestQueryGraphIsValidEnvelope: the payload is a well-formed binder.report/v1
// envelope tagged with the query_graph command (design §12.3).
func TestQueryGraphIsValidEnvelope(t *testing.T) {
	res := callTool(t, "query_graph", map[string]any{"op": "lookup", "id": "tables/orders", "today": fixedToday, "bundle": goldenBundle})
	if res.IsError {
		t.Fatalf("lookup on a conformant bundle must not be a tool error: %s", toolText(t, res))
	}
	var env struct {
		Binder  string          `json:"binder"`
		Command string          `json:"command"`
		Schema  string          `json:"schema"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(toolText(t, res)), &env); err != nil {
		t.Fatalf("payload is not a valid envelope: %v", err)
	}
	if env.Command != "query_graph" {
		t.Errorf("command = %q, want query_graph", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}
	if env.Binder != "binder/"+testVersion {
		t.Errorf("binder = %q, want binder/%s", env.Binder, testVersion)
	}
}

// TestQueryGraphNodeKeyEcho: a non-empty id_key is echoed verbatim with
// honored:false and strategy:path — the §14.1 amendment, observable rather than a
// silent no-op.
func TestQueryGraphNodeKeyEcho(t *testing.T) {
	payload := toolText(t, callTool(t, "query_graph", map[string]any{
		"op": "lookup", "id": "tables/orders", "id_key": "concept-id", "today": fixedToday, "bundle": goldenBundle,
	}))
	var env struct {
		Result struct {
			NodeKey graph.ResultNodeKey `json:"node_key"`
			Nodes   []graph.Node        `json:"nodes"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		t.Fatalf("payload does not parse: %v", err)
	}
	if env.Result.NodeKey.Strategy != "path" {
		t.Errorf("strategy = %q, want path", env.Result.NodeKey.Strategy)
	}
	if env.Result.NodeKey.Key != "concept-id" {
		t.Errorf("key = %q, want concept-id (echoed verbatim)", env.Result.NodeKey.Key)
	}
	if env.Result.NodeKey.Honored {
		t.Error("honored = true, want false (id_key never re-keys identity in v1)")
	}
	// Identity is unchanged: still the path-derived Concept.ID.
	if len(env.Result.Nodes) != 1 || env.Result.Nodes[0].ID != "tables/orders" {
		t.Errorf("identity changed under id_key: %+v", env.Result.Nodes)
	}
}

// TestQueryGraphNeverReject: a well-formed query with no matches returns
// IsError=false with an empty result in the payload — for every op that can match
// nothing (design §12.7).
func TestQueryGraphNeverReject(t *testing.T) {
	cases := []struct {
		name   string
		args   map[string]any
		expect string // a substring the empty-but-well-formed payload must contain
	}{
		{"lookup absent id", map[string]any{"op": "lookup", "id": "no/such", "today": fixedToday}, `"not_found": true`},
		{"lookup empty label", map[string]any{"op": "lookup", "label": "NoSuchLabel", "today": fixedToday}, `"nodes": []`},
		{"neighbors absent", map[string]any{"op": "neighbors", "id": "no/such", "today": fixedToday}, `"not_found": true`},
		{"neighborhood absent", map[string]any{"op": "neighborhood", "id": "no/such", "depth": 2, "today": fixedToday}, `"not_found": true`},
		{"pattern no match", map[string]any{"op": "pattern", "label": "Policy", "where": map[string]any{"prop": "stale", "eq": "true"}, "today": fixedToday}, `"nodes": []`},
		{"path unreachable", map[string]any{"op": "path", "from": "tables/orders", "to": "metrics/revenue", "max_depth": 5, "today": fixedToday}, `"exists": false`},
		{"path absent endpoint", map[string]any{"op": "path", "from": "no/such", "to": "metrics/revenue", "max_depth": 5, "today": fixedToday}, `"not_found": true`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.args["bundle"] = goldenBundle
			res := callTool(t, "query_graph", c.args)
			if res.IsError {
				t.Fatalf("no-match query must not be a tool error: %s", toolText(t, res))
			}
			if payload := toolText(t, res); !strings.Contains(payload, c.expect) {
				t.Fatalf("payload missing %q:\n%s", c.expect, payload)
			}
		})
	}
}

// TestQueryGraphUsageErrors: only malformed input is a usage error (IsError=true).
// Covers unknown op, missing required params, out-of-range depth, bad direction,
// invalid where.prop, and the lookup one-of violation (design §12.5/§12.7).
func TestQueryGraphUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{"unknown op", map[string]any{"op": "traverse", "bundle": goldenBundle}},
		{"missing op", map[string]any{"bundle": goldenBundle}},
		{"lookup neither id nor label", map[string]any{"op": "lookup"}},
		{"lookup both id and label", map[string]any{"op": "lookup", "id": "x", "label": "y"}},
		{"neighbors missing id", map[string]any{"op": "neighbors"}},
		{"neighbors bad direction", map[string]any{"op": "neighbors", "id": "x", "direction": "sideways"}},
		{"neighborhood depth zero", map[string]any{"op": "neighborhood", "id": "x", "depth": 0}},
		{"neighborhood depth over cap", map[string]any{"op": "neighborhood", "id": "x", "depth": 6}},
		{"pattern missing label", map[string]any{"op": "pattern", "to_label": "Metric"}},
		{"pattern no predicate", map[string]any{"op": "pattern", "label": "Policy"}},
		{"pattern bad prop", map[string]any{"op": "pattern", "label": "Policy", "where": map[string]any{"prop": "color", "eq": "red"}}},
		{"path missing to", map[string]any{"op": "path", "from": "x"}},
		{"path depth over cap", map[string]any{"op": "path", "from": "x", "to": "y", "max_depth": 99}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Include a bundle where one is not already provided, to prove the
			// usage error is about the semantic params, not a missing bundle.
			if _, ok := c.args["bundle"]; !ok {
				c.args["bundle"] = goldenBundle
			}
			res := callTool(t, "query_graph", c.args)
			if !res.IsError {
				t.Fatalf("malformed input must be a usage error, got success: %s", toolText(t, res))
			}
		})
	}
}

// TestQueryGraphBundleIOError: an unloadable bundle is an IO-class tool error,
// exactly as the other tools behave.
func TestQueryGraphBundleIOError(t *testing.T) {
	res := callTool(t, "query_graph", map[string]any{"op": "lookup", "label": "Metric", "bundle": "../../testdata/does-not-exist"})
	if !res.IsError {
		t.Fatalf("unloadable bundle must be a tool error, got success: %s", toolText(t, res))
	}
}
