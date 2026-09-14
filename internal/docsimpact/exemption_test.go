package docsimpact_test

// This file holds the control for the docs-impact job's EXEMPTION — the `if:`
// expression in .github/workflows/docs-impact.yml that decides whether the gate
// runs at all. The matcher controls in docsimpact_test.go answer "does the gate
// judge a body correctly?"; this one answers the question that comes first,
// "does the gate run on this PR?", which is where issue #127 lived: the
// exemption read `user.type != 'Bot'`, and the release PR it was meant to cover
// is authored by a PAT, so it reports `type: "User"` and the gate red-ticked
// every release.
//
// The control evaluates the expression AS WRITTEN IN THE WORKFLOW FILE — it does
// not transcribe, re-state, or pattern-match the expression, because a control
// that asserts "the YAML contains this string" only proves the string was not
// edited, and would pass just as happily against an exemption that had widened
// to cover every PR. Reading the file and evaluating it means BOTH failure
// directions break the build: an exemption that stops covering the release PR
// (the #127 regression) and one that grows to cover a normal code PR (the
// "skip docs-impact whenever it is inconvenient" drift).
//
// This control also answers the objection recorded on #127 against fixing the
// defect in the workflow at all — that "an `if:` expression is only testable by
// cutting a release", where an in-gate rule would be unit-testable. It is
// testable here, on every `make check`, without a release: the exemption stays
// one expression in the file that GitHub actually reads, and the tests that
// judge it run beside the matcher controls.
//
// The evaluator below implements only the operator subset the expression uses.
// That is deliberate: an expression written in syntax it cannot parse fails the
// job and NAMES the evaluator as the thing to extend, the same drift-detection
// discipline mustChange gives the mutators in docsimpact_test.go. Silence is
// never an option — an unparseable or unmodelled expression is a red, not a skip.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	workflowPath      = "../../.github/workflows/docs-impact.yml"
	releaseWorkflow   = "../../.github/workflows/release-please.yml"
	gateJobName       = "docs-impact"
	goModPath         = "../../go.mod"
	moduleHostPrefix  = "github.com/"
	releaseRefPrefix  = "release-please--branches--"
	releasePRUserType = "User" // PAT-authored: #184's author is {"type": "User"}
)

// releaseRefPrefix is release-please's own branch-naming convention, external to
// this repo, so it cannot be derived from anything here; it is confirmed by every
// release PR the project has opened (#92, #122, #184 — head ref
// `release-please--branches--main`). Everything else the controls need IS derived:
// the target branch from release-please.yml, and the repository slug from the Go
// module path. Deriving them keeps the controls truthful if either changes, and
// none of them is read from the expression under test, so the control cannot
// agree with a wrong expression by construction.

// ---------------------------------------------------------------------------
// Reading the artifact under test
// ---------------------------------------------------------------------------

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	if len(b) == 0 {
		t.Fatalf("%s is empty", path)
	}
	return b
}

