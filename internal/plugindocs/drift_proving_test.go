package plugindocs

import (
	"strings"
	"testing"
)

// These tests prove the gate itself. A gate nobody proved is exactly how the
// #106 drift survived four minor versions: the audit's first instrument compared
// only top-level keys and anchored fences at column 0, and the first gate then
// matched objects by fuzzy key-overlap and SKIPPED anything below a threshold —
// so the worse a block drifted, the more likely it was skipped silently, and a
// concept stripped to four keys was MISATTRIBUTED to graph.nodes[] with a bogus
// "missing title". Every case below feeds a realistic block (the shape as it
// appears in the contract doc) drifted the way that used to escape, and asserts
// the path-anchored gate now catches it, names the RIGHT shape, and stays silent
// on correct docs and on free-form data maps.
//
// "Fail before, pass after": run against the previous gate (commit a43b60e,
//   git checkout a43b60e -- internal/plugindocs/drift_test.go)
// the values-1of6, values-renamed and concept-1of7 blocks below produce NO
// finding (silent) and the concept-4of7 block produces a finding MISATTRIBUTED
// to graph.nodes[] naming a missing "title". Under this gate they are all
// flagged against the correct shape.

// --- realistic drifted fixtures (as the block appears in the docs) ----------

const fxValues1of6 = "```json\n" + `{
  "binder": "binder/0.5.1", "command": "config", "schema": "binder.config/v1",
  "result": { "config_file": "", "values": {
    "default_type": { "value": "Note", "source": "default" }
  } }
}` + "\n```\n"

const fxValuesRenamed = "```json\n" + `{
  "binder": "binder/0.5.1", "command": "config", "schema": "binder.config/v1",
  "result": { "config_file": "", "values": {
    "a1": { "value": "x", "source": "default" },
    "a2": { "value": "x", "source": "default" },
    "a3": { "value": "x", "source": "default" },
    "a4": { "value": "x", "source": "default" },
    "a5": { "value": "x", "source": "default" },
    "a6": { "value": "x", "source": "default" }
  } }
}` + "\n```\n"

// The founding defect (instance a) verbatim: values keeping default_type +
// verified_by, missing the four gemini_* keys.
const fxValues2of6 = "```json\n" + `{
  "binder": "binder/0.5.1", "command": "config", "schema": "binder.config/v1",
  "result": { "config_file": "", "values": {
    "default_type": { "value": "Note",        "source": "default" },
    "verified_by":  { "value": "human:alice", "source": "file"    }
  } }
}` + "\n```\n"

func reviewWithConcept(concept string) string {
	return "```json\n" + `{ "root": "bundle", "today": "2026-08-15", "num_concepts": 1,
  "by_type": { "Note": 1 }, "tiers": { "unverified": 1 },
  "orphans": [], "entrypoints": [], "stale": [], "attested": [],
  "unresolved": [], "unparsed_frontmatter": [],
  "concepts": [ ` + concept + ` ] }` + "\n```\n"
}

// --- helpers ----------------------------------------------------------------

func scanFix(t *testing.T, doc string) ([]finding, []coverage) {
	t.Helper()
	idx := buildLiveIndex(t, repoRoot(t))
	return scanDoc("fixture.md", doc, idx)
}

func mentions(fs []finding, pathSub, key string) bool {
	for _, f := range fs {
		if strings.Contains(f.path, pathSub) {
			for _, m := range f.missing {
				if m == key {
					return true
				}
			}
		}
	}
	return false
}

func flaggedShape(fs []finding, pathSub string) string {
	for _, f := range fs {
		if strings.Contains(f.path, pathSub) {
			return f.shape
		}
	}
	return ""
}

func anyMissingOrExtra(fs []finding, key string) bool {
	for _, f := range fs {
		for _, m := range f.missing {
			if m == key {
				return true
			}
		}
		for _, e := range f.extra {
			if e == key {
				return true
			}
		}
	}
	return false
}

func render(fs []finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.String() + "\n")
	}
	return b.String()
}

func anyUnanchored(cov []coverage, pathSub string) bool {
	for _, c := range cov {
		if c.unanchored() && strings.Contains(c.path, pathSub) {
			return true
		}
	}
	return false
}

// --- REQUIREMENT 1: negative fixtures for the cases that used to go silent ---

// Catastrophic subset: values keeping 1 of 6. Old gate: jaccard 0.167 -> SILENT.
func TestGate_FlagsCatastrophicValues1of6(t *testing.T) {
	fs, _ := scanFix(t, fxValues1of6)
	if !mentions(fs, ".values", "gemini_backend") {
		t.Fatalf("gate did not flag values missing gemini_* at 1-of-6; got:\n%s", render(fs))
	}
	if s := flaggedShape(fs, ".values"); s != "config.result.values" {
		t.Fatalf("values flagged against wrong shape %q, want config.result.values", s)
	}
}

