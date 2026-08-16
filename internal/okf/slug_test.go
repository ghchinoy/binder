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
			name: "punctuation dropped, hyphen runs preserved (GitHub parity)",
			body: "## What's New?! -- Really\n",
			want: []string{"whats-new----really"},
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
		{
			// #76: dropped "/" leaves two adjacent spaces -> "--"; GitHub keeps
			// both hyphens. binder previously collapsed them to one.
			name: "slash yields adjacent double hyphen (GitHub parity)",
			body: "## Agent Skill / Plugin\n",
			want: []string{"agent-skill--plugin"},
		},
		{
			// #76: underscore is a word character on GitHub and must round-trip.
			name: "underscore is kept",
			body: "## The id_key field\n",
			want: []string{"the-id_key-field"},
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

// TestSlugify pins the per-heading slug rules directly, with emphasis on the two
// #76 divergences from github-slugger (hyphen-run preservation and kept
// underscores) plus mixed and edge-position punctuation.
func TestSlugify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase and spaces", "Hello World", "hello-world"},
		{"underscore kept", "The id_key field", "the-id_key-field"},
		{"leading underscore kept", "_private", "_private"},
		{"trailing underscore kept", "field_", "field_"},
		{"double underscore kept", "a__b", "a__b"},
		// Hyphen runs are NOT collapsed (github-slugger parity).
		{"slash drops to adjacent double hyphen", "Agent Skill / Plugin", "agent-skill--plugin"},
		{"literal double hyphen preserved", "JSON output --json", "json-output---json"},
		{"long hyphen run preserved", "a -- b --- c", "a----b-----c"},
		// Mixed punctuation is dropped; word chars, spaces, hyphens survive.
		{"apostrophe and question dropped", "What's New?", "whats-new"},
		{"parens dropped", "JSON output (--json)", "json-output---json"},
		{"digits kept", "Section 42_beta", "section-42_beta"},
		// Leading/trailing punctuation: dropped punctuation is simply removed;
		// spaces still map to hyphens (github-slugger does not trim them).
		{"leading punctuation dropped", "!!! Bang", "-bang"},
		{"trailing punctuation dropped", "Bang !!!", "bang-"},
		{"surrounding spaces map to hyphens", " padded ", "-padded-"},
		{"html tag stripped", "<code>API</code> Guide", "api-guide"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := slugify(tc.in); got != tc.want {
				t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
