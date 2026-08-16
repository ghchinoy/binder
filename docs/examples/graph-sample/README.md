# Graph-format sample

A tiny, self-contained OKF bundle and the two graph views binder derives from it,
so you can see the exact JSON shapes before running the tools on your own corpus.
See [The graph surface](../../user_guide.md#the-graph-surface) in the user guide for
the full walkthrough.

## Contents

| File | What it is |
|---|---|
| [`orders-kb/`](orders-kb/) | The bundle: two concepts (`orders` → `customer`) and a generated `index.md`. `orders.md` and `customer.md` carry an authored `slug` frontmatter key used for the identity-model example. |
| [`graph.json`](graph.json) | Output of `binder graph --format json` — the raw `{nodes, edges}` export. |
| [`list_graphs.json`](list_graphs.json) | The `list_graphs` MCP tool payload — the LPG schema descriptor (labels, counts, property declarations). |

## Regenerating these files

Every byte here was produced by running the tools; nothing is hand-authored. From
the repository root, with a `binder` on your `PATH`:

```bash
# graph.json — the raw graph export
binder graph docs/examples/graph-sample/orders-kb --format json > docs/examples/graph-sample/graph.json

# list_graphs.json — the schema descriptor, over MCP (list_graphs has no CLI verb)
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"p","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_graphs","arguments":{"bundle":"docs/examples/graph-sample/orders-kb"}}}' \
  | binder mcp 2>/dev/null \
  | grep '"id":2' | jq -r '.result.content[0].text' > docs/examples/graph-sample/list_graphs.json
```

The graph is a read-only projection derived from the bundle on every call — binder
never stores or writes it back. The `binder` field in `list_graphs.json` is the
runtime version string of the binary that produced it, not a fixed constant.
