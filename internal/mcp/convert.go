package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
)

// convertInput mirrors `binder convert` flags 1:1 (design §Tool surface). Raw
// map/list params use the same "k=v,k=v" / "a,b" grammar as the CLI flags and
// are parsed with the same convert.Parse* helpers.
type convertInput struct {
	Src                string `json:"src" jsonschema:"source markdown corpus directory to convert"`
	Out                string `json:"out,omitempty" jsonschema:"output bundle directory (required unless dry_run)"`
	DryRun             bool   `json:"dry_run,omitempty" jsonschema:"report what would be written without writing anything (the ingestion-analysis preview)"`
	DefaultType        string `json:"default_type,omitempty" jsonschema:"type applied when none is present or mapped (default \"Note\")"`
	TypeMap            string `json:"type_map,omitempty" jsonschema:"per-directory type overrides, e.g. \"docs=Guide,adr=Decision\""`
	FMRefKeys          string `json:"fm_ref_keys,omitempty" jsonschema:"frontmatter keys treated as relationship edges, e.g. \"related,parent\""`
	SourceKeys         string `json:"source_keys,omitempty" jsonschema:"frontmatter keys to map into sources entries, e.g. \"source,author\""`
	MapCitations       bool   `json:"map_citations,omitempty" jsonschema:"map a body \"# Citations\" list into sources entries"`
	MapDraft           bool   `json:"map_draft,omitempty" jsonschema:"map a draft:true marker to status:draft when status is absent"`
	StatusMap          string `json:"status_map,omitempty" jsonschema:"per-directory status, e.g. \"archive=deprecated,drafts=draft,default=active\" (set only when status absent)"`
	CanonicalizeStatus bool   `json:"canonicalize_status,omitempty" jsonschema:"opt-in: rewrite known status_map aliases to the OKF §5.4 vocabulary (active->stable, wip/in-progress->draft, archived/legacy->deprecated); off by default, each rewrite is reported in status_notes"`
	StaleAfterMap      string `json:"stale_after_map,omitempty" jsonschema:"per-directory stale_after relative to now, e.g. \"07-benchmarks=+6m,legacy=+0d\" (grammar +Nd/+Nm/+Ny)"`
	VerifiedBy         string `json:"verified_by,omitempty" jsonschema:"actor to append as a verified stamp, e.g. \"human:ghchinoy\" (applied ONLY when set; never auto-stamped)"`
	WorkspaceRoot      string `json:"workspace_root,omitempty" jsonschema:"boundary within which file:// links resolve to internal edges (default: the src root)"`
	GroupByType        bool   `json:"group_by_type,omitempty" jsonschema:"append an additive \"# Catalog\" of all concepts grouped by type to the root index.md"`
	IncludeBacklinks   bool   `json:"include_backlinks,omitempty" jsonschema:"annotate catalog entries with inbound resolved edges (requires group_by_type)"`
	IncludeGraph       bool   `json:"include_graph,omitempty" jsonschema:"annotate catalog entries with outbound resolved edges (requires group_by_type)"`
	Strict             bool   `json:"strict,omitempty" jsonschema:"gate semantics only; does not change the payload (parity with the CLI flag)"`
}

// registerConvert wires the convert tool. dry_run:true → convert.Analyze (the
// preview; never writes); dry_run:false → convert.Convert writing to out. The
// returned *convert.Report is byte-identical to `binder convert --json` /
// `binder convert --dry-run --json`. Malformed maps and an invalid verified_by
// are usage-class tool errors; verified_by is applied ONLY when explicitly set
// (never-fabricate-trust).
func registerConvert(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "convert",
		Description: "Convert a markdown corpus into an OKF v0.2 bundle. With dry_run it writes " +
			"nothing and returns the ingestion-analysis preview. Returns the binder.report/v1 " +
			"convert payload (identical to `binder convert --json`).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in convertInput) (*mcp.CallToolResult, any, error) {
		if in.Out == "" && !in.DryRun {
			return nil, nil, fmt.Errorf("out is required (or set dry_run:true)")
		}

		typeMap, err := convert.ParseTypeMap(in.TypeMap)
		if err != nil {
			return nil, nil, err
		}
		statusMap, statusDefault, err := convert.ParseStatusMap(in.StatusMap)
		if err != nil {
			return nil, nil, err
		}
		// Mirror the CLI's OKF §5.4 status-vocabulary handling (issue #23) so the
		// two surfaces do not diverge: non-conformant values are surfaced in the
		// report's status_notes, and canonicalize_status opts into the same fixed
		// alias rewrite. Non-conformant values are reported, never rejected, keeping
		// parity with this tool's never-reject payload posture (strict here is
		// gate-semantics only and does not change the payload).
		statusMap, statusDefault, statusVocab := convert.ResolveStatusVocabulary(statusMap, statusDefault, in.CanonicalizeStatus)
		staleAfterMap, err := convert.ParseStaleAfterMap(in.StaleAfterMap)
		if err != nil {
			return nil, nil, err
		}

		// Never-fabricate-trust: apply verified_by ONLY when explicitly passed;
		// an invalid actor is a usage-class error (same okf.IsValidActor gate as
		// the CLI). The server never auto-stamps verified/sources.
		if in.VerifiedBy != "" && !okf.IsValidActor(in.VerifiedBy) {
			return nil, nil, fmt.Errorf("invalid actor %q; valid forms: human:<id>, process:<id>, "+
				"team:<id>, or <producer>/<version> (e.g. binder/0.3.0)", in.VerifiedBy)
		}

		// default_type mirrors the CLI flag default ("Note") when unset.
		defaultType := in.DefaultType
		if defaultType == "" {
			defaultType = "Note"
		}

		opts := convert.Options{
			Codec:            d.codec,
			DefaultType:      defaultType,
			TypeMap:          typeMap,
			StatusMap:        statusMap,
			StatusDefault:    statusDefault,
			StatusNotes:      statusVocab.Notes,
			StaleAfterMap:    staleAfterMap,
			VerifiedBy:       in.VerifiedBy,
			FMRefKeys:        convert.ParseFMRefKeys(in.FMRefKeys),
			Version:          d.version,
			Now:              resolveNow(),
			DryRun:           in.DryRun,
			MapCitations:     in.MapCitations,
			SourceKeys:       convert.ParseFMRefKeys(in.SourceKeys),
			MapDraft:         in.MapDraft,
			WorkspaceRoot:    in.WorkspaceRoot,
			GroupByType:      in.GroupByType,
			IncludeBacklinks: in.IncludeBacklinks,
			IncludeGraph:     in.IncludeGraph,
		}

		report, err := convert.Convert(in.Src, in.Out, opts)
		if err != nil {
			return nil, nil, err
		}
		return d.encode("convert", report)
	})
}
