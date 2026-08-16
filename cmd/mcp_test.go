package cmd

import (
	"regexp"
	"testing"

	mcpserver "github.com/ghchinoy/binder/internal/mcp"
	"github.com/ghchinoy/binder/internal/okf/native"
)

// TestMCPLongEnumeratesAllTools is the regression guard for Defect #1: the
// `binder mcp` Long help enumerates the tool set, and that enumeration must
// cover every registered tool. The bug that shipped was registered-set == 7
// while the prose named 5; TestListTools pins the registered set but says
// nothing about the prose, so it passed straight through that bug. This couples
// the two sides that drifted — and only those two: it asserts each registered
// tool name (from mcpserver.ToolNames, pinned to newServer by TestListTools)
// appears in Long. It pins no wording, ordering, or phrasing, so an innocuous
// help edit will not trip it; adding a tool forces its name into Long.
//
// The match is word-boundary anchored, applied UNIFORMLY to every name — never a
// plain substring. "graph" is a substring of both "list_graphs" and
// "query_graph", so strings.Contains would report the standalone graph tool as
// enumerated whenever either of those appears, leaving graph silently unpinned:
// deleting graph from Long would keep the guard GREEN — the exact silent-prose
// drift this guard exists to catch. Go's \b treats underscore as a word
// character, so `\bgraph\b` does not match inside list_graphs/query_graph, while
// `\blist_graphs\b` etc. still match their own tokens (verified against the SDK's
// regexp/RE2). Handling one name specially would be a test a future reader
// misreads, so the boundary anchor is applied to all seven identically.
func TestMCPLongEnumeratesAllTools(t *testing.T) {
	long := newMCPCmd(native.New()).Long
	if long == "" {
		t.Fatal("mcp command has empty Long help")
	}
	for _, name := range mcpserver.ToolNames() {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		if !re.MatchString(long) {
			t.Errorf("mcp Long help omits registered tool %q (word-boundary match); every registered tool must be enumerated in Long:\n%s", name, long)
		}
	}
}
