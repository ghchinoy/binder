## binder convert

Convert a markdown corpus into an OKF v0.2 bundle

### Synopsis

Convert walks a plain-markdown corpus and writes a conformant OKF v0.2
bundle: one concept per non-reserved .md, standard markdown links rewritten
to bundle-relative form, a root index.md declaring okf_version, and a
generated provenance stamp. Output is
deterministic for identical inputs, with a single time-varying field: the
generated provenance timestamp (generated.at), which records when the run
actually happened. Pin SOURCE_DATE_EPOCH to make output byte-identical across runs.

```
binder convert <src> [flags]
```

### Options

```
      --canonicalize-status         opt-in: rewrite known --status-map aliases to the OKF §5.4 vocabulary (active->stable, wip/in-progress->draft, archived/legacy->deprecated); off by default, each rewrite is reported
      --default-type string         type applied when none is present or mapped (default "Note")
      --dry-run                     report what would be written without writing anything
      --external-root stringArray   declare a KNOWN sibling-workspace root (repeatable); file:// links under it stay external but suppress the outside-root advisory
      --fm-ref-keys string          frontmatter keys treated as relationship edges, e.g. "related,parent"
      --group-by-type               append an additive "# Catalog" of all concepts grouped by type to the root index.md
  -h, --help                        help for convert
      --include-backlinks           annotate catalog entries with inbound resolved edges (requires --group-by-type)
      --include-graph               annotate catalog entries with outbound resolved edges (requires --group-by-type)
      --json                        emit the run report as deterministic JSON (schema binder.report/v1) instead of prose
      --map-citations               map a body "# Citations" list into sources entries
      --map-draft                   map a draft:true marker to status:draft when status is absent
  -o, --output string               output bundle directory
      --report string               also write the run report to this file
      --source-keys string          frontmatter keys to map into sources entries, e.g. "source,author"
      --stale-after-map string      per-directory stale_after relative to now, e.g. "07-benchmarks=+6m,legacy=+0d" (grammar +Nd/+Nm/+Ny; set only when absent)
      --status-map string           per-directory status, e.g. "archive=deprecated,drafts=draft,default=active" (set only when status absent)
      --strict                      gate (exit 1) on unresolved links or recovery warnings; without it these never gate (never-reject)
      --type-map string             per-directory type overrides, e.g. "docs=Guide,adr=Decision"
      --verified-by string          actor to append as a verified stamp, e.g. "human:ghchinoy" or "binder/0.3.0"; a stamp is written ONLY when passed here, or when verified_by is set in your GLOBAL config (neither BINDER_VERIFIED_BY nor a repo-local .binder.yaml authorizes stamping; valid forms: human:<id>, process:<id>, team:<id>, or <producer>/<version> (e.g. binder/0.3.0))
      --workspace-root string       boundary within which file:// links resolve to internal edges (default: the <src> root)
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

