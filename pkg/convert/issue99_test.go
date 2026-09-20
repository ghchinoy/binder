package convert

import "testing"

// TestIssue99CodeBracketDoesNotSwallowLater locks issue #99: an unmatched "["
// inside a code region (inline span OR fenced block) must not make the link
// extractor lose a real link that follows it. On main mdLinkRE's match starts at
// the stray bracket, its [^\]]* class runs across newlines to the ] of the next
// real link, and the whole match is discarded by the start-offset InCodeRegion
// test — so the real link is never seen (FindAll never revisits those bytes).
func TestIssue99CodeBracketDoesNotSwallowLater(t *testing.T) {
	srcToOut := map[string]string{
		"intro.md":         "intro.md",
		"tables/orders.md": "tables/orders.md",
	}

	cases := []struct {
		name string
		body string
		want string // expected rewritten body
	}{
		{
			name: "unmatched bracket in inline code span",
			body: "Regex `[` here.\n\nsee [orders](tables/orders.md)\n",
			want: "Regex `[` here.\n\nsee [orders](/tables/orders.md)\n",
		},
		{
			name: "unmatched bracket in fenced code block",
			body: "```\nregex [ here\n```\n\nsee [orders](tables/orders.md)\n",
			want: "```\nregex [ here\n```\n\nsee [orders](/tables/orders.md)\n",
		},
		{
			// The review trap: this link's TEXT contains an inline code span
			// with brackets. A fix that sliced text from the masked copy would
			// blank the `[x]` span; slicing from the original body keeps it.
			name: "code span with brackets inside link text",
			body: "see [the `[x]` form](tables/orders.md)\n",
			want: "see [the `[x]` form](/tables/orders.md)\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, links := rewriteLinks(tc.body, "intro.md", srcToOut)
			if len(links) != 1 {
				t.Fatalf("link after a code-region bracket was dropped: got %d links %+v", len(links), links)
			}
			if links[0].RawTarget != "tables/orders.md" || !links[0].Resolved {
				t.Fatalf("unexpected link: %+v", links[0])
			}
			if out != tc.want {
				t.Fatalf("unexpected rewrite:\n got: %q\nwant: %q", out, tc.want)
			}
		})
	}
}

// TestIssue99LinkTextCodeSpanNotBlanked isolates the review trap as its own
// assertion so a regression that blanks link-text code spans fails here loudly,
// independent of the swallow case above. Its control is the sibling assertion:
// the same body with the code span REMOVED from the text must still round-trip,
// so a change that mangled all link text (not just masked bytes) would break the
// control too, proving this assertion is not vacuous.
func TestIssue99LinkTextCodeSpanNotBlanked(t *testing.T) {
	srcToOut := map[string]string{
		"intro.md":         "intro.md",
		"tables/orders.md": "tables/orders.md",
	}

	// Trap case: code span with brackets inside the link text must survive verbatim.
	body := "see [the `[x]` form](tables/orders.md)\n"
	want := "see [the `[x]` form](/tables/orders.md)\n"
	out, links := rewriteLinks(body, "intro.md", srcToOut)
	if len(links) != 1 {
		t.Fatalf("link dropped: got %d links %+v", len(links), links)
	}
	if out != want {
		t.Fatalf("link-text code span was corrupted:\n got: %q\nwant: %q", out, want)
	}
	if links[0].Text != "the `[x]` form" {
		t.Fatalf("extracted link Text lost its code span: got %q want %q", links[0].Text, "the `[x]` form")
	}

	// Control: identical link with a plain-prose text (no code span). If the
	// rewrite corrupted link text in general rather than only masked bytes, this
	// would fail too — so it proves the assertion above is about the code span.
	cbody := "see [the plain form](tables/orders.md)\n"
	cwant := "see [the plain form](/tables/orders.md)\n"
	cout, clinks := rewriteLinks(cbody, "intro.md", srcToOut)
	if len(clinks) != 1 || cout != cwant || clinks[0].Text != "the plain form" {
		t.Fatalf("control (plain link text) did not round-trip: out=%q links=%+v", cout, clinks)
	}
}

// TestIssue99CodeStaysInert is the negative control: a complete, link-shaped
// string inside a code span / fence is still NOT an edge and the body is
// untouched. A "fix" that simply stopped honouring code regions would pass the
// swallow test above and FAIL this one. It passes on both main and the fix.
func TestIssue99CodeStaysInert(t *testing.T) {
	srcToOut := map[string]string{"intro.md": "intro.md", "tables/orders.md": "tables/orders.md"}
	body := "Inline `[fake](tables/orders.md)` and a fence:\n\n```\n[alsofake](tables/orders.md)\n```\n"
	out, links := rewriteLinks(body, "intro.md", srcToOut)
	if len(links) != 0 {
		t.Fatalf("link-shaped text in code must not be an edge: %+v", links)
	}
	if out != body {
		t.Fatalf("code must be left untouched:\n got: %q\nwant: %q", out, body)
	}
}
