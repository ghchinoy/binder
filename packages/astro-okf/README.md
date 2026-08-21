# astro-okf

An [Astro](https://astro.build) Content Layer loader for
[Open Knowledge Format](https://github.com/GoogleCloudPlatform/knowledge-catalog)
(OKF) v0.2 bundles.

Point it at a bundle directory and every concept becomes an entry in an Astro
content collection, with the OKF frontmatter preserved exactly as written and
the trust signals the spec says to *derive* attached separately, in a namespaced
`_okf` block, so a template can never present a computed tier as though the
author had stored it.

> **Status: Phase 1.** This is the first vertical slice — bundle in, concept
> entries out, with derived trust tier and staleness. It is a working, published
> package, not a stub, but it is not yet the whole design. See
> [What is not here yet](#what-is-not-here-yet).

## Install

```sh
npm install astro-okf
```

`astro` is a peer dependency (`^7.0.0`). Node `>=22.19.0`.

## Use

```ts
// src/content.config.ts
import { defineCollection } from "astro:content";
import { okfLoader, okfSchema } from "astro-okf";

export const collections = {
  kb: defineCollection({
    loader: okfLoader({ bundle: "../my-okf-bundle" }),
    schema: okfSchema(),
  }),
};
```

```astro
---
// src/pages/kb/[...slug].astro
import { getCollection, render } from "astro:content";

export async function getStaticPaths() {
  const entries = await getCollection("kb");
  return entries.map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
}

const { entry } = Astro.props;
const { Content } = await render(entry);
const okf = entry.data._okf;
---
<p data-okf-tier={okf.tier} data-okf-tier-derived="true">
  Trust tier: {okf.tier} — derived from this concept's verified entries,
  not stored in the bundle.
</p>
<Content />
```

A complete, runnable version of this is in [`example/`](./example), which builds
binder's own `testdata/expected-rich` bundle and is what the package's
acceptance tests assert against.

### Options

| Option | Type | Meaning |
| --- | --- | --- |
| `bundle` | `string` (required) | Path to the OKF bundle root. Relative paths resolve against the Astro project root (`config.root`). |
| `now` | `() => Date` | Clock used for the staleness comparison. Inject it to make builds and tests deterministic; defaults to the real current time. |

## What the loader derives, and what it leaves alone

The raw frontmatter families (`type`, `title`, `generated`, `verified`,
`sources`, `status`, `stale_after`, `usage_window`, `tags`, and any key the
producer invented) reach `entry.data` carrying exactly what the bundle said.
Unknown keys are preserved, because OKF §4.1 says a consumer must not reject a
document over them.

Everything the loader *computed* lives under `entry.data._okf`:

| Field | Rule |
| --- | --- |
| `_okf.kind` | `"concept"`. Phase 1 emits concepts only. |
| `_okf.tier` | `unverified` / `machine-confirmed` / `human-reviewed`, per OKF §5.3. No `verified` entries means `unverified`; any `verified[].by` with a `human:` prefix means `human-reviewed`; otherwise `machine-confirmed`. |
| `_okf.tierBasis` | The exact `verified[]` stamps that justify the tier — the evidence, so a reader can check the derivation rather than trust it. Empty for `unverified`. |
| `_okf.stale` | `evaluatedOn >= stale_after`, per OKF §5.5. No `stale_after` means never stale. |
| `_okf.staleAfter` | The `stale_after` the decision was made against, if any. |
| `_okf.evaluatedOn` | The `YYYY-MM-DD` UTC day staleness was evaluated for. |

The split is the point. `verified[]` is what the author wrote; `_okf.tier` is
what this package concluded from it. Render them from their own fields and
label the second one derived, and a page cannot quietly upgrade a claim.

The tier and staleness rules are a direct port of binder's
`internal/okf/trust.go`, so a bundle renders the tier its producer computes for
it.

### Normalization

OKF §5.2 says a bare `{ by, at }` mapping must be read as a one-element list.
The loader normalizes `verified` before validation, so consumers see one shape
whichever spelling the bundle used.

## What is not here yet

Absent rather than stubbed, so nothing reports a result it did not compute:

- **Reserved files.** `index.md` and `log.md` are recognised and **skipped**;
  they are not emitted as entries and are never validated as concepts
  (OKF §11). Landing pages, per-directory listings and the log timeline are
  Phase 2.
- **The link graph.** No `outlinks` / `backlinks` / `brokenLinks` — Phase 3.
  Note that the `_okf` block omits these fields entirely rather than defaulting
  them to `[]`, because an empty `brokenLinks` would read as "checked, none
  found", which is not true yet.
- **Footnote joins and advisories.** OKF §5.1 footnote-to-`sources[].id`
  binding, and the well-formedness advisories binder's `ValidateTrust` emits —
  Phase 2.
- **The Starlight preset** (`astro-okf/starlight`) — Phase 4.
- **The no-fabrication test kit** (`astro-okf/testkit`) — Phase 5.

## Development

From the repository root (the package is a member of binder's npm workspace):

```sh
npm ci
npm run build --workspace astro-okf        # tsc -> dist/
npm test --workspace astro-okf             # unit + loader tests
npm run build:example --workspace astro-okf
npm run test:example --workspace astro-okf # acceptance, against built HTML
```

The tests read binder's in-tree fixtures under `testdata/` directly. Nothing is
vendored, so a change to what binder emits is a change to what these tests
assert against.

## Publishing

The package is released on its own tag namespace, `astro-okf-v*`, which cannot
match the `v*` glob that drives binder's Go release — the two release lines are
provably disjoint.

`.github/workflows/publish-astro-okf.yml` publishes on that tag using **npm
trusted publishing (OIDC)**: no `NPM_TOKEN` secret exists or is needed, and
`--provenance` attaches a signed attestation tying the tarball to this
repository and commit.

That workflow cannot work until the package exists on npm, because npm attaches
a trusted publisher to an existing package. The one-time bootstrap, for the repo
owner:

1. Confirm the name is available: `npm view astro-okf`. (A `404` means free.)
   If it is taken, choose a scoped name you control and update `name` in
   `packages/astro-okf/package.json` and the workspace references to it.
2. From a clean checkout of `main`:
   ```sh
   npm ci
   npm run build --workspace astro-okf
   npm publish --workspace astro-okf --access public
   ```
   (Equivalently `cd packages/astro-okf && npm publish --access public`.)
3. On npmjs.com, open the package's **Settings → Trusted publisher**, add a
   **GitHub Actions** publisher with:
   - Organization/user: `ghchinoy`
   - Repository: `binder`
   - Workflow filename: `publish-astro-okf.yml`
4. Every release after that is just a tag:
   ```sh
   # bump "version" in packages/astro-okf/package.json first
   git tag astro-okf-v0.1.1 && git push origin astro-okf-v0.1.1
   ```
   Never tag `v0.1.1` — that namespace belongs to binder's Go release.

## License

Apache-2.0, the same as binder.
