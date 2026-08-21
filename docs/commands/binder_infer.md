## binder infer

Inspect a source markdown corpus and propose a --type-map

### Synopsis

Infer inspects a source markdown corpus and proposes a directory-to-type
mapping string (e.g. "docs=Guide,subsystems=Subsystem") and structured report.

It evaluates a tiered signal ladder: deterministic offline signals by default
(folder structure, filename patterns, frontmatter hints), plus an optional
opt-in Gemini semantic tier (--gemini) supporting API keys and Google Cloud
Vertex AI with Application Default Credentials.

Infer is proposal-only: it never writes to disk. Review the proposal, then
pass it to `binder convert --type-map` or `binder enrich --type-map`.

```
binder infer <corpus> [flags]
```

### Options

```
      --backend string        Gemini auth backend: auto, api, or vertex (default "auto")
      --default-type string   fallback concept type (default "Note")
      --gemini                enable Gemini semantic inference tier (requires API key or Google Cloud ADC)
      --gemini-model string   Gemini model for semantic inference (default "gemini-3.5-flash-lite")
      --gemini-required       fail on Gemini inference error instead of degrading to deterministic tiers
  -h, --help                  help for infer
      --json                  emit the inference report as deterministic JSON (schema binder.report/v1)
      --location string       Google Cloud location for Vertex AI (default "global")
      --project string        Google Cloud project for Vertex AI (defaults to ADC / GOOGLE_CLOUD_PROJECT)
      --strict                gate (exit 1) if any warning or failure occurs
```

### SEE ALSO

* [binder](binder.md)	 - Convert a plain-markdown corpus into a conformant OKF v0.2 bundle

