// Package plugindocs holds the drift gate for the plugin skill docs under
// plugins/. It is the plugins/ analogue of internal/gendocs's byte-equality
// gate for docs/commands/: docs/commands/ is regenerated from the Cobra tree
// and pinned byte-for-byte, but plugins/ carried hand-copied JSON transcripts
// with no mechanical tie to the binary — which is exactly how the okf-convert
// contract drifted four minor versions (issue #106) while docs/ did not.
//
// # How the gate works: PATH-ANCHORING (not fuzzy matching)
//
//   - It runs the CLI IN-PROCESS over each plugin's own assets/sample-corpus/
//     and indexes the key set of every documented result shape from live output.
//   - It extracts every fenced json/jsonc block from plugins/**/*.md (globbed,
//     not hardcoded to okf-convert) — indented fences inside list items included.
//   - For each block it ROUTES the root to a known shape (an envelope by its
//     command/schema, or a bare result payload by key overlap), then DESCENDS
//     the object tree in lockstep with a declared anchor tree, mapping every
//     nested object to its live shape by its STRUCTURAL PATH — e.g. a values map
//     reached at $.result.values under a config envelope is always checked
//     against config.result.values, regardless of how badly that object itself
//     has drifted. At each anchored object it asserts KEY-SET EQUALITY (missing
//     or extra keys) against the live shape; illustrative values and jsonc
//     comments are ignored, so byte-equality is not imposed on the teaching text.
//
// # Why path-anchoring rather than best-overlap matching
//
// The first cut of this gate matched every object to its best-overlapping live
// shape and skipped anything below a Jaccard floor. That inverted the gate: the
// worse a block had drifted, the lower its overlap, so past the floor it was
// skipped SILENTLY — loudest on trivial drift, quiet on catastrophic drift. It
// also MISATTRIBUTED: a review concept stripped to {id,type,tier,stale} matched
// graph.nodes[] and reported a missing "title", a key concepts never had. Both
// failures were measured against the live binary (issue #106, round-2 review).
// Anchoring by path removes the fuzzy per-object step below the root: a
// catastrophically drifted object is still checked because we know WHAT it is
// from its position, and it can only ever be compared to its own live shape.
//
// # Completeness and exemptions (the anchoring escape hatch, closed)
//
// Path-anchoring moves the risk rather than removing it: an object at a path no
// anchor describes would go unchecked. So the gate is COMPLETE by construction —
// every JSON object under plugins/ must resolve to an anchor or to an explicit
// exemption, and anything that is neither is reported as UNANCHORED and fails
// TestPluginDocs_EveryBlockAnchored. The exemption list is deliberately tiny and
// each entry is commented with why it is free-form DATA (keys are values, not a
// schema) rather than a shape — an exemption is a fail-open surface, so adding to
// it must be a conscious, reviewed act.
//
// # KNOWN LIMIT — version literals are NOT checked
//
// The gate compares key SETS, not values, and in particular not the
// `binder/<version>` value. Two reasons, both structural:
//
//  1. An in-process test build is unstamped: it reports `binder/dev`, because
//     goreleaser injects the real tag via -ldflags only at release time. So
//     inside `go test` there is no trustworthy current version to compare a doc
//     literal against — a check would either misflag the correct release literal
//     or degrade to a no-op.
//  2. Prose version references cannot be blanket-checked. The contract docs
//     legitimately cite older versions (the minimum-version floor, historical
//     "as of binder/0.3.1" notes); telling those apart from a stale
//     capture-provenance claim is a semantic judgment, not a mechanical one.
//
// Version-literal drift in transcripts is therefore a known gap, tracked in #169
// rather than pretended-covered. It is caught today by the human sweep instrument
// (sweep-106-keydrift.py), which runs against a stamped release binary.
//
// This test is part of `make check` automatically: `make check` runs
// `go test ./...`, which includes this package.
package plugindocs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/cmd"
)

// --- fenced-block extraction ------------------------------------------------

// fenceRe matches a fenced ```json / ```jsonc / ```json5 block. It does NOT
// anchor the fence at column 0: fences are indented when they sit inside a
// markdown list item, and an indent-blind matcher never sees them (the second
// blind spot the #106 audit's first instrument had).
var fenceRe = regexp.MustCompile("(?ms)^[ \t]*```(?:json[c5]?)[ \t]*\r?\n(.*?)^[ \t]*```")

