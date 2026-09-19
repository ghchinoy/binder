package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ghchinoy/binder/internal/binder"
	"github.com/ghchinoy/binder/internal/config"
	"github.com/ghchinoy/binder/internal/okf"
)

// convertInput mirrors `binder convert`'s conversion flags 1:1 (design §Tool
// surface). Raw map/list params use the same "k=v,k=v" / "a,b" grammar as the
// CLI flags and are parsed with the same convert.Parse* helpers (now inside the
// shared service); external_root is the one repeatable path flag, so it is a genuine
// []string (matching the CLI's StringArrayVar) rather than a comma-joined string —
// paths can contain commas, and inventing a split would create an escaping problem.
// The transport-only --json/--report flags are intentionally absent: this tool is a
// transport, not a report-producing command (design §Non-Goals).
type convertInput struct {
	Src                string   `json:"src" jsonschema:"source markdown corpus directory to convert"`
	Out                string   `json:"out,omitempty" jsonschema:"output bundle directory (required unless dry_run)"`
	DryRun             bool     `json:"dry_run,omitempty" jsonschema:"report what would be written without writing anything (the ingestion-analysis preview)"`
	DefaultType        string   `json:"default_type,omitempty" jsonschema:"type applied when none is present or mapped (default \"Note\")"`
	TypeMap            string   `json:"type_map,omitempty" jsonschema:"per-directory type overrides, e.g. \"docs=Guide,adr=Decision\""`
	FMRefKeys          string   `json:"fm_ref_keys,omitempty" jsonschema:"frontmatter keys treated as relationship edges, e.g. \"related,parent\""`
	SourceKeys         string   `json:"source_keys,omitempty" jsonschema:"frontmatter keys to map into sources entries, e.g. \"source,author\""`
	MapCitations       bool     `json:"map_citations,omitempty" jsonschema:"map a body \"# Citations\" list into sources entries"`
	MapDraft           bool     `json:"map_draft,omitempty" jsonschema:"map a draft:true marker to status:draft when status is absent"`
	StatusMap          string   `json:"status_map,omitempty" jsonschema:"per-directory status, e.g. \"archive=deprecated,drafts=draft,default=active\" (set only when status absent)"`
	CanonicalizeStatus bool     `json:"canonicalize_status,omitempty" jsonschema:"opt-in: rewrite known status_map aliases to the OKF §5.4 vocabulary (active->stable, wip/in-progress->draft, archived/legacy->deprecated); off by default, each rewrite is reported in status_notes"`
	StaleAfterMap      string   `json:"stale_after_map,omitempty" jsonschema:"per-directory stale_after relative to now, e.g. \"07-benchmarks=+6m,legacy=+0d\" (grammar +Nd/+Nm/+Ny)"`
	VerifiedBy         string   `json:"verified_by,omitempty" jsonschema:"actor to append as a verified stamp, e.g. \"human:ghchinoy\" (applied ONLY when set; never auto-stamped)"`
	WorkspaceRoot      string   `json:"workspace_root,omitempty" jsonschema:"boundary within which file:// links resolve to internal edges (default: the src root)"`
	ExternalRoot       []string `json:"external_root,omitempty" jsonschema:"declare KNOWN sibling-workspace roots (repeatable); file:// links under them stay external but suppress the outside-root advisory"`
	GroupByType        bool     `json:"group_by_type,omitempty" jsonschema:"append an additive \"# Catalog\" of all concepts grouped by type to the root index.md"`
	IncludeBacklinks   bool     `json:"include_backlinks,omitempty" jsonschema:"annotate catalog entries with inbound resolved edges (requires group_by_type)"`
	IncludeGraph       bool     `json:"include_graph,omitempty" jsonschema:"annotate catalog entries with outbound resolved edges (requires group_by_type)"`
	// Strict is accepted for CLI flag parity but IGNORED by this handler: the MCP
	// surface never gates (the call below hardcodes Strict:false) and Strict does not
	// change the payload, so reading it would be a no-op. Base ignored it for the
	// payload too, so this is not a behavior change. The field is retained for now to
	// avoid a transport-schema change; its removal is deferred to Phase 5.
	Strict bool `json:"strict,omitempty" jsonschema:"gate semantics only; does not change the payload (parity with the CLI flag)"`
}

