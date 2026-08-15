package cmd

import (
	"strings"
	"testing"
)

// TestVersionMatchesEnvelope asserts --version prints "binder/<version>", the
// exact string used in the JSON envelope's "binder" field (the documented
// discovery surface, #13 §4.5).
func TestVersionMatchesEnvelope(t *testing.T) {
	out, code := runCLI(t, "--version")
	if code != 0 {
		t.Fatalf("--version exit = %d, want 0", code)
	}
	want := "binder/" + Version + "\n"
	if out != want {
		t.Errorf("--version = %q, want %q", out, want)
	}
}

// TestHelpListsJSONFlag asserts each command's --help exposes the --json flag,
// so the help output is a sufficient discovery surface for agents.
func TestHelpListsJSONFlag(t *testing.T) {
	for _, cmd := range []string{"convert", "validate", "review", "graph"} {
		out, code := runCLI(t, cmd, "--help")
		if code != 0 {
			t.Fatalf("%s --help exit = %d, want 0", cmd, code)
		}
		if !strings.Contains(out, "--json") {
			t.Errorf("%s --help does not mention --json:\n%s", cmd, out)
		}
	}
}
