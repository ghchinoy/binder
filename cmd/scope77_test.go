package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// writeScopeBundle writes files into a temp dir and returns its path.
func writeScopeBundle(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// garbageReservedBundle is a bundle whose one concept is conformant but whose
// reserved files (index.md/log.md) are arbitrary garbage. Before #77 this read
// as flatly "conformant" over a surface the validator never examined.
func garbageReservedBundle(t *testing.T) string {
	return writeScopeBundle(t, map[string]string{
		"a.md":     "---\ntype: concept\ntitle: A\n---\n\nBody\n",
		"index.md": "THIS IS NOT VALID INDEX STRUCTURE AT ALL\n\ngarbage\n",
		"log.md":   "garbage log with no structure whatsoever\n",
	})
}

// TestValidateScopeVisibleProse: on a bundle with reserved files, the prose
// verdict must make its unchecked scope explicit (#77 item 1) while STILL
// reporting conformant and STILL exiting 0 — bare and under --strict. This is a
// 0-exit-code-delta change; the scope qualifier must not turn into a gate.
func TestValidateScopeVisibleProse(t *testing.T) {
	dir := garbageReservedBundle(t)

	for _, args := range [][]string{
		{"validate", dir},
		{"validate", dir, "--strict"},
	} {
		out, code := runCLI(t, args...)
		if code != clijson.ExitSuccess {
			t.Fatalf("%v exit = %d, want 0 (must not gate on unchecked scope); out:\n%s", args, code, out)
		}
		if !strings.Contains(out, "RESULT: conformant") {
			t.Errorf("%v verdict changed; out:\n%s", args, out)
		}
		if !strings.Contains(out, "reserved-file structure") || !strings.Contains(out, "not validated") {
			t.Errorf("%v prose does not make unchecked reserved-file scope visible; out:\n%s", args, out)
		}
	}
}

// TestValidateScopeVisibleJSON: the --json envelope must carry the scope
// explicitly (reserved_structure_checked=false) so a machine consumer is not
// misled into reading `conformant` as covering the reserved files. Exit stays 0.
func TestValidateScopeVisibleJSON(t *testing.T) {
	dir := garbageReservedBundle(t)

	out, code := runCLI(t, "validate", dir, "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("validate --json exit = %d, want 0; out:\n%s", code, out)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("validate --json not valid JSON: %v\n%s", err, out)
	}
	result := env.Result.(map[string]any)
	checked, ok := result["reserved_structure_checked"]
	if !ok {
		t.Fatalf("json missing reserved_structure_checked key; result: %v", result)
	}
	if checked != false {
		t.Errorf("reserved_structure_checked = %v, want false (structure not examined)", checked)
	}
	// The counted-but-unchecked surface must be observable together.
	if nr, _ := result["num_reserved"].(float64); nr != 2 {
		t.Errorf("num_reserved = %v, want 2", nr)
	}
	// --strict must not change the payload's scope nor the exit code.
	if _, code := runCLI(t, "validate", dir, "--strict", "--json"); code != clijson.ExitSuccess {
		t.Errorf("validate --strict --json exit = %d, want 0", code)
	}
}

// TestValidateScopeDeterministic: two --json runs on one input are byte-identical.
func TestValidateScopeDeterministic(t *testing.T) {
	dir := garbageReservedBundle(t)
	a, _ := runCLI(t, "validate", dir, "--json")
	b, _ := runCLI(t, "validate", dir, "--json")
	if a != b {
		t.Errorf("validate --json not deterministic:\n%s\n---\n%s", a, b)
	}
}

// TestValidateScopeNoteOnlyWhenReserved: a bundle with no reserved files must
// not sprout a reserved-file scope note — the qualifier appears only when there
// is an unchecked reserved surface to disclose.
func TestValidateScopeNoteOnlyWhenReserved(t *testing.T) {
	dir := writeScopeBundle(t, map[string]string{
		"a.md": "---\ntype: concept\ntitle: A\n---\n\nBody\n",
	})
	out, code := runCLI(t, "validate", dir)
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
	}
	if strings.Contains(out, "reserved-file structure") {
		t.Errorf("scope note emitted for a bundle with no reserved files:\n%s", out)
	}
}

// TestValidateScopePositiveControl: the scope qualifier must not weaken real
// validation. A genuinely non-conformant CONCEPT (even alongside garbage
// reserved files) must still be reported non-conformant and must still gate
// (exit 1) under both bare validate and --strict.
func TestValidateScopePositiveControl(t *testing.T) {
	dir := writeScopeBundle(t, map[string]string{
		"bad.md":   "---\ntitle: Has No Type\n---\n\nno type field\n",
		"index.md": "garbage index\n",
		"log.md":   "garbage log\n",
	})

	for _, args := range [][]string{
		{"validate", dir},
		{"validate", dir, "--strict"},
	} {
		out, code := runCLI(t, args...)
		if code != clijson.ExitFindings {
			t.Fatalf("%v exit = %d, want 1 (real violation must still gate); out:\n%s", args, code, out)
		}
		if !strings.Contains(out, "RESULT: NOT conformant") {
			t.Errorf("%v did not report non-conformance; out:\n%s", args, out)
		}
	}

	// The concept violation is present in the JSON payload alongside the scope.
	out, code := runCLI(t, "validate", dir, "--strict", "--json")
	if code != clijson.ExitFindings {
		t.Fatalf("--json exit = %d, want 1; out:\n%s", code, out)
	}
	var env clijson.Envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	result := env.Result.(map[string]any)
	if findings, _ := result["findings"].([]any); len(findings) == 0 {
		t.Errorf("expected a real finding in the payload; result: %v", result)
	}
	if result["reserved_structure_checked"] != false {
		t.Errorf("reserved_structure_checked = %v, want false", result["reserved_structure_checked"])
	}
}
