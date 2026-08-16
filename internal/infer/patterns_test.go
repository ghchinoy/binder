package infer

import "testing"

func TestInferFromPatterns(t *testing.T) {
	cases := []struct {
		name     string
		files    []string
		wantType string
	}{
		{
			name:     "specs pattern",
			files:    []string{"auth-spec.md", "audio-spec.md", "other.md"},
			wantType: "Specification",
		},
		{
			name:     "runbooks pattern",
			files:    []string{"troubleshooting.md", "deploy-runbook.md"},
			wantType: "Runbook",
		},
		{
			name:     "adrs pattern",
			files:    []string{"adr-001.md", "adr-002.md", "notes.md"},
			wantType: "Decision",
		},
		{
			name:     "rfcs pattern",
			files:    []string{"rfc-01.md", "rfc-02.md"},
			wantType: "Proposal",
		},
		{
			name:     "no pattern majority",
			files:    []string{"a.md", "b.md", "c.md"},
			wantType: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := InferFromPatterns(tc.files)
			if got != tc.wantType {
				t.Errorf("InferFromPatterns() = %q, want %q", got, tc.wantType)
			}
		})
	}
}
