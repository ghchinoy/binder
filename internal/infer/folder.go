package infer

import (
	"path"
	"strings"
)

var wellKnownDirs = map[string]string{
	"subsystem":      "Subsystem",
	"subsystems":     "Subsystem",
	"runbook":        "Runbook",
	"runbooks":       "Runbook",
	"playbook":       "Playbook",
	"playbooks":      "Playbook",
	"proposal":       "Proposal",
	"proposals":      "Proposal",
	"guide":          "Guide",
	"guides":         "Guide",
	"doc":            "Guide",
	"docs":           "Guide",
	"documentation":  "Guide",
	"developer":      "Guide",
	"developers":     "Guide",
	"decision":       "Decision",
	"decisions":      "Decision",
	"adr":            "Decision",
	"adrs":           "Decision",
	"rfc":            "Proposal",
	"rfcs":           "Proposal",
	"spec":           "Specification",
	"specs":          "Specification",
	"specification":  "Specification",
	"specifications": "Specification",
	"metric":         "Metric",
	"metrics":        "Metric",
	"table":          "Table",
	"tables":         "Table",
	"dataset":        "Dataset",
	"datasets":       "Dataset",
	"reference":      "Reference",
	"references":     "Reference",
	"computation":    "Computation",
	"computations":   "Computation",
	"policy":         "Policy",
	"policies":       "Policy",
	"benchmark":      "Benchmark",
	"benchmarks":     "Benchmark",
	"capability":     "Capability",
	"capabilities":   "Capability",
	"journal":        "Journal",
	"journals":       "Journal",
	"post":           "Post",
	"posts":          "Post",
	"article":        "Article",
	"articles":       "Article",
	"tutorial":       "Tutorial",
	"tutorials":      "Tutorial",
	"architecture":   "Architecture",
	"api":            "APIReference",
	"apis":           "APIReference",
}

// InferFromFolder suggests a type name based on a directory path.
func InferFromFolder(dirPath string) (suggestedType string, rationale string) {
	clean := strings.Trim(dirPath, "/")
	if clean == "" || clean == "." {
		return "", ""
	}
	segments := strings.Split(clean, "/")

	// Check segments from deepest to shallowest for well-known directory names
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		lower := strings.ToLower(seg)
		if typ, ok := wellKnownDirs[lower]; ok {
			return typ, "well-known directory name \"" + seg + "\""
		}
	}

	base := path.Base(clean)
	sing := singularize(base)
	title := humanizeTitle(sing)
	if title != "" {
		return title, "inferred from directory name \"" + base + "\""
	}
	return "", ""
}

// singularize applies basic English noun singularization.
func singularize(word string) string {
	lower := strings.ToLower(word)
	if len(lower) <= 3 {
		return word
	}
	if strings.HasSuffix(lower, "ies") && len(lower) > 4 {
		return word[:len(word)-3] + "y"
	}
	if strings.HasSuffix(lower, "ses") && len(lower) > 4 {
		return word[:len(word)-2]
	}
	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") && !strings.HasSuffix(lower, "us") {
		return word[:len(word)-1]
	}
	return word
}

// humanizeTitle formats "data-sources" as "Data Source".
func humanizeTitle(name string) string {
	name = strings.NewReplacer("-", " ", "_", " ").Replace(name)
	fields := strings.Fields(name)
	for i, f := range fields {
		if len(f) == 0 {
			continue
		}
		r := []rune(f)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		fields[i] = string(r)
	}
	return strings.Join(fields, " ")
}
