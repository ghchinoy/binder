## binder enrich

Inject missing OKF frontmatter into a source markdown tree, in place

### Synopsis

Enrich adds the missing required OKF frontmatter (type, title, generated)
to the markdown files under <src>, IN PLACE. It touches FRONTMATTER ONLY:
unlike `binder convert`, it does no link rewriting, no index generation, no
"## Related" section, and no tag merge — bodies are otherwise untouched.

It operates on the YAML only, so its writes stay reviewable on a
git-tracked tree: additive/never-clobber (it adds only ABSENT keys and
never overwrites an existing value; the sole exception is an authorized
`verified` stamp, which is APPENDED to any existing `verified` list, never
replacing a prior attestation), idempotent unless a `verified` stamp advances
(a rerun writes nothing when no verifier is set or the clock is pinned via
SOURCE_DATE_EPOCH; with a live verifier under a moving clock a rerun appends a
fresh stamp, since stamps dedup on (by, at)), and atomic (temp file + rename, so
an interrupted run leaves the source as it was rather than half-written).
Files needing no key are not written at all. Files whose frontmatter will not
parse, and reserved files (index.md/log.md), are skipped and never mutated.

Additive/never-clobber is the DEFAULT. --overwrite-keys <k1,k2,...> is an
opt-in exception that REFRESHES only the named keys in place even when they
already exist (e.g. --overwrite-keys status,stale_after after a new
benchmark release). Every other pre-existing key, custom frontmatter, and
key order are left in place; it respects --dry-run, the
atomic write, and skip-unchanged. Trust/attestation keys (verified,
verified_by, sources, generated, and the other provenance keys) are REFUSED
(exit 2) — overwriting them could destroy a human attestation.

Use --dry-run to preview. Skipped files are advisory: bare enrich exits 0;
--strict gates (exit 1) when any file is skipped.

```
binder enrich <src> [flags]
```

### Options

```
      --canonicalize-status      opt-in: rewrite known --status-map aliases to the OKF §5.4 vocabulary (active->stable, wip/in-progress->draft, archived/legacy->deprecated); off by default, each rewrite is reported
      --default-type string      type applied when none is present or mapped (default "Note")
      --dry-run                  report what would be enriched without writing anything
  -h, --help                     help for enrich
      --json                     emit the run report as deterministic JSON (schema binder.report/v1) instead of prose
      --overwrite-keys string    opt-in: comma-separated keys to REFRESH in place even when present, e.g. "status,stale_after" (default is additive/never-clobber; trust keys attester, computation, executor, generated, parameters, runtime, sources, usage_window, verified, verified_by are refused)
      --stale-after-map string   per-directory stale_after relative to now, e.g. "07-benchmarks=+6m,legacy=+0d" (grammar +Nd/+Nm/+Ny; set only when absent)
      --status-map string        per-directory status, e.g. "archive=deprecated,drafts=draft,default=active" (set only when status absent)
      --strict                   gate (exit 1) when any file is skipped; without it enrich never gates (never-reject)
      --type-map string          per-directory type overrides, e.g. "docs=Guide,adr=Decision"
      --verified-by string       actor to append as a verified stamp, e.g. "human:ghchinoy" or "binder/0.3.0"; a stamp is written ONLY when passed here, or when verified_by is set in your GLOBAL config (neither BINDER_VERIFIED_BY nor a repo-local .binder.yaml authorizes stamping; valid forms: human:<id>, process:<id>, team:<id>, or <producer>/<version> (e.g. binder/0.3.0))
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

