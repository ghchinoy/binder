# Trust discipline — never fabricate trust

This is the single overriding guardrail of the `okf-convert` skill. When you
drive binder you hold the same line binder holds: **propose trust, never
fabricate it; defer all stamping to the deterministic tool.**

## What binder does (and does not) stamp

- binder stamps an **honest** `generated: binder/<version>` provenance mark on
  what it produces. That is a true statement about how the file was generated.
- binder **never invents** a `verified` actor or `sources`. Trust mapping is off
  by default — it only maps signals you explicitly point it at
  (`--source-keys`, `--map-citations`), and only from real corpus content.
- **binder is safe by default (`binder/0.3.1`+): no flag and no default *you*
  set means no `verified` stamp.** A stamp is written without the flag only from
  `verified_by:` in your **global** config (`~/.config/binder/config.yaml`) — a
  machine-wide default counts as you having chosen one, and it makes `convert`
  *and* `enrich` stamp without the flag (still a claim nobody typed *for this
  corpus*, so review it). **Neither `BINDER_VERIFIED_BY` (env) nor a repo-local
  `./.binder.yaml` authorizes a stamp** — an inherited environment export is not a
  per-invocation decision to attest, and a repo-local file can ride inside someone
  else's clone. binder refuses both and discloses the refused value in
  `.result.verified.note` rather than stamping or silently dropping it. Run the
  step 1 pre-flight (`binder config --json`) and pass `--verified-by ""` to
  suppress a global default you cannot vouch for.
- **Every stamp — and every declined co-sign — is disclosed.** `--json` carries a
  `.result.verified` object (`actor`, `source`, `stamped`, `skipped`, `note`) on
  every stamp-writing verb, and the prose emits a `Trust (verified stamps):`
  block. An opt-in you cannot observe taking effect would be indistinguishable
  from auto-stamping; this is how you observe it.
- **binder declines to co-sign another identity.** When a concept already carries
  a `verified` attestation from a *different* identity, a non-explicit (global-config)
  default does **not** add a second stamp — it **skips**, reports the skip under
  `.result.verified.skipped` with the existing actor, and leaves the prior
  attestation byte-for-byte. Only an explicit `--verified-by` co-signs.

## The three invariants (from OKF v0.2 — do not violate)

1. **Never store a credibility score or trust tier.** Record objective signals
   (`generated`, `verified` events, `sources`); the *tier* (`unverified` /
   `machine-confirmed` / `human-reviewed`) is **derived on read** — `binder
   review` computes it, it is never written to frontmatter. No
   `score`/`credibility`/`tier` keys.
2. **Never reject or drop unknown keys/types.** Preserve producer-defined keys
   verbatim; unknown `type` values are legal.
3. **Broken links and missing optional fields are legal.** Never invent a target,
   a source, or a verifier to make a report look cleaner.

## The binder-driving rules (the crux)

- **Do NOT pass `--verified-by` unless a real, named actor actually attests.** The
  flag appends a `verified` stamp; a stamp is a *claim of verification*. Only pass
  it when a genuine actor (`human:<id>`, `process:<id>`, `team:<id>`, or
  `<producer>/<version>`) has actually reviewed the content. Do not auto-pass it,
  do not default it to yourself, do not stamp the agent as a verifier of content
  it merely generated.
- **Do NOT let a default you set stamp on your behalf.** Check
  `binder config --json | jq -c '.result | {config_file, verified_by: .values.verified_by}'`
  first. Only a `source` of `file` where `config_file` is a machine-wide path
  (`~/.config/binder/config.yaml`) **will** stamp — inherited state, not an
  attestation, so pass `--verified-by ""` to suppress it. A `source` of `env`, or
  of `file` where `config_file` is the corpus's own `.binder.yaml`, will **not**
  stamp (both are refused and disclosed in `.result.verified.note`). `source`
  alone cannot distinguish a global from a repo-local `file` — both report
  `file` — yet they behave oppositely; `config_file` is what tells you which file
  is in effect and whether it can stamp.
- **Suppress on `convert` as well as `enrich` when a global default is live.**
  Both resolve `--verified-by` through the same chain, so a global-config actor
  stamps the whole bundle on `convert` even if you never touched `enrich`. Check
  the result with the `tiers` line from `binder review`: `{"unverified": N}` means
  nothing was claimed, `{"human-reviewed": N}` means something was. (With nothing
  configured, the default is already `{"unverified": N}` — the flag is a defensive
  suppressor, not mandatory boilerplate.)
- **Run the pre-flight from the directory you will run everything else from.**
  binder resolves `./.binder.yaml` against the *current* directory. A repo-local
  file does not stamp, but running the check from the corpus directory still lets
  you see the same `config_file` binder will — including a repo-local one shadowing
  the global config that *would* have stamped from a parent directory.
- **Stamps are not idempotent under a wall clock.** Dedup is on `(by, at)`, so
  re-running `enrich` seconds apart appends a fresh stamp for the same actor each
  time. Pin `SOURCE_DATE_EPOCH` when you need a repeatable run.
- **Do NOT enable `--source-keys` / `--map-citations` for noise.** Map only keys
  and citation lists that carry *real* provenance. Review which corpus-native
  provenance is genuine before mapping it.
- **Do NOT add `verified`/`sources` the agent cannot assert.** If provenance is
  unknown, leave it absent — absent is legal and honest; invented is a lie.
- **Propose, then defer.** When trust *should* be added, surface it to the user
  and let them (or a real attester) decide; let deterministic binder do the
  stamping. You are the judgment, not the notary.

## The vocabulary lives in Layer A — refer to it by name

This reference deliberately does **not** re-teach the full provenance / trust /
lifecycle vocabulary. For the complete §5 (`sources`, `generated`, `verified`),
§7 (actor convention), and §10 (Attested Computation) definitions, see the
tool-agnostic **`okf-authoring`** plugin's reference:

> [`ghchinoy/agent-skills` → `plugins/okf-authoring` →
> `references/trust-vocabulary.md`](https://github.com/ghchinoy/agent-skills/blob/main/plugins/okf-authoring/references/trust-vocabulary.md)

The OKF v0.2 spec those definitions derive from is
[`GoogleCloudPlatform/knowledge-catalog` → `okf/SPEC.md`](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md).

That is a **documentation pointer, not a runtime dependency** — Agent Plugins are
self-contained and this skill resolves nothing across repos at run time. Note
`okf-authoring` ships from a *different* plugin marketplace than this one. The
short restatement above is sufficient to drive binder correctly; load the Layer A
vocabulary when you need the full field-by-field definitions.
