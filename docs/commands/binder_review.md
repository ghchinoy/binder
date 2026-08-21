## binder review

Summarize a bundle: concepts, unresolved links, orphans, trust tiers, stale

### Synopsis

Review reports the bundle's concepts by type, derived trust tiers, stale
concepts, Attested Computations, entrypoints, orphans, and unresolved
links. A concept with no inbound links is an ENTRYPOINT when it links out
(or is a recognized root README.md, or is named via --entrypoint)
and a true ORPHAN only when it has no inbound AND no outbound links. Trust
tiers and staleness are derived on demand, never stored (spec §5.1/§5.3).

```
binder review <bundle> [flags]
```

### Options

```
      --entrypoint strings   concept id or path to treat as an entrypoint, not an orphan (repeatable); root README.md is recognized automatically
  -h, --help                 help for review
      --json                 emit the review report as deterministic JSON (schema binder.report/v1) instead of prose
      --strict               gate (exit 1) when any review finding is present (orphans, stale, unresolved, unparsed)
      --today string         date (YYYY-MM-DD) used for staleness; defaults to now
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

