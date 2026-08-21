# Primer diagram assets

DOT sources + rendered WebP for `../lpg-inmemory-primer.md`.
**Commit format: DOT + WebP only** (owner decision — docs/ placement).

| Source | Output | Used in |
|---|---|---|
| `lpg-model.dot` | `lpg-model.webp` | §1 — the LPG example graph (labels + properties on nodes and typed edges) |
| `lpg-vs-rdf.dot` | `lpg-vs-rdf.webp` | §1 — the thesis: one LPG edge vs RDF reification for "Alice worked at Acme since 2021" |
| `bfs-frontier.dot` | `bfs-frontier.webp` | §3 — bounded BFS frontier + highlighted shortest path + visited-set revisit skip |

## Rendering (DOT → high-DPI PNG → lossless WebP)

Graphviz has no native WebP target, so render to PNG first, then convert:

```sh
for f in lpg-model lpg-vs-rdf bfs-frontier; do
  dot -Tpng -Gdpi=192 "$f.dot" -o "/tmp/$f.png"
  cwebp -lossless -q 100 -z 9 "/tmp/$f.png" -o "$f.webp"
done
```

Rendered with Graphviz 2.43 + cwebp 1.2.4. Lossless WebP is used because these are
line diagrams with text (not photos); `-Gdpi=192` keeps the type crisp.

## Font

The diagrams reference **Google Sans Flex** (from fonts.google.com). It is a
proprietary Google font and is **intentionally not committed here**. Install it
locally before rendering (drop the TTF from the Google Fonts API into `~/.fonts`
and run `fc-cache -f`). `fc-match "Google Sans Flex"` should resolve before you
render, otherwise Graphviz falls back to a default face.

## Theme

Colors are the **light** scheme from the owner's Material theme
(`material-theme.json`): surface `#F9FAEF`, primary/primaryContainer
`#4C662B`/`#CDEDA3` (`:Company`), tertiary/tertiaryContainer `#386663`/`#BCECE7`
(`:Person`), secondary `#586249` (edge properties), and error/errorContainer
`#BA1A1A`/`#FFDAD6` (the reified RDF blank node / BFS revisit, flagged as the
awkward path).
