package cmd

import (
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// TestExitCodeContract exercises the stable exit-code contract (#13 §5) across
// the commands, observing the code the same way main.go does.
func TestExitCodeContract(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"success-convert", []string{"convert", "../testdata/corpus-basic", "--dry-run"}, clijson.ExitSuccess},
		{"success-convert-json", []string{"convert", "../testdata/corpus-basic", "--dry-run", "--json"}, clijson.ExitSuccess},
		{"usage-unknown-flag", []string{"convert", "../testdata/corpus-basic", "--dry-run", "--nope"}, clijson.ExitUsage},
		{"usage-missing-arg", []string{"convert"}, clijson.ExitUsage},
		{"usage-missing-output", []string{"convert", "../testdata/corpus-basic"}, clijson.ExitUsage},
		{"io-missing-path", []string{"convert", "../testdata/does-not-exist", "--dry-run"}, clijson.ExitIO},

		// Usage errors that previously escaped unwrapped and mapped to ExitIO
		// (exit 3). Each must now be classified as a usage error (exit 2).
		{"usage-unknown-subcommand", []string{"bogus"}, clijson.ExitUsage},
		{"usage-unknown-subcommand-args", []string{"bogus", "arg"}, clijson.ExitUsage},
		{"usage-graph-bad-format", []string{"graph", "../testdata/expected-rich", "--format", "bogus"}, clijson.ExitUsage},
		{"usage-graph-bad-format-missing-bundle", []string{"graph", "../testdata/does-not-exist", "--format", "bogus"}, clijson.ExitUsage},
		{"usage-convert-bad-type-map", []string{"convert", "../testdata/corpus-basic", "--dry-run", "--type-map", "docs"}, clijson.ExitUsage},

		// Regression: genuine IO/internal failures must stay ExitIO (exit 3),
		// not get reclassified as usage errors by the wiring above.
		{"io-graph-missing-bundle", []string{"graph", "../testdata/does-not-exist"}, clijson.ExitIO},
		{"io-validate-missing-bundle", []string{"validate", "../testdata/does-not-exist"}, clijson.ExitIO},

		// Regression: gating findings must stay ExitFindings (exit 1).
		{"findings-validate-nonconformant", []string{"validate", "../testdata/malformed"}, clijson.ExitFindings},
		{"findings-lint-strict", []string{"lint", "../testdata/corpus-lint-graph", "--today", "2023-11-14", "--strict"}, clijson.ExitFindings},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, code := runCLI(t, c.args...)
			if code != c.want {
				t.Errorf("args %v: exit = %d, want %d", c.args, code, c.want)
			}
		})
	}
}
