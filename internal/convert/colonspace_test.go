package convert

import (
	"io/fs"
	"os"
	"path/filepath"
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

		// --- the counterexample class: shapes that PARSE CLEANLY --------------
		// Each of these once produced a finding against a phantom key even though
		// the frontmatter is valid YAML. They are the reason stage 1 exists: a
		// block that parses has no trap in it, because YAML cannot parse one.
		{
			name: "multi-line DOUBLE-quoted scalar whose continuation looks like a key",
			doc:  "---\ndescription: \"line one\n  note: a: b\"\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "multi-line SINGLE-quoted scalar whose continuation looks like a key",
			doc:  "---\ndescription: 'line one\n  note: a: b'\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "quoted entry in a flow sequence (kvLine cannot split a quoted key)",
			doc:  "---\ntags: [\"a: b: c\", 'd: e: f']\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "quoted block-sequence items",
			doc:  "---\ntags:\n  - \"a: b: c\"\n  - 'd: e: f'\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "explicit-key syntax",
			doc:  "---\n? note\n: a: b\n---\n\n# X\n",
			want: nil,
		},
		{
			name: "quoted value containing a '#'",
			doc:  "---\ntitle: \"a #b: c\"\nother: plain\n---\n\n# X\n",
			want: nil,
		},

		// A multi-line quoted scalar sitting in a file that IS broken elsewhere:
		// stage 1 opens the door, and stage 2 must still name only the real key.
		{
			name: "real trap alongside a multi-line quoted scalar",
			doc:  "---\ndescription: \"line one\n  note: a: b\"\ntitle: Multi-View: Tabs and Windows\n---\n\n# X\n",
			want: []string{"title"},
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

// TestColonSpaceOnlyFiresOnUnparseableFrontmatter holds shut the property the
// advisory's never-gates rationale rests on: the rule fires ONLY on a file whose
// frontmatter convert could not parse — which is exactly the file lint already
// reports as an invalid-frontmatter schema violation. That is a universal claim,
// so it is asserted over every markdown fixture in the repo rather than argued
// from the handful of cases above, and it fails loudly if a future change lets
// the detector speak about a file that parses.
//
// The parse verdict is convert's own (toConcept), not a second opinion, so the
// two cannot drift apart.
func TestColonSpaceOnlyFiresOnUnparseableFrontmatter(t *testing.T) {
	var scanned, fired int
	err := filepath.WalkDir("../../testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return err
		}
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		scanned++
		norm, _ := NormalizeInput(raw)
		keys := colonSpaceKeys(norm)
		if len(keys) == 0 {
			return nil
		}
		fired++
		if _, _, _, perr := toConcept(native.New(), "x.md", raw); perr == nil {
			t.Errorf("%s: advisory fired on %v but the frontmatter PARSES CLEANLY, so no "+
				"invalid-frontmatter violation subsumes it — either the detector has a false "+
				"positive or the never-gates rationale needs restating", p, keys)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking testdata: %v", err)
	}
	if scanned == 0 || fired == 0 {
		t.Fatalf("vacuous: scanned %d file(s), advisory fired on %d — the property was never exercised", scanned, fired)
	}
	t.Logf("property held over %d markdown fixture(s); advisory fired on %d", scanned, fired)
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
