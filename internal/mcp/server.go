// Package mcp is binder's stdio MCP server surface (issue #15). It exposes
// binder's additive verbs (convert/validate/review/lint/graph) as MCP tools
// that return the SAME binder.report/v1 payloads as `--json`.
//
// The server adds NO business logic and NO second serialization path: each tool
// handler decodes typed params, calls the existing internal/* entry point, and
// encodes the returned struct with the existing internal/clijson encoder. The
// bytes are identical to `binder <cmd> --json`.
//
// The official MCP Go SDK (github.com/modelcontextprotocol/go-sdk) is confined
// to this package and cmd/mcp.go; it MUST NOT leak into the core codec or the
// internal/* Report types (dependency-rule invariant, design §Decision 1/2).
package mcp

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
)

// deps carries the shared dependencies every tool handler needs: the injected
// codec (the composition root's choice, per the dependency rule) and the binder
// version stamped into the clijson envelope.
type deps struct {
	codec   okf.Codec
	version string
}

// Serve constructs the binder MCP server and serves it over stdio until the
// client disconnects (ctx cancelled or transport closed). This is the single
// entry point cmd/mcp.go calls, keeping the SDK's Transport out of cmd.
func Serve(ctx context.Context, codec okf.Codec, version string) error {
	return newServer(codec, version).Run(ctx, &mcp.StdioTransport{})
}

// newServer builds the MCP server with all tools registered. It is unexported
// and used by both Serve and the in-package tests (which drive it over the SDK's
// in-memory transport).
func newServer(codec okf.Codec, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "binder",
		Version: version,
	}, nil)
	d := &deps{codec: codec, version: version}

	registerConvert(s, d)
	registerValidate(s, d)
	registerReview(s, d)
	registerLint(s, d)
	registerGraph(s, d)

	return s
}

// resolveNow mirrors cmd.resolveNow: it honors SOURCE_DATE_EPOCH for
// reproducible output, falling back to the wall clock. Duplicated (rather than
// exported from cmd) to keep the SDK-quarantined server free of a cmd import;
// the SOURCE_DATE_EPOCH contract is identical so payloads match the CLI's.
func resolveNow() time.Time {
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(secs, 0).UTC()
		}
	}
	return time.Now()
}

// todayOrNow returns the explicit today param, or today's date derived from
// resolveNow when the param is empty — the same default the CLI applies for
// review/lint/graph.
func todayOrNow(today string) string {
	if today != "" {
		return today
	}
	return resolveNow().Format("2006-01-02")
}

// encode renders result as the deterministic clijson envelope — byte-identical
// to `binder <command> --json` — and returns it as the tool's text content.
// There is deliberately no second serialization path: the same clijson.Encode
// the CLI uses produces the tool payload. Out is `any` and the returned output
// is nil, so the SDK does not attach a structured-output payload and uses this
// Content verbatim.
func (d *deps) encode(command string, result any) (*mcp.CallToolResult, any, error) {
	var buf bytes.Buffer
	if err := clijson.Encode(&buf, d.version, command, result); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: buf.String()}},
	}, nil, nil
}
