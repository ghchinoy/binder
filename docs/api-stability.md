# binder API stability

binder versions **three independent things**. Conflating them is the mistake this
document exists to prevent — a change that is breaking on one axis is often
irrelevant to the other two, and each axis reaches `1.0`/stability on its own
schedule.

| # | Axis | What it covers | Where it is stated | Status |
|---|------|----------------|--------------------|--------|
| a | **Converter output / trust-stamp format** | The bytes binder writes: the OKF v0.2 bundle it emits and the `generated.by` / `verified` trust stamps it stamps. | `docs/RELEASING.md` → "Versioning posture" (the reserved `v1.0.0`). | reserved for the first freeze |
| b | **OKF spec level** | Which OKF specification binder targets. | README / CHANGELOG. | advertised there, never in binder's SemVer |
| c | **Go SDK** | The exported Go surface a program gets by `import`-ing binder's packages. | **this document.** | **provisional (see below)** |

The three do not move together. In particular the Go-SDK axis (c) **must not
piggyback on** the reserved output-format `v1.0.0` (a): a program importing binder
and a corpus converted by binder are different consumers with different
compatibility needs, and either freeze may land first.

## The Go SDK is provisional while binder is `0.x`

binder's Go packages are **importable, but not yet covered by compatibility
guarantees.** While the module is `0.x` the exported surface **may change in any
minor release** — types may be renamed, moved, re-typed, or removed. Pin an exact
version if you depend on it.

This is the honest state today: the surface is still being curated (packages are
migrating out of `internal/`), so committing to it now would freeze it before it
has settled. Declaring it provisional is the reversible choice — we can always
commit to *more* stability later; we cannot walk a premature guarantee back.

### What "Go-SDK stable" will mean, later

A future release will make a distinct, explicit commitment: **semantic versioning
on the published Go surface** — the surface that lives under `pkg/`. That
commitment will be announced separately from the output-format `v1.0.0` (axis a)
and may arrive at a different time. Until it is announced, treat the Go surface as
provisional regardless of the module's version number.

## The first committed slice: six locked report structs

One part of the Go surface is **already** a de-facto public commitment, predating
any stability decision: the JSON report shapes emitted by binder's read-side
commands. The `internal/plugindocs` drift gate has been pinning their key sets
against the live binary — a CI-enforced promise that these shapes do not change by
accident. Rather than let that remain an accident, we **name these six report
structs as the first committed slice of the Go-SDK contract**:

| Command | Go report struct | Package |
|---------|------------------|---------|
| `convert` | `Report` | `internal/convert` |
| `enrich`  | `Report` | `internal/enrich`  |
| `graph`   | `Model`  | `internal/graph`   |
| `infer`   | `Report` | `internal/infer`   |
| `lint`    | `Report` | `internal/lint`    |
| `review`  | `Report` | `internal/review`  |

(The paths above are the pre-publication `internal/` homes; they move to `pkg/`
when the surface is published, with no shape change.)

"First committed slice" means: these are the shapes we are most confident in and
that the drift gate already de-risks — **not** that the rest of the Go surface is
unstable-by-contrast. The whole Go surface is provisional while `0.x` (previous
section); these six simply carry the additional, already-enforced promise that
their **serialized key sets** do not drift silently. A change to any field on one
of them must be a deliberate act that recaptures the pinned transcript, never a
silent edit.

Being already locked is exactly why exporting these is the **lowest-risk** part of
the surface to publish: the gate that protects them is written and green before the
move.

### How the promise is enforced (CI)

Two complementary gates, both in `internal/plugindocs`, both run by
`go test ./...`:

- **Report-shape key sets** — `drift_*_test.go` runs the live CLI over each
  plugin's sample corpus and asserts key-set equality against every documented
  JSON block, and reflects over each array-element struct to prove it keeps a
  mandatory field. This locks the **serialized shape** of the six reports above
  (plus `validate` and the shared element structs). The authoritative field-level
  enumeration is `packaging/phase2/plugindocs-locked-fields.md`.
- **Go surface** — `pubsurface_drift_test.go` locks the **exported Go surface**
  intended for publication in Phase 6: the whole `internal/binder` service surface
  (every `Request`/`Result`, `Service` method, helper) plus the confirmed-MUST
  `okf` vocabulary, compared to a committed golden. It guards additions,
  removals, renames, re-typings, and tag changes across that surface. This gate
  references `internal/` paths today and is repointed to `pkg/` when the surface
  is published, with no logic change.

Neither gate publishes anything or widens the committed API; they widen only what
is *guarded*, so the eventual publication is a move of already-protected code.
