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

	"gopkg.in/yaml.v3"
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

// TestNeverFabricateTrust_NoAutoStamp is the never-fabricate-trust gate for the
// MCP convert tool: a verified actorstamp appears on a written concept ONLY when
// verified_by is explicitly passed in the tool input; the server never invents
// one. It observes the field the invariant is named for by running REAL converts
// and reading the written concept files.
//
// It deliberately does NOT use --dry-run. The dry-run report carries no trust
// fields at all — the byte-parity of that report is pinned separately by
// TestConvertDryRunParity — so a dry-run comparison can never observe stamping
// and would be structurally incapable of catching a fabricated stamp. That is
// the exact defect this test was rewritten to remove.
//
// The positive control lives INSIDE the assertion (anti-vacuity): an explicit
// verified_by MUST raise the verified-stamp count above the source baseline, or
// the test errors rather than passing. So it cannot silently decay into asserting
// nothing if the write path ever stops stamping.
func TestNeverFabricateTrust_NoAutoStamp(t *testing.T) {
	// The corpus already carries some verified stamps in its source frontmatter;
	// those are legitimately carried forward and are the baseline against which a
	// fabricated stamp shows up.
	//
	// We assert on the STAMPS THEMSELVES — the total number of verified stamps and
	// the set of distinct actors — NOT on the number of files that carry any stamp.
	// A file-count detector is structurally blind to the one fabrication we have
	// actually observed on this project: an actor the human never authorised
	// APPENDED onto a concept that was ALREADY verified. That file already carried
	// a verified stamp, so the file count does not move, and the append goes unseen.
	// Counting the stamps and the actors moves in exactly that case.
	baseActors := verifiedActors(t, richCorpus)
	baseSet := actorSet(baseActors)

	// Positive control / anti-vacuity: an explicit actor DOES stamp, proving the
	// instrument can observe the field it asserts on.
	stampedOut := t.TempDir()
	if res := callTool(t, "convert", map[string]any{
		"src":         richCorpus,
		"out":         stampedOut,
		"verified_by": "human:alice",
	}); res.IsError {
		t.Fatalf("convert with an explicit verified_by must succeed: %s", toolText(t, res))
	}
	stampedActors := verifiedActors(t, stampedOut)
	if len(stampedActors) <= len(baseActors) {
		t.Fatalf("anti-vacuity: explicit verified_by did not raise the total verified-stamp count "+
			"above the source baseline (%d <= %d); the test can no longer observe stamping",
			len(stampedActors), len(baseActors))
	}
	if !actorSet(stampedActors)["human:alice"] {
		t.Fatalf("anti-vacuity: explicit verified_by=human:alice produced no human:alice stamp; "+
			"the instrument cannot observe the actor it asserts on (saw %v)", stampedActors)
	}

	// The invariant: with NO verified_by the server invents nothing. Two claims,
	// each of which a file-count detector misses:
	//   1. the TOTAL stamp count is exactly the source baseline — no stamp is
	//      appended anywhere, INCLUDING onto an already-verified concept; and
	//   2. no actor appears that the source did not already carry — a fabricated
	//      actor is a failure wherever it lands, whether or not the file that
	//      receives it was previously verified.
	plainOut := t.TempDir()
	if res := callTool(t, "convert", map[string]any{
		"src": richCorpus,
		"out": plainOut,
	}); res.IsError {
		t.Fatalf("convert without verified_by must succeed: %s", toolText(t, res))
	}
	plainActors := verifiedActors(t, plainOut)
	if len(plainActors) != len(baseActors) {
		t.Fatalf("MCP convert changed the total verified-stamp count from %d (source) to %d without "+
			"an explicit verified_by (never-fabricate-trust violated: the server auto-stamped)\n"+
			"  source: %v\n  written: %v", len(baseActors), len(plainActors), baseActors, plainActors)
	}
	for _, a := range plainActors {
		if !baseSet[a] {
			t.Fatalf("MCP convert introduced verified actor %q that the source never carried, without "+
				"an explicit verified_by (never-fabricate-trust violated: a fabricated actor was "+
				"appended)\n  source actors: %v\n  written actors: %v", a, baseActors, plainActors)
		}
	}
}

