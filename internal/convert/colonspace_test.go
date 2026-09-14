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
			// Issue #93's FOURTH measured instance (a Game Master bundle). YAML
			// rejects the block with "did not find expected ',' or '}'", and the
			// key inside the flow mapping is named.
			name: "single-line flow mapping (the issue's flow-mapping instance)",
			doc:  "---\nmeta: {name: Multi-View: Tabs, x: 1}\n---\n\n# X\n",
			want: []string{"name"},
		},
		{
			// A colon followed by a TAB is the same mapping indicator to YAML.
			name: "colon-tab is the same defect as colon-space",
			doc:  "---\ndesc: value:\ttab after colon\n---\n\n# X\n",
			want: []string{"desc"},
		},
		{
			// O4: an unterminated fence is NOT treated as frontmatter. Such a file is
			// indistinguishable from markdown opening on a thematic break, and
			// scanning it meant reporting body prose as a key. The file is still
			// reported as "invalid frontmatter: unterminated '---' block"; only the
			// key name is given up.
			name: "unterminated fence is left alone (cannot be told from a thematic break)",
			doc:  "---\ntitle: Multi-View: Tabs and Windows\n\n# X\n",
			want: nil,
		},
		{
			name: "body prose after a thematic break is never named as a key",
			doc:  "---\n\nNote: prose: with a colon-space.\n",
			want: nil,
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
			name: "flow mapping with quoted values is left alone",
			doc:  "---\nmeta: {name: \"Multi-View: Tabs\", x: 1}\nbad: oops: here\n---\n\n# X\n",
			want: []string{"bad"},
		},
		{
			name: "flow mapping spanning lines is not guessed at",
			doc:  "---\nmeta: {\n  name: plain,\n}\nbad: oops: here\n---\n\n# X\n",
			want: []string{"bad"},
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
		// the frontmatter is valid YAML. They are why the advisory is gated on the
		// codec's verdict rather than on the scan alone. NOT because "a block that
		// parses has no trap in it" — that was the original, false rationale: a
		// colon-space is perfectly legal inside a block scalar or a quoted
		// continuation, where it is text and not a key. The scan cannot tell those
		// from a real trap, so the gate decides instead.
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
		// the codec's failure opens the door, and the scan must still name only the
		// real key rather than the phantom one in the quoted continuation.
		{
			name: "real trap alongside a multi-line quoted scalar",
			doc:  "---\ndescription: \"line one\n  note: a: b\"\ntitle: Multi-View: Tabs and Windows\n---\n\n# X\n",
			want: []string{"title"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Reproduce convert.Analyze's wiring exactly: the detector is consulted
			// ONLY when the codec failed to parse. No case may pass against a call
			// the production path would never make.
			_, norm, _, perr := toConcept(native.New(), "x.md", []byte(c.doc))
			var got []string
			if perr != nil {
				got = colonSpaceKeys(norm)
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("colonSpaceKeys = %v, want %v (codec parse error: %v)\ndoc:\n%s",
					got, c.want, perr, c.doc)
			}
		})
	}
}

