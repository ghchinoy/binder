// Package enrich implements `binder enrich <src>`: in-place, FRONTMATTER-ONLY
// enrichment of a source markdown tree (issue #5). It injects the missing
// required OKF frontmatter (type/title/generated, plus the optional #7 lifecycle
// stamps) into existing files, preserving the body and every pre-existing
// frontmatter key byte-for-byte.
//
// enrich is NOT `convert --in-place`. It does NO link rewriting, NO index
// generation, NO "## Related" section, NO tag merge — frontmatter only. It
// reuses convert's tested injection helpers (EnsureType/EnsureTitle/
// StampGenerated and the codec's byte-faithful Serialize) but deliberately does
// not run convert's body pipeline. `binder convert` is unchanged.
//
// Safety model (load-bearing — enrich mutates the source):
//   - Additive / never-clobber: only ABSENT keys are added.
//   - Idempotent: a second run adds nothing → no write.
//   - Body + pre-existing keys byte-faithful (codec Serialize).
//   - Skip-unchanged: a file needing no key is never rewritten (no git churn).
//   - Never mutate the unparseable: a file whose frontmatter will not parse is
//     skipped and reported, never rewritten.
//   - Reserved files (index.md/log.md) are skipped.
//   - Atomic write: temp file in the target's dir → fsync → rename, preserving
//     the file mode; an interrupt never leaves a partial/corrupt source file.
//   - Deterministic: generated.at from opts.Now (SOURCE_DATE_EPOCH-aware).
package enrich

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
)

// Status values for a FileResult.
const (
	StatusEnriched    = "enriched"     // keys were injected and the file was written
	StatusUnchanged   = "unchanged"    // every key already present → no write
	StatusWouldEnrich = "would-enrich" // --dry-run: keys would be injected
	StatusSkipped     = "skipped"      // not mutated (unparseable frontmatter, etc.)
)

// Options configures an enrichment run. All injectors are set-when-absent.
type Options struct {
	Codec       okf.Codec         // required
	DefaultType string            // fallback type; defaults to "Note"
	TypeMap     map[string]string // per-directory type overrides
	Version     string            // binder version, used in generated.by
	Now         time.Time         // clock for generated.at; controls determinism
	DryRun      bool              // when true, compute + report but write nothing

	// #7 declarative injectors (Phase 4), OFF by default and set-when-absent.
	StatusMap     map[string]string
	StatusDefault string
	StaleAfterMap map[string]string
	VerifiedBy    string

	// VerifiedByExplicit records that VerifiedBy came from an explicit
	// per-invocation --verified-by (co-sign permitted) rather than the user-set
	// global config exception (co-sign declined — Residual A). Set by the CLI via
	// config.PermitsStampWithoutFlag. Empty (default) means "not explicit".
	VerifiedByExplicit bool
	// VerifiedBySource is the disclosure token for the resolved actor's origin
	// ("flag" | "config" | "none"), surfaced in the report (Residual B). This is a
	// narrower set than report.go's Source, which also lists "input" (MCP): enrich
	// is CLI-fed only (cmd/enrich.go passes vb.Source) and has no MCP tool, so an
	// enrich source can never be "input".
	VerifiedBySource string

	// StatusNotes are pre-computed OKF §5.4 status-vocabulary messages (issue #23)
	// — non-conformant --status-map values and any opt-in canonicalization
	// rewrites — resolved once at the CLI boundary and surfaced additively in the
	// report. Empty (the default) keeps output byte-identical.
	StatusNotes []string

	// OverwriteKeys is the opt-in set of frontmatter keys to REFRESH in place
	// even when already present (issue #22). Empty (the default) preserves the
	// additive/never-clobber behavior byte-for-byte. Scoped strictly to the named
	// keys: every other pre-existing key, custom frontmatter, key order, and
	// surrounding bytes stay untouched. Trust/attestation-carrying keys
	// (okf.ProtectedTrustKeys) are refused at the CLI, never here.
	OverwriteKeys map[string]bool
}