// Total rename: all 6 keys renamed. Old gate: jaccard 0 -> SILENT.
func TestGate_FlagsRenamedValues(t *testing.T) {
	fs, _ := scanFix(t, fxValuesRenamed)
	if !mentions(fs, ".values", "gemini_project") || !mentions(fs, ".values", "default_type") {
		t.Fatalf("gate did not flag renamed values as missing the real keys; got:\n%s", render(fs))
	}
	if s := flaggedShape(fs, ".values"); s != "config.result.values" {
		t.Fatalf("renamed values flagged against wrong shape %q", s)
	}
}

// Catastrophic subset: concept keeping 1 of 7. Old gate: jaccard 0.143 -> SILENT.
func TestGate_FlagsCatastrophicConcept1of7(t *testing.T) {
	fs, _ := scanFix(t, reviewWithConcept(`{ "id": "README" }`))
	if !mentions(fs, "concepts[", "entrypoint") {
		t.Fatalf("gate did not flag concept missing keys at 1-of-7; got:\n%s", render(fs))
	}
	if s := flaggedShape(fs, "concepts["); s != "review.result.concepts[]" {
		t.Fatalf("concept flagged against wrong shape %q", s)
	}
}

// Misattribution case: concept keeping 4 of 7 {id,type,tier,stale}. Old gate:
// matched graph.nodes[] (0.80) and reported a bogus missing "title". This gate
// must name review.result.concepts[] and must NOT mention title.
func TestGate_FlagsConcept4of7_NoMisattribution(t *testing.T) {
	fs, _ := scanFix(t, reviewWithConcept(`{ "id": "README", "type": "Note", "tier": "unverified", "stale": false }`))
	if s := flaggedShape(fs, "concepts["); s != "review.result.concepts[]" {
		t.Fatalf("concept 4-of-7 flagged against wrong shape %q (misattribution not fixed); got:\n%s", s, render(fs))
	}
	for _, want := range []string{"attested", "entrypoint", "orphan"} {
		if !mentions(fs, "concepts[", want) {
			t.Fatalf("concept 4-of-7 missing key %q not reported; got:\n%s", want, render(fs))
		}
	}
	if anyMissingOrExtra(fs, "title") {
		t.Fatalf("gate still reports a bogus 'title' key (graph.nodes[] misattribution); got:\n%s", render(fs))
	}
}

// Finding 1 (round 3): completeness used to recurse into array element v[0]
// ONLY, so a drifted or unanchored NON-FIRST element was both silent AND
// unreported — the same "zero exposure today" reasoning that hid the Jaccard
// floor, reintroduced one layer down. This block's concepts[0] is verbatim
// correct and concepts[1] is stripped to {id}. Against the previous descend
// (git checkout <pre-fix> -- internal/plugindocs/drift_test.go, the case that
// walked only v[0]) it produces NO finding and one coverage entry. Under the
// all-elements descent it flags concepts[1] against review.result.concepts[]
// naming the missing keys, and records coverage for BOTH elements.
func TestGate_FlagsDriftedNonFirstArrayElement(t *testing.T) {
	correct := `{ "id": "README", "type": "Note", "tier": "unverified",
	              "stale": false, "attested": false, "orphan": false,
	              "entrypoint": true }`
	fs, cov := scanFix(t, reviewWithConcept(correct+`, { "id": "orphan" }`))
	// The FIRST element is correct; the finding must be on the SECOND.
	if !mentions(fs, "concepts[1]", "entrypoint") {
		t.Fatalf("gate did not flag the drifted non-first concept element; got:\n%s", render(fs))
	}
	if s := flaggedShape(fs, "concepts[1]"); s != "review.result.concepts[]" {
		t.Fatalf("non-first concept flagged against wrong shape %q", s)
	}
	// Both elements must appear in coverage — the non-first one is no longer skipped.
	var saw0, saw1 bool
	for _, c := range cov {
		if strings.Contains(c.path, "concepts[0]") {
			saw0 = true
		}
		if strings.Contains(c.path, "concepts[1]") {
			saw1 = true
		}
	}
	if !saw0 || !saw1 {
		t.Fatalf("expected coverage for both concepts[0] and concepts[1]; coverage:\n%s", renderCov(cov))
	}
}

// --- REQUIREMENT 2: the N=7 projection, locked down ------------------------

// If the values shape grows by one key (e.g. a new gemini_* setting), the
// founding defect (instance a, values keeping 2 of 6) scored 2/7=0.286 under the
// old gate and went SILENT. Path-anchoring checks the values object by position,
// so a growing shape cannot silence it: the drift is still flagged against
// config.result.values with the four gemini_* keys named.
func TestGate_FoundingDefectSurvivesShapeGrowth(t *testing.T) {
	idx := buildLiveIndex(t, repoRoot(t))
	grown := shapeIndex{}
	for k, v := range idx {
		grown[k] = v
	}
	// config.result.values is a single-object value-MAP schema, so its live
	// shape has required == allowed; grow both by one key to simulate N=7.
	vals := map[string]bool{"gemini_new_setting": true}
	for k := range idx["config.result.values"].allowed {
		vals[k] = true
	}
	grown["config.result.values"] = liveShape{required: vals, allowed: vals} // now N=7

	fs, _ := scanDoc("fixture.md", fxValues2of6, grown)
	for _, want := range []string{"gemini_backend", "gemini_location", "gemini_model", "gemini_project"} {
		if !mentions(fs, ".values", want) {
			t.Fatalf("founding defect went silent under grown (N=7) shape; %q not reported:\n%s", want, render(fs))
		}
	}
}

