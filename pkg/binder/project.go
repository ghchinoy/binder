package binder

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ghchinoy/binder/pkg/bundle"
	"github.com/ghchinoy/binder/pkg/clijson"
	"github.com/ghchinoy/binder/pkg/graph"
	"github.com/ghchinoy/binder/pkg/okf"
)

// projectCommand is the envelope `command` token for the project capability.
const projectCommand = "project"

// ProjectReport is the binder.report/v1 result payload for `binder project`. It moves
// out of cmd/project.go (design §4.9): the report shape, the artifacts[] manifest
// (names + byte lengths), and the fixed emission order are the wire contract and are
// now owned by the service, not the CLI adapter. It carries the node-identity strategy
// (echoing the list_graphs vocabulary), the re-rooting stability signal (OQ-6),
// node/edge counts, the projected_as_of date the frozen tier/stale snapshot reflects
// (OQ-8), the target dialect, and the manifest of emitted files with byte lengths.
type ProjectReport struct {
	Target            string                  `json:"target"`
	NodeKey           graph.NodeKey           `json:"node_key"`
	IdentityStability graph.IdentityStability `json:"identity_stability"`
	Counts            graph.Counts            `json:"counts"`
	ProjectedAsOf     string                  `json:"projected_as_of"`
	Artifacts         []ProjectArtifact       `json:"artifacts"`
}

// ProjectArtifact is one emitted file in the artifacts manifest.
type ProjectArtifact struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
}

// ProjectRequest carries the RESOLVED inputs a project run needs.
type ProjectRequest struct {
	// Bundle is the OKF bundle directory to project.
	Bundle string
	// Out is the output directory the artifacts are written to (required).
	Out string
	// Target is the projection target dialect. When empty the service defaults it to
	// graph.TargetSpanner; the adapter validates it up front for a usage-class error.
	Target string
	// IDKey is the authored frontmatter key to use as node identity; falls back to
	// path identity per concept when empty.
	IDKey string
	// Now / Today: the RESOLVED frozen-snapshot inputs (empty Today defaults to Now's date).
	Now   time.Time
	Today string
	// Version is stamped into the JSON envelope's `binder` field.
	Version string
}

// ProjectResult is the COMPLETE project outcome: the report (owning the manifest) and
// the loaded bundle so the adapter can disclose unparsed files on stderr (warnings as
// data, rendered by the adapter — design §3.3).
type ProjectResult struct {
	// Report is the complete capability report, including the artifacts manifest.
	Report ProjectReport
	// Bundle is the loaded bundle, exposed so the adapter can render the unparsed
	// disclosure (it is not part of the report payload).
	Bundle *okf.Bundle

	// version is captured so the Result renders its own JSON envelope with the same
	// provenance the run used; adapters do not hand-build the envelope.
	version string
}

// Project loads the bundle, builds the deterministic projection, emits the six
// artifacts to req.Out in the FIXED order ("Order is fixed for determinism"), and
// returns the manifest with byte accounting — all owned here now, not in cmd/. The
// empty-Target and empty-Today defaults are the service's.
//
// It writes files to disk (like convert.Convert) but never prints and never exits —
// os.Exit / prose stay at the adapter (design AC5).
func (s *Service) Project(ctx context.Context, req ProjectRequest) (ProjectResult, error) {
	_ = ctx

	target := req.Target
	if target == "" {
		target = string(graph.TargetSpanner)
	}

	b, err := bundle.Load(req.Bundle, s.codec)
	if err != nil {
		return ProjectResult{}, err
	}

	today := req.Today
	if today == "" {
		today = req.Now.Format("2006-01-02")
	}

	proj := graph.Project(b, graph.ProjectOptions{
		Target: graph.Target(target),
		IDKey:  req.IDKey,
		Today:  today,
	})

	if err := os.MkdirAll(req.Out, 0o755); err != nil {
		return ProjectResult{}, fmt.Errorf("creating output dir: %w", err)
	}

	// Emit each artifact and record it in the manifest with its byte length. The
	// emitted set is schema.ddl (G1) + the loader row data (G2) + the G3 provenance
	// artifacts. Order is fixed for determinism.
	var artifacts []ProjectArtifact
	emit := func(name string, data []byte) error {
		if err := os.WriteFile(filepath.Join(req.Out, name), data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
		artifacts = append(artifacts, ProjectArtifact{Name: name, Bytes: len(data)})
		return nil
	}
	for _, a := range []struct {
		name string
		data []byte
	}{
		// G2: the schema plus the loader row data and the DML load statements.
		{"schema.ddl", proj.DDL()},
		{"nodes.csv", proj.NodesCSV()},
		{"edges.csv", proj.EdgesCSV()},
		{"load.sql", proj.LoadSQL()},
		// G3: the NodeVerified rows and the tier/stale derivation view.
		{"node_verified.csv", proj.NodeVerifiedCSV()},
		{"derivation.sql", proj.DerivationView()},
	} {
		if err := emit(a.name, a.data); err != nil {
			return ProjectResult{}, err
		}
	}

	rep := ProjectReport{
		Target:            string(proj.Target),
		NodeKey:           proj.NodeKey,
		IdentityStability: proj.Identity,
		Counts:            proj.Counts,
		ProjectedAsOf:     proj.AsOf,
		Artifacts:         artifacts,
	}
	return ProjectResult{Report: rep, Bundle: b, version: req.Version}, nil
}

// EncodeJSON writes the binder.report/v1 envelope for this result to w via the core
// clijson encoder — the same bytes `binder project` produces.
func (r ProjectResult) EncodeJSON(w io.Writer) error {
	return clijson.Encode(w, r.version, projectCommand, r.Report)
}
