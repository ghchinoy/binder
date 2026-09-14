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
// The evaluator below implements only the operator subset the expression uses,
// and models GitHub's semantics for that subset — including its CASE-INSENSITIVE
// string comparison, which is the one place a plausible-looking Go
// implementation (`==`, strings.HasPrefix) gives a confident WRONG answer rather
// than an error. Everything outside the subset is an error: an expression written
// in syntax it cannot parse fails the job and NAMES the evaluator as the thing to
// extend, the same drift-detection discipline mustChange gives the mutators in
// docsimpact_test.go. Silence is never an option — an unparseable or unmodelled
// expression is a red, not a skip.
//
// The one gap that cannot be closed from inside this repository: the
// `release-please--branches--` prefix is release-please's own naming convention.
// The workflow literal and exprReleaseRefPrefix below are COUPLED to it and to
// each other — if release-please renames the prefix upstream, both must be
// changed together, and until they are this control stays green while the gate
// silently stops exempting the release PR. Nothing in the tree can detect that;
// the next red release PR is the signal.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	// exGateWorkflowPath is the artifact under test: the workflow whose `if:`
	// expression decides whether the docs-impact gate runs.
	exGateWorkflowPath = "../../.github/workflows/docs-impact.yml"
	exGateJobName      = "docs-impact"

	// exReleaseWorkflowPath and exGoModPath are read only to DERIVE the PR shapes
	// the controls are written against, never to decide the verdict.
	exReleaseWorkflowPath = "../../.github/workflows/release-please.yml"
	exGoModPath           = "../../go.mod"
	exModuleHostPrefix    = "github.com/"

	// exReleaseRefPrefix is release-please's own branch-naming convention. It is
	// EXTERNAL to this repository, so it cannot be derived from anything in the
	// tree; it is confirmed by every release PR the project has opened (#92, #122,
	// #170, #179, #184 — head ref `release-please--branches--main`).
	//
	// COUPLED LITERAL: the same prefix appears in the `if:` expression in
	// .github/workflows/docs-impact.yml. If release-please renames it upstream,
	// BOTH must change together — changing only the workflow reds this control
	// (good), but leaving both stale keeps this control green while the gate stops
	// exempting release PRs (the #127 failure, returning silently). There is no
	// in-repo detector for that; it is recorded here and at the workflow literal so
	// whoever touches either finds the other.
	exReleaseRefPrefix = "release-please--branches--"

	// exReleasePRUserType is what GitHub reports for a PAT-authored release PR —
	// the whole reason the bot clause could not reach it (#184: is_bot=false).
	exReleasePRUserType = "User"

	// exDefaultBranchFallback is used when the release target branch cannot be
	// derived from release-please.yml. The derivation is a convenience, not a
	// control: the exemption matches on the PREFIX, so the branch that follows it
	// cannot change any verdict here.
	exDefaultBranchFallback = "main"
)

// ---------------------------------------------------------------------------
// Reading the artifact under test
// ---------------------------------------------------------------------------

