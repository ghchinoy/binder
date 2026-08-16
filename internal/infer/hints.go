package infer

import (
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// FileInfo holds metadata about a source file for inference.
type FileInfo struct {
	RelPath      string
	Title        string
	Frontmatter  *okf.OrderedMap
	AuthoredType string
}

// InferFromFrontmatter checks authored frontmatter types and key presence in a directory.
func InferFromFrontmatter(files []FileInfo) (suggestedType string, rationale string) {
	if len(files) == 0 {
		return "", ""
	}

	// 1. Check existing authored types
	typeCounts := make(map[string]int)
	for _, f := range files {
		if f.AuthoredType != "" {
			typeCounts[f.AuthoredType]++
		}
	}
	bestAuthored := ""
	maxAuthored := 0
	for typ, count := range typeCounts {
		if count > maxAuthored {
			maxAuthored = count
			bestAuthored = typ
		}
	}
	if maxAuthored > 0 && maxAuthored*2 >= len(files) {
		return bestAuthored, "majority of files carry authored type \"" + bestAuthored + "\""
	}

	// 2. Check frontmatter key heuristics
	keyCounts := make(map[string]int)
	for _, f := range files {
		if f.Frontmatter == nil {
			continue
		}
		if f.Frontmatter.Has("goal") {
			keyCounts["Proposal"]++
		}
		if f.Frontmatter.Has("runtime") || f.Frontmatter.Has("entrypoint") {
			keyCounts["Attested Computation"]++
		}
		if f.Frontmatter.Has("sql") || f.Frontmatter.Has("query") {
			keyCounts["Table"]++
		}
		if f.Frontmatter.Has("schema") {
			keyCounts["Schema"]++
		}
		if f.Frontmatter.Has("metric") || f.Frontmatter.Has("formula") {
			keyCounts["Metric"]++
		}
	}

	bestKeyType := ""
	maxKeyCount := 0
	for typ, count := range keyCounts {
		if count > maxKeyCount {
			maxKeyCount = count
			bestKeyType = typ
		}
	}
	if maxKeyCount > 0 && maxKeyCount*2 >= len(files) {
		return bestKeyType, "frontmatter keys in directory suggest " + strings.ToLower(bestKeyType)
	}

	return "", ""
}
