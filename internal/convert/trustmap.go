package convert

import (
	"regexp"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// mapTrust maps corpus-native provenance into v0.2 trust signals, but ONLY where
// the run is explicitly configured to (design-v2 §3.2 / Phase-2 point 7). Every
// mapping is deterministic and additive: original keys are preserved, and binder
// never fabricates a source or a credibility score (§5.1). With no mapping
// options set, this is a no-op and frontmatter round-trips unchanged.
func mapTrust(c *okf.Concept, opts Options) {
	fm := c.Frontmatter

	// draft marker → status (only when status is absent, to never clobber).
	if opts.MapDraft && !hasNonEmpty(fm, "status") {
		if v, ok := fm.Get("draft"); ok && isTrue(v) {
			fm.Set("status", "draft")
		}
	}

	var mapped []any
	// frontmatter source keys → sources entries.
	for _, key := range opts.SourceKeys {
		v, ok := fm.Get(key)
		if !ok {
			continue
		}
		val := strings.TrimSpace(okf.AsString(v))
		if val == "" {
			continue
		}
		entry := map[string]any{}
		if key == "author" {
			entry["author"] = val
		} else {
			entry["resource"] = val
		}
		mapped = append(mapped, entry)
	}

	// body "# Citations" list → sources entries.
	if opts.MapCitations {
		for _, cite := range parseCitations(c.Body) {
			entry := map[string]any{}
			if cite.resource != "" {
				entry["resource"] = cite.resource
			}
			if cite.title != "" {
				entry["title"] = cite.title
			}
			if len(entry) > 0 {
				mapped = append(mapped, entry)
			}
		}
	}

	if len(mapped) == 0 {
		return
	}

	existing, _ := fm.Get("sources")
	list, _ := existing.([]any)
	// De-duplicate against existing sources by (resource,title,author).
	seen := map[string]bool{}
	for _, e := range list {
		seen[sourceKey(e)] = true
	}
	for _, e := range mapped {
		k := sourceKey(e)
		if seen[k] {
			continue
		}
		seen[k] = true
		list = append(list, e)
	}
	fm.Set("sources", list)
}

type citation struct {
	resource string
	title    string
}

var (
	headingRE  = regexp.MustCompile(`(?m)^#{1,6}\s+(.*)$`)
	listItemRE = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
	mdLinkOne  = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
	urlRE      = regexp.MustCompile(`^<?(https?://[^\s>]+)>?$`)
)

// parseCitations extracts list items from a body "# Citations" section (any
// heading level). Each item becomes a citation: a markdown link yields
// {title,resource}; a bare URL yields {resource}; other text yields {title}.
func parseCitations(body string) []citation {
	lines := strings.Split(body, "\n")
	var out []citation
	inSection := false
	for _, line := range lines {
		if m := headingRE.FindStringSubmatch(line); m != nil {
			inSection = strings.EqualFold(strings.TrimSpace(m[1]), "Citations")
			continue
		}
		if !inSection {
			continue
		}
		m := listItemRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		item := strings.TrimSpace(m[1])
		if item == "" {
			continue
		}
		if lm := mdLinkOne.FindStringSubmatch(item); lm != nil {
			out = append(out, citation{title: strings.TrimSpace(lm[1]), resource: strings.TrimSpace(lm[2])})
			continue
		}
		if um := urlRE.FindStringSubmatch(item); um != nil {
			out = append(out, citation{resource: um[1]})
			continue
		}
		out = append(out, citation{title: item})
	}
	return out
}

func sourceKey(e any) string {
	m, ok := e.(map[string]any)
	if !ok {
		return okf.AsString(e)
	}
	return okf.AsString(m["resource"]) + "\x00" + okf.AsString(m["title"]) + "\x00" + okf.AsString(m["author"])
}

func hasNonEmpty(fm *okf.OrderedMap, key string) bool {
	v, ok := fm.Get(key)
	return ok && strings.TrimSpace(okf.AsString(v)) != ""
}

func isTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}