func exReadFile(t *testing.T, path string) []byte {
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

// exGateCondition returns the docs-impact job's `if:` expression, with the
// `${{ }}` wrapper stripped if one is present (GitHub allows both forms in an
// `if:`). A missing job or a missing condition is a hard failure: an exemption
// that has silently disappeared must not read as "nothing to check".
func exGateCondition(t *testing.T) string {
	t.Helper()

	var wf struct {
		Jobs map[string]struct {
			If string `yaml:"if"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(exReadFile(t, exGateWorkflowPath), &wf); err != nil {
		t.Fatalf("cannot parse %s as YAML: %v", exGateWorkflowPath, err)
	}

	job, ok := wf.Jobs[exGateJobName]
	if !ok {
		t.Fatalf("workflow drift: %s has no job %q — update exGateJobName in "+
			"exemption_test.go to the job that runs the gate.", exGateWorkflowPath, exGateJobName)
	}
	cond := strings.TrimSpace(job.If)
	if cond == "" {
		t.Fatalf("job %q in %s has no `if:` condition: the docs-impact gate now "+
			"runs on every pull request, including bot and release PRs. If that is "+
			"intended, this control must be deleted deliberately, not left to pass "+
			"vacuously (issue #127).", exGateJobName, exGateWorkflowPath)
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

// exReleaseTargetBranch derives the branch release-please tracks from
// release-please.yml's push trigger, so the release head ref these controls use
// reads like the real one.
//
// It is BEST EFFORT by design. The exemption matches on the prefix, so the branch
// after it cannot change a verdict — retarget release-please and every case here
// still decides the same way. Failing the exemption control over a release-please
// config change that provably cannot affect the exemption would be a false red
// aimed at a maintainer, i.e. the cry-wolf pattern #127 is itself about. So an
// underivable target branch logs and falls back.
func exReleaseTargetBranch(t *testing.T) string {
	t.Helper()

	fallback := func(reason string) string {
		t.Logf("using the default branch %q for the release head ref: %s. This "+
			"cannot affect any verdict here — the exemption matches on the %q "+
			"prefix, not on the branch after it.",
			exDefaultBranchFallback, reason, exReleaseRefPrefix)
		return exDefaultBranchFallback
	}

	b, err := os.ReadFile(filepath.Clean(exReleaseWorkflowPath))
	if err != nil {
		return fallback(fmt.Sprintf("cannot read %s: %v", exReleaseWorkflowPath, err))
	}
	var wf struct {
		On struct {
			Push struct {
				Branches []string `yaml:"branches"`
			} `yaml:"push"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(b, &wf); err != nil {
		return fallback(fmt.Sprintf("cannot parse %s: %v", exReleaseWorkflowPath, err))
	}
	if len(wf.On.Push.Branches) != 1 {
		return fallback(fmt.Sprintf("%s triggers on %v, not exactly one branch",
			exReleaseWorkflowPath, wf.On.Push.Branches))
	}
	return wf.On.Push.Branches[0]
}

// exRepoSlug derives "owner/repo" — the value GitHub puts in `github.repository`
// — from the Go module path, the one place in the tree that already names the
// repository. This one IS load-bearing: it is the value the same-repository
// clause compares against, so it must be right.
func exRepoSlug(t *testing.T) string {
	t.Helper()

	for _, line := range strings.Split(string(exReadFile(t, exGoModPath)), "\n") {
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		mod := strings.TrimSpace(strings.TrimPrefix(line, "module "))
		if !strings.HasPrefix(mod, exModuleHostPrefix) {
			t.Fatalf("cannot derive the repository slug: module path %q is not "+
				"hosted at %s; update exRepoSlug in exemption_test.go.", mod, exModuleHostPrefix)
		}
		return strings.TrimPrefix(mod, exModuleHostPrefix)
	}
	t.Fatalf("no module line in %s", exGoModPath)
	return ""
}

// ---------------------------------------------------------------------------
// The PR shapes
// ---------------------------------------------------------------------------

// exPRShape is one PR the gate either runs on or is exempt from, expressed as the
// pieces of the `github` context an exemption may legitimately key on.
type exPRShape struct {
	userType   string // github.event.pull_request.user.type
	userLogin  string // github.event.pull_request.user.login
	headRef    string // github.event.pull_request.head.ref
	headRepo   string // github.event.pull_request.head.repo.full_name
	title      string // github.event.pull_request.title
	repository string // github.repository
	baseRef    string // github.event.pull_request.base.ref
	eventName  string // github.event_name
	defaultBr  string // github.event.repository.default_branch
}

// context builds the nested map the evaluator resolves paths against. Every
// field a plausible exemption could read is populated, so an expression that
// keys on something these controls do not model is caught as an unresolved path
// rather than silently evaluating to null.
func (p exPRShape) context() map[string]any {
	return map[string]any{
		"github": map[string]any{
			"repository": p.repository,
			"event_name": p.eventName,
			"ref_name":   p.headRef,
			"actor":      p.userLogin,
			"event": map[string]any{
				"repository": map[string]any{
					"full_name":      p.repository,
					"default_branch": p.defaultBr,
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

// exPRShapes builds the PR shapes the controls are written against, keyed by
// name. Shared by TestReleaseExemption (which evaluates the LIVE expression
// against them) and TestEvaluatorControls (which evaluates expressions this file
// owns against them, to prove the evaluator would catch a widened exemption).
func exPRShapes(t *testing.T) map[string]exPRShape {
	t.Helper()

	slug := exRepoSlug(t)
	owner, repo, ok := strings.Cut(slug, "/")
	if !ok {
		t.Fatalf("derived repository slug %q is not owner/repo", slug)
	}
	branch := exReleaseTargetBranch(t)
	releaseRef := exReleaseRefPrefix + branch
	outsideFork := "outside-contributor/" + repo

	base := exPRShape{
		userType:   "User",
		userLogin:  owner,
		headRef:    "feat/some-change",
		headRepo:   slug,
		title:      "feat: add a flag",
		repository: slug,
		baseRef:    branch,
		eventName:  "pull_request",
		defaultBr:  branch,
	}
	with := func(mut func(*exPRShape)) exPRShape {
		p := base
		mut(&p)
		return p
	}

	return map[string]exPRShape{
		"release-pr": with(func(p *exPRShape) {
			p.userType = exReleasePRUserType // PAT-authored, so NOT "Bot"
			p.headRef = releaseRef
			p.title = "chore: release " + branch
		}),
		"normal-code-pr": base,
		"fork-code-pr": with(func(p *exPRShape) {
			p.headRepo = outsideFork
			p.headRef = "patch-1"
		}),
		"forged-fork-release-ref": with(func(p *exPRShape) {
			p.headRepo = outsideFork
			p.headRef = releaseRef // a fork may name its branch anything
			p.title = "chore: release " + branch
		}),
		"bot-pr": with(func(p *exPRShape) {
			p.userType = "Bot"
			p.userLogin = "dependabot[bot]"
			p.headRef = "dependabot/go_modules/example-1.2.3"
			p.title = "chore(deps): bump example from 1.2.2 to 1.2.3"
		}),
		"dependabot-actions-pr": with(func(p *exPRShape) {
			p.userType = "Bot" // set server-side by GitHub; unforgeable
			p.userLogin = "dependabot[bot]"
			p.headRef = "dependabot/github_actions/actions/checkout-5"
			p.title = "chore(deps): bump actions/checkout from 4 to 5"
		}),
		// A bot that is NOT Dependabot. Both Dependabot shapes above share one
		// login, so on their own they would pin "Dependabot is exempt" rather than
		// "bots are exempt" — an exemption narrowed to a dependabot login test
		// would keep them green. This shape is what makes the controls read
		// user.type, which is the property GitHub sets server-side.
		"other-bot-pr": with(func(p *exPRShape) {
			p.userType = "Bot"
			p.userLogin = "github-actions[bot]"
			p.headRef = "chore/automated-housekeeping"
			p.title = "chore: automated housekeeping"
		}),
	}
}

// ---------------------------------------------------------------------------
// The controls
// ---------------------------------------------------------------------------

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
	cond := exGateCondition(t)
	shapes := exPRShapes(t)

	tests := []struct {
		name    string
		wantRun bool
		why     string
	}{
		{
			name:    "release-pr",
			wantRun: false,
			why: "the release PR is authored by a PAT (type \"User\"), so the bot " +
				"clause cannot reach it; without a clause that matches the release " +
				"branch the gate reds every release — issue #127, live on PR #184",
		},
		{
			name:    "normal-code-pr",
			wantRun: true,
			why: "a normal human code PR from this repo must still be gated; an " +
				"exemption that skips it has widened into \"skip docs-impact whenever " +
				"it is inconvenient\"",
		},
		{
			name:    "fork-code-pr",
			wantRun: true,
			why:     "a fork PR is a normal code PR and must be gated like any other",
		},
		{
			name:    "forged-fork-release-ref",
			wantRun: true,
			why: "head.ref is the branch name on the HEAD repo, so an outside " +
				"contributor can name a fork branch anything; the release exemption " +
				"must additionally require the PR to come FROM this repository, or it " +
				"is an opt-out available to anyone",
		},
		{
			name:    "bot-pr",
			wantRun: false,
			why: "the original bot exemption must survive the #127 fix: a gate that " +
				"nags bots gets disabled within a week",
		},
		{
			name:    "dependabot-actions-pr",
			wantRun: false,
			why: "the `user.type != 'Bot'` clause is LOAD-BEARING for Dependabot and " +
				"must only ever be ANDed onto, never replaced: Dependabot's PRs report " +
				"type \"Bot\" server-side (unforgeable), and their bodies carry no " +
				"\"## Docs impact\" section, so the moment the bot clause goes the gate " +
				"fails every dependency bump — the nagged-bot failure the workflow's own " +
				"comment predicts. Pinned as its own shape because the SHA-pinning work " +
				"that makes Dependabot open github_actions PRs here is landing in the " +
				"same batch as this fix",
		},
		{
			name:    "other-bot-pr",
			wantRun: false,
			why: "the exemption must key on `user.type`, not on a bot's identity: " +
				"with only dependabot-logged shapes above, narrowing the clause to a " +
				"`user.login != 'dependabot[bot]'` test would keep this suite green, " +
				"and would then red the first PR any other bot opens — nothing in this " +
				"repo opens PRs as github-actions[bot] today, so the narrowing and the " +
				"breakage would be separated by however long that takes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pr, ok := shapes[tc.name]
			if !ok {
				t.Fatalf("no PR shape named %q in exPRShapes", tc.name)
			}
			run, err := exEvalCondition(cond, pr.context())
			if err != nil {
				t.Fatalf("cannot evaluate the docs-impact `if:` expression from %s "+
					"against the %q shape: %v\nThis control evaluates the expression "+
					"rather than matching its text, so an expression using syntax or a "+
					"context path it does not model is a RED, never a skip: extend the "+
					"evaluator (or exPRShape.context) in exemption_test.go to cover it.\n"+
					"--- expression ---\n%s", exGateWorkflowPath, tc.name, err, cond)
			}
			if run != tc.wantRun {
				verb := map[bool]string{true: "RUNS on", false: "is EXEMPT from"}
				t.Fatalf("docs-impact %s the %q PR, but it must %s it: %s.\n"+
					"--- expression ---\n%s\n--- PR shape ---\n%+v",
					verb[run], tc.name, verb[tc.wantRun], tc.why, cond, pr)
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
//
// STRING COMPARISON IS CASE-INSENSITIVE, matching GitHub's expression reference:
// "GitHub ignores case when comparing strings", and startsWith/endsWith/contains
// are each documented "This function is not case sensitive." Using Go's `==` and
// strings.HasPrefix here would not merely be imprecise — it would return a
// confident WRONG answer in the widening direction, letting an exemption that
// GitHub honours (e.g. one keyed on 'FEAT/' or on an upper-cased fork slug) pass
// this suite green. TestEvaluatorControls pins both halves; do not "simplify"
// these back to case-sensitive comparisons.
//
// The folding is strings.ToLower on both sides in BOTH places, never
// strings.EqualFold in one and ToLower in the other: the two rules differ (simple
// case folding is more permissive than lowercasing), and one expression must not
// have its halves folded by different rules. CONTEXT PATH RESOLUTION IS NOT
// FOLDED — see resolve.

type exToken struct {
	kind string // "op", "str", "word"
	text string
}

func exLex(src string) ([]exToken, error) {
	var toks []exToken
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
			toks = append(toks, exToken{"str", sb.String()})
		case strings.HasPrefix(src[i:], "&&"), strings.HasPrefix(src[i:], "||"),
			strings.HasPrefix(src[i:], "=="), strings.HasPrefix(src[i:], "!="):
			toks = append(toks, exToken{"op", src[i : i+2]})
			i += 2
		case c == '(' || c == ')' || c == ',' || c == '!':
			toks = append(toks, exToken{"op", string(c)})
			i++
		case exIsWordByte(c):
			j := i
			for j < len(src) && exIsWordByte(src[j]) {
				j++
			}
			toks = append(toks, exToken{"word", src[i:j]})
			i = j
		default:
			return nil, fmt.Errorf("unsupported character %q at offset %d", c, i)
		}
	}
	return toks, nil
}

func exIsWordByte(c byte) bool {
	return c == '_' || c == '-' || c == '.' || c == '*' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// exValue is either a bool or a string; the evaluator refuses to coerce between
// them, so a nonsensical expression errors instead of quietly deciding.
type exValue struct {
	b     bool
	s     string
	isStr bool
}

func exBool(b bool) exValue { return exValue{b: b} }
func exStr(s string) exValue {
	return exValue{s: s, isStr: true}
}

func (v exValue) String() string {
	if v.isStr {
		return "'" + v.s + "'"
	}
	return fmt.Sprintf("%t", v.b)
}

// exAsBool refuses GitHub's truthiness coercion: a string where a condition
// belongs is an expression this evaluator does not model, and must be reported
// rather than guessed at.
func exAsBool(v exValue) (bool, error) {
	if v.isStr {
		return false, fmt.Errorf("expected a condition, got the string %s", v)
	}
	return v.b, nil
}

type exParser struct {
	toks []exToken
	pos  int
	ctx  map[string]any
}

// exEvalCondition evaluates a workflow `if:` expression against a context, and
// reports whether the job RUNS (true) or is exempt (false).
func exEvalCondition(expr string, ctx map[string]any) (bool, error) {
	toks, err := exLex(expr)
	if err != nil {
		return false, err
	}
	p := &exParser{toks: toks, ctx: ctx}
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

func (p *exParser) peek() (exToken, bool) {
	if p.pos >= len(p.toks) {
		return exToken{}, false
	}
	return p.toks[p.pos], true
}

func (p *exParser) accept(kind, text string) bool {
	t, ok := p.peek()
	if !ok || t.kind != kind || t.text != text {
		return false
	}
	p.pos++
	return true
}

func (p *exParser) parseOr() (exValue, error) {
	left, err := p.parseAnd()
	if err != nil {
		return exValue{}, err
	}
	for p.accept("op", "||") {
		right, err := p.parseAnd()
		if err != nil {
			return exValue{}, err
		}
		lb, err := exAsBool(left)
		if err != nil {
			return exValue{}, err
		}
		rb, err := exAsBool(right)
		if err != nil {
			return exValue{}, err
		}
		left = exBool(lb || rb)
	}
	return left, nil
}

func (p *exParser) parseAnd() (exValue, error) {
	left, err := p.parseCmp()
	if err != nil {
		return exValue{}, err
	}
	for p.accept("op", "&&") {
		right, err := p.parseCmp()
		if err != nil {
			return exValue{}, err
		}
		lb, err := exAsBool(left)
		if err != nil {
			return exValue{}, err
		}
		rb, err := exAsBool(right)
		if err != nil {
			return exValue{}, err
		}
		left = exBool(lb && rb)
	}
	return left, nil
}

func (p *exParser) parseCmp() (exValue, error) {
	left, err := p.parseUnary()
	if err != nil {
		return exValue{}, err
	}
	for _, op := range []string{"==", "!="} {
		if !p.accept("op", op) {
			continue
		}
		right, err := p.parseUnary()
		if err != nil {
			return exValue{}, err
		}
		if left.isStr != right.isStr {
			return exValue{}, fmt.Errorf("cannot compare %s with %s: this evaluator "+
				"does not model GitHub's type coercion", left, right)
		}
		eq := left.b == right.b
		if left.isStr {
			// "GitHub ignores case when comparing strings" — expressions reference.
			// Lowercase both sides rather than strings.EqualFold: EqualFold applies
			// Unicode simple case folding, which is strictly more permissive than a
			// lowercase comparison (it folds U+017F LATIN SMALL LETTER LONG S onto
			// "s", for one), and parseCall already lowercases. One folding rule for
			// both halves of an expression is the point; this is fidelity to GHA, not
			// a hardening measure.
			eq = strings.ToLower(left.s) == strings.ToLower(right.s)
		}
		return exBool(eq == (op == "==")), nil
	}
	return left, nil
}

func (p *exParser) parseUnary() (exValue, error) {
	if p.accept("op", "!") {
		v, err := p.parseUnary()
		if err != nil {
			return exValue{}, err
		}
		b, err := exAsBool(v)
		if err != nil {
			return exValue{}, err
		}
		return exBool(!b), nil
	}
	return p.parsePrimary()
}

func (p *exParser) parsePrimary() (exValue, error) {
	t, ok := p.peek()
	if !ok {
		return exValue{}, fmt.Errorf("unexpected end of expression")
	}

	if p.accept("op", "(") {
		v, err := p.parseOr()
		if err != nil {
			return exValue{}, err
		}
		if !p.accept("op", ")") {
			return exValue{}, fmt.Errorf("missing closing parenthesis")
		}
		return v, nil
	}

	switch t.kind {
	case "str":
		p.pos++
		return exStr(t.text), nil
	case "word":
		p.pos++
		if p.accept("op", "(") {
			return p.parseCall(t.text)
		}
		return p.resolve(t.text)
	}
	return exValue{}, fmt.Errorf("unexpected token %q", t.text)
}

func (p *exParser) parseCall(name string) (exValue, error) {
	var args []exValue
	for {
		v, err := p.parseOr()
		if err != nil {
			return exValue{}, err
		}
		args = append(args, v)
		if p.accept("op", ",") {
			continue
		}
		if p.accept("op", ")") {
			break
		}
		return exValue{}, fmt.Errorf("malformed argument list for %s()", name)
	}

	if len(args) != 2 || !args[0].isStr || !args[1].isStr {
		return exValue{}, fmt.Errorf("%s() is modelled only as (string, string)", name)
	}
	// "This function is not case sensitive." — expressions reference, on each of
	// startsWith, endsWith and contains.
	a, b := strings.ToLower(args[0].s), strings.ToLower(args[1].s)
	switch name {
	case "startsWith":
		return exBool(strings.HasPrefix(a, b)), nil
	case "endsWith":
		return exBool(strings.HasSuffix(a, b)), nil
	case "contains":
		return exBool(strings.Contains(a, b)), nil
	}
	return exValue{}, fmt.Errorf("function %s() is not modelled by this evaluator", name)
}

// resolve looks up a dotted context path. Literals `true`/`false` are the only
// bare words that are not paths. An unresolved path is an ERROR, not null: it
// means either the workflow reads something these controls do not model, or it
// reads a path that does not exist in a real event — and both must be seen.
//
// Path lookup is deliberately CASE-SENSITIVE, unlike the string comparison in
// parseCmp and the functions in parseCall. Property dereference is a different
// surface from string equality: GitHub evaluates a mistyped path to null rather
// than matching it case-insensitively, so folding case here would swallow exactly
// the typo this control exists to surface.
func (p *exParser) resolve(path string) (exValue, error) {
	switch path {
	case "true":
		return exBool(true), nil
	case "false":
		return exBool(false), nil
	}

	var cur any = p.ctx
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return exValue{}, fmt.Errorf("context path %q: %q is not an object", path, seg)
		}
		cur, ok = m[seg]
		if !ok {
			return exValue{}, fmt.Errorf("context path %q is not modelled by these "+
				"controls (missing segment %q)", path, seg)
		}
	}
	switch v := cur.(type) {
	case string:
		return exStr(v), nil
	case bool:
		return exBool(v), nil
	}
	return exValue{}, fmt.Errorf("context path %q holds an unmodelled value type", path)
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

		// Case fidelity. GitHub's expression reference: "GitHub ignores case when
		// comparing strings", and startsWith/endsWith/contains are each "not case
		// sensitive". A case-SENSITIVE evaluator answers all four of these the
		// other way — confidently and wrongly, in the direction that hides a
		// widened exemption (see TestWidenedExemptionsAreCaught).
		{"github.event.pull_request.user.type == 'USER'", true},
		{"github.event.pull_request.user.type != 'user'", false},
		{"startsWith(github.event.pull_request.head.ref, 'RELEASE-PLEASE--')", true},
		{"endsWith(github.event.pull_request.head.ref, 'MAIN')", true},
		{"contains(github.repository, 'GHCHINOY')", true},
	}
	for _, tc := range cases {
		got, err := exEvalCondition(tc.expr, ctx)
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
		if _, err := exEvalCondition(expr, ctx); err == nil {
			t.Fatalf("evaluator accepted %q but must report it as unmodelled", expr)
		}
	}
}

// TestWidenedExemptionsAreCaught pins the CONSEQUENCE of the case-fidelity cases
// above, using the two widened exemptions found in review of PR #188. Both are
// expressions GitHub would honour as a real widening of the gate's exemption, and
// both slipped past a case-SENSITIVE evaluator with the whole suite green.
//
// The expressions belong to this file, not to the workflow: each is evaluated
// against the shape it must NOT exempt, and must come out exempt (false) — which
// is exactly what would make TestReleaseExemption red if anyone wrote it into
// docs-impact.yml. That keeps the fidelity claim pinned by a behaviour, not by a
// comment citing the docs.
func TestWidenedExemptionsAreCaught(t *testing.T) {
	shapes := exPRShapes(t)

	tests := []struct {
		name  string
		expr  string
		shape string
		why   string
	}{
		{
			name: "M11-uppercase-feature-branch-skip",
			expr: "github.event.pull_request.user.type != 'Bot'" +
				" && !(startsWith(github.event.pull_request.head.ref, 'release-please--branches--')" +
				" && github.event.pull_request.head.repo.full_name == github.repository)" +
				" && !startsWith(github.event.pull_request.head.ref, 'FEAT/')",
			shape: "normal-code-pr",
			why: "on GitHub this skips the gate on every feat/… branch, because " +
				"startsWith is not case sensitive",
		},
		{
			name: "M12-uppercase-fork-opt-out",
			expr: "github.event.pull_request.user.type != 'Bot'" +
				" && !(startsWith(github.event.pull_request.head.ref, 'release-please--branches--')" +
				" && (github.event.pull_request.head.repo.full_name == github.repository" +
				" || github.event.pull_request.head.repo.full_name == 'OUTSIDE-CONTRIBUTOR/binder'))",
			shape: "forged-fork-release-ref",
			why: "on GitHub this restores the fork opt-out the same-repository clause " +
				"exists to close, because string comparison ignores case",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pr, ok := shapes[tc.shape]
			if !ok {
				t.Fatalf("no PR shape named %q in exPRShapes", tc.shape)
			}
			run, err := exEvalCondition(tc.expr, pr.context())
			if err != nil {
				t.Fatalf("cannot evaluate the %s expression: %v", tc.name, err)
			}
			if run {
				t.Fatalf("the evaluator says docs-impact still RUNS on the %q shape "+
					"under the widened expression %s, so this widening would pass "+
					"TestReleaseExemption green: %s. The evaluator has drifted back to "+
					"case-SENSITIVE comparison — see the case-fidelity note on parseCmp "+
					"and parseCall.\n--- expression ---\n%s",
					tc.shape, tc.name, tc.why, tc.expr)
			}
		})
	}
}