// TestColonSpaceNeverDerivedForCleanFile is the guard on the wiring that carries
// the whole never-gates guarantee: convert.Analyze consults the detector ONLY for
// a file the codec failed to parse, so no clean file can draw an advisory.
//
// The three documents below are the reviewer's reproducers for PR #190, and each
// parses cleanly for the codec while the detector, called directly, names a
// phantom key that is literal text inside a block scalar. They reach it through
// the value-decode/node-decode divergence: yaml.Unmarshal into an `any` rejects
// duplicate keys and bad tag coercion, a yaml.Node decode accepts both, so any
// second opinion inside the detector opens on blocks convert parses happily — and
// the line scan cannot carry that alone, because a sequence-item (`- |`),
// anchored (`key: &a |`) or tagged (`key: !!str |`) block scalar leaves
// blockIndent at -1 and its interior is read as mapping entries.
//
// Rather than teach the scan those three shapes — and the next three — the caller
// gates on the codec's own verdict, which closes every such door at once. This
// test asserts BOTH halves, so it cannot pass by the detector having quietly gone
// silent: the direct call must still name the phantom key, and the wired path
// must still report nothing.
func TestColonSpaceNeverDerivedForCleanFile(t *testing.T) {
	cases := []struct{ name, doc string }{
		{
			"duplicate keys + sequence-item block scalar",
			"---\ntype: Note\ntitle: T\ndup: 1\ndup: 2\nsteps:\n  - |\n    Note: this is: literal text\n---\n\n# X\n",
		},
		{
			"bad tag coercion + anchored block scalar",
			"---\ntype: Note\ntitle: T\nn: !!int notanumber\nbody: &a |\n  Note: this is: literal text\n---\n\n# X\n",
		},
		{
			"duplicate keys + tagged block scalar",
			"---\ntype: Note\ntitle: T\ndup: 1\ndup: 2\nbody: !!str |\n  Note: this is: literal text\n---\n\n# X\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, norm, _, perr := toConcept(native.New(), "x.md", []byte(c.doc))
			if perr != nil {
				t.Fatalf("premise gone: the codec now REJECTS this document (%v), so it no longer "+
					"witnesses a clean-parsing file the detector would misread\ndoc:\n%s", perr, c.doc)
			}
			// Half one: ungated, the detector really would name a phantom key. Without
			// this the test could pass merely because the detector stopped working.
			if got := colonSpaceKeys(norm); len(got) == 0 {
				t.Fatalf("premise gone: the ungated detector no longer misreads this shape, so the "+
					"gate is no longer what is keeping it quiet\ndoc:\n%s", c.doc)
			}
			// Half two: run the SAME document through the real Analyze and assert the
			// advisory does not surface. An earlier revision re-implemented the gate
			// here instead (`if perr != nil { ... }`) — which, sitting after the Fatalf
			// on that same condition, could never execute: the assertion was dead and
			// the comment claiming both halves were covered was false. Calling Analyze
			// is also the only version that tests the wiring rather than a replica of
			// it, which is the whole point of fixing #93 in the call graph.
			dir := t.TempDir()
			if werr := os.WriteFile(filepath.Join(dir, "x.md"), []byte(c.doc), 0o644); werr != nil {
				t.Fatalf("writing fixture: %v", werr)
			}
			_, facts, _, aerr := Analyze(dir, Options{
				Codec:   native.New(),
				Version: "0.1.0",
				Now:     fixedNowInternal,
			})
			if aerr != nil {
				t.Fatalf("Analyze: %v", aerr)
			}
			if len(facts) != 1 {
				t.Fatalf("vacuous: Analyze returned %d facts, want 1 — the document under test "+
					"was not scanned", len(facts))
			}
			if f := facts[0]; len(f.ColonSpaceKeys) != 0 {
				t.Errorf("ColonSpaceKeys = %v for a file the codec parsed cleanly; the advisory "+
					"would fire with no invalid-frontmatter violation to subsume it",
					f.ColonSpaceKeys)
			}
		})
	}
}

