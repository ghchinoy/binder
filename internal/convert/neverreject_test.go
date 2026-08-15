package convert_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/okf/native"
	"github.com/ghchinoy/binder/internal/validate"
)

// TestConvertNeverRejectsUnparseableFrontmatter proves real-corpus realities —
// a closed fence with invalid YAML, and an unterminated fence — do NOT abort the
// run and are NOT dropped. Each is preserved as a plain-markdown concept
// (original text kept verbatim in the body), stamped with a default type so the
// bundle stays conformant, and reported (design-v2 §4 never-reject robustness).
func TestConvertNeverRejectsUnparseableFrontmatter(t *testing.T) {
	cases := map[string]struct {
		content string
		marker  string // a verbatim substring that must survive into the body
	}{
		// Invalid YAML in a CLOSED fence: unquoted colons in a scalar value.
		"invalid-yaml": {
			content: "---\ntitle: thing: with an unquoted colon\ngoal: another: bad line\n---\n\n# Real Heading\n\nBody survives.\n",
			marker:  "thing: with an unquoted colon",
		},
		// UNTERMINATED fence: opening --- with no closing fence.
		"unterminated-fence": {
			content: "---\ntitle: never closed\ntags: [a, b]\n\n# Heading After\n\nStill body.\n",
			marker:  "title: never closed",
		},
	}

	codec := native.New()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := t.TempDir()
			write(t, src, "bad.md", tc.content)
			write(t, src, "ok.md", "---\ntype: Note\ntitle: Fine\n---\n\n# Fine\n")

			out := filepath.Join(t.TempDir(), "bundle")
			rep, err := convert.Convert(src, out, convert.Options{Codec: native.New(), Version: "0.1.0", Now: fixedNow})
			if err != nil {
				t.Fatalf("convert must not abort on unparseable frontmatter: %v", err)
			}
			if rep.NumConcepts != 2 {
				t.Errorf("NumConcepts = %d, want 2 (bad file preserved, not dropped)", rep.NumConcepts)
			}

			// Reported by file + reason.
			var warned bool
			for _, w := range rep.Warnings {
				if strings.Contains(w, "bad.md") && strings.Contains(w, "did not parse") {
					warned = true
				}
			}
			if !warned {
				t.Errorf("expected a parse warning naming bad.md: %v", rep.Warnings)
			}

			// Original content preserved verbatim in the body.
			gotBytes, err := os.ReadFile(filepath.Join(out, "bad.md"))
			if err != nil {
				t.Fatal(err)
			}
			got := string(gotBytes)
			if !strings.Contains(got, tc.marker) || !strings.Contains(got, "Heading") {
				t.Errorf("preserved output missing original text %q:\n%s", tc.marker, got)
			}

			// The emitted concept re-parses cleanly: its NEW frontmatter is valid
			// YAML and carries a type; the old fence is inert body text.
			c, err := codec.ParseConcept("bad.md", gotBytes)
			if err != nil {
				t.Fatalf("recovered concept must re-parse cleanly: %v", err)
			}
			if strings.TrimSpace(c.Type) == "" {
				t.Error("recovered concept must carry a stamped type")
			}

			// Bundle is conformant after recovery.
			res, err := validate.Bundle(out, native.New(), okf.SpecV02)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Conformant() {
				t.Errorf("bundle should be conformant after recovery: %v", res.Errors())
			}

			// Re-running convert over the same source is byte-identical.
			out2 := filepath.Join(t.TempDir(), "bundle2")
			if _, err := convert.Convert(src, out2, convert.Options{Codec: native.New(), Version: "0.1.0", Now: fixedNow}); err != nil {
				t.Fatal(err)
			}
			again, err := os.ReadFile(filepath.Join(out2, "bad.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(again) != got {
				t.Error("recovery is not deterministic across runs")
			}
		})
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