// countActorFiles counts concept files under root that carry a verified stamp
// for the given actor. It reads the actor STRUCTURALLY from parsed frontmatter
// (see verifiedActors), not by substring, so it detects the actor identically no
// matter how the serializer quoted it — human:alice, 'human:alice', or
// "human:alice", in a block or a flow mapping — and never mistakes a "by:" under
// generated: (or any other key) for a verified stamp. This is what makes the
// detector outlive a change to binder's serialization quoting.
func countActorFiles(t *testing.T, root, actor string) int {
	t.Helper()
	n := 0
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, a := range verifiedActorsIn(t, p, b) {
			if a == actor {
				n++
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return n
}

// verifiedActors parses the YAML frontmatter of every .md file under root with
// the repo's own parser (gopkg.in/yaml.v3 — the library binder itself uses to
// read and write these files) and returns the actor of EVERY verified stamp, one
// element per stamp so repeated actors and multiple stamps on one file are all
// counted. Reading the actor structurally rather than by substring means an actor
// is reported identically whether it was serialized human:alice, 'human:alice',
// or "human:alice", in a block or a flow mapping.
func verifiedActors(t *testing.T, root string) []string {
	t.Helper()
	var actors []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		actors = append(actors, verifiedActorsIn(t, p, b)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return actors
}

// verifiedActorsIn returns the actor of each verified stamp in a single file's
// frontmatter, honouring both shapes binder emits and accepts: a sequence of
// {by,at} mappings (the written form) and a single {by,at} mapping (a source
// form, spec §5.2 treats it as a one-element list). A "by:" under any other key —
// generated:, for instance — is never read as a verified stamp.
func verifiedActorsIn(t *testing.T, path string, b []byte) []string {
	t.Helper()
	fm := frontmatterBytes(b)
	if fm == "" {
		return nil
	}
	var doc struct {
		Verified yaml.Node `yaml:"verified"`
	}
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		t.Fatalf("parsing frontmatter of %s: %v", path, err)
	}
	type stamp struct {
		By string `yaml:"by"`
	}
	var stamps []stamp
	switch doc.Verified.Kind {
	case yaml.SequenceNode:
		if err := doc.Verified.Decode(&stamps); err != nil {
			t.Fatalf("decoding verified list of %s: %v", path, err)
		}
	case yaml.MappingNode:
		var one stamp
		if err := doc.Verified.Decode(&one); err != nil {
			t.Fatalf("decoding verified mapping of %s: %v", path, err)
		}
		stamps = []stamp{one}
	default:
		// Absent, explicit null, or a spec-invalid scalar: no actor to attribute.
		return nil
	}
	var actors []string
	for _, s := range stamps {
		if s.By != "" {
			actors = append(actors, s.By)
		}
	}
	return actors
}

// frontmatterBytes returns the YAML frontmatter block of a markdown file's bytes
// — the text between the opening "---" fence and the next "---" fence line — or ""
// if the document has no frontmatter. Fence handling mirrors the codec's own
// splitFrontmatter (a fence is a line that is exactly "---" after trimming
// CR/LF).
func frontmatterBytes(b []byte) string {
	lines := strings.SplitAfter(string(b), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != "---" {
		return ""
	}
	offset := len(lines[0])
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r\n") == "---" {
			return string(b)[len(lines[0]):offset]
		}
		offset += len(lines[i])
	}
	return ""
}

// actorSet collects a multiset of actors into a presence set.
func actorSet(actors []string) map[string]bool {
	s := make(map[string]bool, len(actors))
	for _, a := range actors {
		s[a] = true
	}
	return s
}

// TestActorDetectionIsSerializationIndependent proves — it does not assert — that
// the trust detector reads an actor from parsed structure, not from serialization
// form. binder-bytefaithful-fix is actively changing how these very keys are
// quoted (bare scalars single-quoted on flow-mapping re-serialization); a
// substring detector's blindness would move with that change, silently and in the
// unsafe direction (a fabricated actor going unseen). This control constructs the
// SAME stamp in every quoting/nesting form binder could emit and requires the
// detector to return the identical answer for each, so the fix outlives that
// change landing.
//
// It is deliberately NOT invariant-named: it guards the detector, not a product
// guarantee.
func TestActorDetectionIsSerializationIndependent(t *testing.T) {
	const actor = "human:alice"

	// The same verified stamp for `actor`, written five ways. A form-dependent
	// detector returns a different count for at least one of these; a structural
	// one returns 1 for every one.
	forms := map[string]string{
		"block_unquoted": "---\ntype: Note\nverified:\n  - by: human:alice\n    at: 2026-01-01T00:00:00Z\n---\n\n# X\n",
		"block_single":   "---\ntype: Note\nverified:\n  - by: 'human:alice'\n    at: 2026-01-01T00:00:00Z\n---\n\n# X\n",
		"block_double":   "---\ntype: Note\nverified:\n  - by: \"human:alice\"\n    at: 2026-01-01T00:00:00Z\n---\n\n# X\n",
		"flow_list":      "---\ntype: Note\nverified: [{by: human:alice, at: 2026-01-01T00:00:00Z}]\n---\n\n# X\n",
		"flow_mapping":   "---\ntype: Note\nverified: {by: 'human:alice', at: 2026-01-01T00:00:00Z}\n---\n\n# X\n",
	}
	for name, content := range forms {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "c.md"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := countActorFiles(t, dir, actor); got != 1 {
				t.Fatalf("form %s: detector saw %d files stamping %s, want 1 — actor detection "+
					"depends on serialization form", name, got, actor)
			}
		})
	}

	// Negative control #1: a DIFFERENT verified actor must not match, or the
	// detector is just returning 1 for everything and the positives prove nothing.
	t.Run("other_actor_not_matched", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\ntype: Note\nverified:\n  - by: 'human:bob'\n    at: 2026-01-01T00:00:00Z\n---\n\n# X\n"
		if err := os.WriteFile(filepath.Join(dir, "c.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := countActorFiles(t, dir, actor); got != 0 {
			t.Fatalf("negative control: detector matched %s in a file whose only verified stamp is "+
				"human:bob (got %d)", actor, got)
		}
	})

	// Negative control #2: the actor under generated: (NOT verified:) must not be
	// read as a verified stamp — even written unquoted, the exact form the OLD
	// substring detector ("by: "+actor) DID match. This is the substring detector's
	// false positive that the structural one removes.
	t.Run("generated_actor_not_a_verified_stamp", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\ntype: Note\ngenerated:\n  by: human:alice\n  at: 2026-01-01T00:00:00Z\n---\n\n# X\n"
		if err := os.WriteFile(filepath.Join(dir, "c.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := countActorFiles(t, dir, actor); got != 0 {
			t.Fatalf("detector counted a GENERATED actor as a verified stamp (got %d) — it is not "+
				"reading the verified key structurally", got)
		}
	})
}