// TestColonSpaceCleanCorpusYieldsNoAdvisory drives the same guarantee through the
// real Analyze pipeline rather than the wiring in miniature, over a corpus fixture
// built from the reviewer's reproducer. It is the end-to-end half of the test
// above: whatever Analyze does internally, a corpus the codec parses must surface
// no ColonSpaceKeys at all.
func TestColonSpaceCleanCorpusYieldsNoAdvisory(t *testing.T) {
	_, facts, _, err := Analyze("../../testdata/corpus-lint-colonspace-clean", Options{
		Codec:   native.New(),
		Version: "0.1.0",
		Now:     fixedNowInternal,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("vacuous: the fixture corpus produced no facts")
	}
	var witnessed int
	for _, f := range facts {
		if f.Recovered {
			t.Errorf("%s: fixture is supposed to PARSE cleanly but convert recovered it (%s)",
				f.RelPath, f.RecoverErr)
		}
		if len(f.ColonSpaceKeys) != 0 {
			t.Errorf("%s: ColonSpaceKeys = %v on a cleanly-parsing file", f.RelPath, f.ColonSpaceKeys)
		}
		// This test's subject is an ABSENCE, so confirm the fixture still witnesses
		// the hazard the gate exists to stop. If the files were ever "tidied" into
		// ordinary clean markdown, every assertion above would pass while testing
		// nothing.
		raw, rerr := os.ReadFile(filepath.Join("../../testdata/corpus-lint-colonspace-clean", f.RelPath))
		if rerr != nil {
			t.Fatalf("reading fixture %s: %v", f.RelPath, rerr)
		}
		norm, _ := NormalizeInput(raw)
		if len(colonSpaceKeys(norm)) > 0 {
			witnessed++
		}
	}
	if witnessed == 0 {
		t.Fatal("vacuous: no fixture in this corpus would draw a phantom finding if the gate " +
			"were removed, so it no longer tests the gate")
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
		var keys []string
		if perr != nil {
			keys = colonSpaceKeys(norm)
		}
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

// TestFrontmatterRegionAgreesWithCodec pins frontmatterRegion to the codec's
// fence rules (O5). The two implementations live in different packages —
// native.splitFrontmatter is unexported, so it cannot simply be reused — and a
// silent disagreement about where frontmatter STOPS would mean the advisory
// scanning text the codec never parsed, or missing text it did.
//
// The codec's region is read back from Concept.OriginalFrontmatter, which
// splitFrontmatter populates verbatim, so this compares the real outputs rather
// than two restatements of the same rule.
func TestFrontmatterRegionAgreesWithCodec(t *testing.T) {
	docs := []struct{ name, doc string }{
		{"ordinary", "---\ntype: Note\ntitle: X\n---\n\n# X\n"},
		{"empty frontmatter", "---\n---\n\n# X\n"},
		{"CRLF", "---\r\ntype: Note\r\n---\r\n\r\n# X\r\n"},
		{"nested and sequences", "---\na:\n  b: c\nl:\n  - 1\n  - 2\n---\n\n# X\n"},
		{"body contains a --- thematic break", "---\ntype: Note\n---\n\n# X\n\n---\n\nmore\n"},
		{"no closing fence", "---\ntype: Note\n\n# X\n"},
		{"no fence at all", "# X\n\nprose\n"},
		{"bare --- only", "---"},
		{"fence then EOF", "---\ntype: Note\n---"},
		{"block scalar inside", "---\ns: |\n  a: b\n---\n\n# X\n"},
	}
	var compared int
	for _, d := range docs {
		t.Run(d.name, func(t *testing.T) {
			norm, _ := NormalizeInput([]byte(d.doc))
			got, ok := frontmatterRegion(strings.ReplaceAll(string(norm), "\r\n", "\n"))

			con, err := native.New().ParseConcept("x.md", norm)
			// The codec also rejects a frontmatter block that is not a mapping or does
			// not parse; those are parse verdicts, not fence verdicts, so only fence
			// errors are compared here.
			fenceErr := err != nil && (strings.Contains(err.Error(), "missing frontmatter") ||
				strings.Contains(err.Error(), "unterminated"))
			if ok == fenceErr {
				t.Fatalf("fence verdicts disagree: frontmatterRegion ok=%v, codec fence error=%v (err=%v)",
					ok, fenceErr, err)
			}
			if !ok {
				return
			}
			compared++
			want := strings.TrimRight(string(con.OriginalFrontmatter), "\n")
			if got != want {
				t.Errorf("region mismatch:\n frontmatterRegion: %q\n codec:             %q", got, want)
			}
		})
	}
	// Most cases above exit at the fence-verdict check; without this, a change that
	// made every document fenceless would leave the region comparison unexercised
	// and the test still green.
	if compared < 5 {
		t.Fatalf("vacuous: only %d document(s) reached the region comparison", compared)
	}
}
