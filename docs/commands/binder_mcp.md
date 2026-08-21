## binder mcp

Run binder as a stdio MCP server (additive verbs as MCP tools)

### Synopsis

MCP starts a Model Context Protocol server over stdio, exposing binder's
additive verbs as tools to an MCP-capable agent harness (Claude Code,
Cursor, Zed). Each tool returns the same deterministic binder.report/v1
payload as the corresponding `binder <cmd> --json`, reusing the same
internal entry points and JSON encoder — no second serialization path.

Tools: convert, validate, review, lint, graph, list_graphs, query_graph.

The server serves over stdio until the client disconnects. Wire it into a
harness, e.g.: claude mcp add binder -- binder mcp

```
binder mcp [flags]
```

### Options

```
  -h, --help   help for mcp
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