// TestCharacterize_MCPDoesNotResolveConfig is a CHARACTERIZATION test: it records
// what the two surfaces currently DO, not what they must do. With a global user
// config supplying verified_by and no explicit actor on either surface, the CLI
// resolves the actor from config and stamps every concept, while the MCP server
// does not load config at all and stamps nothing.
//
// This behaviour is UNDER ACTIVE REVIEW BY THE OWNER and is not settled. A future
// change to it — making MCP resolve config, or making the CLI stop — is a
// DECISION, NOT A REGRESSION; whoever makes it should update this test to match
// rather than read a red here as a bug they caused. The name is deliberately NOT
// invariant-shaped so it cannot launder disputed behaviour into a guarantee.
//
// It uses XDG_CONFIG_HOME to arm a GLOBAL config with no cwd manipulation, so it
// is hermetic. It uses REAL converts, not --dry-run (the dry-run report carries
// no trust fields). The anti-vacuity control lives inside the assertion: if the
// CLI side stamps nothing the config no longer arms the stamp, so the test errors
// rather than silently pinning "0 == 0".
func TestCharacterize_MCPDoesNotResolveConfig(t *testing.T) {
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "binder"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(xdg, "binder", "config.yaml")
	if err := os.WriteFile(cfg, []byte("verified_by: human:alice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)

	out := t.TempDir()
	cliOut := filepath.Join(out, "cli")
	mcpOut := filepath.Join(out, "mcp")

	// Real converts, not --dry-run: the stamp only ever exists on disk.
	cliJSON(t, "convert", richCorpus, "-o", cliOut, "--json")
	if res := callTool(t, "convert", map[string]any{"src": richCorpus, "out": mcpOut}); res.IsError {
		t.Fatalf("MCP convert must succeed: %s", toolText(t, res))
	}

	cliStamped := countActorFiles(t, cliOut, "human:alice")
	mcpStamped := countActorFiles(t, mcpOut, "human:alice")

	if cliStamped == 0 {
		t.Fatalf("anti-vacuity: CLI produced 0 config-sourced verified stamps (config %s no longer "+
			"arms the stamp?); the test can no longer observe the divergence", cfg)
	}
	if mcpStamped != 0 {
		t.Fatalf("characterization changed: MCP stamped %d concept(s) from config — the server now "+
			"loads config. If that change is intentional, update this test: it is a decision, not a "+
			"regression", mcpStamped)
	}
	t.Logf("current behaviour: cli stamped=%d  mcp stamped=%d", cliStamped, mcpStamped)
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
