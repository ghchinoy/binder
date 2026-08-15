package convert

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ghchinoy/binder/internal/okf/native"
)

// fixedNowInternal matches the golden fixtures' SOURCE_DATE_EPOCH=1700000000.
var fixedNowInternal = time.Unix(1700000000, 0).UTC()

func readTreeInternal(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAnalyzeMatchesConvert is the behavior-preserving regression guard for the
// Convert → Analyze+writeBundle refactor (issue #8). For both a defaults run and
// a flags-on run it asserts that:
//   - the bundle written by Convert is byte-identical to one written by calling
//     Analyze and then writeBundle directly (the refactor seam is transparent);
//   - Convert's Report equals Analyze's Report (with Out set), field for field.
//
// If a future change makes Analyze and Convert diverge, this fails loudly.
func TestAnalyzeMatchesConvert(t *testing.T) {
	cases := []struct {
		name string
		src  string
		opts Options
	}{
		{
			name: "defaults",
			src:  "../../testdata/corpus-basic",
			opts: Options{Codec: native.New(), Version: "0.1.0", Now: fixedNowInternal},
		},
		{
			name: "flags-on",
			src:  "../../testdata/corpus-basic",
			opts: Options{
				Codec:            native.New(),
				Version:          "0.1.0",
				Now:              fixedNowInternal,
				DefaultType:      "Guide",
				FMRefKeys:        []string{"related", "parent"},
				GroupByType:      true,
				IncludeBacklinks: true,
				IncludeGraph:     true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Path A: Convert writes the bundle.
			outConvert := t.TempDir()
			convertRep, err := Convert(tc.src, outConvert, tc.opts)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}

			// Path B: Analyze (read-only) then writeBundle directly.
			concepts, facts, analyzeRep, err := Analyze(tc.src, tc.opts)
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if len(facts) != len(concepts) {
				t.Errorf("facts count %d != concepts count %d", len(facts), len(concepts))
			}
			outAnalyze := t.TempDir()
			if err := writeBundle(outAnalyze, concepts, tc.opts.Codec, IndexOptions{
				GroupByType:      tc.opts.GroupByType,
				IncludeBacklinks: tc.opts.IncludeBacklinks,
				IncludeGraph:     tc.opts.IncludeGraph,
			}); err != nil {
				t.Fatalf("writeBundle: %v", err)
			}

			// The two output trees must be byte-identical.
			gotA := readTreeInternal(t, outConvert)
			gotB := readTreeInternal(t, outAnalyze)
			if len(gotA) != len(gotB) {
				t.Fatalf("file counts differ: Convert=%d Analyze+write=%d", len(gotA), len(gotB))
			}
			for name, a := range gotA {
				if b, ok := gotB[name]; !ok {
					t.Errorf("Analyze+write missing %q", name)
				} else if a != b {
					t.Errorf("file %q differs:\n--- Convert ---\n%s\n--- Analyze+write ---\n%s", name, a, b)
				}
			}

			// Reports must match once Out (set only by Convert) is aligned.
			analyzeRep.Out = outConvert
			if !reflect.DeepEqual(convertRep, analyzeRep) {
				t.Errorf("Report mismatch:\nConvert=%+v\nAnalyze=%+v", convertRep, analyzeRep)
			}
		})
	}
}

// TestAnalyzeIsReadOnly asserts Analyze mutates neither the source corpus nor the
// working directory (it has no output path and must never write a bundle).
func TestAnalyzeIsReadOnly(t *testing.T) {
	src := "../../testdata/corpus-basic"
	before := readTreeInternal(t, src)
	concepts, facts, rep, err := Analyze(src, Options{Codec: native.New(), Version: "0.1.0", Now: fixedNowInternal})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(concepts) == 0 || len(facts) == 0 || rep.NumConcepts == 0 {
		t.Fatal("Analyze returned an empty analysis")
	}
	if rep.Out != "" {
		t.Errorf("Analyze set Report.Out = %q, want empty (only Convert knows the output path)", rep.Out)
	}
	after := readTreeInternal(t, src)
	if !reflect.DeepEqual(before, after) {
		t.Error("Analyze mutated the source corpus")
	}
}
