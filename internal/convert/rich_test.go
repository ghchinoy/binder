package convert_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf/native"
)

const (
	corpusRich   = "../../testdata/corpus-rich"
	expectedRich = "../../testdata/expected-rich"
)

// richOptions turns on every Phase-2 signal so the golden bundle exercises
// wikilinks, anchor links, frontmatter refs, hashtag/tag merge, per-dir index,
// reserved-collision rename, and corpus-native trust mapping together.
func richOptions() convert.Options {
	return convert.Options{
		Codec:        native.New(),
		Version:      "0.1.0",
		Now:          fixedNow,
		FMRefKeys:    []string{"related"},
		MapCitations: true,
		SourceKeys:   []string{"source"},
		MapDraft:     true,
	}
}

func convertRich(t *testing.T, out string) *convert.Report {
	t.Helper()
	rep, err := convert.Convert(corpusRich, out, richOptions())
	if err != nil {
		t.Fatalf("convert rich: %v", err)
	}
	return rep
}

// TestConvertRichGolden converts corpus-rich with all signals enabled and
// compares the whole tree to the committed expected-rich fixture. Run with
// -update to regenerate after an intentional change.
func TestConvertRichGolden(t *testing.T) {
	if *update {
		if err := os.RemoveAll(expectedRich); err != nil {
			t.Fatal(err)
		}
		convertRich(t, expectedRich)
		t.Logf("regenerated golden fixture at %s", expectedRich)
		return
	}

	out := t.TempDir()
	convertRich(t, out)
	got := readTree(t, out)
	want := readTree(t, expectedRich)
	for _, name := range union(keys(got), keys(want)) {
		g, gok := got[name]
		w, wok := want[name]
		switch {
		case !wok:
			t.Errorf("unexpected output file %q:\n%s", name, g)
		case !gok:
			t.Errorf("missing expected output file %q", name)
		case g != w:
			t.Errorf("file %q differs from golden:\n--- want ---\n%s\n--- got ---\n%s", name, w, g)
		}
	}
}

func TestConvertRichReportSignals(t *testing.T) {
	rep := convertRich(t, t.TempDir())

	// Every §4.2 signal that resolves is an edge; the one bad wikilink is not.
	// 5 concepts: intro, guides/setup, the renamed guides/index-note (reserved
	// collision, never dropped), tables/orders, attested/calc.
	if rep.NumConcepts != 5 {
		t.Errorf("NumConcepts = %d, want 5", rep.NumConcepts)
	}
	if rep.NumResolved == 0 {
		t.Error("expected resolved edges (wikilinks/anchors/fm-refs)")
	}
	if rep.NumUnresolved != 1 {
		t.Errorf("NumUnresolved = %d, want 1 ([[Nonexistent Topic]])", rep.NumUnresolved)
	}

	// The unresolved wikilink is reported, by source and raw target.
	var found bool
	for _, u := range rep.Unresolved {
		if strings.Contains(u.RawTarget, "Nonexistent Topic") {
			found = true
		}
	}
	if !found {
		t.Errorf("unresolved [[Nonexistent Topic]] not reported: %+v", rep.Unresolved)
	}

	// The reserved-name collision is warned, not dropped.
	var warned bool
	for _, w := range rep.Warnings {
		if strings.Contains(w, "guides/index.md") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a reserved-collision warning for guides/index.md: %v", rep.Warnings)
	}
}

func TestConvertRichDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	convertRich(t, a)
	convertRich(t, b)
	ta, tb := readTree(t, a), readTree(t, b)
	if len(ta) != len(tb) {
		t.Fatalf("different file counts: %d vs %d", len(ta), len(tb))
	}
	for name, ca := range ta {
		if ca != tb[name] {
			t.Errorf("non-deterministic output for %q", name)
		}
	}
}
