package convert

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestResolveStatusVocabularyConformant: every legal §5.4 value passes clean —
// no notes, no warnings, and the map/default are returned unchanged.
func TestResolveStatusVocabularyConformant(t *testing.T) {
	for _, canon := range []bool{false, true} {
		prefixes := map[string]string{"archive": "deprecated", "drafts": "draft"}
		out, def, res := ResolveStatusVocabulary(prefixes, "stable", canon)
		if len(res.Notes) != 0 || len(res.Warnings) != 0 {
			t.Fatalf("canon=%v conformant produced notes=%v warnings=%v", canon, res.Notes, res.Warnings)
		}
		if res.NonConformant() {
			t.Fatalf("canon=%v conformant reported NonConformant", canon)
		}
		if def != "stable" || !reflect.DeepEqual(out, prefixes) {
			t.Fatalf("canon=%v conformant changed values: def=%q out=%v", canon, def, out)
		}
	}
}

// TestResolveStatusVocabularyDefaultPathWarnsNoRewrite: on the default path a
// non-conformant value is warned about, named with its key and the legal set,
// cites §5.4, and is passed through UNCHANGED (binder never guesses intent).
func TestResolveStatusVocabularyDefaultPathWarnsNoRewrite(t *testing.T) {
	prefixes := map[string]string{"archive": "active"}
	out, def, res := ResolveStatusVocabulary(prefixes, "", false)

	if !res.NonConformant() || len(res.Warnings) != 1 {
		t.Fatalf("want 1 non-conformance warning, got %v", res.Warnings)
	}
	w := res.Warnings[0]
	for _, want := range []string{`"active"`, `"archive"`, "draft|stable|deprecated", "§5.4", "wrote it unchanged"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning %q missing %q", w, want)
		}
	}
	// Known alias on the default path points the user at the opt-in flag.
	if !strings.Contains(w, "--canonicalize-status") || !strings.Contains(w, `"stable"`) {
		t.Errorf("warning should hint at --canonicalize-status and target: %q", w)
	}
	// Value passed through unchanged; caller's map not mutated.
	if def != "" || out["archive"] != "active" {
		t.Fatalf("value rewritten on default path: out=%v def=%q", out, def)
	}
}

// TestResolveStatusVocabularyCanonicalizeEachAlias: with canonicalization ON,
// every listed alias maps to exactly its target, a rewrite note is produced, and
// nothing remains non-conformant.
func TestResolveStatusVocabularyCanonicalizeEachAlias(t *testing.T) {
	cases := map[string]string{
		"active":      "stable",
		"wip":         "draft",
		"in-progress": "draft",
		"archived":    "deprecated",
		"legacy":      "deprecated",
	}
	for alias, want := range cases {
		prefixes := map[string]string{"dir": alias}
		out, _, res := ResolveStatusVocabulary(prefixes, "", true)
		if res.NonConformant() {
			t.Errorf("alias %q still non-conformant after canonicalize", alias)
		}
		if out["dir"] != want {
			t.Errorf("alias %q canonicalized to %q, want %q", alias, out["dir"], want)
		}
		if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "canonicalized") ||
			!strings.Contains(res.Notes[0], `"`+want+`"`) {
			t.Errorf("alias %q rewrite note = %v", alias, res.Notes)
		}
		// Input map must be untouched (copy-on-write).
		if prefixes["dir"] != alias {
			t.Errorf("alias %q: input map mutated to %q", alias, prefixes["dir"])
		}
	}
}

// TestResolveStatusVocabularyCanonicalizeUnknownStillWarns: an out-of-table value
// is NOT an alias — even with canonicalization on it stays a non-conformance
// warning and is never rewritten (criterion 6).
func TestResolveStatusVocabularyCanonicalizeUnknownStillWarns(t *testing.T) {
	prefixes := map[string]string{"dir": "experimental"}
	out, _, res := ResolveStatusVocabulary(prefixes, "", true)
	if !res.NonConformant() || len(res.Warnings) != 1 {
		t.Fatalf("unknown value should warn: %v", res.Warnings)
	}
	if out["dir"] != "experimental" {
		t.Fatalf("unknown value rewritten: %q", out["dir"])
	}
	// With canonicalize ON we do not dangle the --canonicalize-status hint.
	if strings.Contains(res.Warnings[0], "--canonicalize-status") {
		t.Errorf("hint should not appear when canonicalize is already on: %q", res.Warnings[0])
	}
}

// TestResolveStatusVocabularyDefaultKeyAttribution: the special default= value is
// attributed to the key "default" in its message.
func TestResolveStatusVocabularyDefaultKeyAttribution(t *testing.T) {
	_, _, res := ResolveStatusVocabulary(nil, "active", false)
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], `"default"`) {
		t.Fatalf("default= value not attributed to key \"default\": %v", res.Warnings)
	}
}

// TestResolveStatusVocabularyDeterministicOrder: notes are sorted, so the same
// input yields byte-identical report output regardless of map iteration order.
func TestResolveStatusVocabularyDeterministicOrder(t *testing.T) {
	prefixes := map[string]string{"zeta": "active", "alpha": "wip", "mid": "bogus"}
	_, _, res := ResolveStatusVocabulary(prefixes, "legacy", true)
	if !sort.StringsAreSorted(res.Notes) {
		t.Errorf("notes not sorted: %v", res.Notes)
	}
	if !sort.StringsAreSorted(res.Warnings) {
		t.Errorf("warnings not sorted: %v", res.Warnings)
	}
	// Notes = rewrites ∪ warnings.
	if len(res.Notes) != len(res.Warnings)+3 { // zeta,alpha,legacy rewritten; mid warns
		t.Errorf("notes=%v warnings=%v (unexpected partition)", res.Notes, res.Warnings)
	}
}
