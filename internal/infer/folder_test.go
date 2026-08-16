package infer

import "testing"

func TestInferFromFolder(t *testing.T) {
	cases := []struct {
		dir      string
		wantType string
	}{
		{"subsystems", "Subsystem"},
		{"subsystems/audio", "Subsystem"},
		{"runbooks", "Runbook"},
		{"proposals", "Proposal"},
		{"developers", "Guide"},
		{"docs", "Guide"},
		{"decisions", "Decision"},
		{"adr", "Decision"},
		{"rfcs", "Proposal"},
		{"specs", "Specification"},
		{"metrics", "Metric"},
		{"tables", "Table"},
		{"policies", "Policy"},
		{"benchmarks", "Benchmark"},
		{"capabilities", "Capability"},
		{"custom-tools", "Custom Tool"},
		{"data_sources", "Data Source"},
		{"", ""},
		{".", ""},
	}

	for _, tc := range cases {
		got, _ := InferFromFolder(tc.dir)
		if got != tc.wantType {
			t.Errorf("InferFromFolder(%q) = %q, want %q", tc.dir, got, tc.wantType)
		}
	}
}

func TestSingularize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"subsystems", "subsystem"},
		{"policies", "policy"},
		{"statuses", "status"},
		{"tools", "tool"},
		{"bus", "bus"},
		{"boss", "boss"},
	}
	for _, tc := range cases {
		if got := singularize(tc.in); got != tc.want {
			t.Errorf("singularize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
