// Package convert is the novel Phase-1 work: it walks a plain-markdown corpus
// and emits a conformant OKF v0.2 bundle. It depends only on the binder-owned
// okf interfaces (never on a concrete codec or factile). See design-v2 §4.
//
// Phase-1 scope: one concept per non-reserved .md; type defaulting; title
// derivation; standard markdown-link extraction + bundle-relative rewrite;
// reserved-file collision handling; a root index.md carrying okf_version; a
// generated provenance stamp; and byte-faithful preservation of all existing
// frontmatter (including trust / Attested-Computation families). Wikilinks,
// tags, frontmatter-refs and per-directory indexes are deferred to Phase 2.
package convert

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghchinoy/binder/internal/okf"
)

// Options configures a conversion run.
type Options struct {
	Codec       okf.Codec         // required
	DefaultType string            // fallback type; defaults to "Note"
	TypeMap     map[string]string // per-directory type overrides
	FMRefKeys   []string          // frontmatter keys treated as relationship edges (§4.2)
	Version     string            // binder version, used in generated.by
	Now         time.Time         // clock for generated.at; controls determinism
	DryRun      bool              // when true, write nothing

	// WorkspaceRoot bounds file:// URI resolution (issue #6). A file:// target
	// whose absolute path falls inside this root is treated as an internal edge
	// candidate; targets outside stay external. Defaults to the corpus <src>
	// root. Relative values are resolved against the process working directory.
	WorkspaceRoot string

	// Trust-mapping options (design-v2 §3.2 / Phase-2 point 7). All are OFF by
	// default and deterministic; binder never fabricates provenance.
	MapCitations bool     // map a body "# Citations" list to sources entries
	SourceKeys   []string // frontmatter keys to map into sources entries
	MapDraft     bool     // map a `draft: true` marker to status: draft

	// Index-catalog options (issue #9). All OFF by default → generated index.md
	// output is byte-identical to before. See IndexOptions.
	GroupByType      bool // append the additive "# Catalog" to the root index.md
	IncludeBacklinks bool // annotate catalog entries with inbound resolved edges
	IncludeGraph     bool // annotate catalog entries with outbound resolved edges
}

