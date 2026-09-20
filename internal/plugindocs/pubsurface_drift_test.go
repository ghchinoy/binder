package plugindocs

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestPublicSurface_NoDrift is the Phase-5 expansion of the plugindocs drift
// discipline to the Go-API surface intended for publication in Phase 6 (design
// Decision 3.4 / §3.6). Where the plugindocs gate (drift_test.go) locks the JSON
// KEY SETS of the six report structs against live plugin transcripts, THIS gate
// locks the exported GO SURFACE that Phase 6 will move internal/ -> pkg/:
//
//  1. the WHOLE exported surface of pkg/binder — every service Request,
//     Result, Service method, helper func, exported var/const/type — because that
//     package becomes pkg/binder verbatim at publish (design §3.6); and
//  2. the confirmed-MUST okf vocabulary from the Phase-2 export trace
//     (packaging/phase2/okf-export-trace.md): the direct SC/AD seam plus the
//     transitive set dragged in by Bundle's exported fields.
//
// It does NOT widen the committed surface (nothing is exported from okf and
// nothing moves to pkg/ here); it only widens what the DRIFT TEST guards, so a
// later phase that adds, removes, renames, re-types, or re-tags any of that
// surface must do so deliberately (regenerate the golden) rather than silently.
// The gate references internal/ paths today and is repointed to pkg/ in Phase 6
// with no logic change.
//
// The golden is GENERATED, never hand-edited: run with UPDATE_PUBSURFACE=1 to
// recapture it after an intentional surface change, exactly as the other derived
// goldens in this repo are recaptured.
func TestPublicSurface_NoDrift(t *testing.T) {
	root := repoRoot(t)

	var b strings.Builder
	b.WriteString("# ==== binder service surface (pkg/binder, published Phase 6) ====\n")
	binderEntries, _ := packageSurface(t, filepath.Join(root, "pkg", "binder"), nil)
	for _, e := range binderEntries {
		b.WriteString(e)
		b.WriteString("\n")
	}
	// Render the FULL exported pkg/okf surface (want=nil), exactly as pkg/binder is
	// rendered above — NOT an allowlist-filtered subset. The whole point of this
	// phase is "okf == MUST and nothing more", so the gate must see over-exports,
	// which an allowlist filter is structurally blind to (R1). okfNames is the set
	// of ALL exported top-level okf identifiers; the equality check below fails on
	// any name outside the MUST set (over-export) AND any MUST name missing.
	b.WriteString("\n# ==== okf published surface (== MUST set; packaging/phase2/okf-export-trace.md) ====\n")
	okfEntries, okfNames := packageSurface(t, filepath.Join(root, "pkg", "okf"), nil)
	for _, e := range okfEntries {
		b.WriteString(e)
		b.WriteString("\n")
	}
	got := b.String()

	// Vacuous-pass guards: a broken parse must fail LOUD, never green-by-emptiness
	// (the #169 failure mode the sibling invariant test also defends against).
	for _, sentinel := range []string{
		"type ConvertRequest struct",
		"type LintResult struct",
		"func (*Service) Convert",
		"func ResolveNow(",
		"var Version",
	} {
		if !strings.Contains(got, sentinel) {
			t.Fatalf("binder surface is missing sentinel %q: the source walk is broken, so a "+
				"green result would be vacuous. Fix the parse before trusting this gate.", sentinel)
		}
	}
	// okf SET-EQUALITY (not allowlist-subset): the published pkg/okf surface must be
	// EXACTLY the MUST set. Fail on (a) any exported okf identifier NOT in the MUST
	// set — the over-export direction an allowlist filter could never see (this is
	// the C1-class hole; a stray exported constructor/func/type now turns the gate
	// RED); and (b) any MUST identifier missing — a seam id renamed or removed. Both
	// directions are checked here so "okf == MUST and nothing more" is enforced on
	// the one-way publish door, not merely asserted.
	var missing, extra []string
	for name := range okfMustVocabulary {
		if !okfNames[name] {
			missing = append(missing, name)
		}
	}
	for name := range okfNames {
		if !okfMustVocabulary[name] {
			extra = append(extra, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("confirmed-MUST okf identifiers not exported from pkg/okf: %v. Either the "+
			"Phase-2 trace's public seam changed (update okfMustVocabulary AND the design trace) "+
			"or an identifier was renamed/removed (restore it or make the change deliberate).",
			missing)
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		t.Errorf("pkg/okf exports identifiers OUTSIDE the MUST set (over-export; AC6 "+
			"\"okf == MUST and nothing more\" violation on the irreversible publish): %v. Unexport "+
			"them (move to internal/okfrules or lowercase), or — only if genuinely part of the "+
			"public seam — add them to okfMustVocabulary AND the Phase-2 trace deliberately.",
			extra)
	}

	goldenPath := filepath.Join(root, "internal", "plugindocs", "testdata", "public-surface.golden")
	if os.Getenv("UPDATE_PUBSURFACE") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (regenerate with UPDATE_PUBSURFACE=1): %v", err)
	}
	if string(want) != got {
		t.Errorf("public surface drifted from the committed golden.\n"+
			"If this change is intentional, regenerate with:\n"+
			"  UPDATE_PUBSURFACE=1 go test ./internal/plugindocs -run TestPublicSurface_NoDrift\n"+
			"and review the diff as a deliberate change to the Phase-6 publication surface.\n\n"+
			"--- want (golden) ---\n%s\n--- got (live) ---\n%s", string(want), got)
	}
}

// okfMustVocabulary is the confirmed-MUST okf export set from the Phase-2 trace
// (packaging/phase2/okf-export-trace.md §"MUST"): the direct SC/AD seam plus the
// transitive set forced public by okf.Bundle's exported fields. This is the EXACT
// expected published pkg/okf surface: the gate asserts the full exported okf
// top-level identifier set equals this map — no more (over-export ⇒ RED) and no
// less (missing MUST ⇒ RED). The DEFER and KEEP-INTERNAL sets are excluded because
// they are NOT part of the surface Phase 6 publishes; anything okf exports beyond
// this map fails the gate on the one-way publish door.
var okfMustVocabulary = map[string]bool{
	// direct seam (referenced by the service core and/or the adapters)
	"Codec":              true,
	"Bundle":             true,
	"SpecVersion":        true,
	"DefaultSpecVersion": true,
	"IsValidISODate":     true,
	"IsValidActor":       true,
	"ProtectedTrustKeys": true,
	// transitive MUST (dragged in by Bundle's exported fields)
	"Concept":         true,
	"Link":            true,
	"TrustSignals":    true,
	"UnparsedConcept": true,
	"OrderedMap":      true,
	// NewOrderedMap is DEFERRED, not exported (OQ2 owner ruling): OrderedMap is
	// public, its constructor is not. Callers use &okf.OrderedMap{} or the
	// internal okfrules helper. Dropped from the guarded MUST set accordingly.
	"Source":     true,
	"Actorstamp": true,
	"DateRange":  true,
	"Span":       true,
}

// packageSurface parses every non-test .go file in dir and renders a normalized,
// sorted list of exported-declaration surface entries. When want is nil the whole
// exported surface is rendered (used for pkg/binder, published verbatim, and for
// pkg/okf, whose full surface must equal the MUST set); when want is non-nil only
// declarations whose owning name is a key of want are rendered. Regardless of want,
// the second return value is the set of ALL exported TOP-LEVEL identifier names
// (types, funcs, vars, consts — not methods) present in the package, so a caller
// can assert set-equality against an expected surface (the okf over-export guard).
// Comments are dropped (parse mode 0) so the rendering is a pure function of the
// API shape, not its prose.
func packageSurface(t *testing.T, dir string, want map[string]bool) ([]string, map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package dir %s: %v", dir, err)
	}
	names := map[string]bool{}
	var out []string
	// Group methods by receiver base type so a type's method set renders with it.
	type typeSpec struct {
		name string
		spec *ast.TypeSpec
	}
	var typeSpecs []typeSpec
	methodsByRecv := map[string][]*ast.FuncDecl{}
	var funcs []*ast.FuncDecl
	var valueDecls []*ast.GenDecl

	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".go") || strings.HasSuffix(de.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, de.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", de.Name(), err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv != nil && len(d.Recv.List) == 1 {
					methodsByRecv[recvBaseName(d.Recv.List[0].Type)] = append(
						methodsByRecv[recvBaseName(d.Recv.List[0].Type)], d)
				} else {
					funcs = append(funcs, d)
					names[d.Name.Name] = true // exported top-level func
				}
			case *ast.GenDecl:
				switch d.Tok {
				case token.TYPE:
					for _, s := range d.Specs {
						ts := s.(*ast.TypeSpec)
						if ts.Name.IsExported() {
							typeSpecs = append(typeSpecs, typeSpec{ts.Name.Name, ts})
							names[ts.Name.Name] = true // exported top-level type
						}
					}
				case token.VAR, token.CONST:
					valueDecls = append(valueDecls, d)
					for _, s := range d.Specs {
						vs := s.(*ast.ValueSpec)
						for _, n := range vs.Names {
							if n.IsExported() {
								names[n.Name] = true // exported top-level var/const
							}
						}
					}
				}
			}
		}
	}

	include := func(name string) bool {
		return want == nil || want[name]
	}

	// Types (with their method sets rendered inline, sorted).
	for _, ts := range typeSpecs {
		if !include(ts.name) {
			continue
		}
		out = append(out, renderType(fset, ts.spec, methodsByRecv[ts.name], want, names))
	}
	// Top-level funcs.
	for _, fn := range funcs {
		if !include(fn.Name.Name) {
			continue
		}
		out = append(out, "func "+fn.Name.Name+renderFuncSig(fset, fn.Type))
	}
	// Exported vars and consts.
	for _, gd := range valueDecls {
		kw := "var"
		if gd.Tok == token.CONST {
			kw = "const"
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			for _, name := range vs.Names {
				if !name.IsExported() || !include(name.Name) {
					continue
				}
				line := kw + " " + name.Name
				if vs.Type != nil {
					line += " " + printNode(fset, vs.Type)
				}
				out = append(out, line)
			}
		}
	}

	sort.Strings(out)
	return out, names
}

