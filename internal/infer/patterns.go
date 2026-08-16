package infer

import (
	"path/filepath"
	"strings"
)

type patternRule struct {
	check func(filename string) bool
	typ   string
	desc  string
}

var patternRules = []patternRule{
	{
		check: func(f string) bool {
			return strings.HasSuffix(f, "-spec.md") || strings.HasSuffix(f, "_spec.md") || strings.HasPrefix(f, "spec-")
		},
		typ:  "Specification",
		desc: "filename pattern *-spec.md",
	},
	{
		check: func(f string) bool {
			return strings.HasSuffix(f, "-runbook.md") || strings.HasSuffix(f, "_runbook.md") ||
				strings.HasPrefix(f, "troubleshooting") || strings.HasPrefix(f, "runbook-")
		},
		typ:  "Runbook",
		desc: "filename pattern *runbook* / troubleshooting*",
	},
	{
		check: func(f string) bool {
			return strings.HasPrefix(f, "adr-") || strings.HasPrefix(f, "adr_") || strings.HasSuffix(f, "-adr.md")
		},
		typ:  "Decision",
		desc: "filename pattern adr-*",
	},
	{
		check: func(f string) bool {
			return strings.HasPrefix(f, "rfc-") || strings.HasPrefix(f, "rfc_") || strings.HasSuffix(f, "-rfc.md")
		},
		typ:  "Proposal",
		desc: "filename pattern rfc-*",
	},
	{
		check: func(f string) bool {
			return strings.HasSuffix(f, "-guide.md") || strings.HasSuffix(f, "_guide.md") ||
				strings.HasPrefix(f, "guide-") || strings.HasPrefix(f, "howto")
		},
		typ:  "Guide",
		desc: "filename pattern *guide* / howto*",
	},
	{
		check: func(f string) bool {
			return strings.HasPrefix(f, "benchmark-") || strings.HasSuffix(f, "-benchmark.md")
		},
		typ:  "Benchmark",
		desc: "filename pattern *benchmark*",
	},
	{
		check: func(f string) bool {
			return strings.HasPrefix(f, "tutorial-") || strings.HasSuffix(f, "-tutorial.md")
		},
		typ:  "Tutorial",
		desc: "filename pattern *tutorial*",
	},
	{
		check: func(f string) bool {
			return strings.HasPrefix(f, "api-") || strings.HasSuffix(f, "-api.md")
		},
		typ:  "APIReference",
		desc: "filename pattern *api*",
	},
}

// InferFromPatterns inspects file names in a directory and suggests a type if a majority match.
func InferFromPatterns(filenames []string) (suggestedType string, rationale string) {
	if len(filenames) == 0 {
		return "", ""
	}
	counts := make(map[string]int)
	descriptions := make(map[string]string)

	for _, rel := range filenames {
		base := strings.ToLower(filepath.Base(rel))
		for _, rule := range patternRules {
			if rule.check(base) {
				counts[rule.typ]++
				descriptions[rule.typ] = rule.desc
				break
			}
		}
	}

	bestType := ""
	maxCount := 0
	for typ, count := range counts {
		if count > maxCount {
			maxCount = count
			bestType = typ
		}
	}

	// Majority rule: at least 50% of files match the pattern, or at least 2 files
	if maxCount > 0 && (maxCount*2 >= len(filenames) || maxCount >= 2) {
		return bestType, descriptions[bestType]
	}
	return "", ""
}