// FileResult is the per-file outcome of an enrichment run.
type FileResult struct {
	Path   string `json:"path"`   // source-relative
	Status string `json:"status"` // enriched | unchanged | would-enrich | skipped
	// Added lists keys injected because they were ABSENT (additive/never-clobber),
	// sorted. Overwritten lists keys REFRESHED in place because they were named in
	// --overwrite-keys and their value changed (issue #22), sorted. Overwritten is
	// omitted (nil) when --overwrite-keys is not used, so default output is
	// byte-identical.
	Added       []string `json:"added,omitempty"`
	Overwritten []string `json:"overwritten,omitempty"`
	Reason      string   `json:"reason,omitempty"` // for skipped, e.g. "unparseable frontmatter: <err>"
	// Normalized lists the boundary-normalizations binder applied to this file
	// before frontmatter recognition (#124): "stripped-utf8-bom" and/or
	// "translated-lone-cr". It is set ONLY when the file was actually written
	// (enriched, or would-enrich under --dry-run) — i.e. when the normalized bytes
	// were persisted — so it never claims a change that did not reach disk. It is
	// an ADDED optional field in binder.report/v1, omitted (nil) when nothing was
	// normalized; consumers ignoring it and the default (no BOM/lone-CR) output are
	// unaffected. A non-empty value means the written file does NOT round-trip
	// byte-for-byte against the source; the run also raises a top-level advisory.
	Normalized []string `json:"normalized,omitempty"`
}

// Report summarizes an enrichment run. Slices are always initialized so --json
// serializes an empty run to [] rather than null. Files is sorted by path.
type Report struct {
	Src          string       `json:"src"`
	DryRun       bool         `json:"dry_run"`
	NumFiles     int          `json:"num_files"`
	NumEnriched  int          `json:"num_enriched"` // would-enrich under --dry-run
	NumUnchanged int          `json:"num_unchanged"`
	NumSkipped   int          `json:"num_skipped"`
	Files        []FileResult `json:"files"`
	// Warnings holds advisory notes that are NOT per-file skips — currently the
	// preserve-or-advise carry-forward notes for spec-invalid `verified` values
	// (issue #7): the authored value is preserved unchanged and reported here
	// rather than silently dropped. Each is "path: message". Initialized to [].
	Warnings []string `json:"warnings"`
	// StatusNotes carries the OKF §5.4 status-vocabulary notes for a run (issue
	// #23): non-conformant --status-map values and any opt-in canonicalization
	// rewrites. It is additive and always emitted, initialised to [] on a
	// conformant run so the envelope shape is stable (matching PR #56's
	// empty-array fix); a nil slice would marshal to null.
	StatusNotes []string `json:"status_notes"`
	// Verified discloses the never-fabricate-trust decision for this run (Residual
	// B): which actor (if any) was stamped, from where, every source file it
	// stamped, and every file where a global config stamp was DECLINED because a
	// different identity had already attested it (Residual A). Additive to
	// binder.report/v1 and always emitted so the decision is observable.
	Verified convert.VerifiedStampReport `json:"verified"`
}

// NumFindings returns the count of advisory findings enrich produces: skipped
// files (unparseable frontmatter) plus preserve-or-advise warnings. All are
// advisory — the run always completes; under --strict they gate (exit 1).
func (r *Report) NumFindings() int { return r.NumSkipped + len(r.Warnings) }

