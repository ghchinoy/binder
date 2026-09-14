package convert

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/okf/native"
	"gopkg.in/yaml.v3"
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
			// Drive stage 1 from the REAL codec verdict, exactly as convert does,
			// so no case can pass against a parse the production path never makes.
			_, norm, _, perr := toConcept(native.New(), "x.md", []byte(c.doc))
			got := colonSpaceKeys(norm, perr != nil)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("colonSpaceKeys = %v, want %v (codec parse error: %v)\ndoc:\n%s",
					got, c.want, perr, c.doc)
			}
		})
	}
}

// TestColonSpaceStage1IsTheCallersVerdictAlone pins the contract that makes the
// never-gates rationale structural: when the caller says the frontmatter PARSED,
// the detector reports nothing — whatever the bytes happen to contain.
//
// This is the guard against re-deriving the verdict inside the detector. An
// earlier revision did exactly that, with yaml.Unmarshal into an `any`; the codec
// unmarshals into a yaml.Node, and those are NOT the same acceptance predicate
// (see TestCodecAndPlainParseDisagree). Any such second parse resurrects the
// possibility of the detector calling a file broken that convert accepted.
//
// The documents below contain REAL traps, so a detector that re-parsed would find
// them and return keys; one that honours its caller returns nothing. That is what
// makes this test bite rather than pass by luck.
func TestColonSpaceStage1IsTheCallersVerdictAlone(t *testing.T) {
	traps := []string{
		"---\ntitle: Multi-View: Tabs and Windows\n---\n\n# X\n",
		"---\ndescription: Goal: ship the thing\ngoal: another: bad line\n---\n\n# X\n",
	}
	for _, doc := range traps {
		norm, _ := NormalizeInput([]byte(doc))
		// Sanity: with a failed-parse verdict these documents DO produce findings,
		// so a nil result below cannot be nil for some unrelated reason.
		if got := colonSpaceKeys(norm, true); len(got) == 0 {
			t.Fatalf("premise gone: no findings even when told the parse failed\ndoc:\n%s", doc)
		}
		if got := colonSpaceKeys(norm, false); got != nil {
			t.Errorf("colonSpaceKeys = %v when told the frontmatter PARSED; stage 1 is deciding "+
				"for itself instead of taking convert's verdict\ndoc:\n%s", got, doc)
		}
	}
}

// TestCodecAndPlainParseDisagree records WHY the verdict is passed in rather than
// recomputed: the obvious lookalike is not equivalent to the codec.
//
// The codec unmarshals frontmatter into a yaml.Node. Unmarshalling the same bytes
// into an `any` additionally rejects duplicate keys and unresolvable tags, so a
// detector using it would call these documents broken while convert accepts them
// — no recovery, hence no invalid-frontmatter violation to subsume a finding.
//
// Honest scope: this divergence is a LATENT hazard, not a live bug. Stage 2 is
// conservative enough that these particular documents yield no finding even when
// stage 1 is wrongly opened, because a line stage 2 would flag is itself invalid
// YAML and so breaks the codec too. The fix removes the hazard by construction
// instead of leaving it resting on stage 2 staying conservative forever.
func TestCodecAndPlainParseDisagree(t *testing.T) {
	cases := []struct{ name, doc string }{
		{"duplicate keys", "---\ntitle: A\ntitle: B\nnote: plain\n---\n\n# X\n"},
		{"unresolvable tag", "---\nn: !!int notanint\n---\n\n# X\n"},
		// A third divergence — a self-referential anchor ("a: &x" / "  b: *x") — is
		// deliberately NOT exercised: it crashes the codec outright on origin/main
		// (native.nodeToValue recurses forever on the alias → stack overflow →
		// process death), independently of this advisory. Reported separately; out
		// of scope for #93, and a test that kills the runner proves nothing.
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, norm, _, perr := toConcept(native.New(), "x.md", []byte(c.doc))
			if perr != nil {
				t.Fatalf("premise gone: the codec now REJECTS this document (%v)\ndoc:\n%s", perr, c.doc)
			}
			fmText, _ := frontmatterRegion(strings.ReplaceAll(string(norm), "\r\n", "\n"))
			var whole any
			if yaml.Unmarshal([]byte(fmText), &whole) == nil {
				t.Fatalf("premise gone: an `any` parse now ACCEPTS this too, so the two no longer "+
					"diverge and the argument for passing the verdict in needs restating\ndoc:\n%s", c.doc)
			}
			if got := colonSpaceKeys(norm, perr != nil); got != nil {
				t.Errorf("colonSpaceKeys = %v on a document the codec parsed cleanly\ndoc:\n%s", got, c.doc)
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
		_, norm, _, perr := toConcept(native.New(), "x.md", raw)
		keys := colonSpaceKeys(norm, perr != nil)
		if len(keys) == 0 {
			return nil
		}
		fired++
		if perr == nil {
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