// trailingCommaRe removes JSONC trailing commas (,} and ,]) so encoding/json
// can parse the block after comments are stripped.
var trailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)

type block struct {
	line int
	body string
}

// extractBlocks returns every fenced json/jsonc block in text, with the 1-based
// line number of the opening fence for diagnostics.
func extractBlocks(text string) []block {
	var out []block
	for _, m := range fenceRe.FindAllStringSubmatchIndex(text, -1) {
		line := strings.Count(text[:m[0]], "\n") + 1
		out = append(out, block{line: line, body: text[m[2]:m[3]]})
	}
	return out
}

// stripJSONC removes // line comments and /* */ block comments that are not
// inside a string literal, then removes trailing commas. String-aware so a
// "https://..." value (or any "//" inside a string) survives — a naive
// regex-only strip would corrupt those.
func stripJSONC(s string) string {
	var b strings.Builder
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			b.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '/' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				b.WriteByte('\n')
			}
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++ // land on the closing '/'; the loop's i++ steps past it
			continue
		}
		b.WriteByte(c)
	}
	return trailingCommaRe.ReplaceAllString(b.String(), "$1")
}

// --- shape index ------------------------------------------------------------

// liveShape is the live key contract for one shape. `allowed` is the UNION of
// keys seen across the live sample(s); `required` is the keys present in EVERY
// live element (their INTERSECTION). For a single object — or a single-element
// array — required == allowed, so the check is exact key-set equality. Only a
// genuine MULTI-element live array can make required a proper subset of allowed,
// and only for the keys the live output itself shows vary (i.e. omitempty
// fields), which are then optional per element (#172).
type liveShape struct {
	required map[string]bool // present in every live element (must appear)
	allowed  map[string]bool // union across live elements (may appear)
}

// shapeIndex maps a shape name to the live key contract the binary emits for it.
type shapeIndex map[string]liveShape