// ParseOverwriteKeys parses a --overwrite-keys value of the form
// "status,stale_after" into a set of keys to refresh in place (issue #22). Empty
// input yields a nil set (the flag is off → additive/never-clobber default).
// Keys are trimmed; empty entries are ignored. Naming a trust/attestation-carrying
// key (okf.ProtectedTrustKeys) is REFUSED LOUDLY as a usage error that names the
// offending key — overwriting such a key could destroy a human attestation and
// violate the never-fabricate-trust invariant, so it is never silently ignored.
func ParseOverwriteKeys(s string) (map[string]bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			continue
		}
		if okf.IsProtectedTrustKey(key) {
			return nil, fmt.Errorf("--overwrite-keys: refusing to overwrite trust-provenance key %q "+
				"(protected: %s); these can carry human attestations and overwriting them would violate the "+
				"never-fabricate-trust invariant", key, strings.Join(okf.ProtectedTrustKeys(), ", "))
		}
		out[key] = true
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// Enrich walks src and injects missing frontmatter into each non-reserved file
// in place (unless opts.DryRun). It returns the run Report. A bad/unreadable src
// path is a caller-side usage error surfaced by the command; a mid-walk IO
// failure (read/write) is returned as an error (exit 3). Given identical input
// and opts.Now the run is deterministic.
func Enrich(src string, opts Options) (*Report, error) {
	if opts.Codec == nil {
		return nil, fmt.Errorf("enrich: codec is required")
	}
	if opts.DefaultType == "" {
		opts.DefaultType = "Note"
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}

	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("enrich: source %q: %w", src, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("enrich: source %q is not a directory", src)
	}

	files, err := convert.WalkCorpus(src)
	if err != nil {
		return nil, fmt.Errorf("enrich: walking source: %w", err)
	}

	// StatusNotes is always emitted (issue #23); a nil opts value becomes [] so
	// the envelope shape is stable and never marshals to null (mirrors Warnings).
	statusNotes := opts.StatusNotes
	if statusNotes == nil {
		statusNotes = []string{}
	}
	rep := &Report{
		Src:         src,
		DryRun:      opts.DryRun,
		Files:       []FileResult{},
		Warnings:    []string{},
		StatusNotes: statusNotes,
		Verified:    convert.NewVerifiedStampReport(),
	}
	// Trust disclosure metadata (Residual B): actor + origin are known up front;
	// per-file stamped/skipped tallies accrue during the walk.
	rep.Verified.Actor = opts.VerifiedBy
	if opts.VerifiedBySource != "" {
		rep.Verified.Source = opts.VerifiedBySource
	}

	codec := opts.Codec
	for _, f := range files {
		// Reserved files (index.md/log.md) are never touched.
		if codec.IsReservedFile(f.Rel) {
			continue
		}
		rep.NumFiles++

		raw, rerr := os.ReadFile(f.Abs)
		if rerr != nil {
			return nil, fmt.Errorf("enrich: reading %q: %w", f.Rel, rerr)
		}

		res, vres, werr := enrichFile(codec, f, raw, opts)
		if werr != nil {
			return nil, werr
		}
		rep.Files = append(rep.Files, res)
		if len(res.Normalized) > 0 {
			// Non-optional disclosure (#124, design §9 AC5): a written file whose
			// bytes were normalized raises a top-level advisory in addition to its
			// per-file `normalized` signal, so a "silent-success" enrich of a
			// normalized file is impossible.
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"%s: input normalized before frontmatter recognition (%s); written file does not round-trip byte-for-byte against the source",
				f.Rel, strings.Join(res.Normalized, ", ")))
		}
		if vres.Advisory != "" {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: %s", f.Rel, vres.Advisory))
		}
		if vres.Stamped {
			rep.Verified.Stamped = append(rep.Verified.Stamped, f.Rel)
		}
		if vres.Skipped {
			rep.Verified.Skipped = append(rep.Verified.Skipped,
				convert.VerifiedSkip{Path: f.Rel, ExistingActor: vres.ExistingActor})
		}
		switch res.Status {
		case StatusEnriched, StatusWouldEnrich:
			rep.NumEnriched++
		case StatusUnchanged:
			rep.NumUnchanged++
		case StatusSkipped:
			rep.NumSkipped++
		}
	}
	sort.Strings(rep.Warnings)
	sort.Strings(rep.Verified.Stamped)
	sort.Slice(rep.Verified.Skipped, func(i, j int) bool {
		return rep.Verified.Skipped[i].Path < rep.Verified.Skipped[j].Path
	})
	rep.Verified.NumStamped = len(rep.Verified.Stamped)
	rep.Verified.NumSkipped = len(rep.Verified.Skipped)

	sort.Slice(rep.Files, func(i, j int) bool { return rep.Files[i].Path < rep.Files[j].Path })
	return rep, nil
}

