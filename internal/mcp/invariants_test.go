package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const brokenLinksCorpus = "../../testdata/corpus-lint-links"

// TestNeverReject: a tool run that produces findings returns them IN the payload
// with IsError=false — findings are never surfaced as MCP tool errors.
func TestNeverReject(t *testing.T) {
	res := callTool(t, "lint", map[string]any{"src": brokenLinksCorpus, "today": fixedToday})
	if res.IsError {
		t.Fatalf("lint with findings must not be a tool error: %s", toolText(t, res))
	}
	payload := toolText(t, res)
	if !strings.Contains(payload, `"broken_links"`) {
		t.Fatalf("expected findings in the payload, got:\n%s", payload)
	}
	// Byte-identical to the CLI, which also returns the report (exit is about the
	// run, not the payload).
	if want := cliJSON(t, "lint", brokenLinksCorpus, "--today", fixedToday, "--json"); payload != want {
		t.Fatalf("never-reject payload not byte-identical to CLI\n--- MCP ---\n%s\n--- CLI ---\n%s", payload, want)
	}
}

// TestNeverFabricateTrust_InvalidActor: an invalid verified_by is a usage-class
// tool error (IsError=true), not a crash and not a silent no-op.
func TestNeverFabricateTrust_InvalidActor(t *testing.T) {
	res := callTool(t, "convert", map[string]any{
		"src":         richCorpus,
		"dry_run":     true,
		"verified_by": "not a valid actor!!",
	})
	if !res.IsError {
		t.Fatalf("invalid verified_by must be a tool error, got success: %s", toolText(t, res))
	}
	if txt := toolText(t, res); !strings.Contains(txt, "invalid actor") {
		t.Fatalf("expected an invalid-actor message, got: %s", txt)
	}
}

// TestNeverFabricateTrust_NoAutoStamp: the server never auto-applies verified_by.
// Convert WITHOUT verified_by is byte-identical to the CLI without --verified-by;
// no stamp is invented. (A stamp only ever appears when explicitly passed.)
func TestNeverFabricateTrust_NoAutoStamp(t *testing.T) {
	got := toolText(t, callTool(t, "convert", map[string]any{"src": richCorpus, "dry_run": true}))
	want := cliJSON(t, "convert", richCorpus, "--dry-run", "--json")
	if got != want {
		t.Fatalf("convert without verified_by diverged from CLI (possible fabricated trust)\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// TestDeterminism_SourceDateEpoch: with SOURCE_DATE_EPOCH set, the default
// `today` is derived identically by the tool and the CLI, so the payloads are
// byte-identical without any explicit today param.
func TestDeterminism_SourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000") // 2023-11-14T22:13:20Z
	got := toolText(t, callTool(t, "review", map[string]any{"bundle": goldenBundle}))
	want := cliJSON(t, "review", goldenBundle, "--json")
	if got != want {
		t.Fatalf("review payload not deterministic under SOURCE_DATE_EPOCH\n--- MCP ---\n%s\n--- CLI ---\n%s", got, want)
	}
}

// fingerprintDir returns a stable hash over every file's relative path and
// bytes under root, so any write/mutation to the bundle changes it.
func fingerprintDir(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel, _ := filepath.Rel(root, p)
		h.Write([]byte(rel))
		h.Write([]byte{0})
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestListGraphsReadOnly is the read-only invariant gate (design §C.3 #2): a
// list_graphs call — including one that passes an id_key, the only new authoring
// surface — leaves the bundle bytes byte-for-byte unchanged. It never writes to
// the bundle, never mutates frontmatter, and never mints an id.
func TestListGraphsReadOnly(t *testing.T) {
	before := fingerprintDir(t, goldenBundle)

	// Plain call.
	if res := callTool(t, "list_graphs", map[string]any{"bundle": goldenBundle, "today": fixedToday}); res.IsError {
		t.Fatalf("list_graphs must not be a tool error on a conformant bundle: %s", toolText(t, res))
	}
	// Call with an id_key (the identity/authoring surface) — still read-only: an
	// absent key must NOT be stamped into any concept's frontmatter.
	if res := callTool(t, "list_graphs", map[string]any{
		"bundle": goldenBundle,
		"today":  fixedToday,
		"id_key": "concept-id",
	}); res.IsError {
		t.Fatalf("list_graphs with id_key must not be a tool error: %s", toolText(t, res))
	}

	if after := fingerprintDir(t, goldenBundle); after != before {
		t.Fatalf("bundle bytes changed after list_graphs (read-only invariant violated)\nbefore=%s\nafter =%s", before, after)
	}
}

// TestQueryGraphReadOnly is the read-only invariant gate for the query tool
// (design §10/§12.2): a call of EVERY op — including one that passes an id_key,
// the only authoring-adjacent surface — leaves the bundle bytes byte-for-byte
// unchanged. It never writes to the bundle, never mutates frontmatter, and never
// mints an id.
//
// This test has teeth: fingerprintDir hashes every file's relative path AND its
// full bytes under the bundle, so ANY write — a new frontmatter key, a stamped
// id, a rewritten file, a created/removed file — changes the digest and fails the
// assertion. I convinced myself of this by construction (the same helper already
// guards list_graphs) and by confirming the query path only ever reads: the verbs
// operate on the in-memory *Model from graph.Build and return copies of its
// Node/Edge values; no code path in internal/graph/query.go or querygraph.go
// opens the bundle for writing. The id_key subcase specifically drives the
// never-mint invariant: "concept-id" is absent from the fixture's frontmatter, so
// if the tool ever tried to persist a minted key the digest would change.
func TestQueryGraphReadOnly(t *testing.T) {
	before := fingerprintDir(t, goldenBundle)

	calls := []map[string]any{
		{"op": "lookup", "id": "tables/orders"},
		{"op": "lookup", "label": "Metric"},
		{"op": "neighbors", "id": "metrics/gross-margin", "direction": "both"},
		{"op": "neighborhood", "id": "metrics/gross-margin", "depth": 3, "direction": "out"},
		{"op": "pattern", "label": "Policy", "to_label": "Metric"},
		{"op": "path", "from": "metrics/gross-margin", "to": "computations/revenue-ytd", "max_depth": 4},
		// The identity/authoring surface: an absent id_key must NOT be stamped.
		{"op": "lookup", "id": "tables/orders", "id_key": "concept-id"},
		{"op": "pattern", "label": "Policy", "to_label": "Metric", "id_key": "concept-id"},
	}
	for _, args := range calls {
		args["bundle"] = goldenBundle
		args["today"] = fixedToday
		if res := callTool(t, "query_graph", args); res.IsError {
			t.Fatalf("query_graph %v must not be a tool error on a conformant bundle: %s", args, toolText(t, res))
		}
	}

	if after := fingerprintDir(t, goldenBundle); after != before {
		t.Fatalf("bundle bytes changed after query_graph (read-only invariant violated)\nbefore=%s\nafter =%s", before, after)
	}
}

// TestUsageError_MissingRequiredParam: a missing required param yields a tool
// error, not a crash.
func TestUsageError_MissingRequiredParam(t *testing.T) {
	res := callTool(t, "validate", map[string]any{}) // no bundle
	if !res.IsError {
		t.Fatalf("missing required param must be a tool error, got success: %s", toolText(t, res))
	}
}
