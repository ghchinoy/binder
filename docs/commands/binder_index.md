## binder index

(Re)generate the per-directory index.md nav tree (spec §8)

### Synopsis

Index regenerates each directory's index.md as a navigation tree listing
that directory's concepts and immediate subdirectories (spec §8). The
bundle-root index.md declares okf_version (spec §12). log.md files are
never touched. Existing index.md files are regenerated; each write is
reported so nothing is overwritten silently.

```
binder index <bundle> [flags]
```

### Options

```
      --dry-run             report which index.md files would be written without writing
      --group-by-type       append an additive "# Catalog" of all concepts grouped by type to the root index.md
  -h, --help                help for index
      --include-backlinks   annotate catalog entries with inbound resolved edges (requires --group-by-type)
      --include-graph       annotate catalog entries with outbound resolved edges (requires --group-by-type)
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