// enrichFile applies the injectors to one file and, unless dry-run, writes it
// atomically when the frontmatter changed. It returns the file's result, an
// advisory message (else ""), and an error. A parse failure yields a skipped
// result (never mutated); an IO write failure returns an error (exit 3).
func enrichFile(codec okf.Codec, f convert.SourceFile, raw []byte, opts Options) (FileResult, convert.VerifiedResult, error) {
	// Normalize ONCE at the read boundary (#124, design §4.4): strip a leading
	// UTF-8 BOM and translate lone CR→LF BEFORE fence detection, then route on and
	// parse the SAME normalized bytes. Before this, a BOM-prefixed or lone-CR-fenced
	// file failed recognition, fell into the else branch, and had its real (often
	// human-verified) frontmatter silently demoted to body under a synthetic block.
	norm, notes := convert.NormalizeInput(raw)

	var c *okf.Concept
	if convert.OpensFrontmatterFence(norm) {
		parsed, perr := codec.ParseConcept(f.Rel, norm)
		if perr != nil {
			// Never mutate what we cannot safely parse — skip and report. After
			// normalization a BOM/lone-CR fence now OPENS, so a genuinely-broken one
			// reaches this existing skip-and-disclose path instead of being demoted
			// (the transition #124's remedy depends on; design §9 AC3).
			return FileResult{
				Path:   f.Rel,
				Status: StatusSkipped,
				Reason: fmt.Sprintf("unparseable frontmatter: %v", perr),
			}, convert.VerifiedResult{}, nil
		}
		c = parsed
	} else {
		// No frontmatter: a fresh valid block will be injected.
		c = convert.PlainConcept(codec, f.Rel, norm)
	}

	// Snapshot the frontmatter (keys + deep-copied values) BEFORE injection so
	// the change signal catches not only added keys but a modified value (e.g. a
	// verified actorstamp appended to an existing list) — set-when-absent covers
	// the rest, but --verified-by mutates an existing key in place.
	before := snapshotFM(c.Frontmatter)

	// Injectors — all set-when-absent / never-clobber. Order mirrors convert.
	convert.EnsureType(c.Frontmatter, f.Rel, opts.TypeMap, opts.DefaultType)
	convert.EnsureTitle(c.Frontmatter, f.Rel, c.Body)
	// #7 declarative injectors (off unless the corresponding option is set), plus
	// the preserve-or-advise verified handling.
	cOpts := convert.Options{
		StatusMap:          opts.StatusMap,
		StatusDefault:      opts.StatusDefault,
		StaleAfterMap:      opts.StaleAfterMap,
		VerifiedBy:         opts.VerifiedBy,
		VerifiedByExplicit: opts.VerifiedByExplicit,
		VerifiedBySource:   opts.VerifiedBySource,
		Now:                opts.Now,
	}
	convert.ApplyLifecycleMaps(c, f.Rel, cOpts)
	vres := convert.ApplyVerifiedBy(c, cOpts)
	convert.StampGenerated(c.Frontmatter, opts.Version, opts.Now)

	// Opt-in overwrite pass (issue #22): refresh ONLY the named keys, in place.
	// This runs AFTER the additive injectors so it is a no-op when
	// --overwrite-keys is unused (default behavior stays byte-identical).
	if len(opts.OverwriteKeys) > 0 {
		applyOverwrites(c, f, cOpts, opts)
	}

	changed := changedKeys(before, c.Frontmatter)
	if len(changed) == 0 {
		// Nothing changed → no write (no mtime/git churn). The verified outcome (a
		// preserved-scalar advisory, or a Residual-A skip) is still surfaced by the
		// caller — a skip is by construction a no-write, and must still be disclosed.
		return FileResult{Path: f.Rel, Status: StatusUnchanged}, vres, nil
	}

	added, overwritten := splitChanged(before, changed, opts.OverwriteKeys)

	// Disclose boundary-normalization ONLY on a written result: the normalized
	// bytes reach disk exactly when the file is (or, under --dry-run, would be)
	// rewritten. A StatusUnchanged/StatusSkipped file is left byte-identical, so
	// no normalization is persisted and none is claimed (#124, design §9 AC5).
	if opts.DryRun {
		return FileResult{Path: f.Rel, Status: StatusWouldEnrich, Added: added, Overwritten: overwritten, Normalized: notes}, vres, nil
	}

	out, serr := codec.Serialize(c)
	if serr != nil {
		return FileResult{}, convert.VerifiedResult{}, fmt.Errorf("enrich: serializing %q: %w", f.Rel, serr)
	}
	if err := atomicWrite(f.Abs, out); err != nil {
		return FileResult{}, convert.VerifiedResult{}, fmt.Errorf("enrich: writing %q: %w", f.Rel, err)
	}
	return FileResult{Path: f.Rel, Status: StatusEnriched, Added: added, Overwritten: overwritten, Normalized: notes}, vres, nil
}

