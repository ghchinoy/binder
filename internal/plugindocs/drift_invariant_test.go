package plugindocs

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/enrich"
	"github.com/ghchinoy/binder/internal/graph"
	"github.com/ghchinoy/binder/internal/infer"
	"github.com/ghchinoy/binder/internal/lint"
	"github.com/ghchinoy/binder/internal/okf"
	"github.com/ghchinoy/binder/internal/review"
)

// hasMandatoryJSONField reports whether t serializes AT LEAST ONE JSON key that
// is always present — an exported field with no `,omitempty` (or with no json
// tag, in which case encoding/json emits it under its field name unconditionally).
// A `json:"-"` field is never serialized and does not count.
func hasMandatoryJSONField(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported: never serialized by encoding/json
			continue
		}
		tag, ok := f.Tag.Lookup("json")
		if !ok {
			return true // no json tag -> always emitted under the field name
		}
		parts := strings.Split(tag, ",")
		if parts[0] == "-" {
			continue // explicitly not serialized
		}
		omitempty := false
		for _, o := range parts[1:] {
			if o == "omitempty" {
				omitempty = true
			}
		}
		if !omitempty {
			return true
		}
	}
	return false
}

// jsonTagNames returns the set of JSON key names t serializes: the tag name for
// each exported field (the field name when the tag is absent or is just options
// like `,omitempty`), excluding `json:"-"`. omitempty is irrelevant here — we
// want every key the type COULD emit, so an observed live key can be checked
// against it.
func jsonTagNames(t reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		name := f.Name
		if tag, ok := f.Tag.Lookup("json"); ok {
			n := strings.Split(tag, ",")[0]
			if n == "-" {
				continue // never serialized
			}
			if n != "" {
				name = n // `json:",omitempty"` keeps the field name
			}
		}
		out[name] = true
	}
	return out
}

// collectArrayElemShapes walks the gate's OWN anchor forest and returns every
// shape reached as an ARRAY element. An arr(elem) wrapper is structurally
// distinct from a value-map: arr() produces an anchor with an empty shape and a
// non-nil elem, whereas a value-map (e.g. config.result.values) carries a
// non-empty shape alongside its elem. So "shape == \"\" && elem != nil" is
// exactly the set of array wrappers, and elem.shape is the element type the gate
// indexes there. This is the authoritative, self-updating list of the shapes
// registerElem covers.
func collectArrayElemShapes() map[string]bool {
	out := map[string]bool{}
	seen := map[*anchor]bool{}
	var walk func(a *anchor)
	walk = func(a *anchor) {
		if a == nil || seen[a] {
			return
		}
		seen[a] = true
		if a.shape == "" && a.elem != nil && a.elem.shape != "" {
			out[a.elem.shape] = true
		}
		for _, c := range a.fields {
			walk(c)
		}
		walk(a.elem)
	}
	roots := []*anchor{aConfigEnvelope, aReportEnvelope}
	for _, a := range bareRoots {
		roots = append(roots, a)
	}
	for _, a := range roots {
		walk(a)
	}
	return out
}

