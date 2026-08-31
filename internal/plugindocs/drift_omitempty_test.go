package plugindocs

import "testing"

// TestRegisterElem_OmitemptyVarianceIsNotDrift is the permanent NEGATIVE FIXTURE
// for #172: the drift gate must NOT over-report a faithful MULTI-element array
// capture whose non-first element legitimately differs in key set via
// `omitempty`.
//
// enrich FileResult has mandatory `path`/`status` and omitempty
// `added`/`overwritten`/`reason`/`normalized` (internal/enrich.FileResult). A
// live run can therefore emit element 0 with `added` (an additive file) and
// element 1 with `reason` (a skipped file) — same struct, different JSON key set.
//
// Before the fix, registerElem indexed element ZERO only, so element 1 was keyed
// against {path,status,added} and wrongly flagged MISSING added / NOT-IN-BINARY
// reason. After the fix registerElem folds the UNION (allowed) and INTERSECTION
// (required) of all live elements, so `added`/`reason` are optional (in allowed,
// absent from required) while `path`/`status` stay mandatory — no drift.
func TestRegisterElem_OmitemptyVarianceIsNotDrift(t *testing.T) {
	idx := shapeIndex{}
	// Live output whose two elements differ by omitempty exactly as the binary
	// can: element 0 additive (added), element 1 skipped (reason).
	idx.registerElem("enrich.result.files[]", []any{
		map[string]any{"path": "adr/one.md", "status": "would-enrich", "added": []any{"title"}},
		map[string]any{"path": "adr/two.md", "status": "skipped", "reason": "unparseable frontmatter"},
	})

	// A faithful doc capture with the same legitimate per-element variance.
	doc := []any{
		map[string]any{"path": "adr/one.md", "status": "would-enrich", "added": []any{"title"}},
		map[string]any{"path": "adr/two.md", "status": "skipped", "reason": "unparseable frontmatter"},
	}
	var findings []finding
	var cov []coverage
	descend(doc, "$.files", arr(aEnrichFiles), idx, "test.md", 1, &findings, &cov)

	if len(findings) != 0 {
		t.Fatalf("omitempty variance across live elements must not be drift, got findings:\n%v", findings)
	}
}

// TestRegisterElem_UnionStillCatchesRealDrift is the permanent ANTI-INVERSION
// guard for #172. The union relaxation makes omitempty-varying keys optional, but
// it must not blunt real detection: a REQUIRED key (present in every live
// element) omitted by a doc element, and a key NO live element emits, must still
// be flagged — including on a NON-FIRST element, precisely the case the union
// change makes more permissive.
func TestRegisterElem_UnionStillCatchesRealDrift(t *testing.T) {
	idx := shapeIndex{}
	idx.registerElem("enrich.result.files[]", []any{
		map[string]any{"path": "adr/one.md", "status": "would-enrich", "added": []any{"title"}},
		map[string]any{"path": "adr/two.md", "status": "skipped", "reason": "x"},
	})
	// Non-first doc element drops required `status` (MISSING) and carries a key no
	// live element emits (`bogus`, NOT-IN-BINARY). The optional `added`/`reason`
	// variance must NOT contribute findings.
	doc := []any{
		map[string]any{"path": "adr/one.md", "status": "would-enrich", "added": []any{"title"}},
		map[string]any{"path": "adr/two.md", "reason": "x", "bogus": true},
	}
	var findings []finding
	var cov []coverage
	descend(doc, "$.files", arr(aEnrichFiles), idx, "test.md", 1, &findings, &cov)

	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding on files[1], got %d: %v", len(findings), findings)
	}
	f := findings[0]
	if got := f.missing; len(got) != 1 || got[0] != "status" {
		t.Errorf("expected MISSING-FROM-DOC=[status], got %v", got)
	}
	if got := f.extra; len(got) != 1 || got[0] != "bogus" {
		t.Errorf("expected NOT-IN-BINARY=[bogus], got %v", got)
	}
}