// applyOverwrites refreshes the frontmatter keys named in opts.OverwriteKeys with
// the values the declarative injectors WOULD produce for a fresh file, updating
// each IN PLACE so key order and every other key stay byte-faithful (issue #22).
//
// It computes the fresh values on a throwaway candidate concept whose frontmatter
// is a deep copy of c's with the overwrite keys removed, so the set-when-absent
// injectors recompute them from --type-map/--default-type and the lifecycle maps.
// A key is only written back when the candidate actually produced a value; if the
// run supplies no source for it (e.g. `status` named with no --status-map), the
// authored value is left untouched rather than clobbered to empty. Trust keys are
// refused at the CLI, so they never reach here.
func applyOverwrites(c *okf.Concept, f convert.SourceFile, cOpts convert.Options, opts Options) {
	cand := &okf.Concept{
		Frontmatter: cloneFMExcluding(c.Frontmatter, opts.OverwriteKeys),
		Body:        c.Body,
	}
	convert.EnsureType(cand.Frontmatter, f.Rel, opts.TypeMap, opts.DefaultType)
	convert.EnsureTitle(cand.Frontmatter, f.Rel, cand.Body)
	convert.ApplyLifecycleMaps(cand, f.Rel, cOpts)

	for key := range opts.OverwriteKeys {
		if v, ok := cand.Frontmatter.Get(key); ok {
			// Set preserves position for an existing key (OrderedMap.Set), so the
			// refreshed value lands in place; for an absent key this is an additive
			// add, identical to the default injector outcome.
			c.Frontmatter.Set(key, v)
		}
	}
}

// cloneFMExcluding returns a deep copy of fm omitting the given keys, so the
// set-when-absent injectors recompute fresh values for them.
func cloneFMExcluding(fm *okf.OrderedMap, exclude map[string]bool) *okf.OrderedMap {
	out := okf.NewOrderedMap()
	for _, k := range fm.Keys() {
		if exclude[k] {
			continue
		}
		v, _ := fm.Get(k)
		out.Set(k, deepCopyValue(v))
	}
	return out
}

// splitChanged partitions the changed keys into additive adds and in-place
// overwrites: a key is an overwrite when it was named in overwriteKeys AND was
// already present before injection; everything else is an add. With no
// overwriteKeys every changed key is an add, so Added keeps its original meaning
// and Overwritten stays nil.
func splitChanged(before fmSnapshot, changed []string, overwriteKeys map[string]bool) (added, overwritten []string) {
	for _, k := range changed {
		_, existedBefore := before[k]
		if overwriteKeys[k] && existedBefore {
			overwritten = append(overwritten, k)
		} else {
			added = append(added, k)
		}
	}
	return added, overwritten
}

// fmSnapshot captures a frontmatter's values by deep copy so a later comparison
// detects in-place value changes (not just added keys).
type fmSnapshot map[string]any

// snapshotFM deep-copies every top-level value of fm so an injector that mutates
// a value in place (e.g. appending to a verified list) is still detectable.
func snapshotFM(fm *okf.OrderedMap) fmSnapshot {
	s := make(fmSnapshot, fm.Len())
	for _, k := range fm.Keys() {
		v, _ := fm.Get(k)
		s[k] = deepCopyValue(v)
	}
	return s
}

// changedKeys returns the keys whose value in fm differs from the snapshot (a
// new key, or a modified value), sorted. Empty ⟺ the frontmatter is unchanged,
// so it is the complete write/no-write signal AND the reported "added" set.
func changedKeys(before fmSnapshot, fm *okf.OrderedMap) []string {
	var changed []string
	for _, k := range fm.Keys() {
		v, _ := fm.Get(k)
		old, existed := before[k]
		if !existed || !reflect.DeepEqual(old, v) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed
}

// deepCopyValue recursively copies plain frontmatter values (maps, slices,
// scalars) so a snapshot is immune to later in-place mutation.
func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[k] = deepCopyValue(val)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, val := range t {
			s[i] = deepCopyValue(val)
		}
		return s
	default:
		return v
	}
}

// atomicWrite writes data to a temp file in the target's directory, fsyncs it,
// then renames it over the target — a same-filesystem atomic replace. The
// original file's mode is preserved (or 0644 for a new file). An interrupt
// leaves either the untouched original or the fully written replacement, never a
// partial/corrupt file. The temp file is cleaned up on any error before rename.
// After a successful rename the parent directory is fsync'd best-effort so the
// rename is durably persisted across a crash; this only hardens durability (the
// atomicity invariant already holds without it) and never fails the write.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".binder-enrich-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// On any failure before the rename, remove the temp file so we never litter.
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Best-effort parent-directory fsync so the rename entry survives a crash.
	// Many platforms/filesystems don't support directory fsync (and it is not
	// portable), so any error opening or syncing the directory is deliberately
	// ignored: the rename already succeeded and durability is a hardening, not a
	// correctness, concern here.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
