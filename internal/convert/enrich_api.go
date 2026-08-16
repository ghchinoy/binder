package convert

import (
	"time"

	"github.com/ghchinoy/binder/internal/okf"
)

// This file exports thin, behavior-preserving wrappers around convert's
// internal frontmatter-injection helpers so a sibling package (internal/enrich,
// issue #5) can reuse the EXACT same tested machinery without forking it. Each
// wrapper delegates verbatim to the unexported helper it names; convert's own
// code paths and output are untouched (the byte-identical regression still
// holds). enrich needs frontmatter-only injection — it deliberately does NOT
// reuse Analyze, whose pipeline rewrites bodies (links, ## Related, tag merge).

// SourceFile is one markdown file discovered under a corpus root: a
// slash-separated path relative to the root and its absolute filesystem path.
type SourceFile struct {
	Rel string
	Abs string
}

// WalkCorpus returns every .md file under root in deterministic (sorted) order.
// It is the exported view of the walk convert uses, so enrich discovers files
// identically (same ordering, same .md filter).
func WalkCorpus(root string) ([]SourceFile, error) {
	files, err := walkCorpus(root)
	if err != nil {
		return nil, err
	}
	out := make([]SourceFile, 0, len(files))
	for _, f := range files {
		out = append(out, SourceFile{Rel: f.rel, Abs: f.abs})
	}
	return out, nil
}

// OpensFrontmatterFence reports whether raw begins with a "---" fence line, i.e.
// the file intends to carry frontmatter (even if unterminated or invalid). It is
// the router enrich uses to decide ParseConcept vs a fresh plainConcept block.
func OpensFrontmatterFence(raw []byte) bool { return opensFrontmatterFence(raw) }

// PlainConcept builds a concept from raw bytes treated as plain markdown: an
// empty frontmatter block plus the whole file as the body. enrich uses it for
// no-frontmatter files, which then receive a fresh injected block.
func PlainConcept(codec okf.Codec, relPath string, raw []byte) *okf.Concept {
	return plainConcept(codec, relPath, raw)
}

// EnsureType applies the type precedence (existing → --type-map → --default-type)
// and returns the effective value, set-when-absent. Behavior is identical to the
// helper convert uses.
func EnsureType(fm *okf.OrderedMap, relPath string, typeMap map[string]string, defaultType string) string {
	return ensureType(fm, relPath, typeMap, defaultType)
}

// EnsureTitle applies the title precedence (existing → first H1 → humanized
// filename), set-when-absent. Behavior is identical to the helper convert uses.
func EnsureTitle(fm *okf.OrderedMap, relPath, body string) {
	ensureTitle(fm, relPath, body)
}

// StampGenerated records generated: {by: "binder/<ver>", at: <ISO8601>} but only
// when the concept carries no generated stamp (set-when-absent, idempotent).
func StampGenerated(fm *okf.OrderedMap, version string, now time.Time) {
	stampGenerated(fm, version, now)
}

// ApplyLifecycleMaps sets status/stale_after from opts.StatusMap/StatusDefault/
// StaleAfterMap matched against relPath's directory prefix, each set-when-absent.
// Behavior is identical to the helper convert uses (#7 declarative injectors).
func ApplyLifecycleMaps(c *okf.Concept, relPath string, opts Options) {
	applyLifecycleMaps(c, relPath, opts)
}

// ApplyVerifiedBy appends a verified actorstamp for opts.VerifiedBy (dedup by
// by,at), set-when-absent/idempotent, and returns a VerifiedResult so enrich can
// disclose what it wrote or declined (Residual B). It honors Residual A (a
// non-explicit config/env actor never co-signs a different identity) and the
// preserve-or-advise carry-forward for a spec-invalid scalar verified value.
// Behavior is identical to the helper convert uses.
func ApplyVerifiedBy(c *okf.Concept, opts Options) VerifiedResult {
	return applyVerifiedBy(c, opts)
}
