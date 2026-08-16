package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// listGraphsGolden is the committed descriptor fixture for the acme_retail
// bundle at fixedToday; {{BUNDLE_ROOT}} is substituted with the actual bundle
// path so the golden is portable across the caller's working directory.
const listGraphsGolden = "../../testdata/list_graphs/acme_retail.json"

// TestListGraphsGoldenFile: the tool payload equals the committed golden
// descriptor byte-for-byte (design §C.3 #4, golden-file coverage).
func TestListGraphsGoldenFile(t *testing.T) {
	raw, err := os.ReadFile(listGraphsGolden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want := strings.ReplaceAll(string(raw), "{{BUNDLE_ROOT}}", goldenBundle)
	got := toolText(t, callTool(t, "list_graphs", map[string]any{
		"bundle": goldenBundle,
		"today":  fixedToday,
	}))
	if got != want {
		t.Fatalf("list_graphs payload does not match golden %s\n--- GOT ---\n%s\n--- WANT ---\n%s", listGraphsGolden, got, want)
	}
}

// listGraphsEnvelope is the expected binder.report/v1 payload built from the
// SAME entry points the tool uses (bundle.Load → graph.Describe → clijson.Encode
// with command "list_graphs"). There is no CLI surface for list_graphs, so
// envelope parity is asserted against the existing encoder directly (design §C.3
// #3): the tool must add no second serialization path.
func listGraphsEnvelope(t *testing.T, bundlePath, today, idKey string) string {
	t.Helper()
	b, err := bundle.Load(bundlePath, native.New())
	if err != nil {
		t.Fatalf("load %s: %v", bundlePath, err)
	}
	desc := graph.Describe(b, today, idKey)
	var buf bytes.Buffer
	if err := clijson.Encode(&buf, testVersion, "list_graphs", desc); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.String()
}

// TestListGraphsEnvelopeParity: the tool payload is byte-identical to encoding
// graph.Describe through the existing clijson encoder — the same discipline
// every other tool follows.
func TestListGraphsEnvelopeParity(t *testing.T) {
	got := toolText(t, callTool(t, "list_graphs", map[string]any{
		"bundle": goldenBundle,
		"today":  fixedToday,
	}))
	want := listGraphsEnvelope(t, goldenBundle, fixedToday, "")
	if got != want {
		t.Fatalf("list_graphs payload not byte-identical to clijson.Encode(Describe(...))\n--- MCP ---\n%s\n--- WANT ---\n%s", got, want)
	}
}

// TestListGraphsIsValidEnvelope: the payload is a well-formed binder.report/v1
// envelope tagged with the list_graphs command.
func TestListGraphsIsValidEnvelope(t *testing.T) {
	res := callTool(t, "list_graphs", map[string]any{"bundle": goldenBundle, "today": fixedToday})
	if res.IsError {
		t.Fatalf("list_graphs on a conformant bundle must not be a tool error: %s", toolText(t, res))
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
	if env.Command != "list_graphs" {
		t.Errorf("command = %q, want list_graphs", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}
	if env.Binder != "binder/"+testVersion {
		t.Errorf("binder = %q, want binder/%s", env.Binder, testVersion)
	}

	var result graph.SchemaSet
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("result is not a SchemaSet: %v", err)
	}
	if len(result.Graphs) != 1 {
		t.Fatalf("graphs = %d, want exactly 1", len(result.Graphs))
	}
	g := result.Graphs[0]
	if g.Name != "acme_retail" {
		t.Errorf("name = %q, want acme_retail (bundle dir basename)", g.Name)
	}
	if g.Source.Kind != "okf-bundle" || g.Source.Root != goldenBundle {
		t.Errorf("source = %+v, want {okf-bundle %s}", g.Source, goldenBundle)
	}
	if g.NodeKey.Strategy != "path" || g.NodeKey.Key != "" {
		t.Errorf("node_key = %+v, want {path ''} with no id_key", g.NodeKey)
	}
	if len(g.NodeLabels) == 0 {
		t.Error("expected at least one node label for a non-empty bundle")
	}
	if len(g.EdgeLabels) != 1 || g.EdgeLabels[0].Label != "LINKS" {
		t.Errorf("edge_labels = %+v, want a single LINKS label", g.EdgeLabels)
	}
}

// TestListGraphsSchemaFidelity: the advertised node/edge labels + counts match
// exactly what `binder graph --format json` emits for the same bundle (both flow
// from the same graph.Build), per design §C.3 #5.
func TestListGraphsSchemaFidelity(t *testing.T) {
	// The raw graph export (identical to `binder graph --format json`).
	rawGraph := toolText(t, callTool(t, "graph", map[string]any{
		"bundle": goldenBundle,
		"format": "json",
		"today":  fixedToday,
	}))
	var model graph.Model
	if err := json.Unmarshal([]byte(rawGraph), &model); err != nil {
		t.Fatalf("graph export does not parse: %v", err)
	}

	// Expected label→count from the export's nodes.
	wantByType := map[string]int{}
	for _, n := range model.Nodes {
		wantByType[n.Type]++
	}

	var env struct {
		Result graph.SchemaSet `json:"result"`
	}
	payload := toolText(t, callTool(t, "list_graphs", map[string]any{"bundle": goldenBundle, "today": fixedToday}))
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		t.Fatalf("list_graphs payload does not parse: %v", err)
	}
	g := env.Result.Graphs[0]

	if g.Counts.Nodes != len(model.Nodes) {
		t.Errorf("counts.nodes = %d, want %d (graph export)", g.Counts.Nodes, len(model.Nodes))
	}
	if g.Counts.Edges != len(model.Edges) {
		t.Errorf("counts.edges = %d, want %d (graph export)", g.Counts.Edges, len(model.Edges))
	}
	if g.EdgeLabels[0].Count != len(model.Edges) {
		t.Errorf("LINKS count = %d, want %d (graph export edges)", g.EdgeLabels[0].Count, len(model.Edges))
	}
	gotByType := map[string]int{}
	for _, nl := range g.NodeLabels {
		gotByType[nl.Label] = nl.Count
	}
	if len(gotByType) != len(wantByType) {
		t.Errorf("node label set = %v, want %v", gotByType, wantByType)
	}
	for typ, n := range wantByType {
		if gotByType[typ] != n {
			t.Errorf("label %q count = %d, want %d", typ, gotByType[typ], n)
		}
	}
}

// TestListGraphsDeterminismSourceDateEpoch: with SOURCE_DATE_EPOCH set, the
// default `today` is derived identically to the direct encoder path, so the
// payloads match without an explicit today param (design §C.3 #4).
func TestListGraphsDeterminismSourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000") // 2023-11-14T22:13:20Z
	got := toolText(t, callTool(t, "list_graphs", map[string]any{"bundle": goldenBundle}))
	want := listGraphsEnvelope(t, goldenBundle, todayOrNow(""), "")
	if got != want {
		t.Fatalf("list_graphs not deterministic under SOURCE_DATE_EPOCH\n--- MCP ---\n%s\n--- WANT ---\n%s", got, want)
	}
	// Two identical calls are byte-identical.
	again := toolText(t, callTool(t, "list_graphs", map[string]any{"bundle": goldenBundle}))
	if again != got {
		t.Fatalf("list_graphs not deterministic across identical calls")
	}
}

// TestListGraphsUsageErrorMissingBundle: a missing required param is a tool
// error, not a crash.
func TestListGraphsUsageErrorMissingBundle(t *testing.T) {
	res := callTool(t, "list_graphs", map[string]any{})
	if !res.IsError {
		t.Fatalf("missing required bundle must be a tool error, got success: %s", toolText(t, res))
	}
}
