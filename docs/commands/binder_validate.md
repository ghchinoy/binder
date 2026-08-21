## binder validate

Check a bundle for OKF v0.2 conformance (spec §11)

### Synopsis

Validate checks the hard conformance rules: every non-reserved .md has a
parseable frontmatter block with a non-empty type. It reports trust
well-formedness as advisories and NEVER rejects a bundle for missing
optional fields, unknown keys, unknown type values, broken links, or
absent trust families.

```
binder validate <bundle> [flags]
```

### Options

```
  -h, --help     help for validate
      --json     emit the validation result as deterministic JSON (schema binder.report/v1) instead of prose
      --strict   gate (exit 1) on trust well-formedness advisories, not just hard non-conformance
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