func keySet(m map[string]any) map[string]bool {
	s := make(map[string]bool, len(m))
	for k := range m {
		s[k] = true
	}
	return s
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// register records the key set of obj under name. Empty objects are ignored:
// they carry no key contract to compare against (e.g. an envelope's "result":{}
// placeholder).
func (idx shapeIndex) register(name string, obj any) {
	if m, ok := obj.(map[string]any); ok && len(m) > 0 {
		ks := keySet(m)
		idx[name] = liveShape{required: ks, allowed: ks}
	}
}

// registerElem folds the key sets of ALL live elements of a JSON array into the
// indexed shape: `allowed` is their union, `required` is their intersection.
//
// A typed Go []T slice gives one anchor per array (all elements are the same
// struct), but NOT one JSON key set — a field tagged `omitempty` (e.g. enrich
// FileResult.added/reason, convert ConceptReport.normalized, graph Edge.text,
// infer Mapping.rationale) is dropped from any element whose value is the zero
// value. Indexing element ZERO only (the pre-#172 behavior) over-reported a
// faithful MULTI-element capture whose non-first element legitimately differed
// (#172): the doc-side descent walks every element (finding 1, #106), so element
// 1 was keyed against element 0's set and wrongly flagged.
//
// Folding every element instead lets the per-element check treat a key the live
// output itself shows varying (present in `allowed`, absent from `required`) as
// OPTIONAL, while a key present in EVERY live element stays MANDATORY and a key
// no live element emits is still caught. The optional set is DERIVED from live
// output, not guessed from struct tags. When the live array is single-element
// (every documented array today), required == allowed and the check is exactly
// the pre-#172 key-set equality — no detection is relaxed. Empty-object and
// scalar elements carry no key contract and are skipped, matching register.
//
// INVARIANT this relies on (#172 FYI-1): every array-element type the gate
// indexes has AT LEAST ONE non-omitempty JSON field, so across any live capture
// `required` (the intersection) is non-empty and the per-element check keeps its
// MISSING-key power. Verified structurally as of this writing — each type has a
// mandatory field with no `,omitempty`: enrich.FileResult{path,status},
// convert.ConceptReport{rel_path,type,title,num_links,num_unresolved},
// convert.UnresolvedLink{from,raw_target,text}, review.ConceptView{id,type,tier,
// stale,attested,orphan,entrypoint}, review.Edge{from,raw_target,text},
// graph.Node{id,title,type,tier,stale}, graph.Edge{from,to},
// infer.Mapping{dir,suggested_type,source}, okf.Finding{concept_id,severity,
// message} (validate findings[]), lint.Finding{concept,detail}. If a future
// element type is ever ALL-omitempty (or a multi-element live array shares no
// common key), `required` degrades to empty and this check weakens to
// extra-keys-only for that shape — so keep a mandatory field on these types.
func (idx shapeIndex) registerElem(name string, arr any) {
	a, ok := arr.([]any)
	if !ok {
		return
	}
	var allowed, required map[string]bool
	for _, e := range a {
		m, ok := e.(map[string]any)
		if !ok || len(m) == 0 {
			continue
		}
		ks := keySet(m)
		if allowed == nil {
			allowed, required = map[string]bool{}, ks
			for k := range ks {
				allowed[k] = true
			}
			continue
		}
		for k := range ks {
			allowed[k] = true
		}
		required = intersect(required, ks)
	}
	if allowed != nil {
		idx[name] = liveShape{required: required, allowed: allowed}
	}
}

// intersect returns the keys present in both a and b.
func intersect(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

// --- anchors ----------------------------------------------------------------

// anchor is a node in the expected-shape tree the gate descends in lockstep with
// a documented object. shape names the live shape to key-check this object
// against ("" = a structural node that is not itself key-checked, e.g. an
// envelope placeholder). fields maps a child key to its anchor; elem is the
// anchor for array ELEMENTS and, for a value-map like config.result.values, the
// anchor shared by every map child. exempt marks a free-form subtree: covered,
// but neither checked nor descended.
type anchor struct {
	shape  string
	exempt bool
	fields map[string]*anchor
	elem   *anchor
}

// arr wraps an array-valued object field: the field itself is not key-checked,
// its elements carry shape `elem`.
func arr(elem *anchor) *anchor { return &anchor{elem: elem} }

// exempt marks a free-form data map (keys are DATA, not a schema).
func exemptAnchor() *anchor { return &anchor{exempt: true} }

// Shared leaf/element anchors.
var (
	aVerified      = &anchor{shape: "result.verified"} // convert & enrich verified are identical
	aConfigValueK  = &anchor{shape: "config.result.values.<k>"}
	aConvConcept   = &anchor{shape: "convert.result.concepts[]"}
	aUnresolvedC   = &anchor{shape: "convert.result.unresolved[]"}
	aUnresolvedR   = &anchor{shape: "review.result.unresolved[]"}
	aFindings      = &anchor{shape: "validate.result.findings[]"}
	aReviewConcept = &anchor{shape: "review.result.concepts[]"}
	aBrokenLinks   = &anchor{shape: "lint.result.broken_links[]"}
	aSchemaViol    = &anchor{shape: "lint.result.schema_violations[]"}
	aEnrichFiles   = &anchor{shape: "enrich.result.files[]"}
	aGraphNodes    = &anchor{shape: "graph.nodes[]"}
	aGraphEdges    = &anchor{shape: "graph.edges[]"}
	aInferMap      = &anchor{shape: "infer.result.mappings[]"}
)

// config .result.values is a value-MAP: its own key set is a schema (the setting
// names, checked against config.result.values) and every child is a {value,source}.
var aConfigValues = &anchor{shape: "config.result.values", elem: aConfigValueK}

var aConfigResult = &anchor{shape: "config.result", fields: map[string]*anchor{
	"values": aConfigValues,
}}

var aConfigEnvelope = &anchor{shape: "config.envelope", fields: map[string]*anchor{
	"result": aConfigResult,
}}

// report envelope: per-command result payloads are documented as their own bare
// blocks, so here result is shown as an empty placeholder. shape:"" is a
// structural node (not key-checked); with no fields/elem a NON-empty result
// surfaces as UNANCHORED and forces it to be anchored (fail-closed).
var aReportResult = &anchor{shape: ""}
var aReportEnvelope = &anchor{shape: "report.envelope", fields: map[string]*anchor{
	"result": aReportResult,
}}

// Bare result payloads (no envelope), keyed for root routing.
var aConvertResult = &anchor{shape: "convert.result", fields: map[string]*anchor{
	"concepts":   arr(aConvConcept),
	"unresolved": arr(aUnresolvedC),
	"verified":   aVerified,
}}
var aValidateResult = &anchor{shape: "validate.result", fields: map[string]*anchor{
	"findings": arr(aFindings),
}}
var aReviewResult = &anchor{shape: "review.result", fields: map[string]*anchor{
	"concepts":   arr(aReviewConcept),
	"unresolved": arr(aUnresolvedR),

	// --- EXEMPTIONS (fail-open surface; keep tiny, each justified) ---
	// Free-form data maps whose KEYS are data, not a schema, so a fixed
	// key-set contract does not apply and checking them would false-positive.
	"by_type": exemptAnchor(), // {"Guide":2,"Note":1} — keys are concept-type names
	"tiers":   exemptAnchor(), // {"unverified":3}     — keys are tier names
}}
var aLintResult = &anchor{shape: "lint.result", fields: map[string]*anchor{
	"broken_links":      arr(aBrokenLinks),
	"schema_violations": arr(aSchemaViol),
}}
var aEnrichResult = &anchor{shape: "enrich.result", fields: map[string]*anchor{
	"files":    arr(aEnrichFiles),
	"verified": aVerified,
}}
var aGraphRaw = &anchor{shape: "graph.raw", fields: map[string]*anchor{
	"nodes": arr(aGraphNodes),
	"edges": arr(aGraphEdges),
}}
var aInferResult = &anchor{shape: "infer.result", fields: map[string]*anchor{
	"mappings": arr(aInferMap),
}}

// bareRoots are the candidate root shapes for a block that is not an envelope.
// A block whose root matches none of these (zero key overlap) is UNANCHORED.
var bareRoots = map[string]*anchor{
	"convert.result":  aConvertResult,
	"validate.result": aValidateResult,
	"review.result":   aReviewResult,
	"lint.result":     aLintResult,
	"enrich.result":   aEnrichResult,
	"infer.result":    aInferResult,
	"graph.raw":       aGraphRaw,
	"result.verified": aVerified, // the standalone .result.verified block in SKILL.md
}

// routeRoot picks the anchor tree for a block's root object. Envelopes are
// identified structurally (binder/schema/result) and split by command value;
// bare payloads are routed to the best key-overlap root. On no match it returns
// nil with a reason, which the caller records as UNANCHORED — never silence.
func routeRoot(root any, idx shapeIndex) (*anchor, string) {
	m, ok := root.(map[string]any)
	if !ok {
		return nil, "root is not a JSON object"
	}
	keys := keySet(m)
	if keys["binder"] && keys["schema"] && keys["result"] {
		if cmd, _ := m["command"].(string); cmd == "config" {
			return aConfigEnvelope, "config.envelope"
		}
		return aReportEnvelope, "report.envelope"
	}
	var best *anchor
	bestName, bestScore := "", 0.0
	for name, a := range bareRoots {
		live, ok := idx[a.shape]
		if !ok {
			continue
		}
		if s := jaccard(keys, live.allowed); s > bestScore {
			best, bestName, bestScore = a, name, s
		}
	}
	if best == nil || bestScore <= 0 {
		return nil, "unroutable (matches no known result shape)"
	}
	return best, bestName
}

// --- descent, findings, coverage --------------------------------------------

// finding is one key-set mismatch between a documented object and its anchored
// live shape.
type finding struct {
	file    string
	line    int
	path    string
	shape   string
	missing []string // keys the binary emits but the doc omits (the #106 class)
	extra   []string // keys the doc carries but the binary does not (retired keys)
}

func (f finding) String() string {
	return fmt.Sprintf("%s:%d: %s vs live %s: MISSING-FROM-DOC=%v NOT-IN-BINARY=%v",
		f.file, f.line, f.path, f.shape, orDash(f.missing), orDash(f.extra))
}

func orDash(s []string) any {
	if len(s) == 0 {
		return "-"
	}
	return s
}

// coverage records, for every object the gate walks, how it was accounted for:
// checked against a shape, exempted, a structural node, or UNANCHORED. The
// completeness gate fails on any UNANCHORED entry.
type coverage struct {
	file, path, status string
	line               int
}

func (c coverage) unanchored() bool { return strings.HasPrefix(c.status, "UNANCHORED") }

// container reports whether x is worth descending into: an object, or an array
// with ANY container element. Scalars and scalar arrays are ignored. (It checks
// every element, not just the first, so a mixed array whose first entry is a
// scalar is still descended for its object entries — finding 1.)
func container(x any) bool {
	switch v := x.(type) {
	case map[string]any:
		return true
	case []any:
		for _, e := range v {
			if container(e) {
				return true
			}
		}
		return false
	}
	return false
}

// scanDoc routes and descends every fenced json/jsonc block of text, returning
// key-set drift findings and a coverage entry for every object walked.
func scanDoc(file, text string, idx shapeIndex) (findings []finding, cov []coverage) {
	for _, blk := range extractBlocks(text) {
		var root any
		if err := json.Unmarshal([]byte(stripJSONC(blk.body)), &root); err != nil {
			findings = append(findings, finding{file: file, line: blk.line, path: "$", shape: "(unparseable)",
				missing: []string{"block did not parse as JSON after jsonc strip: " + err.Error()}})
			cov = append(cov, coverage{file: file, path: "$", status: "UNANCHORED (unparseable)", line: blk.line})
			continue
		}
		a, status := routeRoot(root, idx)
		if a == nil {
			cov = append(cov, coverage{file: file, path: "$", status: "UNANCHORED (" + status + ")", line: blk.line})
			continue
		}
		descend(root, "$", a, idx, file, blk.line, &findings, &cov)
	}
	return
}

// descend walks o against anchor a, appending findings and coverage.
func descend(o any, path string, a *anchor, idx shapeIndex, file string, line int, findings *[]finding, cov *[]coverage) {
	switch v := o.(type) {
	case map[string]any:
		switch {
		case a == nil:
			*cov = append(*cov, coverage{file: file, path: path, status: "UNANCHORED", line: line})
			return
		case a.exempt:
			*cov = append(*cov, coverage{file: file, path: path, status: "exempt", line: line})
			return
		case a.shape == "":
			*cov = append(*cov, coverage{file: file, path: path, status: "structural", line: line})
		default:
			*cov = append(*cov, coverage{file: file, path: path, status: "checked:" + a.shape, line: line})
			live, ok := idx[a.shape]
			if !ok {
				*findings = append(*findings, finding{file: file, line: line, path: path, shape: a.shape,
					missing: []string{"live shape not indexed (test bug): " + a.shape}})
			} else {
				keys := keySet(v)
				// missing: a REQUIRED live key (present in every live element) the
				// doc omits — the #106 class. extra: a doc key no live element
				// emits (not in the allowed union) — a retired/invented key. A key
				// the live output shows varies by omitempty (in allowed, not
				// required) is optional and contributes to neither.
				if missing, extra := diff(live.required, keys), diff(keys, live.allowed); len(missing) > 0 || len(extra) > 0 {
					*findings = append(*findings, finding{file: file, line: line, path: path,
						shape: a.shape, missing: missing, extra: extra})
				}
			}
		}
		for _, k := range sortedKeys(v) {
			child := v[k]
			if !container(child) {
				continue
			}
			var ca *anchor
			if a.fields != nil {
				ca = a.fields[k]
			}
			if ca == nil {
				ca = a.elem // wildcard child of a value-map
			}
			descend(child, path+"."+k, ca, idx, file, line, findings, cov)
		}
	case []any:
		// Walk EVERY container element, not just v[0]: a drifted or unanchored
		// non-first element must be checked and reported, never silently skipped
		// (issue #106, finding 1). Every object-bearing array under plugins/ is
		// homogeneous by contract — each is emitted from a Go []T slice, so all
		// elements share the one element anchor (a.elem). A heterogeneous array
		// (elements differing in shape by design) does not exist here and is not
		// idiomatic for these typed slices; were one ever introduced it would
		// have to be anchored per-element or exempted by name like by_type/tiers,
		// not silently averaged to the first element's shape.
		var ea *anchor
		if a != nil {
			ea = a.elem
		}
		for i, e := range v {
			if !container(e) {
				continue
			}
			descend(e, fmt.Sprintf("%s[%d]", path, i), ea, idx, file, line, findings, cov)
		}
	}
}

func jaccard(a, b map[string]bool) float64 {
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// diff returns the sorted keys in a that are not in b.
func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// --- live index construction (in-process CLI) -------------------------------

// runJSON runs the binder root command in-process with args and returns its
// stdout parsed as JSON. Commands that exit non-zero for a reason we expect
// (e.g. validate on a non-conformant bundle) still print their JSON report, so
// the exit status is ignored; only a stdout that fails to parse is fatal.
func runJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	root := cmd.NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	_ = root.Execute()
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("binder %s: output is not JSON: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return m
}

func result(m map[string]any) map[string]any {
	if r, ok := m["result"].(map[string]any); ok {
		return r
	}
	return nil
}

// findCorpora returns every plugin sample-corpus directory (…/assets/sample-corpus)
// under root. Globbed rather than hardcoded: okf-convert is the only plugin
// today, but a second plugin's corpus and docs are picked up for free.
func findCorpora(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "sample-corpus" && filepath.Base(filepath.Dir(path)) == "assets" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(out)
	return out
}

// copyTree copies the directory tree at src into dst.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

// buildLiveIndex runs the documented commands over each plugin corpus and
// registers the key set of every documented result shape. The registered names
// mirror the shapes the anchor trees reference.
func buildLiveIndex(t *testing.T, repoRoot string) shapeIndex {
	t.Helper()
	// Deterministic, config-free environment: no global or repo-local config,
	// no ambient verifier, a pinned clock.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BINDER_VERIFIED_BY", "")
	os.Unsetenv("BINDER_VERIFIED_BY")
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	idx := shapeIndex{}
	corpora := findCorpora(t, filepath.Join(repoRoot, "plugins"))
	if len(corpora) == 0 {
		t.Fatal("no plugin sample-corpus found under plugins/; cannot derive live shapes")
	}

	for _, corpus := range corpora {
		bundle := filepath.Join(t.TempDir(), "bundle")
		if _, err := os.Stat(corpus); err != nil {
			t.Fatalf("corpus %s: %v", corpus, err)
		}
		// Produce a bundle to point the bundle-reading commands at.
		if m := runJSON(t, "convert", corpus, "-o", bundle, "--json"); result(m) == nil {
			t.Fatalf("convert %s produced no result", corpus)
		}

		conv := runJSON(t, "convert", corpus, "--dry-run", "--json")
		idx.register("report.envelope", conv)
		cr := result(conv)
		idx.register("convert.result", cr)
		idx.registerElem("convert.result.concepts[]", cr["concepts"])
		idx.registerElem("convert.result.unresolved[]", cr["unresolved"])
		idx.register("convert.result.verified", cr["verified"])
		idx.register("result.verified", cr["verified"]) // canonical; enrich's is identical

		val := result(runJSON(t, "validate", bundle, "--json"))
		idx.register("validate.result", val)

		// A non-conformant bundle so the findings[] element shape is live-indexed.
		bad := filepath.Join(t.TempDir(), "badbundle")
		copyTree(t, bundle, bad)
		breakOneType(t, bad)
		if br := result(runJSON(t, "validate", bad, "--json")); br != nil {
			idx.registerElem("validate.result.findings[]", br["findings"])
		}

		rev := result(runJSON(t, "review", bundle, "--json"))
		idx.register("review.result", rev)
		idx.registerElem("review.result.concepts[]", rev["concepts"])
		idx.registerElem("review.result.unresolved[]", rev["unresolved"])

		lin := result(runJSON(t, "lint", corpus, "--json"))
		idx.register("lint.result", lin)
		idx.registerElem("lint.result.broken_links[]", lin["broken_links"])
		idx.registerElem("lint.result.schema_violations[]", lin["schema_violations"])

		enr := result(runJSON(t, "enrich", corpus, "--dry-run", "--json"))
		idx.register("enrich.result", enr)
		idx.registerElem("enrich.result.files[]", enr["files"])
		idx.register("enrich.result.verified", enr["verified"])

		gra := runJSON(t, "graph", bundle, "--json")
		idx.register("graph.raw", gra)
		idx.registerElem("graph.nodes[]", gra["nodes"])
		idx.registerElem("graph.edges[]", gra["edges"])

		inf := result(runJSON(t, "infer", corpus, "--json"))
		idx.register("infer.result", inf)
		idx.registerElem("infer.result.mappings[]", inf["mappings"])

		cfg := runJSON(t, "config", "--json")
		idx.register("config.envelope", cfg)
		cfgr := result(cfg)
		idx.register("config.result", cfgr)
		if vals, ok := cfgr["values"].(map[string]any); ok {
			idx.register("config.result.values", vals)
			for _, v := range vals {
				idx.register("config.result.values.<k>", v)
				break
			}
		}
	}
	return idx
}

// breakOneType rewrites the first `type:` frontmatter line found in a .md file
// under dir to an empty value, making the bundle non-conformant (OKF §11.2) so
// validate emits a findings[] element to index.
func breakOneType(t *testing.T, dir string) {
	t.Helper()
	var target string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || target != "" || filepath.Ext(path) != ".md" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if regexp.MustCompile(`(?m)^type:`).Match(b) {
			target = path
		}
		return nil
	})
	if target == "" {
		return // no type: line found; findings[] simply won't be indexed
	}
	b, _ := os.ReadFile(target)
	fixed := regexp.MustCompile(`(?m)^type:.*$`).ReplaceAll(b, []byte(`type: ""`))
	if err := os.WriteFile(target, fixed, 0o644); err != nil {
		t.Fatalf("rewrite %s: %v", target, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return abs
}

// pluginMarkdown returns every *.md file under plugins/.
func pluginMarkdown(t *testing.T, repoRoot string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(filepath.Join(repoRoot, "plugins"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".md" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk plugins: %v", err)
	}
	sort.Strings(out)
	return out
}

// scanPlugins runs the gate over every plugin markdown file and returns all
// findings and coverage entries.
func scanPlugins(t *testing.T) ([]finding, []coverage) {
	t.Helper()
	root := repoRoot(t)
	idx := buildLiveIndex(t, root)
	var findings []finding
	var cov []coverage
	for _, f := range pluginMarkdown(t, root) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		rel, _ := filepath.Rel(root, f)
		fs, cv := scanDoc(rel, string(b), idx)
		findings = append(findings, fs...)
		cov = append(cov, cv...)
	}
	return findings, cov
}

// --- the gates --------------------------------------------------------------

// TestPluginDocs_NoDrift is the drift gate. It fails when any fenced json/jsonc
// block in plugins/**/*.md documents an object whose key set (at any anchored
// nesting level) disagrees with what the live binary emits for that shape. That
// is the #106 class: a documented example missing a key the binary emits (or
// carrying one it retired). Regenerate by recapturing the drifted block from a
// live run against the plugin's assets/sample-corpus/ — do NOT hand-edit the
// missing key in, that is how the transcript drifted in the first place (#106,
// #112).
func TestPluginDocs_NoDrift(t *testing.T) {
	findings, _ := scanPlugins(t)
	if len(findings) > 0 {
		var msg strings.Builder
		msg.WriteString("plugin doc JSON has drifted from live binder output.\n")
		msg.WriteString("Recapture the affected block from a live run against the plugin's\n")
		msg.WriteString("assets/sample-corpus/ (do NOT hand-edit the key in). Findings:\n")
		for _, f := range findings {
			msg.WriteString("  " + f.String() + "\n")
		}
		t.Fatal(msg.String())
	}
}

// TestPluginDocs_EveryBlockAnchored is the completeness gate. Path-anchoring
// only checks objects it can place; an object at a path no anchor describes
// would go unchecked forever with nothing saying so. This fails if any object in
// any plugin JSON block is neither anchored to a live shape nor explicitly
// exempted — so a newly added transcript block cannot slip in unchecked, and the
// only way to silence an object is to add it to the (reviewed, commented)
// exemption list in the anchor trees above.
func TestPluginDocs_EveryBlockAnchored(t *testing.T) {
	_, cov := scanPlugins(t)
	var bad []coverage
	for _, c := range cov {
		if c.unanchored() {
			bad = append(bad, c)
		}
	}
	if len(bad) > 0 {
		var msg strings.Builder
		msg.WriteString("plugin doc JSON has objects the gate cannot verify (not anchored, not exempt).\n")
		msg.WriteString("Add an anchor for the shape, or — only if it is genuinely free-form DATA —\n")
		msg.WriteString("an explicit, commented exemption. Unanchored:\n")
		for _, c := range bad {
			msg.WriteString(fmt.Sprintf("  %s:%d: %s [%s]\n", c.file, c.line, c.path, c.status))
		}
		t.Fatal(msg.String())
	}
}