// gateCondition returns the docs-impact job's `if:` expression, with the
// `${{ }}` wrapper stripped if one is present (GitHub allows both forms in an
// `if:`). A missing job or a missing condition is a hard failure: an exemption
// that has silently disappeared must not read as "nothing to check".
func gateCondition(t *testing.T) string {
	t.Helper()

	var wf struct {
		Jobs map[string]struct {
			If string `yaml:"if"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(readFile(t, workflowPath), &wf); err != nil {
		t.Fatalf("cannot parse %s as YAML: %v", workflowPath, err)
	}

	job, ok := wf.Jobs[gateJobName]
	if !ok {
		t.Fatalf("workflow drift: %s has no job %q — update gateJobName in "+
			"exemption_test.go to the job that runs the gate.", workflowPath, gateJobName)
	}
	cond := strings.TrimSpace(job.If)
	if cond == "" {
		t.Fatalf("job %q in %s has no `if:` condition: the docs-impact gate now "+
			"runs on every pull request, including bot and release PRs. If that is "+
			"intended, this control must be deleted deliberately, not left to pass "+
			"vacuously (issue #127).", gateJobName, workflowPath)
	}
	if strings.HasPrefix(cond, "${{") && strings.HasSuffix(cond, "}}") {
		cond = strings.TrimSpace(cond[3 : len(cond)-2])
	}
	// The expression must reach GitHub as a single line. A folded scalar keeps
	// the newline before any line indented MORE than the first, which would rely
	// on GitHub's expression parser accepting an embedded line break — a detail
	// this control cannot verify and the workflow should not depend on. Indent the
	// continuation lines identically instead.
	if strings.ContainsAny(cond, "\n\r") {
		t.Fatalf("the docs-impact `if:` expression folds to more than one line:\n%q\n"+
			"Indent every continuation line of the folded scalar identically so YAML "+
			"joins them with spaces.", cond)
	}
	return cond
}

// releaseTargetBranch derives the branch release-please tracks from
// release-please.yml's push trigger, so the release head ref the controls use
// follows a retarget instead of going stale.
func releaseTargetBranch(t *testing.T) string {
	t.Helper()

	var wf struct {
		On struct {
			Push struct {
				Branches []string `yaml:"branches"`
			} `yaml:"push"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(readFile(t, releaseWorkflow), &wf); err != nil {
		t.Fatalf("cannot parse %s as YAML: %v", releaseWorkflow, err)
	}
	if len(wf.On.Push.Branches) != 1 {
		t.Fatalf("workflow drift: expected exactly one push branch in %s, got %v — "+
			"the release head ref these controls model is derived from it.",
			releaseWorkflow, wf.On.Push.Branches)
	}
	return wf.On.Push.Branches[0]
}

// repoSlug derives "owner/repo" — the value GitHub puts in `github.repository`
// — from the Go module path, the one place in the tree that already names the
// repository.
func repoSlug(t *testing.T) string {
	t.Helper()

	for _, line := range strings.Split(string(readFile(t, goModPath)), "\n") {
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		mod := strings.TrimSpace(strings.TrimPrefix(line, "module "))
		if !strings.HasPrefix(mod, moduleHostPrefix) {
			t.Fatalf("cannot derive the repository slug: module path %q is not "+
				"hosted at %s; update repoSlug in exemption_test.go.", mod, moduleHostPrefix)
		}
		return strings.TrimPrefix(mod, moduleHostPrefix)
	}
	t.Fatalf("no module line in %s", goModPath)
	return ""
}

// ---------------------------------------------------------------------------
// The controls
// ---------------------------------------------------------------------------

// prShape is one PR the gate either runs on or is exempt from, expressed as the
// pieces of the `github` context an exemption may legitimately key on.
type prShape struct {
	userType     string // github.event.pull_request.user.type
	userLogin    string // github.event.pull_request.user.login
	headRef      string // github.event.pull_request.head.ref
	headRepo     string // github.event.pull_request.head.repo.full_name
	title        string // github.event.pull_request.title
	repository   string // github.repository
	baseRef      string // github.event.pull_request.base.ref
	eventName    string // github.event_name
	defaultBrnch string // github.event.repository.default_branch
}

// context builds the nested map the evaluator resolves paths against. Every
// field a plausible exemption could read is populated, so an expression that
// keys on something these controls do not model is caught as an unresolved path
// rather than silently evaluating to null.
func (p prShape) context() map[string]any {
	return map[string]any{
		"github": map[string]any{
			"repository": p.repository,
			"event_name": p.eventName,
			"ref_name":   p.headRef,
			"actor":      p.userLogin,
			"event": map[string]any{
				"repository": map[string]any{
					"full_name":      p.repository,
					"default_branch": p.defaultBrnch,
				},
				"pull_request": map[string]any{
					"title": p.title,
					"draft": false,
					"user": map[string]any{
						"type":  p.userType,
						"login": p.userLogin,
					},
					"head": map[string]any{
						"ref":  p.headRef,
						"repo": map[string]any{"full_name": p.headRepo},
					},
					"base": map[string]any{
						"ref":  p.baseRef,
						"repo": map[string]any{"full_name": p.repository},
					},
				},
			},
		},
	}
}

// TestReleaseExemption pins the exemption in both directions against the
// expression as it is written in the workflow file.
//
// The load-bearing pair is release-pr (must be exempt — the #127 fix) and
// normal-code-pr (must still run — the fix must not become "skip the gate when
// it is inconvenient"). The forged-fork-release-ref case is why the release
// clause is ANDed with a same-repository check: `head.ref` is the branch name on
// the HEAD repo, which an outside contributor controls, so the branch-name prefix
// on its own would hand every fork an opt-out.
func TestReleaseExemption(t *testing.T) {
	cond := gateCondition(t)
	slug := repoSlug(t)
	releaseRef := releaseRefPrefix + releaseTargetBranch(t)
	base := releaseTargetBranch(t)

	shape := func(mut func(*prShape)) prShape {
		p := prShape{
			userType:     "User",
			userLogin:    strings.SplitN(slug, "/", 2)[0],
			headRef:      "feat/some-change",
			headRepo:     slug,
			title:        "feat: add a flag",
			repository:   slug,
			baseRef:      base,
			eventName:    "pull_request",
			defaultBrnch: base,
		}
		mut(&p)
		return p
	}

	tests := []struct {
		name    string
		pr      prShape
		wantRun bool
		why     string
	}{
		{
			name: "release-pr",
			pr: shape(func(p *prShape) {
				p.userType = releasePRUserType // PAT-authored, so NOT "Bot"
				p.headRef = releaseRef
				p.title = "chore: release main"
			}),
			wantRun: false,
			why: "the release PR is authored by a PAT (type \"User\"), so the bot " +
				"clause cannot reach it; without a clause that matches the release " +
				"branch the gate reds every release — issue #127, live on PR #184",
		},
		{
			name:    "normal-code-pr",
			pr:      shape(func(*prShape) {}),
			wantRun: true,
			why: "a normal human code PR from this repo must still be gated; an " +
				"exemption that skips it has widened into \"skip docs-impact whenever " +
				"it is inconvenient\"",
		},
		{
			name: "fork-code-pr",
			pr: shape(func(p *prShape) {
				p.headRepo = "outside-contributor/" + strings.SplitN(slug, "/", 2)[1]
				p.headRef = "patch-1"
			}),
			wantRun: true,
			why:     "a fork PR is a normal code PR and must be gated like any other",
		},
		{
			name: "forged-fork-release-ref",
			pr: shape(func(p *prShape) {
				p.headRepo = "outside-contributor/" + strings.SplitN(slug, "/", 2)[1]
				p.headRef = releaseRef // a fork may name its branch anything
				p.title = "chore: release main"
			}),
			wantRun: true,
			why: "head.ref is the branch name on the HEAD repo, so an outside " +
				"contributor can name a fork branch anything; the release exemption " +
				"must additionally require the PR to come FROM this repository, or it " +
				"is an opt-out available to anyone",
		},
		{
			name: "bot-pr",
			pr: shape(func(p *prShape) {
				p.userType = "Bot"
				p.userLogin = "dependabot[bot]"
				p.headRef = "dependabot/go_modules/example-1.2.3"
				p.title = "chore(deps): bump example from 1.2.2 to 1.2.3"
			}),
			wantRun: false,
			why: "the original bot exemption must survive the #127 fix: a gate that " +
				"nags bots gets disabled within a week",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run, err := evalCondition(cond, tc.pr.context())
			if err != nil {
				t.Fatalf("cannot evaluate the docs-impact `if:` expression from %s "+
					"against the %q shape: %v\nThis control evaluates the expression "+
					"rather than matching its text, so an expression using syntax or a "+
					"context path it does not model is a RED, never a skip: extend the "+
					"evaluator (or prShape.context) in exemption_test.go to cover it.\n"+
					"--- expression ---\n%s", workflowPath, tc.name, err, cond)
			}
			if run != tc.wantRun {
				verb := map[bool]string{true: "RUNS on", false: "is EXEMPT from"}
				t.Fatalf("docs-impact %s the %q PR, but it must %s it: %s.\n"+
					"--- expression ---\n%s\n--- PR shape ---\n%+v",
					verb[run], tc.name, verb[tc.wantRun], tc.why, cond, tc.pr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A minimal evaluator for the GitHub Actions expression subset used above
// ---------------------------------------------------------------------------
//
// Grammar (precedence low to high), matching GitHub's documented operator
// precedence for the operators supported:
//
//	or      := and ( '||' and )*
//	and     := cmp ( '&&' cmp )*
//	cmp     := unary ( ('=='|'!=') unary )?
//	unary   := '!' unary | primary
//	primary := '(' or ')' | call | string | path
//	call    := ('startsWith'|'endsWith'|'contains') '(' or ',' or ')'
//
// Anything outside it — arithmetic, `format()`, `toJSON()`, array filters — is
// reported as an error, which the caller turns into a failing test.

type token struct {
	kind string // "op", "str", "word"
	text string
}

func lex(src string) ([]token, error) {
	var toks []token
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'':
			// GitHub escapes a single quote inside a literal by doubling it.
			var sb strings.Builder
			i++
			for {
				if i >= len(src) {
					return nil, fmt.Errorf("unterminated string literal")
				}
				if src[i] == '\'' {
					if i+1 < len(src) && src[i+1] == '\'' {
						sb.WriteByte('\'')
						i += 2
						continue
					}
					i++
					break
				}
				sb.WriteByte(src[i])
				i++
			}
			toks = append(toks, token{"str", sb.String()})
		case strings.HasPrefix(src[i:], "&&"), strings.HasPrefix(src[i:], "||"),
			strings.HasPrefix(src[i:], "=="), strings.HasPrefix(src[i:], "!="):
			toks = append(toks, token{"op", src[i : i+2]})
			i += 2
		case c == '(' || c == ')' || c == ',' || c == '!':
			toks = append(toks, token{"op", string(c)})
			i++
		case isWordByte(c):
			j := i
			for j < len(src) && isWordByte(src[j]) {
				j++
			}
			toks = append(toks, token{"word", src[i:j]})
			i = j
		default:
			return nil, fmt.Errorf("unsupported character %q at offset %d", c, i)
		}
	}
	return toks, nil
}

func isWordByte(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == '*' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// value is either a bool or a string; the evaluator refuses to coerce between
// them, so a nonsensical expression errors instead of quietly deciding.
type value struct {
	b     bool
	s     string
	isStr bool
}

func boolVal(b bool) value  { return value{b: b} }
func strVal(s string) value { return value{s: s, isStr: true} }

// asBool refuses GitHub's truthiness coercion: a string where a condition
// belongs is an expression this evaluator does not model, and must be reported
// rather than guessed at.
func asBool(v value) (bool, error) {
	if v.isStr {
		return false, fmt.Errorf("expected a condition, got the string %s", v)
	}
	return v.b, nil
}

func (v value) String() string {
	if v.isStr {
		return "'" + v.s + "'"
	}
	return fmt.Sprintf("%t", v.b)
}

type parser struct {
	toks []token
	pos  int
	ctx  map[string]any
}

// evalCondition evaluates a workflow `if:` expression against a context, and
// reports whether the job RUNS (true) or is exempt (false).
func evalCondition(expr string, ctx map[string]any) (bool, error) {
	toks, err := lex(expr)
	if err != nil {
		return false, err
	}
	p := &parser{toks: toks, ctx: ctx}
	v, err := p.parseOr()
	if err != nil {
		return false, err
	}
	if p.pos != len(p.toks) {
		return false, fmt.Errorf("trailing input at token %d (%q)", p.pos, p.toks[p.pos].text)
	}
	if v.isStr {
		return false, fmt.Errorf("expression evaluated to the string %s, not a condition", v)
	}
	return v.b, nil
}

func (p *parser) peek() (token, bool) {
	if p.pos >= len(p.toks) {
		return token{}, false
	}
	return p.toks[p.pos], true
}

func (p *parser) accept(kind, text string) bool {
	t, ok := p.peek()
	if !ok || t.kind != kind || t.text != text {
		return false
	}
	p.pos++
	return true
}

func (p *parser) parseOr() (value, error) {
	left, err := p.parseAnd()
	if err != nil {
		return value{}, err
	}
	for p.accept("op", "||") {
		right, err := p.parseAnd()
		if err != nil {
			return value{}, err
		}
		lb, err := asBool(left)
		if err != nil {
			return value{}, err
		}
		rb, err := asBool(right)
		if err != nil {
			return value{}, err
		}
		left = boolVal(lb || rb)
	}
	return left, nil
}

func (p *parser) parseAnd() (value, error) {
	left, err := p.parseCmp()
	if err != nil {
		return value{}, err
	}
	for p.accept("op", "&&") {
		right, err := p.parseCmp()
		if err != nil {
			return value{}, err
		}
		lb, err := asBool(left)
		if err != nil {
			return value{}, err
		}
		rb, err := asBool(right)
		if err != nil {
			return value{}, err
		}
		left = boolVal(lb && rb)
	}
	return left, nil
}

func (p *parser) parseCmp() (value, error) {
	left, err := p.parseUnary()
	if err != nil {
		return value{}, err
	}
	for _, op := range []string{"==", "!="} {
		if !p.accept("op", op) {
			continue
		}
		right, err := p.parseUnary()
		if err != nil {
			return value{}, err
		}
		if left.isStr != right.isStr {
			return value{}, fmt.Errorf("cannot compare %s with %s: this evaluator "+
				"does not model GitHub's type coercion", left, right)
		}
		eq := left.b == right.b
		if left.isStr {
			eq = left.s == right.s
		}
		return boolVal(eq == (op == "==")), nil
	}
	return left, nil
}

func (p *parser) parseUnary() (value, error) {
	if p.accept("op", "!") {
		v, err := p.parseUnary()
		if err != nil {
			return value{}, err
		}
		b, err := asBool(v)
		if err != nil {
			return value{}, err
		}
		return boolVal(!b), nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (value, error) {
	t, ok := p.peek()
	if !ok {
		return value{}, fmt.Errorf("unexpected end of expression")
	}

	if p.accept("op", "(") {
		v, err := p.parseOr()
		if err != nil {
			return value{}, err
		}
		if !p.accept("op", ")") {
			return value{}, fmt.Errorf("missing closing parenthesis")
		}
		return v, nil
	}

	switch t.kind {
	case "str":
		p.pos++
		return strVal(t.text), nil
	case "word":
		p.pos++
		if p.accept("op", "(") {
			return p.parseCall(t.text)
		}
		return p.resolve(t.text)
	}
	return value{}, fmt.Errorf("unexpected token %q", t.text)
}

func (p *parser) parseCall(name string) (value, error) {
	var args []value
	for {
		v, err := p.parseOr()
		if err != nil {
			return value{}, err
		}
		args = append(args, v)
		if p.accept("op", ",") {
			continue
		}
		if p.accept("op", ")") {
			break
		}
		return value{}, fmt.Errorf("malformed argument list for %s()", name)
	}

	if len(args) != 2 || !args[0].isStr || !args[1].isStr {
		return value{}, fmt.Errorf("%s() is modelled only as (string, string)", name)
	}
	switch name {
	case "startsWith":
		return boolVal(strings.HasPrefix(args[0].s, args[1].s)), nil
	case "endsWith":
		return boolVal(strings.HasSuffix(args[0].s, args[1].s)), nil
	case "contains":
		return boolVal(strings.Contains(args[0].s, args[1].s)), nil
	}
	return value{}, fmt.Errorf("function %s() is not modelled by this evaluator", name)
}

// resolve looks up a dotted context path. Literals `true`/`false` are the only
// bare words that are not paths. An unresolved path is an ERROR, not null: it
// means either the workflow reads something these controls do not model, or it
// reads a path that does not exist in a real event — and both must be seen.
func (p *parser) resolve(path string) (value, error) {
	switch path {
	case "true":
		return boolVal(true), nil
	case "false":
		return boolVal(false), nil
	}

	var cur any = p.ctx
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return value{}, fmt.Errorf("context path %q: %q is not an object", path, seg)
		}
		cur, ok = m[seg]
		if !ok {
			return value{}, fmt.Errorf("context path %q is not modelled by these "+
				"controls (missing segment %q)", path, seg)
		}
	}
	switch v := cur.(type) {
	case string:
		return strVal(v), nil
	case bool:
		return boolVal(v), nil
	}
	return value{}, fmt.Errorf("context path %q holds an unmodelled value type", path)
}

// TestEvaluatorControls proves the evaluator itself is not vacuous: if it
// returned true (or false) for everything, TestReleaseExemption would be
// meaningless in one direction. These are the evaluator's own red/green pairs,
// written against expressions this file owns — never against the workflow's.
func TestEvaluatorControls(t *testing.T) {
	ctx := map[string]any{"github": map[string]any{
		"event": map[string]any{"pull_request": map[string]any{
			"head": map[string]any{"ref": "release-please--branches--main"},
			"user": map[string]any{"type": "User"},
		}},
		"repository": "ghchinoy/binder",
	}}

	cases := []struct {
		expr string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"!false", true},
		{"true && false", false},
		{"false || true", true},
		{"github.event.pull_request.user.type != 'Bot'", true},
		{"github.event.pull_request.user.type == 'Bot'", false},
		{"startsWith(github.event.pull_request.head.ref, 'release-please--')", true},
		{"startsWith(github.event.pull_request.head.ref, 'feat/')", false},
		{"!(true && false) && (false || true)", true},
		{"github.repository == 'ghchinoy/binder'", true},
	}
	for _, tc := range cases {
		got, err := evalCondition(tc.expr, ctx)
		if err != nil {
			t.Fatalf("evaluator failed on %q: %v", tc.expr, err)
		}
		if got != tc.want {
			t.Fatalf("evaluator returned %t for %q, want %t", got, tc.expr, tc.want)
		}
	}

	// Unsupported input must ERROR, so an expression the evaluator cannot model
	// can never be read as a passing control.
	for _, expr := range []string{
		"format('{0}', github.repository) == 'x'", // unmodelled function
		"github.event.pull_request.nope == 'x'",   // unmodelled context path
		"github.repository == ",                   // malformed
		"'a string on its own'",                   // not a condition
	} {
		if _, err := evalCondition(expr, ctx); err == nil {
			t.Fatalf("evaluator accepted %q but must report it as unmodelled", expr)
		}
	}
}
