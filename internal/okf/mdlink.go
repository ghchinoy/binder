package okf

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// This is the single markdown-link code path shared by the source side (the
// converter's link rewriter) and the output side (a Codec's LinkGraph). It is
// goldmark-based rather than a bare regex so that link-like text inside fenced
// code blocks, indented code blocks, and code spans is NOT mistaken for a real
// edge (design-v2 §4/§6; the earlier factile regex extractor could not tell
// them apart). Both callers agree on what a link is because they call here.

// MarkdownLink is a raw inline link discovered in a concept/source body, before
// any bundle-relative resolution.
type MarkdownLink struct {
	Text string // link text, e.g. "orders" in [orders](tables/orders.md)
	Dest string // raw destination exactly as written, e.g. "tables/orders.md#x"
}

// Span is a half-open [Start,End) byte range within a body.
type Span struct {
	Start int
	End   int
}

var mdParser = goldmark.DefaultParser()

// ExtractMarkdownLinks returns every real inline markdown link (and image) in
// body, in source order, skipping any that fall inside code. Autolinks and
// reference-style links are included; footnote references (which have no
// destination) are not.
func ExtractMarkdownLinks(body string) []MarkdownLink {
	src := []byte(body)
	doc := mdParser.Parse(text.NewReader(src))
	var out []MarkdownLink
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			out = append(out, MarkdownLink{Text: nodeText(v, src), Dest: string(v.Destination)})
		case *ast.Image:
			out = append(out, MarkdownLink{Text: nodeText(v, src), Dest: string(v.Destination)})
		case *ast.AutoLink:
			out = append(out, MarkdownLink{Dest: string(v.URL(src))})
		}
		return ast.WalkContinue, nil
	})
	return out
}

// CodeRegions returns the byte ranges of body that goldmark considers code
// (fenced/indented code blocks and inline code spans), so a byte-level rewriter
// can leave link-like text inside them untouched.
func CodeRegions(body string) []Span {
	src := []byte(body)
	doc := mdParser.Parse(text.NewReader(src))
	var spans []Span
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			if s, ok := linesSpan(n); ok {
				spans = append(spans, s)
			}
		case *ast.CodeSpan:
			if s, ok := textSpan(n); ok {
				spans = append(spans, s)
			}
		}
		return ast.WalkContinue, nil
	})
	return spans
}

// InCodeRegion reports whether offset falls within any of the given spans.
func InCodeRegion(offset int, spans []Span) bool {
	for _, s := range spans {
		if offset >= s.Start && offset < s.End {
			return true
		}
	}
	return false
}

// MaskCode returns body with every byte that goldmark classifies as code —
// fenced code blocks, indented code blocks, and inline code spans — replaced by a
// single space, leaving newlines and every byte offset intact. A line-anchored
// scanner run over the result therefore reads only prose: a checkbox- or
// heading-shaped line that is really quoted code is blanked, exactly as GitHub
// renders it as literal text.
//
// This is the markdown-aware alternative to a line scanner that enumerates which
// constructs count as code (and inevitably misses one). It is shared so any
// caller that must ignore code regions can reuse one CommonMark parse instead of
// re-deriving the rule. Today the docs-impact gate is its only adopter.
//
// It is AVAILABLE FOR ADOPTION by the heading-slug (issue #96) and inline-span
// (issue #99) paths, which have the identical indented-/inline-code blind spot.
// Neither has adopted it yet: naming them here marks where the helper could go,
// not a record that either bug is fixed. As of this writing #99's linkcheck path
// still consumes CodeRegions directly rather than MaskCode. An intention in a
// comment is not a call site.
func MaskCode(body string) string {
	spans := CodeRegions(body)
	if len(spans) == 0 {
		return body
	}
	b := []byte(body)
	for _, s := range spans {
		for i := s.Start; i < s.End && i < len(b); i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}
	return string(b)
}

func nodeText(n ast.Node, src []byte) string {
	if n.Type() == ast.TypeInline {
		return string(n.Text(src)) //nolint:staticcheck // Text is sufficient for a label
	}
	return ""
}

// linesSpan returns the byte range spanning a block node's raw source lines.
func linesSpan(n ast.Node) (Span, bool) {
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return Span{}, false
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)
	return Span{Start: first.Start, End: last.Stop}, true
}

// textSpan returns the byte range spanning an inline node's text segments.
func textSpan(n ast.Node) (Span, bool) {
	start, end, found := -1, -1, false
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			seg := t.Segment
			if !found || seg.Start < start {
				start = seg.Start
			}
			if !found || seg.Stop > end {
				end = seg.Stop
			}
			found = true
		}
	}
	if !found {
		return Span{}, false
	}
	return Span{Start: start, End: end}, true
}
