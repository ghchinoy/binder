## binder graph query

Query the bundle's concept graph (lookup|neighbors|neighborhood|pattern|path)

### Synopsis

Query runs one of five read-only verbs over the property graph binder
projects from an OKF bundle — the same graph as `binder graph`, `list_graphs`,
and `project`, so it stays in edge/identity parity by construction.

  --op lookup        list a concept by --id or all of a --label
  --op neighbors     one-hop from --id (--direction out|in|both, optional --rel)
  --op neighborhood  bounded k-hop BFS from --id (--depth 1..5)
  --op pattern       source nodes of --label linking to --to-label and/or a
                     --where-prop/--where-eq predicate (type|tier|stale)
  --op path          bounded shortest hop-path from --from to --to (--max-depth 1..5)

Every traversal is bounded; a query that matches nothing is a result, not an
error. Output is the binder.report/v1 query_graph payload. --id-key is accepted
for parity with schema-describe but does NOT re-key traversal identity in this
version (identity is always the path-derived concept id).

```
binder graph query <bundle> --op <verb> [flags]
```

### Options

```
      --depth int           neighborhood: BFS depth, required, 1..5
      --direction string    neighbors/neighborhood/path: out|in|both (default out)
      --from string         path: the source node id (required for path)
  -h, --help                help for query
      --id string           lookup/neighbors/neighborhood: the subject node id (path-derived concept id)
      --id-key string       accepted for parity with schema-describe; does NOT re-key traversal identity in this version
      --label string        lookup: the concept type to list; pattern: the source concept type (required for pattern)
      --max-depth int       path: maximum hop depth, required, 1..5
      --op string           the query verb: lookup|neighbors|neighborhood|pattern|path (required)
      --rel string          neighbors/neighborhood/pattern: optional exact-match filter on the edge relationship text
      --to string           path: the target node id (required for path)
      --to-label string     pattern: optional target concept type
      --today string        date (YYYY-MM-DD) used for staleness; defaults to now
      --where-eq string     pattern: the exact value the property must equal
      --where-prop string   pattern: property predicate target: type|tier|stale
```

### SEE ALSO

* [binder graph](binder_graph.md)	 - Export the bundle's concept graph (dot|json|graphml|html)

