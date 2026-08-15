package cmd

import (
	"github.com/spf13/cobra"

	mcpserver "github.com/ghchinoy/binder/internal/mcp"
	"github.com/ghchinoy/binder/internal/okf"
)

// newMCPCmd builds `binder mcp`: a stdio MCP server exposing binder's additive
// verbs (convert/validate/review/lint/graph) as MCP tools whose payloads are the
// SAME binder.report/v1 envelopes as `--json`. It is a transport, not a
// report-producing command, so it has no `--json` flag (design §Non-Goals).
//
// The MCP SDK dependency is confined to internal/mcp; this command only calls
// mcpserver.Serve, keeping the SDK out of the rest of cmd/.
func newMCPCmd(codec okf.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run binder as a stdio MCP server (convert/validate/review/lint/graph tools)",
		Long: "MCP starts a Model Context Protocol server over stdio, exposing binder's\n" +
			"additive verbs as tools to an MCP-capable agent harness (Claude Code,\n" +
			"Cursor, Zed). Each tool returns the same deterministic binder.report/v1\n" +
			"payload as the corresponding `binder <cmd> --json`, reusing the same\n" +
			"internal entry points and JSON encoder — no second serialization path.\n\n" +
			"The server serves over stdio until the client disconnects. Wire it into a\n" +
			"harness, e.g.: claude mcp add binder -- binder mcp",
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mcpserver.Serve(cmd.Context(), codec, Version)
		},
	}
}
