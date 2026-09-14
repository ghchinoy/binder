## binder graph schema-describe

Introspect the property graph(s) binder can project from a bundle

### Synopsis

Schema-describe reports the property graph(s) binder can project from an OKF
bundle: graph name, node labels (the concept types present) and the single
LINKS edge label, each with property declarations and counts. It is derived
from the same projection as `binder graph`, so it stays in parity by
construction.

Read-only; output is the binder.report/v1 list_graphs payload. Node identity
(node_key) is the concept's authored frontmatter value under --id-key when
present and non-empty, otherwise the path-derived concept id (spec §2).

```
binder graph schema-describe <bundle> [flags]
```

### Options

```
  -h, --help            help for schema-describe
      --id-key string   frontmatter key to prefer as the stable node key; empty falls back to path-as-identity (spec §2)
      --today string    date (YYYY-MM-DD) used for staleness; defaults to now
```

### SEE ALSO

* [binder graph](binder_graph.md)	 - Export the bundle's concept graph (dot|json|graphml|html)