// registerConvert wires the convert tool. dry_run:true → the analysis preview
// (writes nothing); dry_run:false → writes the bundle to out. It drives the SAME
// shared binder.Service.Convert the CLI uses, so the returned envelope is
// byte-identical to `binder convert --json` / `binder convert --dry-run --json`. The
// service parses the flag grammars, routes the trust decision, and assembles
// convert.Options — this handler only validates the transport-level inputs and maps
// the result. Malformed maps and an invalid verified_by are usage-class tool errors;
// verified_by is applied ONLY when explicitly set (never-fabricate-trust).
func registerConvert(s *mcp.Server, d *deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "convert",
		Description: "Convert a markdown corpus into an OKF v0.2 bundle. With dry_run it writes " +
			"nothing and returns the ingestion-analysis preview. Returns the binder.report/v1 " +
			"convert payload (identical to `binder convert --json`).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in convertInput) (*mcp.CallToolResult, any, error) {
		if in.Out == "" && !in.DryRun {
			return nil, nil, fmt.Errorf("out is required (or set dry_run:true)")
		}

		// Validation precedence (deliberate; see PR #226 review FYI-2). This adapter
		// validates the transport-level inputs — external_root-empty and invalid-actor —
		// BEFORE the shared service parses the map grammars, whereas base parsed the maps
		// first. That reorder is a STRUCTURAL consequence of the Phase-3 collapse: map
		// parsing now lives in the shared Service.Convert, while these two checks stay
		// surface-specific (their wording differs from the CLI's — external_root vs
		// --external-root, and the MCP actor error uses config.ActorFormsHint instead of
		// the CLI's clijson.Usage-wrapped form), so they cannot move into the shared core.
		// Restoring the base order would require re-adding adapter-side map parsing —
		// undoing the exact duplication this phase removed. For any SINGLE invalid input
		// the emitted error and result are byte-identical to base; only a call carrying
		// MULTIPLE invalid inputs at once changes WHICH usage-class error surfaces first
		// (both remain usage-class errors). The CLI adapter reordered identically, so the
		// two surfaces stay mutually consistent — as they were in base (both parse-first),
		// they are now both edge-check-first.
		//
		// --external-root parity (issue #25). Declared sibling roots are a genuine
		// repeatable list, so external_root is a []string mirroring the CLI's
		// StringArrayVar. An empty value is a usage-class tool error, the same gate as
		// the CLI (which phrases it as --external-root); a well-formed path that does
		// not exist is accepted on purpose (a declared sibling may be absent from this
		// checkout) and no stat is done.
		for _, er := range in.ExternalRoot {
			if strings.TrimSpace(er) == "" {
				return nil, nil, fmt.Errorf("external_root value must not be empty")
			}
		}

		// Never-fabricate-trust: apply verified_by ONLY when explicitly passed; an
		// invalid actor is a usage-class error (same okf.IsValidActor gate as the CLI).
		// The server never auto-stamps. The forms hint is taken from
		// config.ActorFormsHint rather than restated here, so this surface cannot drift
		// from the CLI's wording — and so the worked example tracks the live version
		// instead of a hand-maintained literal (issue #60). The CLI's
		// config.InvalidActorError is deliberately NOT reused: it wraps the error in
		// clijson.Usage to set a CLI exit code, which is meaningless over MCP.
		if in.VerifiedBy != "" && !okf.IsValidActor(in.VerifiedBy) {
			return nil, nil, fmt.Errorf("invalid actor %q; %s", in.VerifiedBy, config.ActorFormsHint())
		}

		// default_type mirrors the CLI flag default ("Note") when unset.
		defaultType := in.DefaultType
		if defaultType == "" {
			defaultType = "Note"
		}

		// MCP resolves verified_by from tool input ONLY and never loads config: a set
		// actor is an EXPLICIT per-invocation act (binder.TrustInput → stamps, may
		// co-sign, source "input"); unset is binder.TrustNone. Strict is false — the
		// MCP surface never gates; the payload is identical either way.
		origin := binder.TrustNone
		if in.VerifiedBy != "" {
			origin = binder.TrustInput
		}

		// Read SOURCE_DATE_EPOCH at the adapter edge and apply the shared determinism
		// rule (the one that used to be duplicated as this package's resolveNow on the
		// convert path); a malformed epoch falls back to the wall clock, matching the CLI.
		now, _ := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), time.Now())

		res, err := d.svc.Convert(ctx, binder.ConvertRequest{
			Src:                in.Src,
			Out:                in.Out,
			DryRun:             in.DryRun,
			DefaultType:        defaultType,
			TypeMapRaw:         in.TypeMap,
			StatusMapRaw:       in.StatusMap,
			StaleAfterMapRaw:   in.StaleAfterMap,
			FMRefKeysRaw:       in.FMRefKeys,
			SourceKeysRaw:      in.SourceKeys,
			CanonicalizeStatus: in.CanonicalizeStatus,
			MapCitations:       in.MapCitations,
			MapDraft:           in.MapDraft,
			VerifiedBy:         in.VerifiedBy,
			TrustOrigin:        origin,
			WorkspaceRoot:      in.WorkspaceRoot,
			ExternalRoots:      in.ExternalRoot,
			GroupByType:        in.GroupByType,
			IncludeBacklinks:   in.IncludeBacklinks,
			IncludeGraph:       in.IncludeGraph,
			Version:            d.version,
			Now:                now,
			Strict:             false,
		})
		if err != nil {
			return nil, nil, err
		}
		// The envelope is produced by the core Result and framed by the one shared
		// helper — byte-identical to `binder convert --json` and to the CLI's output.
		return encodeResult(res)
	})
}
