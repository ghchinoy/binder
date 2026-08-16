package infer

import (
	"testing"

	"github.com/ghchinoy/binder/internal/okf"
)

func TestInferFromFrontmatter(t *testing.T) {
	fmProposal := okf.NewOrderedMap()
	fmProposal.Set("goal", "migrate to new auth")

	fmAttested := okf.NewOrderedMap()
	fmAttested.Set("runtime", "python:3.12")

	cases := []struct {
		name     string
		files    []FileInfo
		wantType string
	}{
		{
			name: "authored types majority",
			files: []FileInfo{
				{RelPath: "a.md", AuthoredType: "Decision"},
				{RelPath: "b.md", AuthoredType: "Decision"},
				{RelPath: "c.md", AuthoredType: "Note"},
			},
			wantType: "Decision",
		},
		{
			name: "goal key hints proposal",
			files: []FileInfo{
				{RelPath: "a.md", Frontmatter: fmProposal},
				{RelPath: "b.md", Frontmatter: fmProposal},
			},
			wantType: "Proposal",
		},
		{
			name: "runtime key hints attested computation",
			files: []FileInfo{
				{RelPath: "calc.md", Frontmatter: fmAttested},
			},
			wantType: "Attested Computation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := InferFromFrontmatter(tc.files)
			if got != tc.wantType {
				t.Errorf("InferFromFrontmatter() = %q, want %q", got, tc.wantType)
			}
		})
	}
}
