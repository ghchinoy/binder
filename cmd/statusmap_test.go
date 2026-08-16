package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// statusCorpus writes a one-file, frontmatter-free corpus so a --status-map
// value is the sole source of the written status.
func statusCorpus(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "note.md"), []byte("# Note\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

// readStatusValue returns the `status:` frontmatter value written to note.md in
// dir, or "" if the file or key is absent.
func readStatusValue(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "note.md"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "status:") {
			v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "status:"))
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// statusSurface abstracts running enrich (in place) vs convert (to a bundle) so
// one test body proves the two surfaces behave identically (criterion 5). run
// returns combined output, the exit code, and the directory where artifacts land
// (src for enrich, the output bundle for convert).
type statusSurface struct {
	name string
	run  func(t *testing.T, src string, extra ...string) (out string, code int, artifactDir string)
}

func statusSurfaces() []statusSurface {
	return []statusSurface{
		{
			name: "enrich",
			run: func(t *testing.T, src string, extra ...string) (string, int, string) {
				out, code := runCLI(t, append([]string{"enrich", src}, extra...)...)
				return out, code, src
			},
		},
		{
			name: "convert",
			run: func(t *testing.T, src string, extra ...string) (string, int, string) {
				outDir := filepath.Join(t.TempDir(), "bundle")
				out, code := runCLI(t, append([]string{"convert", src, "-o", outDir}, extra...)...)
				return out, code, outDir
			},
		},
	}
}

// Criterion 1: a conformant --status-map value behaves exactly like today —
// exit 0, the value is written, and no status-vocabulary note appears.
func TestStatusMapConformant(t *testing.T) {
	for _, s := range statusSurfaces() {
		t.Run(s.name, func(t *testing.T) {
			t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
			isolateConfig(t)
			src := statusCorpus(t)
			out, code, dir := s.run(t, src, "--status-map", "default=stable")
			if code != clijson.ExitSuccess {
				t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
			}
			if got := readStatusValue(t, dir); got != "stable" {
				t.Fatalf("written status = %q, want stable", got)
			}
			if strings.Contains(out, "not one of") || strings.Contains(out, "§5.4") {
				t.Errorf("conformant run emitted a vocabulary note:\n%s", out)
			}
		})
	}
}

// Criterion 2: a non-conformant value on the default path warns up front (names
// value + key, lists the legal set, cites §5.4), exits 0, and writes the user's
// ORIGINAL value unmodified.
func TestStatusMapNonConformantWarnsDefault(t *testing.T) {
	for _, s := range statusSurfaces() {
		t.Run(s.name, func(t *testing.T) {
			t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
			isolateConfig(t)
			src := statusCorpus(t)
			out, code, dir := s.run(t, src, "--status-map", "default=active")
			if code != clijson.ExitSuccess {
				t.Fatalf("exit = %d, want 0 (default path warns, never rejects); out:\n%s", code, out)
			}
			for _, want := range []string{`"active"`, `"default"`, "draft|stable|deprecated", "§5.4"} {
				if !strings.Contains(out, want) {
					t.Errorf("warning missing %q in:\n%s", want, out)
				}
			}
			if got := readStatusValue(t, dir); got != "active" {
				t.Fatalf("written status = %q, want the original unmodified \"active\"", got)
			}
		})
	}
}

// Criterion 3: the same non-conformant value under --strict gates (exit 1).
func TestStatusMapNonConformantStrictGates(t *testing.T) {
	for _, s := range statusSurfaces() {
		t.Run(s.name, func(t *testing.T) {
			t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
			isolateConfig(t)
			src := statusCorpus(t)
			out, code, _ := s.run(t, src, "--strict", "--status-map", "default=active")
			if code != clijson.ExitFindings {
				t.Fatalf("exit = %d, want %d (gate); out:\n%s", code, clijson.ExitFindings, out)
			}
		})
	}
}

