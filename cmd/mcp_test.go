package cmd

import (
	"strings"
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
func TestMCPLongEnumeratesAllTools(t *testing.T) {
	long := newMCPCmd(native.New()).Long
	if long == "" {
		t.Fatal("mcp command has empty Long help")
	}
	for _, name := range mcpserver.ToolNames() {
		if !strings.Contains(long, name) {
			t.Errorf("mcp Long help omits registered tool %q; every registered tool must be enumerated in Long:\n%s", name, long)
		}
	}
}
