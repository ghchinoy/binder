// Package plugindocs holds the drift gate for the plugin skill docs under
// plugins/. It is the plugins/ analogue of internal/gendocs's byte-equality
// gate for docs/commands/: docs/commands/ is regenerated from the Cobra tree
// and pinned byte-for-byte, but plugins/ carried hand-copied JSON transcripts
// with no mechanical tie to the binary — which is exactly how the okf-convert
// contract drifted four minor versions (issue #106) while docs/ did not.
//
// The gate here is deliberately weaker than byte-equality and stronger than a
// top-level key check:
//
//   - It runs the CLI IN-PROCESS over each plugin's own assets/sample-corpus/
//     and indexes the key set of every documented result shape from live output.
//   - It extracts every fenced json/jsonc block from plugins/**/*.md (globbed,
//     not hardcoded to okf-convert) — indented fences inside list items included.
//   - It asserts KEY-SET EQUALITY at every nesting level, not byte-equality:
//     the examples use illustrative values (docs/guide, human:alice) and jsonc
//     comments that carry teaching weight, and byte-equality would force those
//     out and make the docs worse. Key-set equality catches exactly the #106
//     class — a documented object missing a key the binary emits, or carrying a
//     key the binary retired — and nothing else.
//   - It WALKS nested objects. A top-level-only checker passes every instance in
//     #106 (the drift sat one level down, in .result.values and concepts[]); the
//     audit's own first instrument did exactly that and produced a false CLEAN.
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

// shapeIndex maps a shape name to the set of keys the live binary emits for it.
type shapeIndex map[string]map[string]bool

func keySet(m map[string]any) map[string]bool {
	s := make(map[string]bool, len(m))
	for k := range m {
		s[k] = true
	}
	return s
}

// register records the key set of obj under name. Empty objects are ignored:
// they carry no key contract to compare against (e.g. an envelope's "result":{}
// placeholder).
func (idx shapeIndex) register(name string, obj any) {
	if m, ok := obj.(map[string]any); ok && len(m) > 0 {
		idx[name] = keySet(m)
	}
}

// registerElem records the shape of the first element of a JSON array, if any.
func (idx shapeIndex) registerElem(name string, arr any) {
	if a, ok := arr.([]any); ok && len(a) > 0 {
		idx.register(name, a[0])
	}
}

// finding is one key-set mismatch between a documented object and the closest
// live shape.
type finding struct {
	file    string
	line    int
	path    string
	shape   string
	score   float64
	missing []string // keys the binary emits but the doc omits (the #106 class)
	extra   []string // keys the doc carries but the binary does not (retired keys)
}

func (f finding) String() string {
	return fmt.Sprintf("%s:%d: %s vs live %s (key overlap %.2f): MISSING-FROM-DOC=%v NOT-IN-BINARY=%v",
		f.file, f.line, f.path, f.shape, f.score, orDash(f.missing), orDash(f.extra))
}

func orDash(s []string) any {
	if len(s) == 0 {
		return "-"
	}
	return s
}

// matchThreshold is the minimum Jaccard key-overlap for a documented object to
// be considered a candidate for a live shape. It must be <= 0.33 so the sharpest
// real instance (config --json values missing 4 of 6 keys, overlap 2/6 = 0.33)
// is still matched and flagged; below it, an object matching no live shape well
// (a free-form data map such as review.result.by_type, or a partial excerpt) is
// left alone rather than reported, so the gate has no false positives.
const matchThreshold = 0.30

// scanText walks every nested object in every fenced json/jsonc block of text,
// matches each to the closest live shape by key overlap, and reports key-set
// inequality for confident matches. file is used only for diagnostics.
func scanText(file, text string, idx shapeIndex) []finding {
	var findings []finding
	for _, blk := range extractBlocks(text) {
		var root any
		if err := json.Unmarshal([]byte(stripJSONC(blk.body)), &root); err != nil {
			findings = append(findings, finding{
				file: file, line: blk.line, path: "$", shape: "(unparseable)",
				missing: []string{"block did not parse as JSON after jsonc strip: " + err.Error()},
			})
			continue
		}
		walk(root, "$", func(path string, keys map[string]bool) {
			if len(keys) == 0 {
				return
			}
			best, score := "", 0.0
			for name, live := range idx {
				if j := jaccard(keys, live); j > score {
					best, score = name, j
				}
			}
			if best == "" || score < matchThreshold {
				return // no confident match: free-form map or partial excerpt
			}
			missing := diff(idx[best], keys)
			extra := diff(keys, idx[best])
			if len(missing) > 0 || len(extra) > 0 {
				findings = append(findings, finding{
					file: file, line: blk.line, path: path, shape: best,
					score: score, missing: missing, extra: extra,
				})
			}
		})
	}
	return findings
}

// walk visits every object nested anywhere in o, calling fn with its path and
// key set. Arrays are represented by their first element (shapes are uniform),
// matching the audit instrument's behaviour.
func walk(o any, path string, fn func(path string, keys map[string]bool)) {
	switch v := o.(type) {
	case map[string]any:
		fn(path, keySet(v))
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walk(v[k], path+"."+k, fn)
		}
	case []any:
		if len(v) > 0 {
			walk(v[0], path+"[]", fn)
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
// mirror the shapes that actually appear as fenced blocks in the contract docs.
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

		lin := result(runJSON(t, "lint", corpus, "--json"))
		idx.register("lint.result", lin)

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

// --- the gate ---------------------------------------------------------------

// TestPluginDocs_NoDrift is the gate. It fails when any fenced json/jsonc block
// in plugins/**/*.md documents an object whose key set (at any nesting level)
// disagrees with what the live binary emits for that shape. That is the #106
// class: a documented example missing a key the binary emits (or carrying one it
// retired). Regenerate by recapturing the drifted block from a live run against
// the plugin's assets/sample-corpus/ — do NOT hand-edit the missing key in, that
// is how the transcript drifted from the binary in the first place (#106, #112).
func TestPluginDocs_NoDrift(t *testing.T) {
	root := repoRoot(t)
	idx := buildLiveIndex(t, root)

	var all []finding
	for _, f := range pluginMarkdown(t, root) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		rel, _ := filepath.Rel(root, f)
		all = append(all, scanText(rel, string(b), idx)...)
	}

	if len(all) > 0 {
		var msg strings.Builder
		msg.WriteString("plugin doc JSON has drifted from live binder output.\n")
		msg.WriteString("Recapture the affected block from a live run against the plugin's\n")
		msg.WriteString("assets/sample-corpus/ (do NOT hand-edit the key in). Findings:\n")
		for _, f := range all {
			msg.WriteString("  " + f.String() + "\n")
		}
		t.Fatal(msg.String())
	}
}