// Criterion 4: validation happens BEFORE any write — after a --strict failure the
// corpus (enrich) and the output bundle (convert) are untouched.
func TestStatusMapStrictWritesNothing(t *testing.T) {
	t.Run("enrich", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		isolateConfig(t)
		src := statusCorpus(t)
		before, err := os.ReadFile(filepath.Join(src, "note.md"))
		if err != nil {
			t.Fatal(err)
		}
		_, code := runCLI(t, "enrich", src, "--strict", "--status-map", "default=active")
		if code != clijson.ExitFindings {
			t.Fatalf("exit = %d, want %d", code, clijson.ExitFindings)
		}
		after, _ := os.ReadFile(filepath.Join(src, "note.md"))
		if string(after) != string(before) {
			t.Fatalf("source mutated on --strict failure:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})
	t.Run("convert", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		isolateConfig(t)
		src := statusCorpus(t)
		outDir := filepath.Join(t.TempDir(), "bundle")
		_, code := runCLI(t, "convert", src, "-o", outDir, "--strict", "--status-map", "default=active")
		if code != clijson.ExitFindings {
			t.Fatalf("exit = %d, want %d", code, clijson.ExitFindings)
		}
		if _, err := os.Stat(outDir); !os.IsNotExist(err) {
			t.Fatalf("output bundle created on --strict failure (err=%v); nothing should be written", err)
		}
	})
}

// Criterion 6: canonicalization is opt-in; on, it maps exactly the listed aliases
// and reports each rewrite; an out-of-table value is still just a warning.
func TestStatusMapCanonicalizeOptIn(t *testing.T) {
	aliases := map[string]string{
		"active":      "stable",
		"wip":         "draft",
		"in-progress": "draft",
		"archived":    "deprecated",
		"legacy":      "deprecated",
	}
	for _, s := range statusSurfaces() {
		for alias, want := range aliases {
			t.Run(s.name+"/"+alias, func(t *testing.T) {
				t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
				isolateConfig(t)
				src := statusCorpus(t)
				out, code, dir := s.run(t, src, "--canonicalize-status", "--status-map", "default="+alias)
				if code != clijson.ExitSuccess {
					t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
				}
				if got := readStatusValue(t, dir); got != want {
					t.Fatalf("canonicalized status = %q, want %q", got, want)
				}
				if !strings.Contains(out, "canonicalized") || !strings.Contains(out, `"`+want+`"`) {
					t.Errorf("rewrite not reported for %s->%s:\n%s", alias, want, out)
				}
			})
		}
	}
}

// Criterion 6 (continued): an out-of-table value under --canonicalize-status is
// NOT rewritten — it stays a non-conformance warning, exit 0.
func TestStatusMapCanonicalizeUnknownStillWarns(t *testing.T) {
	for _, s := range statusSurfaces() {
		t.Run(s.name, func(t *testing.T) {
			t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
			isolateConfig(t)
			src := statusCorpus(t)
			out, code, dir := s.run(t, src, "--canonicalize-status", "--status-map", "default=experimental")
			if code != clijson.ExitSuccess {
				t.Fatalf("exit = %d, want 0; out:\n%s", code, out)
			}
			if got := readStatusValue(t, dir); got != "experimental" {
				t.Fatalf("out-of-table value rewritten to %q; must be left unchanged", got)
			}
			if !strings.Contains(out, "not one of") {
				t.Errorf("out-of-table value should still warn:\n%s", out)
			}
		})
	}
}

// Criterion 7: a malformed --status-map argument is a usage error (exit 2) whose
// message names the problem — distinct from a well-formed non-spec value.
func TestStatusMapMalformedExit2(t *testing.T) {
	for _, s := range statusSurfaces() {
		t.Run(s.name, func(t *testing.T) {
			t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
			isolateConfig(t)
			src := statusCorpus(t)
			out, code, _ := s.run(t, src, "--status-map", "not-a-pair")
			if code != clijson.ExitUsage {
				t.Fatalf("exit = %d, want %d (usage); out:\n%s", code, clijson.ExitUsage, out)
			}
		})
	}
	// The error message must name the offending argument.
	err := runCLIErr(t, "convert", statusCorpus(t), "-o", t.TempDir(), "--status-map", "not-a-pair")
	if err == nil || !strings.Contains(err.Error(), "--status-map") {
		t.Fatalf("malformed error = %v, want it to name --status-map", err)
	}
}

// Criterion 8: two --json runs on the SAME inputs are byte-identical
// (determinism), status_notes is present when non-conformant, and omitted when
// conformant (purely additive to binder.report/v1).
func TestStatusMapJSONDeterministicAndAdditive(t *testing.T) {
	// argsFor builds an identical invocation (same src + same out) so a second run
	// must be byte-for-byte identical if the command is deterministic.
	surfaces := map[string]func(src, out, value string) []string{
		"enrich": func(src, _ /*out*/, value string) []string {
			return []string{"enrich", src, "--dry-run", "--json", "--status-map", "default=" + value}
		},
		"convert": func(src, out, value string) []string {
			return []string{"convert", src, "-o", out, "--dry-run", "--json", "--status-map", "default=" + value}
		},
	}
	for name, argsFor := range surfaces {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
			isolateConfig(t)
			src := statusCorpus(t)
			out := filepath.Join(t.TempDir(), "bundle")

			// Non-conformant: status_notes present, two identical runs.
			a, code := runCLI(t, argsFor(src, out, "active")...)
			if code != clijson.ExitSuccess {
				t.Fatalf("exit = %d, want 0", code)
			}
			b, _ := runCLI(t, argsFor(src, out, "active")...)
			if a != b {
				t.Fatalf("non-deterministic --json output:\nA:\n%s\nB:\n%s", a, b)
			}
			if !strings.Contains(a, "status_notes") {
				t.Errorf("status_notes absent from non-conformant --json:\n%s", a)
			}

			// Conformant: status_notes omitted (omitempty) → additive, no new field.
			c, _ := runCLI(t, argsFor(src, out, "stable")...)
			if strings.Contains(c, "status_notes") {
				t.Errorf("status_notes should be omitted on a conformant run:\n%s", c)
			}
		})
	}
}
