package convert_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// TestConvertPreservesNestedAndListOrderWhenStamping is the regression guard for
// PR#1 review finding R1. When a source concept carries nested trust maps whose
// keys are in NON-alphabetical (source) order and has NO `generated` stamp, the
// converter adds `generated` — which takes the re-encode path. That path MUST
// preserve nested-map key order and list-item order at every level (design-v2
// §3.2), not alphabetise them. A naive decode→re-encode would reorder
// {by,at}->{at,by}, [{title,id,resource}]->[{id,resource,title}], and
// {resource,receipt}->{receipt,resource}.
func TestConvertPreservesNestedAndListOrderWhenStamping(t *testing.T) {
	src := t.TempDir()
	// Deliberately non-alphabetical nested keys / list-item keys, and NO `generated`.
	doc := "---\n" +
		"type: Metric\n" +
		"title: Revenue\n" +
		"verified:\n" +
		"  by: human:alice\n" +
		"  at: '2026-02-01T00:00:00Z'\n" +
		"sources:\n" +
		"- title: General Ledger\n" +
		"  id: ledger\n" +
		"  resource: https://example.com/ledger\n" +
		"executor:\n" +
		"  resource: run-42\n" +
		"  receipt: sha256:abc\n" +
		"---\n\n# Revenue\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(src, "revenue.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	if _, err := convert.Convert(src, out, convert.Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNow,
	}); err != nil {
		t.Fatalf("convert: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "revenue.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)

	// The stamp must actually have been added (otherwise this would trivially
	// pass via the verbatim fast path and prove nothing).
	if !strings.Contains(s, "\ngenerated:\n") {
		t.Fatalf("expected a synthesised `generated` stamp; got:\n%s", s)
	}

	// Each nested/list block must keep its SOURCE key order. Scope each check to
	// its own block so a repeated key (e.g. `resource:`) can't satisfy the wrong
	// one.
	assertOrder(t, block(s, "verified:", "sources:"), "verified", "by:", "at:")
	assertOrder(t, block(s, "sources:", "executor:"), "sources item", "title:", "id:", "resource:")
	assertOrder(t, block(s, "executor:", "generated:"), "executor", "resource:", "receipt:")
}

// block returns the substring of s between the first occurrence of start and the
// next occurrence of end (start included, end excluded).
func block(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i:]
	if j := strings.Index(rest, end); j >= 0 {
		return rest[:j]
	}
	return rest
}

// assertOrder fails unless each needle appears, in the given order, in s.
func assertOrder(t *testing.T, s, label string, needles ...string) {
	t.Helper()
	prev := -1
	for _, n := range needles {
		i := strings.Index(s, n)
		if i < 0 {
			t.Fatalf("%s: %q missing from output:\n%s", label, n, s)
		}
		if i <= prev {
			t.Fatalf("%s: %q is out of source order (nested/list order not preserved):\n%s", label, n, s)
		}
		prev = i
	}
}
