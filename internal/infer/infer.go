package infer

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// Infer inspects a markdown corpus at src and proposes a type-map report.
func Infer(ctx context.Context, src string, codec okf.Codec, opts Options) (*Report, error) {
	if codec == nil {
		return nil, fmt.Errorf("infer: codec is required")
	}
	defaultType := strings.TrimSpace(opts.DefaultType)
	if defaultType == "" {
		defaultType = "Note"
	}

	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("infer: source %q: %w", src, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("infer: source %q is not a directory", src)
	}

	files, err := walkCorpus(src)
	if err != nil {
		return nil, fmt.Errorf("infer: walking source: %w", err)
	}

	// Group files by directory
	dirFiles := make(map[string][]sourceFile)
	for _, f := range files {
		dir := path.Dir(f.rel)
		if dir == "." {
			continue // Root files fall back to default-type
		}
		dirFiles[dir] = append(dirFiles[dir], f)
	}

	dirInfo := make(map[string][]FileInfo)
	dirSampleTitles := make(map[string][]string)
	dirFileNames := make(map[string][]string)

	for dir, fList := range dirFiles {
		for _, sf := range fList {
			raw, err := os.ReadFile(sf.abs)
			if err != nil {
				continue
			}

			c, err := parseConceptSafe(codec, sf.rel, raw)
			if err != nil {
				continue
			}

			var authoredType string
			if c.Frontmatter != nil {
				if v, ok := c.Frontmatter.Get("type"); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						authoredType = strings.TrimSpace(s)
					}
				}
			}
			h1 := firstH1(c.Body)
			title := h1
			if c.Frontmatter != nil {
				if v, ok := c.Frontmatter.Get("title"); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						title = strings.TrimSpace(s)
					}
				}
			}
			fi := FileInfo{
				RelPath:      sf.rel,
				Title:        title,
				Frontmatter:  c.Frontmatter,
				AuthoredType: authoredType,
			}
			dirInfo[dir] = append(dirInfo[dir], fi)
			dirFileNames[dir] = append(dirFileNames[dir], sf.rel)
			if title != "" {
				dirSampleTitles[dir] = append(dirSampleTitles[dir], title)
			}
		}
	}

	// 1. Compute deterministic proposals (Tiers 1-3)
	mappingMap := make(map[string]Mapping)

	for dir, fList := range dirInfo {
		var (
			suggestedType string
			source        string
			rationale     string
		)

		// Tier 3: Frontmatter hints / authored types
		if fmType, fmRationale := InferFromFrontmatter(fList); fmType != "" {
			suggestedType = fmType
			source = SourceFrontmatter
			rationale = fmRationale
		}

		// Tier 2: Filename / heading patterns
		if suggestedType == "" {
			if patType, patRationale := InferFromPatterns(dirFileNames[dir]); patType != "" {
				suggestedType = patType
				source = SourcePattern
				rationale = patRationale
			}
		}

		// Tier 1: Folder name heuristic
		if suggestedType == "" {
			if folderType, folderRationale := InferFromFolder(dir); folderType != "" {
				suggestedType = folderType
				source = SourceFolder
				rationale = folderRationale
			}
		}

		if suggestedType != "" {
			var samples []string
			for _, f := range fList {
				samples = append(samples, f.RelPath)
				if len(samples) >= 3 {
					break
				}
			}
			mappingMap[dir] = Mapping{
				Dir:           dir,
				SuggestedType: suggestedType,
				Source:        source,
				Rationale:     rationale,
				SampleFiles:   samples,
			}
		}
	}

	// Non-nil so an empty run marshals warnings as [] rather than null, keeping
	// the JSON envelope's shape stable (docs/user_guide.md).
	warnings := []string{}

	// 2. Optional Tier 4: Gemini semantic inference
	if opts.UseGemini {
		var (
			geminiClient GeminiClient
			modelName    string
			backendName  string
		)

		if opts.GeminiClient != nil {
			geminiClient = opts.GeminiClient
			modelName = opts.GeminiModel
			if modelName == "" {
				modelName = "gemini-3.5-flash-lite"
			}
			backendName = "mock"
		} else {
			client, m, b, err := NewGeminiClient(ctx, opts)
			if err != nil {
				if opts.GeminiRequired {
					return nil, fmt.Errorf("gemini client initialization: %w", err)
				}
				warnings = append(warnings, fmt.Sprintf("gemini tier disabled: %v", err))
			} else {
				geminiClient = client
				modelName = m
				backendName = b
			}
		}

		if geminiClient != nil {
			geminiMap, err := geminiClient.InferDirectoryTypes(ctx, dirFileNames, dirSampleTitles)
			if err != nil {
				if opts.GeminiRequired {
					return nil, fmt.Errorf("gemini semantic inference: %w", err)
				}
				warnings = append(warnings, fmt.Sprintf("gemini inference warning: %v", err))
			} else {
				for dir, gType := range geminiMap {
					cleanType := strings.TrimSpace(gType)
					if cleanType == "" {
						continue
					}
					var samples []string
					if fList, ok := dirInfo[dir]; ok {
						for _, f := range fList {
							samples = append(samples, f.RelPath)
							if len(samples) >= 3 {
								break
							}
						}
					}
					mappingMap[dir] = Mapping{
						Dir:           dir,
						SuggestedType: cleanType,
						Source:        SourceGemini,
						Rationale:     "suggested by Gemini semantic analysis",
						SampleFiles:   samples,
						Model:         modelName,
						Backend:       backendName,
					}
				}
			}
		}
	}

	// 3. Assemble and sort mappings deterministically
	var sortedDirs []string
	for dir := range mappingMap {
		sortedDirs = append(sortedDirs, dir)
	}
	sort.Strings(sortedDirs)

	// Non-nil so an empty corpus marshals mappings as [] rather than null,
	// matching warnings and keeping the whole JSON envelope's shape stable
	// (docs/user_guide.md; same defect class as U8).
	mappings := []Mapping{}
	var typeMapParts []string

	for _, dir := range sortedDirs {
		m := mappingMap[dir]
		mappings = append(mappings, m)
		typeMapParts = append(typeMapParts, fmt.Sprintf("%s=%s", m.Dir, m.SuggestedType))
	}

	report := &Report{
		Src:         src,
		TypeMap:     strings.Join(typeMapParts, ","),
		DefaultType: defaultType,
		Mappings:    mappings,
		Warnings:    warnings,
	}

	return report, nil
}

func parseConceptSafe(codec okf.Codec, rel string, raw []byte) (*okf.Concept, error) {
	if opensFrontmatterFence(raw) {
		c, err := codec.ParseConcept(rel, raw)
		if err == nil {
			return c, nil
		}
	}
	id, _ := codec.ConceptIDFromRel(rel)
	return &okf.Concept{
		ID:          id,
		RelPath:     rel,
		Frontmatter: okf.NewOrderedMap(),
		Body:        strings.ReplaceAll(string(raw), "\r\n", "\n"),
	}, nil
}

func opensFrontmatterFence(raw []byte) bool {
	s := strings.TrimLeft(string(raw), "\ufeff") // strip UTF-8 BOM
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.HasPrefix(s, "---\n") || s == "---"
}

type sourceFile struct {
	rel string
	abs string
}

func walkCorpus(root string) ([]sourceFile, error) {
	var files []sourceFile
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, sourceFile{rel: filepath.ToSlash(rel), abs: p})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, nil
}

func firstH1(body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "# "))
		}
	}
	return ""
}