// --- REQUIREMENT 3: the completeness assertion itself works -----------------

// A nested object at a path no anchor describes must be reported UNANCHORED, not
// silently skipped — this is the anchoring-era analogue of the old low-overlap
// escape hatch and is what TestPluginDocs_EveryBlockAnchored relies on.
func TestCompleteness_FlagsUnanchoredNestedObject(t *testing.T) {
	// A convert result payload with a stray nested object field.
	doc := "```json\n" + `{ "src": "c", "out": "b", "concepts": [], "unresolved": [],
  "num_concepts": 0, "num_links": 0, "num_resolved": 0, "num_unresolved": 0,
  "num_recovered": 0, "dry_run": true, "status_notes": [], "warnings": [],
  "verified": { "actor": "", "source": "none", "stamped": [], "num_stamped": 0,
                "skipped": [], "num_skipped": 0 },
  "surprise": { "unexpected": true } }` + "\n```\n"
	_, cov := scanFix(t, doc)
	if !anyUnanchored(cov, ".surprise") {
		t.Fatalf("completeness did not flag the unanchored nested object $.surprise; coverage:\n%s", renderCov(cov))
	}
}

// A block whose root matches no known shape must be reported UNANCHORED, never
// silently accepted.
func TestCompleteness_FlagsUnroutableBlock(t *testing.T) {
	doc := "```json\n" + `{ "totally": 1, "alien": 2, "keys": 3 }` + "\n```\n"
	_, cov := scanFix(t, doc)
	if !anyUnanchored(cov, "$") {
		t.Fatalf("completeness did not flag the unroutable block; coverage:\n%s", renderCov(cov))
	}
}

func renderCov(cov []coverage) string {
	var b strings.Builder
	for _, c := range cov {
		b.WriteString(c.path + " [" + c.status + "]\n")
	}
	return b.String()
}

// --- silence on correct docs and free-form data ----------------------------

// A verbatim-correct config block must not fire.
func TestGate_SilentOnCleanConfig(t *testing.T) {
	doc := "```json\n" + `{
  "binder": "binder/0.5.1", "command": "config", "schema": "binder.config/v1",
  "result": { "config_file": "", "values": {
    "default_type":    { "value": "Note",                  "source": "default" },
    "gemini_backend":  { "value": "auto",                  "source": "default" },
    "gemini_location": { "value": "global",                "source": "default" },
    "gemini_model":    { "value": "gemini-3.5-flash-lite", "source": "default" },
    "gemini_project":  { "value": "",                      "source": "default" },
    "verified_by":     { "value": "human:alice",           "source": "file"    }
  } }
}` + "\n```\n"
	fs, cov := scanFix(t, doc)
	if len(fs) != 0 {
		t.Fatalf("gate fired on a verbatim-correct config block:\n%s", render(fs))
	}
	if anyUnanchored(cov, "$") {
		t.Fatalf("clean config block reported unanchored objects:\n%s", renderCov(cov))
	}
}

// Free-form data maps (by_type, tiers) whose keys are arbitrary data must be
// exempt by PATH, not by content: even with keys that never appear in the live
// corpus, they must produce no finding and be recorded as exempt.
func TestGate_SilentOnFreeFormMap(t *testing.T) {
	doc := "```json\n" + `{ "root": "bundle", "today": "2026-08-15", "num_concepts": 1,
  "by_type": { "Wibble": 9, "Wobble": 4 }, "tiers": { "invented": 7 },
  "orphans": [], "entrypoints": [], "stale": [], "attested": [],
  "unresolved": [], "unparsed_frontmatter": [],
  "concepts": [ { "id": "README", "type": "Note", "tier": "unverified",
                  "stale": false, "attested": false, "orphan": false,
                  "entrypoint": true } ] }` + "\n```\n"
	fs, cov := scanFix(t, doc)
	if len(fs) != 0 {
		t.Fatalf("gate fired on a correct review block with free-form maps:\n%s", render(fs))
	}
	if !covHasExempt(cov, ".by_type") || !covHasExempt(cov, ".tiers") {
		t.Fatalf("by_type/tiers were not recorded as exempt; coverage:\n%s", renderCov(cov))
	}
}

func covHasExempt(cov []coverage, pathSub string) bool {
	for _, c := range cov {
		if c.status == "exempt" && strings.Contains(c.path, pathSub) {
			return true
		}
	}
	return false
}
