package okf

import (
	"reflect"
	"testing"
)

func TestHeadingSlugs(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "basic lowercasing and spaces",
			body: "# Hello World\n",
			want: []string{"hello-world"},
		},
		{
			name: "punctuation dropped, hyphens collapsed",
			body: "## What's New?! -- Really\n",
			want: []string{"whats-new-really"},
		},
		{
			name: "html tags stripped",
			body: "# <code>API</code> Guide\n",
			want: []string{"api-guide"},
		},
		{
			name: "duplicate headings get -1, -2 in document order",
			body: "# Section\n\ntext\n\n# Section\n\nmore\n\n# Section\n",
			want: []string{"section", "section-1", "section-2"},
		},
		{
			name: "all heading levels, document order",
			body: "# One\n## Two\n###### Six\n",
			want: []string{"one", "two", "six"},
		},
		{
			name: "headings inside fenced code are ignored",
			body: "# Real\n\n```\n# fake heading in code\n```\n\n## Another\n",
			want: []string{"real", "another"},
		},
		{
			name: "not a heading without a space after hashes",
			body: "#nothashheading\n\n# Real Heading\n",
			want: []string{"real-heading"},
		},
		{
			name: "no headings",
			body: "just body text, no headings here\n",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HeadingSlugs(tc.body)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("HeadingSlugs(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
