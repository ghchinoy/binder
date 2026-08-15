package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/okf/native"
)

// testVersion is the binder version the tests stamp; it must match the CLI's
// cmd.Version so the parity comparison is byte-identical.
const testVersion = "0.1.0"

// goldenBundle is a stable, conformant OKF v0.2 fixture shared with the
// validate/review/graph tests.
const goldenBundle = "../../testdata/okf-bundles/acme_retail"

// binderBin is the path to a binder binary built once for the CLI parity tests.
var binderBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "binder-mcp-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	binderBin = filepath.Join(dir, "binder")
	// Build the real CLI from the module root so parity tests exercise the
	// genuine cmd/*.go path (offline: deps are vendored). Pin cmd.Version via
	// ldflags to testVersion exactly as the release pipeline does
	// (.goreleaser.yaml), so the stamped "binder/<version>" is deterministic and
	// matches the MCP side — otherwise Go embeds a VCS pseudo-version, which the
	// cmd.Version build-info fallback would surface.
	cmd := exec.Command("go", "build",
		"-ldflags", "-X github.com/ghchinoy/binder/cmd.Version="+testVersion,
		"-o", binderBin, "github.com/ghchinoy/binder")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("building binder for parity tests: " + err.Error())
	}
	os.Exit(m.Run())
}

// callTool drives newServer over the SDK's in-memory transport: it connects a
// client, calls the named tool with args, and returns the result.
func callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()

	server := newServer(native.New(), testVersion)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call tool %q: %v", name, err)
	}
	return res
}

// toolText extracts the single TextContent payload from a tool result.
func toolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("expected exactly 1 content block, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

// cliJSON runs `binder <args...> --json` and returns stdout bytes.
func cliJSON(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(binderBin, args...).Output()
	if err != nil {
		// A gating exit code (1) still writes the report to stdout; only treat a
		// non-ExitError (e.g. binary missing) as fatal.
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("running binder %v: %v", args, err)
		}
	}
	return string(out)
}

// TestListTools asserts the server advertises exactly the five additive tools,
// each with a non-empty input schema — and NOT the deferred Non-Goals
// (enrich/emit_concept/read/search).
func TestListTools(t *testing.T) {
	ctx := context.Background()
	server := newServer(native.New(), testVersion)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", tool.Name)
		}
	}
	want := []string{"convert", "validate", "review", "lint", "graph"}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tool %q not advertised", name)
		}
	}
	if len(res.Tools) != len(want) {
		t.Errorf("advertised %d tools, want exactly %d (%v); got %v", len(res.Tools), len(want), want, got)
	}
	for _, ng := range []string{"enrich", "emit_concept", "read", "search"} {
		if got[ng] {
			t.Errorf("Non-Goal tool %q must not be exposed", ng)
		}
	}
}

// TestValidateRoundTrip is the Phase-1 gate: a validate call over the SDK
// transport returns a well-formed binder.report/v1 envelope in-band (findings,
// if any, are in the payload — not an MCP error).
func TestValidateRoundTrip(t *testing.T) {
	res := callTool(t, "validate", map[string]any{"bundle": goldenBundle})
	if res.IsError {
		t.Fatalf("validate on a conformant bundle must not be a tool error: %s", toolText(t, res))
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
	if env.Command != "validate" {
		t.Errorf("command = %q, want %q", env.Command, "validate")
	}
	if env.Schema != "binder.report/v1" {
		t.Errorf("schema = %q, want binder.report/v1", env.Schema)
	}
	if env.Binder != "binder/"+testVersion {
		t.Errorf("binder = %q, want binder/%s", env.Binder, testVersion)
	}
}

// TestValidateParityCLI is the Phase-1 parity gate: the tool payload is
// BYTE-IDENTICAL to `binder validate <bundle> --json`.
func TestValidateParityCLI(t *testing.T) {
	got := toolText(t, callTool(t, "validate", map[string]any{"bundle": goldenBundle}))
	want := cliJSON(t, "validate", goldenBundle, "--json")
	if got != want {
		t.Fatalf("MCP validate payload not byte-identical to `validate --json`\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}
