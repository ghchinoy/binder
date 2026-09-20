package version_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// repoRoot is this package's directory two levels up: internal/version -> repo.
// Same relative-path convention as internal/gendocs' drift test.
const repoRoot = "../.."

// versionish matches a hard-coded binder version inside a string literal:
// "binder/" immediately followed by a digit, or by a "v" and a digit. The
// v-prefixed shape is called out separately by #60 because PR #52 established
// that the producer string carries NO leading "v" even though the git tag does.
var versionish = regexp.MustCompile(`binder/v?[0-9]`)

// mustVisit is the minimum-coverage inventory: the four shipped files issue #60
// enumerated as carrying a hand-maintained "binder/0.3.0". The gate asserts it
// actually parsed each one, so "0 findings" cannot mean "0 findings because the
// walk reached nothing". A bare file count would be satisfiable by coincidence
// (one file dropping out as another appears); naming the files is not.
//
// This mirrors the EXPECTED_COVERAGE inventory in
// scripts/check-shipped-version-literals.py, and for the same reason: a version
// gate that silently inspects nothing and exits green is the exact
// silent-permissive failure these gates exist to remove.
var mustVisit = []string{
	"cmd/convert.go",
	"cmd/enrich.go",
	"internal/config/config.go",
	"pkg/mcp/convert.go",
}

// skipDirs are trees whose version literals are legitimate and must NOT track
// the release: fixtures and golden files deliberately pin historical versions,
// and the non-Go workspaces have their own gates.
var skipDirs = map[string]bool{
	"testdata":     true,
	".git":         true,
	"node_modules": true,
	"site":         true,
	"packages":     true,
}

type literalFinding struct {
	pos     string
	literal string
}

// scanShippedLiterals walks root and reports every hard-coded binder version
// found in a string literal of a shipped (non-test) Go file, plus the set of
// files it actually parsed. It is a function rather than inline test code so the
// proving tests below can run the REAL scanner over synthetic trees — a gate
// proved by a reimplementation of itself is not proved.
//
// It inspects the AST rather than the raw bytes on purpose. A regex over the
// source would flag the comments in this very package and in internal/config
// that quote "binder/0.3.0" while explaining the defect — a false positive that
// would push the next author toward loosening the pattern. Parsing to string
// literals makes the check exact, so it can stay zero-tolerance.
func scanShippedLiterals(root string) ([]literalFinding, map[string]bool, error) {
	var findings []literalFinding
	visited := map[string]bool{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Test files legitimately pin literals: cmd/version_test.go asserts a
		// version it sets itself, and several suites use historical fixtures.
		// They are self-consistent by construction and ship to nobody.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		visited[filepath.ToSlash(rel)] = true

		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		// Findings are keyed by the position of the leftmost string literal they
		// involve, so one defect is reported once. `a + b + c` parses as
		// `(a+b)+c`, and ast.Inspect visits the outer expression, the inner one,
		// and every operand — all of which share a leftmost literal.
		seen := map[token.Pos]bool{}
		record := func(anchor token.Pos, s string) {
			if anchor == token.NoPos || seen[anchor] {
				return
			}
			seen[anchor] = true
			findings = append(findings, literalFinding{fset.Position(anchor).String(), s})
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.BinaryExpr:
				// A literal split across a concatenation ("binder/" + "0.3.0")
				// must not launder past the gate. Fold the chain first, with
				// every non-literal operand replaced by a sentinel that cannot
				// bridge the pattern — that is what keeps the FIX itself
				// ("binder/" + version) unflagged.
				if e.Op != token.ADD {
					return true
				}
				joined, anchor, hasLit := foldConcat(e)
				if hasLit && versionish.MatchString(joined) {
					record(anchor, joined)
				}
			case *ast.BasicLit:
				if e.Kind != token.STRING {
					return true
				}
				if s := unquote(e); versionish.MatchString(s) {
					record(e.Pos(), s)
				}
			}
			return true
		})
		return nil
	})
	return findings, visited, err
}

// unquote returns a string literal's value. A literal that will not unquote is
// scanned as written rather than skipped silently.
func unquote(lit *ast.BasicLit) string {
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return lit.Value
	}
	return s
}

// foldConcat flattens a `+` expression into the text it produces, substituting
// concatSentinel for every operand that is not a string literal. It returns the
// folded text, the position of the leftmost string literal (the finding's
// anchor), and whether any string literal was present at all.
func foldConcat(e ast.Expr) (text string, anchor token.Pos, hasLit bool) {
	switch v := e.(type) {
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			lt, la, lh := foldConcat(v.X)
			rt, ra, rh := foldConcat(v.Y)
			anchor = la
			if anchor == token.NoPos {
				anchor = ra
			}
			return lt + rt, anchor, lh || rh
		}
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return unquote(v), v.Pos(), true
		}
	}
	return concatSentinel, token.NoPos, false
}

// concatSentinel stands in for a non-literal operand when folding a `+` chain.
// A NUL can never appear in the pattern, so it reliably breaks a match across
// the gap — `"binder/" + version` folds to "binder/\x00" and is correctly NOT
// flagged, while `"binder/" + "0.3.0"` folds to "binder/0.3.0" and is.
const concatSentinel = "\x00"

