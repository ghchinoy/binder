package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghchinoy/binder/internal/clijson"
)

const projectCorpus = "../testdata/project/corpus"

// projectEnvelope is the shape of a `binder project` report envelope, used to
// assert the binder.report/v1 contract (command, schema) and the result fields
// G1 requires. Fields not asserted are left out; encoding/json ignores extras.
type projectEnvelope struct {
	Binder  string `json:"binder"`
	Command string `json:"command"`
	Schema  string `json:"schema"`
	Result  struct {
		Target  string `json:"target"`
		NodeKey struct {
			Strategy string `json:"strategy"`
			Key      string `json:"key"`
		} `json:"node_key"`
		IdentityStability struct {
			ReRootingStable   bool `json:"re_rooting_stable"`
			PathFallbackCount int  `json:"path_fallback_count"`
		} `json:"identity_stability"`
		Counts struct {
			Nodes int `json:"nodes"`
			Edges int `json:"edges"`
		} `json:"counts"`
		ProjectedAsOf string `json:"projected_as_of"`
		Artifacts     []struct {
			Name  string `json:"name"`
			Bytes int    `json:"bytes"`
		} `json:"artifacts"`
	} `json:"result"`
}

func runProject(t *testing.T, extra ...string) (projectEnvelope, string, int) {
	t.Helper()
	outDir := t.TempDir()
	args := append([]string{"project", projectCorpus, "--out", outDir, "--today", "2026-08-18"}, extra...)
	stdout, code := runCLI(t, args...)
	var env projectEnvelope
	if code == clijson.ExitSuccess {
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("envelope is not valid JSON: %v\n%s", err, stdout)
		}
	}
	return env, outDir, code
}

