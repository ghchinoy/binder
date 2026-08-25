package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

// badFM is a frontmatter block no YAML parser accepts (an unquoted plain scalar
// containing ": "), mirroring testdata/corpus-lint-schema/badyaml.md. It is the
// #161/#163 negative-fixture payload used across these end-to-end tests.
const badFM = "---\ntitle: thing: with an unquoted colon\ngoal: another: bad line\n---\n\n# Bad\n"

// TestReviewStrictGatesOnUnparseableConcept is the I-2 negative fixture end to
// end: a bundle whose only concept is unparseable made `binder review --strict`
// exit 0 with "unparsed_frontmatter": [] — silently clean — while `binder
// validate` exited 1 on the same bundle. After the fix the file is recovered and
// disclosed, so review discloses it and --strict gates. This fails against the
// pre-fix loader and passes now.
func TestReviewStrictGatesOnUnparseableConcept(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	bundle := writeTree(t, map[string]string{
		"index.md": "---\ntype: index\ntitle: Index\nokf_version: \"0.2\"\n---\n\n# Index\n",
		"bad.md":   badFM,
	})

	// review without --strict never gates (never-reject), but must DISCLOSE the file.
	stdout, _, code := runCLISplit(t, "review", bundle, "--json")
	if code != clijson.ExitSuccess {
		t.Fatalf("review without --strict: exit = %d, want 0 (never-reject)", code)
	}
	var env struct {
		Result struct {
			UnparsedFrontmatter []string `json:"unparsed_frontmatter"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("review json parse: %v\n%s", err, stdout)
	}
	if len(env.Result.UnparsedFrontmatter) != 1 || env.Result.UnparsedFrontmatter[0] != "bad" {
		t.Fatalf("unparsed_frontmatter = %v, want [bad] (I-2: the field could never report a genuinely-unparseable file before the fix)", env.Result.UnparsedFrontmatter)
	}

	// review --strict must now exit non-zero on the same bundle — the two binder
	// gates (validate / review --strict) no longer contradict each other.
	if _, code := runCLI(t, "review", bundle, "--strict"); code != clijson.ExitFindings {
		t.Fatalf("review --strict on an unparseable-only bundle: exit = %d, want 1", code)
	}
	// validate still exits 1, as it always did — the point is that review agrees now.
	if _, code := runCLI(t, "validate", bundle); code != clijson.ExitFindings {
		t.Fatalf("validate on an unparseable-only bundle: exit = %d, want 1", code)
	}
}

// TestReviewProseCountStopsLying guards the misleading prose line I-2 called out:
// "unparsed frontmatter (recovered as body): 0" was printed on a bundle with an
// unparseable file. It must now report the real count.
func TestReviewProseCountStopsLying(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	bundle := writeTree(t, map[string]string{
		"index.md": "---\ntype: index\ntitle: Index\nokf_version: \"0.2\"\n---\n\n# Index\n",
		"bad.md":   badFM,
	})
	stdout, _, _ := runCLISplit(t, "review", bundle)
	if strings.Contains(stdout, "unparsed frontmatter (recovered as body): 0") {
		t.Fatalf("review prose still reports 0 unparsed on a bundle with an unparseable file:\n%s", stdout)
	}
	if !strings.Contains(stdout, "unparsed frontmatter (recovered as body): 1") {
		t.Fatalf("review prose must report 1 unparsed:\n%s", stdout)
	}
}

// TestIndexKeepsUnparseableConceptInNav is the I-3 negative fixture: `binder
// index` rewrote index.md omitting the dropped file, silently claiming the corpus
// was one document smaller. The recovered concept must now appear in the nav, and
// the drop must be disclosed on stderr (never a silent write).
func TestIndexKeepsUnparseableConceptInNav(t *testing.T) {
	bundle := writeTree(t, map[string]string{
		"index.md": "---\ntype: index\ntitle: Index\nokf_version: \"0.2\"\n---\n\n# Index\n",
		"good.md":  "---\ntype: guide\ntitle: Good Doc\n---\n\n# Good Doc\n\nLink to [bad](bad.md).\n",
		"bad.md":   badFM,
	})
	_, stderr, code := runCLISplit(t, "index", bundle)
	if code != clijson.ExitSuccess {
		t.Fatalf("index exit = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(bundle, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "bad.md") {
		t.Fatalf("regenerated index.md must keep bad.md in the nav (I-3):\n%s", got)
	}
	if !strings.Contains(stderr, "bad.md") || !strings.Contains(stderr, "did not parse") {
		t.Fatalf("index must warn on stderr about the unparseable file (I-3); stderr:\n%s", stderr)
	}
}

// TestGraphDeclaresRecoveredNode is half the I-4 negative fixture: `binder graph`
// emitted an edge to a node it never declared (a dangling edge). The recovered
// node must now be declared, and the drop disclosed both on stderr and in the
// --json export's unparsed field (the same disclosure the MCP graph tool surfaces).
func TestGraphDeclaresRecoveredNode(t *testing.T) {
	bundle := writeTree(t, map[string]string{
		"index.md": "---\ntype: index\ntitle: Index\nokf_version: \"0.2\"\n---\n\n# Index\n",
		"good.md":  "---\ntype: guide\ntitle: Good Doc\n---\n\n# Good Doc\n\nLink to [bad](bad.md).\n",
		"bad.md":   badFM,
	})
	dot, stderr, code := runCLISplit(t, "graph", bundle, "--format", "dot")
	if code != clijson.ExitSuccess {
		t.Fatalf("graph exit = %d, want 0", code)
	}
	// The dangling edge good -> bad now terminates at a DECLARED node.
	if !strings.Contains(dot, `"bad" [label=`) {
		t.Fatalf("graph must declare the recovered node \"bad\" (I-4: no dangling edge):\n%s", dot)
	}
	if !strings.Contains(stderr, "bad.md") {
		t.Fatalf("graph must warn on stderr about the unparseable file (I-4); stderr:\n%s", stderr)
	}

	jsonOut, _, _ := runCLISplit(t, "graph", bundle, "--json")
	var m struct {
		Unparsed []string `json:"unparsed"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &m); err != nil {
		t.Fatalf("graph --json parse: %v\n%s", err, jsonOut)
	}
	if len(m.Unparsed) != 1 || m.Unparsed[0] != "bad" {
		t.Fatalf("graph --json unparsed = %v, want [bad]", m.Unparsed)
	}
}

// TestProjectNoDanglingForeignKey is the other half of I-4: `binder project --out
// ddl` emitted an edges.csv row whose to_key was absent from nodes.csv — a
// dangling FK that would fail or create a phantom on load. Every edge endpoint
// must now be present in the node set.
func TestProjectNoDanglingForeignKey(t *testing.T) {
	bundle := writeTree(t, map[string]string{
		"index.md": "---\ntype: index\ntitle: Index\nokf_version: \"0.2\"\n---\n\n# Index\n",
		"good.md":  "---\ntype: guide\ntitle: Good Doc\n---\n\n# Good Doc\n\nLink to [bad](bad.md).\n",
		"bad.md":   badFM,
	})
	out := t.TempDir()
	_, stderr, code := runCLISplit(t, "project", bundle, "--out", out)
	if code != clijson.ExitSuccess {
		t.Fatalf("project exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	nodeKeys := csvColumn(t, filepath.Join(out, "nodes.csv"), "node_key")
	toKeys := csvColumn(t, filepath.Join(out, "edges.csv"), "to_key")
	for tk := range toKeys {
		if !nodeKeys[tk] {
			t.Fatalf("edges.csv to_key %q is a dangling FK: absent from nodes.csv %v (I-4)", tk, nodeKeys)
		}
	}
	// The recovered node must be one of them.
	if !nodeKeys["bad"] {
		t.Fatalf("nodes.csv must include the recovered node \"bad\"; got %v", nodeKeys)
	}
	if !strings.Contains(stderr, "bad.md") {
		t.Fatalf("project must warn on stderr about the unparseable file (I-4); stderr:\n%s", stderr)
	}
}

// TestIndexRootVersionNotAdoptedFromInvalidFrontmatter is the #163 negative
// fixture at the CLI: an index.md whose frontmatter is invalid YAML must not have
// its bogus okf_version written back on regeneration, and the drop must be
// disclosed on stderr.
func TestIndexRootVersionNotAdoptedFromInvalidFrontmatter(t *testing.T) {
	bundle := writeTree(t, map[string]string{
		"good.md":  "---\ntype: guide\ntitle: Good\n---\n\n# Good\n",
		"index.md": "---\ntitle: Index: with a colon\nokf_version: v0.9-bogus\n---\n\n# Index\n",
	})
	_, stderr, code := runCLISplit(t, "index", bundle)
	if code != clijson.ExitSuccess {
		t.Fatalf("index exit = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(bundle, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "v0.9-bogus") {
		t.Fatalf("okf_version v0.9-bogus was adopted from unparseable frontmatter (#163):\n%s", got)
	}
	if !strings.Contains(stderr, "okf_version not adopted") {
		t.Fatalf("index must disclose that the root version was not adopted (#163); stderr:\n%s", stderr)
	}
}

// TestIndexRootVersionNotAdoptedFromBody is the second #163 CLI fixture:
// okf_version appearing only in the body (inside a fenced code block) must never
// be scraped and written back.
func TestIndexRootVersionNotAdoptedFromBody(t *testing.T) {
	bundle := writeTree(t, map[string]string{
		"good.md":  "---\ntype: guide\ntitle: Good\n---\n\n# Good\n",
		"index.md": "---\ntype: index\n---\n\n# Index\n\n```yaml\nokf_version: v9.9-from-body-codeblock\n```\n",
	})
	if _, code := runCLI(t, "index", bundle); code != clijson.ExitSuccess {
		t.Fatalf("index exit = %d, want 0", code)
	}
	got, err := os.ReadFile(filepath.Join(bundle, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "v9.9-from-body-codeblock") {
		t.Fatalf("okf_version was scraped from the body/code block (#163):\n%s", got)
	}
}

// csvColumn reads a header CSV and returns the set of values under the named
// column. It is a minimal reader adequate for the credential-free DDL row files
// (no embedded newlines/quotes in these fixtures).
func csvColumn(t *testing.T, path, col string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	header := strings.Split(lines[0], ",")
	idx := -1
	for i, h := range header {
		if h == col {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("column %q not found in %s header %v", col, path, header)
	}
	out := map[string]bool{}
	for _, line := range lines[1:] {
		fields := strings.Split(line, ",")
		if idx < len(fields) {
			out[fields[idx]] = true
		}
	}
	return out
}