// TestNoHardCodedVersionLiteralInShippedGo is the authoring-time half of the
// #60 guard: no shipped (non-test) Go string literal may contain a hard-coded
// binder version. Every such literal is either stale now or will be at the next
// release — that is the whole content of #60, which fired through v0.4.0 and
// again through v0.5.3. The live version is available via version.ActorExemplar
// (user-facing text) or binder.Version (trust stamps); a literal is never correct.
//
// The complementary half runs in CI against a STAMPED binary
// (scripts/check-shipped-version-literals.py): this test cannot pin literals to
// the real release version, because a `go test` build is unstamped. See that
// script's header for the split.
func TestNoHardCodedVersionLiteralInShippedGo(t *testing.T) {
	findings, visited, err := scanShippedLiterals(repoRoot)
	if err != nil {
		t.Fatalf("walking %s: %v", repoRoot, err)
	}

	// Vacuous-pass guard: prove the walk reached the files this gate exists for
	// before trusting an empty findings list.
	var missing []string
	for _, want := range mustVisit {
		if !visited[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("COVERAGE FAILURE: the walk never reached %v, so a green result "+
			"would be vacuous. Likely cause: a moved file or a changed skip rule. "+
			"(visited %d shipped .go files)", missing, len(visited))
	}

	for _, f := range findings {
		t.Errorf("%s: shipped string literal carries a hard-coded binder version: %q\n"+
			"\tUse version.ActorExemplar() for user-facing examples, or binder.Version for "+
			"trust stamps. A hand-maintained literal goes stale at the next release "+
			"(issue #60).", f.pos, f.literal)
	}
}

// --- proving the gate -------------------------------------------------------
//
// A guard never shown to fail is not known to be a guard. These cases run the
// real scanner over synthetic trees that reproduce, one per case, each way a
// stale literal reached users: the flag-help copy, the inlined error-path copy,
// and the v-prefixed form PR #52 banned. They also pin the two things the gate
// must NOT flag, because a gate that cries wolf gets its pattern loosened.

// writeTree materialises name->content under a fresh temp dir and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGateFlagsStaleLiterals(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			// cmd/convert.go and cmd/enrich.go, verbatim in shape.
			name: "flag help exemplar",
			src: "package x\n" +
				"var _ = \"actor to append as a verified stamp, e.g. \\\"binder/0.3.0\\\"\"\n",
		},
		{
			// internal/mcp/convert.go's inlined copy of the forms hint — the
			// error path, which is why #60 is not cosmetic.
			name: "inlined error-path hint",
			src: "package x\n" +
				"var _ = \"invalid actor; valid forms: ... or <producer>/<version> (e.g. binder/0.3.0)\"\n",
		},
		{
			// Correct for today's release, stale at the next one. The gate must
			// flag this too, or it only catches drift after it has already shipped.
			name: "literal matching the current release",
			src:  "package x\nvar _ = \"binder/0.5.3\"\n",
		},
		{
			// #60's explicit ask: the v-prefixed producer form PR #52 banned.
			name: "v-prefixed producer version",
			src:  "package x\nvar _ = \"binder/v0.5.3\"\n",
		},
		{
			// Concatenation does not launder a literal.
			name: "literal built by concatenation",
			src:  "package x\nvar _ = \"see binder/\" + \"0.3.0 for the form\"\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{"cmd/thing.go": tc.src})
			findings, _, err := scanShippedLiterals(root)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(findings) == 0 {
				t.Fatalf("gate did not flag a stale literal in:\n%s", tc.src)
			}
		})
	}
}

func TestGateStaysSilentOnLegitimateSources(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
	}{
		{
			// The derivation itself: "binder/" with the version appended at run
			// time is the FIX, and must never be flagged.
			name: "derived exemplar",
			files: map[string]string{
				"internal/version/v.go": "package x\nvar v = \"0.5.3\"\nvar _ = \"binder/\" + v\n",
			},
		},
		{
			// Comments quoting the defect while explaining it. This is the case
			// that makes the AST scan necessary rather than a nicety: several
			// files in this repo do exactly this.
			name: "comment quoting a stale literal",
			files: map[string]string{
				"internal/config/c.go": "package x\n// It used to say \"binder/0.3.0\", which nothing bumped.\nvar _ = \"ok\"\n",
			},
		},
		{
			// Test files pin literals deliberately.
			name: "literal in a _test.go file",
			files: map[string]string{
				"cmd/version_test.go": "package x\nconst want = \"binder/0.3.0\"\n",
			},
		},
		{
			// Fixtures and golden files legitimately carry historical versions.
			name: "literal under testdata",
			files: map[string]string{
				"testdata/gen/fixture.go": "package x\nvar _ = \"binder/0.1.0\"\n",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, tc.files)
			findings, _, err := scanShippedLiterals(root)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(findings) != 0 {
				t.Fatalf("gate false-positived on a legitimate source: %+v", findings)
			}
		})
	}
}

// TestGateCoverageInventoryIsNotVacuous locks the vacuous-pass guard itself.
// Point the scanner at a tree that does NOT contain the must-visit files and
// the inventory must come up short — proving the real test's green result is
// backed by files actually parsed, not by a walk that found nothing.
func TestGateCoverageInventoryIsNotVacuous(t *testing.T) {
	root := writeTree(t, map[string]string{"cmd/unrelated.go": "package x\n"})
	_, visited, err := scanShippedLiterals(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, want := range mustVisit {
		if visited[want] {
			t.Fatalf("inventory entry %q reported visited in a tree that lacks it", want)
		}
	}
	if len(visited) != 1 {
		t.Fatalf("visited = %v, want exactly the one file written", visited)
	}
}