// TestProjectSchemaGolden pins G1 criterion 1: byte-golden schema.ddl over the
// fixture bundle. The golden instrument is proven capable of failing below
// (TestProjectSchemaGoldenControl), per C11.
func TestProjectSchemaGolden(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	_, outDir, code := runProject(t, "--id-key", "graph_id")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want success", code)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "schema.ddl"))
	if err != nil {
		t.Fatalf("reading emitted schema.ddl: %v", err)
	}
	want, err := os.ReadFile("../testdata/project/schema.ddl.golden")
	if err != nil {
		t.Fatalf("reading golden: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("schema.ddl != golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestProjectSchemaGoldenControl is the C11 positive control for the golden: a
// mutated expected value must be detected as a mismatch. If a byte-equal compare
// passed here, the golden test above would be vacuous.
func TestProjectSchemaGoldenControl(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	_, outDir, _ := runProject(t, "--id-key", "graph_id")
	got, err := os.ReadFile(filepath.Join(outDir, "schema.ddl"))
	if err != nil {
		t.Fatalf("reading emitted schema.ddl: %v", err)
	}
	mutated := append([]byte("-- MUTATED\n"), got...)
	if string(got) == string(mutated) {
		t.Fatal("control failed: mutated golden compared equal to emitted DDL")
	}
}

// TestProjectRowGoldens pins the G2 byte-golden criterion: nodes.csv, edges.csv
// and load.sql emitted over the fixture bundle match their goldens. The golden
// instruments are proven capable of failing in TestProjectRowGoldensControl
// (C11). Generated with --id-key graph_id under the same pinned clock as the
// schema golden, so node_key = the authored graph_id values.
func TestProjectRowGoldens(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	_, outDir, code := runProject(t, "--id-key", "graph_id")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want success", code)
	}
	for _, name := range []string{"nodes.csv", "edges.csv", "load.sql"} {
		got, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("reading emitted %s: %v", name, err)
		}
		want, err := os.ReadFile(filepath.Join("../testdata/project", name+".golden"))
		if err != nil {
			t.Fatalf("reading golden %s: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s != golden\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
		}
	}
}

// TestProjectRowGoldensControl is the C11 positive control: for EACH row golden,
// a mutated expected value must be detected as a mismatch. If a byte-equal
// compare passed here, the golden test above would be vacuous for that artifact.
func TestProjectRowGoldensControl(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	_, outDir, _ := runProject(t, "--id-key", "graph_id")
	for _, name := range []string{"nodes.csv", "edges.csv", "load.sql"} {
		got, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("reading emitted %s: %v", name, err)
		}
		mutated := append([]byte("MUTATED,\n"), got...)
		if string(got) == string(mutated) {
			t.Errorf("control failed for %s: mutated golden compared equal to emitted bytes", name)
		}
	}
}

// TestProjectDeterministic pins G1 criterion 5: repeated runs under a pinned
// SOURCE_DATE_EPOCH are byte-identical in both the emitted DDL and the report
// envelope. No cloud credentials are used by any path exercised here.
func TestProjectDeterministic(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	run := func() (string, string) {
		outDir := t.TempDir()
		stdout, code := runCLI(t, "project", projectCorpus, "--out", outDir, "--id-key", "graph_id")
		if code != clijson.ExitSuccess {
			t.Fatalf("exit = %d, want success", code)
		}
		var combined string
		for _, name := range []string{"schema.ddl", "nodes.csv", "edges.csv", "load.sql"} {
			data, err := os.ReadFile(filepath.Join(outDir, name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			combined += "\n--- " + name + " ---\n" + string(data)
		}
		return stdout, combined
	}
	env1, ddl1 := run()
	env2, ddl2 := run()
	if ddl1 != ddl2 {
		t.Errorf("emitted artifacts not deterministic:\n%s\n---\n%s", ddl1, ddl2)
	}
	if env1 != env2 {
		t.Errorf("report envelope not deterministic:\n%s\n---\n%s", env1, env2)
	}
	// SOURCE_DATE_EPOCH 1700000000 = 2023-11-14 UTC; projected_as_of must reflect it.
	var env projectEnvelope
	if err := json.Unmarshal([]byte(env1), &env); err != nil {
		t.Fatalf("bad envelope: %v", err)
	}
	if env.Result.ProjectedAsOf != "2023-11-14" {
		t.Errorf("projected_as_of = %q, want 2023-11-14 (from SOURCE_DATE_EPOCH)", env.Result.ProjectedAsOf)
	}
}

// TestProjectEnvelope pins G1 criterion 6: a valid binder.report/v1 envelope for
// command "project", carrying the required result fields, and criterion 2 at the
// CLI boundary (identity strategy/stability echoed in the envelope).
func TestProjectEnvelope(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	env, outDir, code := runProject(t, "--id-key", "graph_id")
	if code != clijson.ExitSuccess {
		t.Fatalf("exit = %d, want success", code)
	}
	if env.Command != "project" {
		t.Errorf("command = %q, want project", env.Command)
	}
	if env.Schema != clijson.SchemaVersion {
		t.Errorf("schema = %q, want %q", env.Schema, clijson.SchemaVersion)
	}
	if env.Result.Target != "spanner" {
		t.Errorf("target = %q, want spanner", env.Result.Target)
	}
	if env.Result.NodeKey.Strategy != "frontmatter" || env.Result.NodeKey.Key != "graph_id" {
		t.Errorf("node_key = %+v, want {frontmatter graph_id}", env.Result.NodeKey)
	}
	if !env.Result.IdentityStability.ReRootingStable || env.Result.IdentityStability.PathFallbackCount != 0 {
		t.Errorf("identity_stability = %+v, want {true 0}", env.Result.IdentityStability)
	}
	if env.Result.Counts.Nodes != 3 || env.Result.Counts.Edges != 2 {
		t.Errorf("counts = %+v, want {3 2}", env.Result.Counts)
	}
	if env.Result.ProjectedAsOf != "2026-08-18" {
		t.Errorf("projected_as_of = %q, want 2026-08-18", env.Result.ProjectedAsOf)
	}
	// artifacts manifest lists each emitted file with its real byte length. The
	// assertion is presence-based (each expected artifact by name) rather than an
	// exact total count, so a parallel phase adding further artifacts does not
	// cause a semantic conflict here.
	byName := map[string]int{}
	for _, a := range env.Result.Artifacts {
		byName[a.Name] = a.Bytes
	}
	for _, name := range []string{"schema.ddl", "nodes.csv", "edges.csv", "load.sql"} {
		bytesReported, ok := byName[name]
		if !ok {
			t.Errorf("artifacts manifest missing %q: %+v", name, env.Result.Artifacts)
			continue
		}
		info, err := os.Stat(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if int64(bytesReported) != info.Size() {
			t.Errorf("%s artifact bytes = %d, want file size %d", name, bytesReported, info.Size())
		}
	}
}

// TestProjectIdentityEnvelopeCases pins G1 criterion 2's mixed/none cases at the
// CLI boundary.
func TestProjectIdentityEnvelopeCases(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	cases := []struct {
		name         string
		flags        []string
		wantStrategy string
		wantStable   bool
		wantFallback int
	}{
		{"none", nil, "path", false, 3},
		{"mixed", []string{"--id-key", "partial_id"}, "frontmatter", false, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env, _, code := runProject(t, c.flags...)
			if code != clijson.ExitSuccess {
				t.Fatalf("exit = %d, want success", code)
			}
			if env.Result.NodeKey.Strategy != c.wantStrategy {
				t.Errorf("strategy = %q, want %q", env.Result.NodeKey.Strategy, c.wantStrategy)
			}
			if env.Result.IdentityStability.ReRootingStable != c.wantStable {
				t.Errorf("re_rooting_stable = %v, want %v", env.Result.IdentityStability.ReRootingStable, c.wantStable)
			}
			if env.Result.IdentityStability.PathFallbackCount != c.wantFallback {
				t.Errorf("path_fallback_count = %d, want %d", env.Result.IdentityStability.PathFallbackCount, c.wantFallback)
			}
		})
	}
}

// TestProjectExitCodes pins G1 criterion 6's exit-code contract (usage=2, IO=3),
// classified exactly as main.go does.
func TestProjectExitCodes(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	out := t.TempDir()
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"success", []string{"project", projectCorpus, "--out", out}, clijson.ExitSuccess},
		{"usage-missing-out", []string{"project", projectCorpus}, clijson.ExitUsage},
		{"usage-bad-target", []string{"project", projectCorpus, "--out", out, "--target", "bigquery"}, clijson.ExitUsage},
		{"usage-bad-today", []string{"project", projectCorpus, "--out", out, "--today", "notadate"}, clijson.ExitUsage},
		{"usage-unknown-flag", []string{"project", projectCorpus, "--out", out, "--nope"}, clijson.ExitUsage},
		{"usage-missing-bundle-arg", []string{"project", "--out", out}, clijson.ExitUsage},
		{"io-missing-bundle", []string{"project", "../testdata/does-not-exist", "--out", out}, clijson.ExitIO},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, code := runCLI(t, c.args...)
			if code != c.want {
				t.Errorf("args %v: exit = %d, want %d", c.args, code, c.want)
			}
		})
	}
}