// TestRegisterElem_EveryIndexedElemTypeHasMandatoryField ENFORCES (not merely
// documents) the invariant registerElem's union model relies on: every
// array-element type the gate indexes has >=1 non-omitempty JSON field, so the
// `required` set (the intersection of live element key sets) is provably
// non-empty and the per-element drift check never degrades to extra-keys-only
// (#172 FYI-1). The regression this guards against — adding `,omitempty` to the
// last mandatory field of an element type — lands in internal/{enrich,graph,...},
// far from registerElem, so a comment there cannot catch it; this test does, at
// `make check`.
//
// The shape -> Go type mapping below is hand-authored BY NECESSITY: buildLiveIndex
// registers element shapes from live CLI output that has already round-tripped
// through encoding/json into map[string]any, so there is no direct runtime
// shape->type path. The hand-list is nonetheless guarded on BOTH axes so it
// cannot silently drift:
//
//   - KEYS — sub-test (2) cross-checks the map keys against the gate's own
//     array-element anchors (collectArrayElemShapes) BIDIRECTIONALLY: every
//     discovered anchor shape must be mapped (a new registerElem-indexed type
//     fails until added) AND every mapped shape must be a real anchor (a stale
//     entry, or discovery returning nothing, fails loudly).
//   - VALUES — sub-test (3) binds each mapped reflect.Type to its shape via LIVE
//     observed data: every JSON key the binary actually emits on that shape's
//     elements must exist in the mapped struct's JSON tag set (observed ⊆ tags).
//     A wrong type (e.g. graph.nodes[] -> graph.Edge{}) is caught because the
//     real element's keys are absent from the wrong struct's tags.
//
// So a wrong KEY and a wrong TYPE VALUE are both caught, and neither key nor
// value guard can pass by examining nothing (each has its own non-empty check).
// What is NOT machine-checked: that the mapped type is the specific struct the
// producing command literally uses — only that it is JSON-tag-compatible with the
// shape's live keys. Two structs with identical tag sets would be
// interchangeable here; that residual is inherent to a data-observed binding.
func TestRegisterElem_EveryIndexedElemTypeHasMandatoryField(t *testing.T) {
	elemGoType := map[string]reflect.Type{
		"enrich.result.files[]":           reflect.TypeOf(enrich.FileResult{}),
		"convert.result.concepts[]":       reflect.TypeOf(convert.ConceptReport{}),
		"convert.result.unresolved[]":     reflect.TypeOf(convert.UnresolvedLink{}),
		"review.result.concepts[]":        reflect.TypeOf(review.ConceptView{}),
		"review.result.unresolved[]":      reflect.TypeOf(review.Edge{}),
		"lint.result.broken_links[]":      reflect.TypeOf(lint.Finding{}),
		"lint.result.schema_violations[]": reflect.TypeOf(lint.Finding{}),
		"graph.nodes[]":                   reflect.TypeOf(graph.Node{}),
		"graph.edges[]":                   reflect.TypeOf(graph.Edge{}),
		"infer.result.mappings[]":         reflect.TypeOf(infer.Mapping{}),
		"validate.result.findings[]":      reflect.TypeOf(okf.Finding{}),
	}

	// (1) ENFORCEMENT: each indexed element type must keep >=1 mandatory field.
	for shape, typ := range elemGoType {
		if !hasMandatoryJSONField(typ) {
			t.Errorf("%s (%s) has NO non-omitempty JSON field: registerElem `required` "+
				"would be empty and the drift check would degrade to extra-keys-only for "+
				"this shape. Keep at least one mandatory (no `,omitempty`) field on the type.",
				shape, typ)
		}
	}

	// (2) DRIFT GUARD, BIDIRECTIONAL. The anchor cross-check must not be able to
	// pass by finding nothing: if collectArrayElemShapes() ever returned an empty
	// set (the anchor structure changes, or the arr-wrapper heuristic stops
	// matching), a one-way "every discovered shape is mapped" loop would iterate
	// zero times and pass GREEN with the guard silently dead — the same
	// discovery-finds-nothing-and-reports-success failure as #169.
	anchorShapes := collectArrayElemShapes()

	// (2a) discovery must find SOMETHING; an empty set means the walk is broken,
	// reported as its own diagnostic rather than as N unmatched map keys.
	if len(anchorShapes) == 0 {
		t.Fatalf("collectArrayElemShapes() returned no array-element shapes: the anchor " +
			"walk or the arr-wrapper heuristic (shape==\"\" && elem!=nil) is broken, so the " +
			"drift guard below is not actually checking anything. Fix discovery before trusting " +
			"this test.")
	}

	// (2b) forward: every shape the gate's anchors carry must be mapped, so a new
	// registerElem-indexed type cannot slip in unenforced.
	for shape := range anchorShapes {
		if _, ok := elemGoType[shape]; !ok {
			t.Errorf("gate anchors carry array-element shape %q with no Go type mapped in "+
				"this test: add it to elemGoType so the non-empty-`required` invariant is "+
				"enforced for it too.", shape)
		}
	}

	// (2c) reverse: every mapped shape must correspond to a real gate anchor. This
	// closes the empty-discovery hole for free (an empty anchorShapes leaves all
	// map keys unmatched → loud RED) and also catches a stale map entry for a
	// shape the gate no longer indexes, which the forward check alone misses.
	for shape := range elemGoType {
		if !anchorShapes[shape] {
			t.Errorf("elemGoType maps shape %q but no gate anchor carries it as an array "+
				"element: either the gate stopped indexing it (drop the stale entry) or "+
				"collectArrayElemShapes() no longer discovers it (discovery is broken).", shape)
		}
	}

	// (3) VALUE CHECK. 2a/2b/2c verify only the KEYS of elemGoType; the reflect.Type
	// VALUES — which struct's mandatory-field invariant sub-test (1) actually
	// enforces — go unchecked, so re-mapping a shape to the wrong struct (e.g.
	// graph.nodes[] -> graph.Edge{}) would pass. Bind the value to the shape via
	// LIVE OBSERVED DATA: every JSON key the binary actually emits on a shape's
	// array elements must exist in the mapped struct's JSON tag set. Under a
	// correct mapping this holds by construction — omitempty only ever OMITS keys,
	// so observed ⊆ tags; under a mis-map, the observed keys of the real element
	// are absent from the wrong struct's tags → RED. This makes (1) provably check
	// the right struct.
	idx := buildLiveIndex(t, repoRoot(t))
	for shape, typ := range elemGoType {
		observed := idx[shape].allowed

		// TRAP 1: a shape with no observed keys makes the subset check vacuously
		// true — a fourth check-that-examines-nothing on a branch whose whole
		// subject is that defect. Fail with its own diagnostic, as (2a) does.
		if len(observed) == 0 {
			t.Errorf("no live-observed keys for %s: buildLiveIndex registered nothing for it, so "+
				"the type-value check for %s is vacuous. The corpus must exercise this shape with a "+
				"non-empty array, or this mapping cannot be verified against live data.", shape, typ)
			continue
		}

		// TRAP 2: observed ⊆ tags — NOT equality, NOT the reverse. A struct
		// legitimately carries tags no live element populates; that is omitempty
		// working, not drift.
		tags := jsonTagNames(typ)
		for k := range observed {
			if !tags[k] {
				t.Errorf("live %s emits key %q that %s has no JSON tag for: elemGoType maps this "+
					"shape to the WRONG Go type, so sub-test (1) would enforce the mandatory-field "+
					"invariant against the wrong struct.", shape, k, typ)
			}
		}
	}
}
