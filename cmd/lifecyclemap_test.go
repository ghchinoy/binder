package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// TestLifecycleMapMalformedExit2 asserts malformed --status-map / --stale-after-map
// values are usage errors (exit 2), per the Phase-2 contract.
func TestLifecycleMapMalformedExit2(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	cases := []struct {
		name string
		args []string
	}{
		{"bad-status-map", []string{"convert", "../testdata/corpus-basic", "--dry-run", "--status-map", "archive"}},
		{"bad-stale-grammar", []string{"convert", "../testdata/corpus-basic", "--dry-run", "--stale-after-map", "a=6m"}},
		{"bad-stale-unit", []string{"convert", "../testdata/corpus-basic", "--dry-run", "--stale-after-map", "a=+6w"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, code := runCLI(t, c.args...)
			if code != clijson.ExitUsage {
				t.Errorf("exit = %d, want %d (usage)", code, clijson.ExitUsage)
			}
		})
	}
}

// TestLifecycleMapIntegration converts a small corpus with --status-map and
// --stale-after-map and asserts the stamped frontmatter, set-when-absent, and
// deterministic relative dates end to end.
func TestLifecycleMapIntegration(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000") // 2023-11-14 UTC

	src := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("archive/old.md", "# Old\n\nbody\n")
	write("bench/run.md", "# Run\n\nbody\n")
	write("notes/keep.md", "---\nstatus: stable\nstale_after: 2030-01-01\n---\n# Keep\n\nbody\n")

	out := t.TempDir()
	_, code := runCLI(t, "convert", src, "-o", out,
		"--status-map", "archive=deprecated,default=active",
		"--stale-after-map", "bench=+6m")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0", code)
	}

	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(out, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	if got := read("archive/old.md"); !contains(got, "status: deprecated") {
		t.Errorf("archive/old.md missing status: deprecated:\n%s", got)
	}
	// notes/keep.md carries authored status + stale_after → never clobbered,
	// and default=active must NOT override the authored status.
	if got := read("notes/keep.md"); !contains(got, "status: stable") || !contains(got, "stale_after: 2030-01-01") {
		t.Errorf("notes/keep.md authored values clobbered:\n%s", got)
	}
	// New scalar dates are emitted quoted by the codec.
	if got := read("bench/run.md"); !contains(got, `stale_after: "2024-05-14"`) {
		t.Errorf("bench/run.md missing deterministic stale_after 2024-05-14:\n%s", got)
	}
}