// renderType renders a type declaration's exported surface: struct fields
// (exported only, sorted), interface method sets (sorted), or a defined type's
// underlying expression, followed by the type's own exported method set (sorted).
func renderType(fset *token.FileSet, ts *ast.TypeSpec, methods []*ast.FuncDecl, want, found map[string]bool) string {
	var b strings.Builder
	switch u := ts.Type.(type) {
	case *ast.StructType:
		b.WriteString("type " + ts.Name.Name + " struct {")
		var fields []string
		for _, f := range u.Fields.List {
			ftype := printNode(fset, f.Type)
			tag := ""
			if f.Tag != nil {
				tag = " " + f.Tag.Value
			}
			if len(f.Names) == 0 { // embedded
				fields = append(fields, ftype+tag)
				continue
			}
			for _, n := range f.Names {
				if n.IsExported() {
					fields = append(fields, n.Name+" "+ftype+tag)
				}
			}
		}
		sort.Strings(fields)
		b.WriteString(strings.Join(fields, "; "))
		b.WriteString("}")
	case *ast.InterfaceType:
		b.WriteString("type " + ts.Name.Name + " interface {")
		var ms []string
		for _, m := range u.Methods.List {
			if ft, ok := m.Type.(*ast.FuncType); ok && len(m.Names) == 1 {
				ms = append(ms, m.Names[0].Name+renderFuncSig(fset, ft))
			} else { // embedded interface
				ms = append(ms, printNode(fset, m.Type))
			}
		}
		sort.Strings(ms)
		b.WriteString(strings.Join(ms, "; "))
		b.WriteString("}")
	default:
		b.WriteString("type " + ts.Name.Name + " " + printNode(fset, ts.Type))
	}
	// Exported methods on this type, rendered under it (sorted, deterministic).
	var mlines []string
	for _, m := range methods {
		if m.Name.IsExported() {
			mlines = append(mlines, "\n  func ("+printNode(fset, m.Recv.List[0].Type)+") "+m.Name.Name+renderFuncSig(fset, m.Type))
		}
	}
	sort.Strings(mlines)
	for _, ml := range mlines {
		b.WriteString(ml)
	}
	return b.String()
}

// renderFuncSig renders a function/method signature (params + results) without
// the leading "func" keyword, so a name can be prefixed uniformly.
func renderFuncSig(fset *token.FileSet, ft *ast.FuncType) string {
	return strings.TrimPrefix(printNode(fset, ft), "func")
}

// render prints an AST node with go/printer in a fixed configuration, collapsing
// whitespace so the result is a stable single-line rendering.
func printNode(fset *token.FileSet, node any) string {
	var b strings.Builder
	cfg := printer.Config{Mode: printer.RawFormat}
	if err := cfg.Fprint(&b, fset, node); err != nil {
		return "<render-error:" + err.Error() + ">"
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// recvBaseName returns the base type name of a method receiver (stripping a
// leading pointer), so methods group with their type.
func recvBaseName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return recvBaseName(e.X)
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr: // generic receiver Foo[T]
		return recvBaseName(e.X)
	case *ast.IndexListExpr:
		return recvBaseName(e.X)
	}
	return ""
}