// Convert runs the corpus→bundle conversion. It never mutates the source. With
// DryRun set it writes nothing. Given identical input and the same Options.Now,
// output is byte-identical (deterministic).
func Convert(src, out string, opts Options) (*Report, error) {
	if opts.Codec == nil {
		return nil, fmt.Errorf("convert: codec is required")
	}
	if strings.TrimSpace(opts.DefaultType) == "" {
		opts.DefaultType = "Note"
	}
	if strings.TrimSpace(opts.Version) == "" {
		opts.Version = "dev"
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}

	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("convert: source %q: %w", src, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("convert: source %q is not a directory", src)
	}

	files, err := walkCorpus(src)
	if err != nil {
		return nil, fmt.Errorf("convert: walking source: %w", err)
	}

	// Absolute corpus source root and workspace boundary for file:// resolution
	// (issue #6). WorkspaceRoot defaults to the corpus <src> root; both are made
	// absolute and cleaned so path comparisons in rewriteLinks are unambiguous.
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return nil, fmt.Errorf("convert: resolving source path %q: %w", src, err)
	}
	absSrc = filepath.Clean(absSrc)
	absWorkspace := absSrc
	if ws := strings.TrimSpace(opts.WorkspaceRoot); ws != "" {
		absWorkspace, err = filepath.Abs(ws)
		if err != nil {
			return nil, fmt.Errorf("convert: resolving workspace root %q: %w", ws, err)
		}
		absWorkspace = filepath.Clean(absWorkspace)
	}

	// Initialize the report slices so an empty run serializes to [] rather than
	// null in --json mode (#13 empty-slice policy). Prose output is unaffected.
	report := &Report{
		Src:        src,
		Out:        out,
		DryRun:     opts.DryRun,
		Concepts:   []ConceptReport{},
		Warnings:   []string{},
		Unresolved: []UnresolvedLink{},
	}

	// Phase 1: resolve output paths, renaming reserved-name source files so they
	// are never dropped (spec §3.1).
	srcToOut, renameWarnings := planOutputPaths(files, opts.Codec)
	for _, w := range renameWarnings {
		report.addWarning("%s", w)
	}

	// Pass 1: parse every file, apply type/title defaulting, and record its
	// identity. Titles must be resolved for ALL files before any body is rewritten
	// because wikilinks resolve against the corpus's titles (design-v2 §4.2).
	type staged struct {
		src string // source-relative path (for relative link resolution)
		out string // output-relative path
		c   *okf.Concept
	}
	var items []staged
	entries := make([]indexEntry, 0, len(files))
	for _, f := range files {
		outRel := srcToOut[f.rel]
		raw, err := os.ReadFile(f.abs)
		if err != nil {
			return nil, fmt.Errorf("convert: reading %q: %w", f.rel, err)
		}
		c, err := toConcept(opts.Codec, outRel, raw)
		recovered := false
		if err != nil {
			// Never-reject (spec §11 / design-v2): a source file whose frontmatter
			// will not parse must not abort the whole conversion, nor be dropped.
			// Preserve it losslessly as a plain-markdown concept — its raw text
			// (including the unparsed fence block) becomes the body — and report it,
			// so the run completes and no content is lost.
			c = plainConcept(opts.Codec, outRel, raw)
			recovered = true
			report.NumRecovered++
			report.addWarning("%s: frontmatter did not parse (%v); converted as plain markdown (original text preserved in body)", f.rel, err)
		}
		typ := ensureType(c.Frontmatter, outRel, opts.TypeMap, opts.DefaultType)
		ensureTitle(c.Frontmatter, outRel, c.Body)
		c.Type = typ
		if recovered {
			// Stamp an explicit, persisted recovery marker (design-v2 §4.6). Both
			// --report (the warning above) and `binder review` derive "recovered"
			// from this single fact, so they can never disagree — and no fragile
			// body-shape heuristic is needed.
			okf.MarkRecovered(c.Frontmatter, "unparseable-frontmatter")
		}

		items = append(items, staged{src: f.rel, out: outRel, c: c})
		entries = append(entries, indexEntry{srcRel: f.rel, outRel: outRel, title: conceptTitle(c)})
	}
	index := buildCorpusIndex(entries)

	// Pass 2: extract every relationship signal, merge tags, map trust where
	// configured, stamp provenance, and project the typed trust view.
	resolver := &linkResolver{
		srcToOut: srcToOut,
		srcRoot:  absSrc,
		wsRoot:   absWorkspace,
		warn:     report.addWarning,
	}
	var concepts []*okf.Concept
	for _, it := range items {
		c := it.c

		// Standard markdown links (P1, incl. file:// URIs) + wikilinks (P2), both
		// rewritten to bundle-relative-absolute form; unresolved links left in place.
		body, links := resolver.rewrite(c.Body, it.src)
		body, wlinks := rewriteWikilinks(body, path.Dir(it.out), index)
		c.Body = body
		links = append(links, wlinks...)

		// Frontmatter-ref edges (§4.2): additive, original frontmatter key/value
		// preserved. Each resolved target is ALSO materialized as a real markdown
		// link in a stable trailing "## Related" section so the relationship
		// survives reload and is visible to `binder graph`/`binder review` — which
		// rebuild edges only from persisted body links. Unresolved refs are omitted
		// from the block and reported like any other unresolved edge.
		fmEdges := frontmatterRefEdges(c.Frontmatter, it.out, opts.FMRefKeys, index)
		links = append(links, fmEdges...)
		c.Body = appendRelatedSection(c.Body, fmEdges)
		c.Links = links

		// Hashtags + frontmatter tags merge/dedupe (spec §4).
		mergeTags(c.Frontmatter, extractHashtags(c.Body))

		// Corpus-native provenance → trust signals, where configured (§3.2).
		mapTrust(c, opts)

		stampGenerated(c.Frontmatter, opts.Version, opts.Now)
		c.Trust = okf.ProjectTrust(c.Frontmatter, c.Type)

		concepts = append(concepts, c)
		report.Concepts = append(report.Concepts, conceptReport(c))
		report.addUnresolved(c)
	}

	// Tally.
	report.NumConcepts = len(concepts)
	for _, c := range concepts {
		for _, l := range c.Links {
			report.NumLinks++
			if l.Resolved {
				report.NumResolved++
			} else {
				report.NumUnresolved++
			}
		}
	}

	if opts.DryRun {
		return report, nil
	}

	if err := writeBundle(out, concepts, opts.Codec, IndexOptions{
		GroupByType:      opts.GroupByType,
		IncludeBacklinks: opts.IncludeBacklinks,
		IncludeGraph:     opts.IncludeGraph,
	}); err != nil {
		return nil, err
	}
	return report, nil
}

