package convert

import (
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf/native"
)

// TestColonSpaceKeys is the unit table for the issue-#93 detector: which
// frontmatter shapes carry the unquoted colon-space trap and which are the
// classic false positives that must stay clean.
func TestColonSpaceKeys(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want []string
	}{
		// --- positives: unquoted plain scalars containing ": " ---------------
		{
			// The wild instance the #93 comment measured in saschb2b/okf-studio:
			// the defect is on title:, not description:, so the rule is keyed on
			// no particular key.
			name: "title (the measured wild instance)",
			doc:  "---\ntitle: Multi-View: Tabs and Windows\n---\n\n# X\n",
			want: []string{"title"},
		},
		{
			name: "description (the original framing)",
			doc:  "---\ndescription: Goal: ship the thing\n---\n\n# X\n",
			want: []string{"description"},
		},
		{
			name: "an arbitrary key, not a known OKF one",
			doc:  "---\nwhatever_key: Heads up: this breaks\n---\n\n# X\n",
			want: []string{"whatever_key"},
		},
		{
			name: "every offending key on the file, in document order",
			doc:  "---\ntitle: thing: with an unquoted colon\ngoal: another: bad line\n---\n\n# X\n",
			want: []string{"title", "goal"},
		},
		{
			name: "nested key below the top level",
			doc:  "---\nmeta:\n  blurb: Note: nested values trap too\n---\n\n# X\n",
			want: []string{"blurb"},
		},
		{
			name: "block-sequence item",
			doc:  "---\nitems:\n  - label: Warning: in a sequence\n---\n\n# X\n",
			want: []string{"label"},
		},
		{
			name: "unterminated fence is still scanned (convert recovers the file)",
			doc:  "---\ntitle: Multi-View: Tabs and Windows\n\n# X\n",
			want: []string{"title"},
		},
		{
			name: "CRLF line endings",
			doc:  "---\r\ntitle: Multi-View: Tabs and Windows\r\n---\r\n\r\n# X\r\n",
			want: []string{"title"},
		},

		// --- negatives: the false positives that must stay clean -------------
		{
			name: "double-quoted value (the fix the advisory asks for)",
			doc:  "---\ntitle: \"Multi-View: Tabs and Windows\"\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "single-quoted value",
			doc:  "---\ntitle: 'Multi-View: Tabs and Windows'\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "URL — a colon NOT followed by a space",
			doc:  "---\nhomepage: https://example.com/a:b\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "timestamp — a colon NOT followed by a space",
			doc:  "---\nstandup: 12:30\nratio: 16:9\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "literal block scalar interior is text, not a mapping",
			doc:  "---\nsummary: |\n  Note: this is ordinary prose.\n  Warning: so is this.\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "folded block scalar interior is text too",
			doc:  "---\nsummary: >\n  Heads up: still prose.\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "flow collection is not a plain scalar",
			doc:  "---\ntags: [alpha, beta]\nmeta: {a: 1}\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "the body is never scanned",
			doc:  "---\ntitle: A\n---\n\n# X\n\nProse: with a colon-space, and a line like key: value: value.\n",
			want: nil,
		},
		{
			name: "no frontmatter fence at all",
			doc:  "# X\n\ntitle: thing: with an unquoted colon\n",
			want: nil,
		},
		{
			name: "empty and nested-mapping keys have no scalar to judge",
			doc:  "---\nempty:\nnested:\n  inner: fine\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "comment lines are not mapping entries",
			doc:  "---\n# Note: a comment with a colon-space\ntitle: A\n---\n\n# X\n",
			want: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			norm, _ := NormalizeInput([]byte(c.doc))
			got := colonSpaceKeys(norm)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("colonSpaceKeys = %v, want %v\ndoc:\n%s", got, c.want, c.doc)
			}
		})
	}
}

// TestColonSpaceKeysSurfacedInFacts: the detector is wired into the authored-state
// facts `binder lint` reads, on the pre-existing #93 positive-control fixture.
func TestColonSpaceKeysSurfacedInFacts(t *testing.T) {
	_, facts, _, err := Analyze("../../testdata/corpus-lint-schema", Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNowInternal,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	for _, f := range facts {
		want := []string(nil)
		if f.ConceptID == "badyaml" {
			want = []string{"title", "goal"}
		}
		if strings.Join(f.ColonSpaceKeys, ",") != strings.Join(want, ",") {
			t.Errorf("%s: ColonSpaceKeys = %v, want %v", f.ConceptID, f.ColonSpaceKeys, want)
		}
	}
}
