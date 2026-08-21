## binder graph

Export the bundle's concept graph (dot|json|graphml|html)

### Synopsis

Graph exports the bundle's concept graph. Edges are exactly the bundle's
resolved links (spec §6), so the graph matches validate and review. Output
is deterministic.

graph is already machine-readable, so --json is an alias for --format json
(the raw {nodes,edges} export, NOT the report envelope used by the other
commands). Combining --json with a conflicting --format is a usage error.

```
binder graph <bundle> [flags]
```

### Options

```
      --format string   output format: dot|json|graphml|html (default "dot")
  -h, --help            help for graph
      --json            alias for --format json (the raw {nodes,edges} export, not the report envelope)
  -o, --output string   write graph to a file instead of stdout
      --today string    date (YYYY-MM-DD) used for staleness; defaults to now
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

