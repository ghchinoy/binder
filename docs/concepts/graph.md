# Relationship extraction & the graph

binder extracts every relationship signal it can from a corpus and projects a
**graph** from the bundle's resolved links. This page collects how relationships
are extracted and what the derived graph is; for the model itself — nodes, edges,
labels, property maps, and the adjacency index — see the
[in-memory LPG primer](../lpg-inmemory-primer.md).

## Relationship extraction

binder extracts every relationship signal it can and rewrites resolved ones into
persisted body links, so the graph survives reload. Unresolved references are
**left in place and reported** — never dropped (spec §6/§11). Link-like text
inside fenced/indented code blocks or inline code spans is ignored throughout
(the same markdown-aware code detection the codec uses).

| Kind | Always on? | Behavior |
|---|---|---|
| **Standard markdown links** | yes | `[text](target.md#anchor)` pointing at a corpus concept is rewritten to `/bundle-relative.md#anchor`. External URLs, `#anchors`, and non-`.md` targets are left untouched. |
| **Wikilinks** | yes | `[[Target]]` and `[[Target\|alias]]` are resolved and rewritten to a standard markdown link `[display](/target.md)`. Unresolved wikilinks stay as `[[...]]` and are reported. |
| **Hashtags** | yes | `#hashtag` tokens in the body are merged (de-duplicated, order-preserving) into frontmatter `tags` (spec §4). A trailing hashtag in an H1 is stripped from the derived title but still tagged. |
| **Frontmatter refs** | opt-in (`--fm-ref-keys`) | Named frontmatter keys (e.g. `related`, `parent`) become edges. The original key/value is preserved, and each resolved target is materialized as a link in a trailing `## Related` section so the edge survives reload. |

## The graph surface

Every binder command that touches relationships works from the same derived
structure: a **graph** binder projects from the bundle's resolved links. binder
does not store a graph — it rebuilds one from the concepts and their links on
every call, hands you a view, and forgets it.

That projection is a labeled property graph
([primer](../lpg-inmemory-primer.md)). Three surfaces read it:

- **`binder graph`** — a CLI command that exports the *whole* graph in one of
  four formats.
- **`list_graphs`** — an MCP tool that describes the graph's *schema* (its
  labels, counts, and property declarations).
- **`query_graph`** — an MCP tool that answers *data* questions about the graph:
  lookup, neighbors, k-hop neighborhood, pattern match, and path existence.

`list_graphs` and `query_graph` are **MCP-only**: there is no `binder
list_graphs` or `binder query-graph` CLI verb.

## The graph model

binder projects a **labeled property graph**:

- **Nodes are concepts.** A node's **label** is its concept `type` (`Table`,
  `Metric`, `Policy`, …). Every node carries the same five queryable
  **properties**: `id`, `title`, `type`, `tier` (the derived trust tier), and
  `stale`.
- **Edges are resolved links.** There is exactly **one edge label, `LINKS`** —
  binder's links are untyped. Each edge carries three properties: `from`, `to`,
  and `text` (the link's text, which serves as a relationship label *by
  convention only*).

![The example LPG: nodes carry labels and a property map; the directed edges carry their own properties too.](../assets/lpg-model.webp)

## A read-only projection

The graph is a **read model**: derived from the bundle on every call, never
stored, never written back. `binder graph`, `list_graphs`, and `query_graph`
cannot change a bundle — not its frontmatter, not its links, not an identity. You
can run any of them against a production bundle, in any order, as often as you
like, and the bundle bytes are unchanged. If you need the graph again, ask again:
it is always recomputed from the current bundle, so it never drifts from the
source.

A runnable sample bundle and its graph views live in
[`examples/graph-sample/`](../examples/graph-sample/). For the full graph surface
— export formats, node identity, and `query_graph` operations — see the
[user guide](../user_guide.md#the-graph-surface). To take the graph offline into
Spanner, see [Graph projection (`project`)](project.md).
