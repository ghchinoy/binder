# Byte-faithful round-trip

binder converts a plain-markdown corpus into a conformant OKF v0.2 bundle and
preserves trust frontmatter byte-for-byte on round-trip. This page collects the
two invariants that make that guarantee precise — and states the bounds they
carry, so you can rely on them for provenance.

> The guarantees below bound what binder does — but they are not unconditional.
> Each states its own scope, so read each with its stated bound rather than as a
> blanket promise across every command.

## Byte-faithful round-trip, where binder recognises the frontmatter

Unmodified YAML frontmatter is passed through verbatim — including nested-map and
list key order and scalar quoting style — so a round-trip changes nothing it did
not have to change. This is scoped to files whose frontmatter binder recognises
**and that need no read-boundary normalization**: the fence must open with `---`
and then a newline, LF or CRLF, at the very start.

A leading UTF-8 BOM or a lone-CR (classic-Mac) fence is now recognised too, but
is normalized before recognition
([#124](https://github.com/ghchinoy/binder/issues/124)) — the fence and any
`verified:` block it guards are preserved, though the round-trip is deliberately
not byte-faithful and is disclosed (a `normalized` signal plus a top-level
advisory). A file with **no** frontmatter fence at all is still read as plain
and synthesized over, leaving its content in the body as text, with exit `0`,
nothing skipped, and no warning. Recognition still leaves byte-level bounds;
for the ones known today, see *Residual bounds* under
[`enrich`](../user_guide.md#enrich).

## Deterministic output

Given identical input and the same clock, `convert` produces byte-identical
output; `review` and `graph` sort their output. `convert` honours
`SOURCE_DATE_EPOCH` for any synthesised timestamp.

## Never fabricate trust, where binder recognises the frontmatter

On a fence binder recognises, binder never invents a source, a credibility
score, or provenance. Trust mapping is opt-in and additive; with no mapping
flags, frontmatter that binder recognised and that needed no read-boundary
normalization round-trips byte-for-byte — bounded exactly as the round-trip
guarantee above.

A leading UTF-8 BOM or a lone-CR fence is now recognised via read-boundary
normalization ([#124](https://github.com/ghchinoy/binder/issues/124)), so its
`verified:` attestation is preserved rather than demoted — but because that
normalization is not byte-faithful it is disclosed (a `normalized` signal plus an
advisory), not a silent round-trip. On a file with **no** frontmatter fence at
all this does not hold: the file is treated as plain, so its content is left in
the body as text while binder synthesizes keys of its own, among them a `type`,
a `title`, and a `generated` provenance stamp, with exit `0`, nothing skipped,
and no warning. `generated` is a key binder itself protects:
`--overwrite-keys generated` is refused as a trust-provenance key.

Trust tiers and staleness are *derived* on demand from frontmatter, never stored
(see [Trust model & tiers](trust.md)). A `verified` attestation in particular is
written only for a verifier **you** supplied on this invocation or set as your
own default; it is never written **over** another identity's attestation from a
default, and every stamp-writing run discloses what it wrote.
