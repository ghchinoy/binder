package lint_test

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/lint"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// TestLintEntrypointsNotOrphans proves the issue #24 reclassification in lint over
// a real corpus: the root README and a non-README node that both link out (no
// inbound) are ENTRYPOINTS, a linked-to node is neither, and a node with no edges
// at all is a true ORPHAN.
func TestLintEntrypointsNotOrphans(t *testing.T) {
	src := "../../testdata/corpus-lint-entrypoints"
	concepts, facts, _, err := convert.Analyze(src, convert.Options{
		Codec: native.New(), Version: "0.1.0", Now: fixedNow,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	rep := lint.Lint(concepts, facts, fixedToday, nil)

	if want := []string{"README", "start"}; !equalStrings(rep.Entrypoints, want) {
		t.Errorf("entrypoints = %v, want %v", rep.Entrypoints, want)
	}
	if want := []string{"lonely"}; !equalStrings(rep.Orphans, want) {
		t.Errorf("orphans = %v, want %v (only the truly disconnected node)", rep.Orphans, want)
	}
	// guide is linked-to, so it is neither.
	for _, id := range append(append([]string{}, rep.Orphans...), rep.Entrypoints...) {
		if id == "guide" {
			t.Errorf("guide has inbound links; it must be neither orphan nor entrypoint")
		}
	}

	s := rep.String()
	if !strings.Contains(s, "entrypoints (no inbound links): 2") ||
		!strings.Contains(s, "orphans (no inbound or outbound links): 1") {
		t.Errorf("prose missing entrypoint/orphan sections:\n%s", s)
	}
}

// TestLintDesignatedEntrypoint: a node that would be a true orphan, named via the
// designation, is reclassified as an entrypoint (proving the flag path in lint).
func TestLintDesignatedEntrypoint(t *testing.T) {
	src := "../../testdata/corpus-lint-entrypoints"
	concepts, facts, _, err := convert.Analyze(src, convert.Options{
		Codec: native.New(), Version: "0.1.0", Now: fixedNow,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	rep := lint.Lint(concepts, facts, fixedToday, []string{"lonely"})
	if len(rep.Orphans) != 0 {
		t.Errorf("orphans = %v, want none once 'lonely' is designated an entrypoint", rep.Orphans)
	}
	if want := []string{"README", "lonely", "start"}; !equalStrings(rep.Entrypoints, want) {
		t.Errorf("entrypoints = %v, want %v", rep.Entrypoints, want)
	}
}
