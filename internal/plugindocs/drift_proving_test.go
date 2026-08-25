package plugindocs

import (
	"strings"
	"testing"
)

// These tests prove the gate itself. A gate nobody proved is exactly how the
// #106 drift survived four minor versions: the audit's own first instrument
// compared only top-level keys and anchored fences at column 0, so it produced
// a false CLEAN — the drift sat one level down, and one fence was indented
// inside a list item. Each case below is built from a REAL captured 0.5.1 shape
// and then deliberately drifted (or left verbatim), so a pass here means the
// gate flags the #106 class at depth and through an indent, and stays silent on
// correct docs.

// provingIndex is the minimal live shape index the proving cases match against,
// captured verbatim from binder/0.5.1 over assets/sample-corpus/. Building it by
// hand (rather than via buildLiveIndex) keeps these tests fast and independent
// of the CLI while exercising the exact matcher the gate uses.
func provingIndex() shapeIndex {
	set := func(keys ...string) map[string]bool {
		m := make(map[string]bool, len(keys))
		for _, k := range keys {
			m[k] = true
		}
		return m
	}
	return shapeIndex{
		// config --json .result.values.<k> and the values map itself (instance a).
		"config.result.values":     set("default_type", "gemini_backend", "gemini_location", "gemini_model", "gemini_project", "verified_by"),
		"config.result.values.<k>": set("value", "source"),
		// review --json per-concept object (instance b).
		"review.result.concepts[]": set("attested", "entrypoint", "id", "orphan", "stale", "tier", "type"),
		// convert/enrich .result.verified (instance c).
		"result.verified": set("actor", "source", "stamped", "num_stamped", "skipped", "num_skipped"),
	}
}

// TestGate_FlagsDriftedNested proves the gate catches drift one level down. The
// block's TOP level (a report envelope) is correct; the drift is nested in
// .result.concepts[] — the review concept object with entrypoint deleted. This
// is instance (b) re-planted, and it is the shape a top-level-only checker walks
// straight past.
func TestGate_FlagsDriftedNested(t *testing.T) {
	doc := "```json\n" + `{
  "binder": "binder/0.5.1", "command": "review", "schema": "binder.report/v1",
  "result": {
    "concepts": [ { "id": "docs/guide", "type": "Guide", "tier": "unverified",
                    "stale": false, "attested": false, "orphan": true } ]
  }
}` + "\n```\n"

	findings := scanText("nested.md", doc, provingIndex())
	if len(findings) == 0 {
		t.Fatal("gate did not flag a nested drifted block (review concept missing 'entrypoint')")
	}
	if !mentions(findings, "concepts[]", "entrypoint") {
		t.Fatalf("gate flagged the wrong thing; want concepts[] missing entrypoint, got:\n%s", render(findings))
	}
}

// TestGate_FlagsDriftedIndented proves the gate sees fences indented inside a
// markdown list item — the second blind spot the audit's first instrument had.
// The block is instance (a) re-planted (config values missing the four gemini_*
// keys), indented four spaces under a list bullet.
func TestGate_FlagsDriftedIndented(t *testing.T) {
	doc := "- Example config output:\n\n" +
		"    ```jsonc\n" +
		"    {\n" +
		"      \"config_file\": \"\",\n" +
		"      \"values\": {\n" +
		"        \"default_type\": { \"value\": \"Note\",        \"source\": \"default\" },\n" +
		"        \"verified_by\":  { \"value\": \"human:alice\", \"source\": \"file\"    }\n" +
		"      }\n" +
		"    }\n" +
		"    ```\n"

	findings := scanText("indented.md", doc, provingIndex())
	if len(findings) == 0 {
		t.Fatal("gate did not flag an INDENTED drifted block (config values missing gemini_* keys)")
	}
	if !mentions(findings, "values", "gemini_backend") {
		t.Fatalf("gate flagged the wrong thing; want values missing gemini_*, got:\n%s", render(findings))
	}
}

// TestGate_FlagsDriftedVerified re-plants instance (c): .result.verified missing
// the stamped[] key.
func TestGate_FlagsDriftedVerified(t *testing.T) {
	doc := "```json\n" + `{ "actor": "human:you", "source": "config", "num_stamped": 3,
  "skipped": [], "num_skipped": 0 }` + "\n```\n"

	findings := scanText("verified.md", doc, provingIndex())
	if !missesKey(findings, "stamped") {
		t.Fatalf("gate did not flag verified missing 'stamped'; got:\n%s", render(findings))
	}
}

// TestGate_FlagsRetiredKey proves the gate also catches a key the doc carries
// that the binary does NOT emit (a retired-key drift), reported as NOT-IN-BINARY.
func TestGate_FlagsRetiredKey(t *testing.T) {
	doc := "```json\n" + `{ "actor": "", "source": "none", "stamped": [], "num_stamped": 0,
  "skipped": [], "num_skipped": 0, "num_recovered": 7 }` + "\n```\n"

	findings := scanText("retired.md", doc, provingIndex())
	got := render(findings)
	if !strings.Contains(got, "num_recovered") || !strings.Contains(got, "NOT-IN-BINARY") {
		t.Fatalf("gate did not flag the retired key num_recovered as NOT-IN-BINARY; got:\n%s", got)
	}
}

// TestGate_SilentOnCleanVerbatim proves the gate does NOT fire on a
// verbatim-correct block: the review concept object with all seven live keys.
// A gate that cried wolf on correct docs would be turned off.
func TestGate_SilentOnCleanVerbatim(t *testing.T) {
	doc := "```jsonc\n" + `{
  "concepts": [ { "id": "README", "type": "Note", "tier": "unverified",
                  "stale": false, "attested": false, "orphan": false,
                  "entrypoint": true } ]      // reachable root
}` + "\n```\n"

	findings := scanText("clean.md", doc, provingIndex())
	if len(findings) != 0 {
		t.Fatalf("gate fired on a verbatim-correct block:\n%s", render(findings))
	}
}

// TestGate_SilentOnFreeFormMap proves the gate does not false-positive on a
// free-form data map (keys are data, not schema) such as review.result.by_type.
func TestGate_SilentOnFreeFormMap(t *testing.T) {
	doc := "```json\n" + `{ "by_type": { "Guide": 2, "Note": 1, "Decision": 3 },
  "tiers": { "unverified": 3 } }` + "\n```\n"

	findings := scanText("freeform.md", doc, provingIndex())
	if len(findings) != 0 {
		t.Fatalf("gate false-positived on a free-form data map:\n%s", render(findings))
	}
}

func missesKey(fs []finding, key string) bool {
	for _, f := range fs {
		for _, m := range f.missing {
			if m == key {
				return true
			}
		}
	}
	return false
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

func render(fs []finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.String() + "\n")
	}
	return b.String()
}
