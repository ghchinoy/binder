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
