package okf

import (
	"strings"
	"testing"
)

// TestMaskCode locks the behaviour the docs-impact gate (and the #96/#99 paths
// that can adopt it) rely on: every construct goldmark classifies as code —
// fenced blocks, indented blocks, and inline spans — is blanked, while prose,
// newlines, and byte offsets are left intact.
func TestMaskCode(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		mustBlank   []string // substrings that must NOT survive in the masked output
		mustSurvive []string // prose substrings that MUST survive
	}{
		{
			name:        "fenced block",
			body:        "prose\n```\n- [x] hidden\n```\ntail",
			mustBlank:   []string{"- [x] hidden"},
			mustSurvive: []string{"prose", "tail"},
		},
		{
			name:        "tilde fenced block",
			body:        "prose\n~~~\n## hidden heading\n~~~\ntail",
			mustBlank:   []string{"## hidden heading"},
			mustSurvive: []string{"prose", "tail"},
		},
		{
			name:        "indented code block",
			body:        "prose\n\n    - [x] hidden\n\ntail",
			mustBlank:   []string{"- [x] hidden"},
			mustSurvive: []string{"prose", "tail"},
		},
		{
			name:        "inline code span",
			body:        "a `- [x] hidden` b",
			mustBlank:   []string{"- [x] hidden"},
			mustSurvive: []string{"a ", " b"},
		},
		{
			name:        "no code passes through unchanged",
			body:        "just prose\nand a - [x] real box",
			mustSurvive: []string{"- [x] real box"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskCode(tc.body)

			// Offset/line preservation: same length, same newline positions.
			if len(got) != len(tc.body) {
				t.Fatalf("MaskCode changed length: got %d want %d", len(got), len(tc.body))
			}
			if strings.Count(got, "\n") != strings.Count(tc.body, "\n") {
				t.Fatalf("MaskCode changed newline count")
			}

			for _, s := range tc.mustBlank {
				if strings.Contains(got, s) {
					t.Fatalf("code fragment survived masking: %q\n--- masked ---\n%s", s, got)
				}
			}
			for _, s := range tc.mustSurvive {
				if !strings.Contains(got, s) {
					t.Fatalf("prose was masked but must survive: %q\n--- masked ---\n%s", s, got)
				}
			}
		})
	}
}
