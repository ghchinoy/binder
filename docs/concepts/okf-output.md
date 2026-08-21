# OKF v0.2 output structure

A converted bundle is an ordinary directory tree:

```text
bundle/
  index.md              # root index: declares okf_version, lists concepts + subdirs
  overview.md           # a concept
  metrics/
    index.md            # per-directory nav (no frontmatter)
    revenue.md          # a concept
```

Key rules binder enforces on emit:

- **One concept per non-reserved `.md`.** A concept's ID is its bundle-relative
  path minus `.md`.
- **`okf_version` lives only in the root `index.md`** (spec §12), and the root
  index is the only index carrying frontmatter (spec §8).

  ```yaml
  ---
  okf_version: "0.2"
  ---
  ```

- **Every concept has a non-empty `type`** — the one hard-required field
  (spec §11). All other frontmatter keys are optional and preserved as-is,
  including keys binder does not understand (spec §4).
- **A `generated` stamp** records the conversion, added only when the concept
  does not already carry one:

  ```yaml
  generated:
    at: "2023-11-14T22:13:20Z"
    by: binder/0.3.0
  ```

- **Reserved-name source files are never dropped.** A source `index.md`/`log.md`
  is renamed to `<stem>-note.md` (with a numeric suffix on collision) so binder
  can generate its own `index.md`, and the rename is reported.
- **Links are rewritten to bundle-relative-absolute form.** A body link
  `[t](../a/b.md#sec)` that resolves to a concept becomes `[t](/a/b.md#sec)`.

For the full reference — including reserved files, the concept/edge vocabulary,
and validation rules — see the [user guide](../user_guide.md#okf-v02-output-structure).
