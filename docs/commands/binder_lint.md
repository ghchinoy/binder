## binder lint

Check a source markdown corpus for broken links, missing titles, orphans, stale, schema issues

### Synopsis

Lint performs a read-only pass over a SOURCE markdown corpus (it writes
nothing) and reports broken links (incl. #anchors), missing titles, orphan
concepts, entrypoints, stale concepts, and schema violations (missing
type:, invalid frontmatter). A concept with no inbound links is an
ENTRYPOINT when it links out (or is a recognized root README.md,
or is named via --entrypoint) and a true ORPHAN only when it has no inbound
AND no outbound links. Unlike `binder review`/`binder validate`, which read
an emitted bundle, lint sees the corpus as authored — a missing title or
type: is masked once convert defaults it.

Findings are advisory: bare lint always exits 0 (entrypoints never gate).
Use --strict to gate (exit 1) when any finding is present, e.g. in CI.

```
binder lint <corpus> [flags]
```

### Options

```
      --entrypoint strings   concept id or path to treat as an entrypoint, not an orphan (repeatable); root README.md is recognized automatically
  -h, --help                 help for lint
      --json                 emit the lint report as deterministic JSON (schema binder.report/v1) instead of prose
      --strict               gate (exit 1) when any lint finding is present; without it lint never gates (never-reject)
      --today string         date (YYYY-MM-DD) used for staleness; defaults to now
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

