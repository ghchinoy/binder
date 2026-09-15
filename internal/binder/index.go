package binder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ghchinoy/binder/internal/bundle"
	"github.com/ghchinoy/binder/internal/convert"
	"github.com/ghchinoy/binder/internal/okf"
)

// IndexRequest carries the RESOLVED inputs an index run needs. index has no JSON
// envelope (it emits a write manifest, not a report), so there is no Version/Strict:
// it never gates and never serializes a binder.report/v1 payload.
type IndexRequest struct {
	// Root is the bundle directory whose per-directory index.md tree is regenerated.
	Root string
	// DryRun reports which index.md files would be written without writing them.
	DryRun bool
	// Index-catalog options (issue #9), off by default → byte-identical output.
	GroupByType      bool
	IncludeBacklinks bool
	IncludeGraph     bool
}

// IndexEntry is one line of the write manifest: the action taken (or that would be
// taken under DryRun) and the bundle-relative index.md path. The adapter renders it
// to stdout — "would <action> <rel>" under DryRun, else "<action> <rel>".
type IndexEntry struct {
	Action string // "write" (new file) | "regenerate" (existing file replaced)
	Rel    string
}

// IndexResult is the COMPLETE index outcome as DATA: the write manifest and the
// unparsed-file advisories. The service performs the actual writes (owning the sorted
// order, path join, write-vs-regenerate classification, and dry-run short-circuit —
// all of which used to live in the CLI RunE); the adapter only renders the manifest
// to stdout and the warnings to stderr, so disk behavior and both streams stay
// byte-identical.
type IndexResult struct {
	// Entries is the manifest of files written (or, under DryRun, that would be),
	// in the same sorted order the writes happened. On a mid-run IO failure it holds
	// exactly the entries whose files were already written — preserving the CLI's
	// interleaved-manifest-on-failure behavior — and Index returns a non-nil error.
	Entries []IndexEntry
	// Warnings are the unparsed-file disclosure lines (stderr) as data — the shared
	// UnparsedWarnings text, so index and the read-side commands cannot drift.
	Warnings []string
	// DryRun echoes the request so the adapter renders "would ..." lines.
	DryRun bool
}

// Index (re)generates the per-directory index.md nav tree for a bundle (spec §8). It
// loads the bundle, collects the unparsed advisories, then writes each index.md in
// sorted order (classifying write vs regenerate by whether the target already
// exists), or, under DryRun, records the manifest without writing. It returns the
// complete IndexResult; on an IO failure it returns the partial result (entries for
// the files already written) plus the error, so the adapter can render what happened
// before surfacing the failure — exactly as the pre-collapse CLI loop did.
func (s *Service) Index(ctx context.Context, req IndexRequest) (IndexResult, error) {
	_ = ctx

	b, err := bundle.Load(req.Root, s.codec)
	if err != nil {
		return IndexResult{}, err
	}

	res := IndexResult{DryRun: req.DryRun, Warnings: UnparsedWarnings(b)}

	indexes := convert.GenerateIndexes(b.Concepts, b.OKFVersion, convert.IndexOptions{
		GroupByType:      req.GroupByType,
		IncludeBacklinks: req.IncludeBacklinks,
		IncludeGraph:     req.IncludeGraph,
	})

	rels := make([]string, 0, len(indexes))
	for rel := range indexes {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	for _, rel := range rels {
		dst := filepath.Join(req.Root, filepath.FromSlash(rel))
		action := "write"
		if _, err := os.Stat(dst); err == nil {
			action = "regenerate"
		}
		if req.DryRun {
			res.Entries = append(res.Entries, IndexEntry{Action: action, Rel: rel})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return res, err
		}
		if err := os.WriteFile(dst, indexes[rel], 0o644); err != nil {
			return res, err
		}
		// Record the entry only after a successful write, so a mid-run failure leaves
		// the manifest naming exactly the files that reached disk (matching the CLI's
		// old print-after-write loop).
		res.Entries = append(res.Entries, IndexEntry{Action: action, Rel: rel})
	}
	return res, nil
}

// UnparsedWarnings returns the stderr disclosure lines for every file the bundle
// loader could not parse (#161/#163), as DATA — one message per unparsed concept
// (kept in the nav, recovered as body, never dropped) and one for an unparseable root
// index.md (okf_version not adopted). Each line is complete but carries no trailing
// newline; the caller frames it. This is the single home for the text that
// Service.Index and cmd.warnUnparsed both emit, so the two cannot drift. It returns
// no lines when the bundle parsed cleanly.
func UnparsedWarnings(b *okf.Bundle) []string {
	var out []string
	for _, u := range b.Unparsed {
		out = append(out, fmt.Sprintf(
			"warning: %s: frontmatter did not parse (%s); kept as body under never-reject and reported as unparsed",
			u.RelPath, u.Err))
	}
	if b.RootVersionUnparsed != nil {
		out = append(out, fmt.Sprintf(
			"warning: %s: frontmatter did not parse (%s); okf_version not adopted, using default %s",
			b.RootVersionUnparsed.RelPath, b.RootVersionUnparsed.Err, b.OKFVersion))
	}
	return out
}