// toConcept parses raw into a Concept. Files without frontmatter (the common
// plain-markdown case) become a concept with an empty frontmatter block and the
// whole file as the body.
// toConcept parses raw into a Concept. A file that opens a "---" fence is routed
// to the codec parser; any parse error it returns (invalid YAML in a closed
// fence, or an unterminated fence) is a frontmatter-parse failure the caller
// recovers from by preserving the file as plain markdown (never-reject). A file
// with no opening fence is plain markdown and never errors here — so the only
// error toConcept can return is a frontmatter-parse failure, never I/O.
func toConcept(codec okf.Codec, outRel string, raw []byte) (*okf.Concept, error) {
	if opensFrontmatterFence(raw) {
		return codec.ParseConcept(outRel, raw)
	}
	return plainConcept(codec, outRel, raw), nil
}

// plainConcept builds a concept from raw bytes treated as plain markdown: an
// empty frontmatter block plus the whole file as the body. It is used for files
// with no frontmatter and, as a never-reject fallback, for files whose
// frontmatter will not parse (the raw text, fence and all, is preserved as body).
func plainConcept(codec okf.Codec, outRel string, raw []byte) *okf.Concept {
	id, _ := codec.ConceptIDFromRel(outRel)
	return &okf.Concept{
		ID:          id,
		RelPath:     outRel,
		Frontmatter: okf.NewOrderedMap(),
		Body:        strings.ReplaceAll(string(raw), "\r\n", "\n"),
	}
}

// planOutputPaths maps each source path to its output path, renaming files whose
// base name is reserved (index.md/log.md) to "<stem>-note.md" with collision
// suffixes, so no content is silently dropped.
func planOutputPaths(files []sourceFile, codec okf.Codec) (map[string]string, []string) {
	srcToOut := make(map[string]string, len(files))
	taken := map[string]bool{}
	var warnings []string

	for _, f := range files {
		if !codec.IsReservedFile(f.rel) {
			srcToOut[f.rel] = f.rel
			taken[f.rel] = true
		}
	}
	for _, f := range files {
		if !codec.IsReservedFile(f.rel) {
			continue
		}
		dir := path.Dir(f.rel)
		stem := strings.TrimSuffix(path.Base(f.rel), ".md")
		candidate := joinRel(dir, stem+"-note.md")
		for n := 2; taken[candidate]; n++ {
			candidate = joinRel(dir, fmt.Sprintf("%s-note-%d.md", stem, n))
		}
		srcToOut[f.rel] = candidate
		taken[candidate] = true
		warnings = append(warnings, fmt.Sprintf(
			"reserved filename %q renamed to %q (spec §3.1); binder generates its own index.md",
			f.rel, candidate))
	}
	return srcToOut, warnings
}

func joinRel(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

func writeBundle(out string, concepts []*okf.Concept, codec okf.Codec, idxOpts IndexOptions) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("convert: creating output %q: %w", out, err)
	}
	for _, c := range concepts {
		data, err := codec.Serialize(c)
		if err != nil {
			return fmt.Errorf("convert: serializing %q: %w", c.RelPath, err)
		}
		dst := filepath.Join(out, filepath.FromSlash(c.RelPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("convert: creating dir for %q: %w", c.RelPath, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("convert: writing %q: %w", c.RelPath, err)
		}
	}
	for rel, data := range GenerateIndexes(concepts, okf.DefaultSpecVersion, idxOpts) {
		dst := filepath.Join(out, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("convert: creating dir for %q: %w", rel, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("convert: writing %q: %w", rel, err)
		}
	}
	return nil
}

func conceptReport(c *okf.Concept) ConceptReport {
	cr := ConceptReport{RelPath: c.RelPath, Type: c.Type, NumLinks: len(c.Links)}
	if v, ok := c.Frontmatter.Get("title"); ok {
		cr.Title, _ = v.(string)
	}
	for _, l := range c.Links {
		if !l.Resolved {
			cr.NumUnresolved++
		}
	}
	return cr
}

// ParseTypeMap parses a --type-map value of the form "dir=Type,dir2=Type2".
func ParseTypeMap(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 || strings.TrimSpace(kv[0]) == "" || strings.TrimSpace(kv[1]) == "" {
			return nil, fmt.Errorf("invalid --type-map entry %q (want dir=Type)", pair)
		}
		out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return out, nil
}
