package graph

import (
	"path"
	"regexp"
	"strings"
)

var markdownLinkRE = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

type Link struct {
	Raw    string
	Target string
}

func ExtractMarkdownLinks(markdown string) []Link {
	matches := markdownLinkRE.FindAllStringSubmatch(markdown, -1)
	links := make([]Link, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		target := strings.TrimSpace(match[1])
		if target == "" || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "#") {
			continue
		}
		if hash := strings.Index(target, "#"); hash >= 0 {
			target = target[:hash]
		}
		links = append(links, Link{Raw: match[1], Target: target})
	}
	return links
}

func ResolveLink(fromConceptPath, target string) (string, bool) {
	if target == "" || strings.HasPrefix(target, "factile://") {
		return "", false
	}
	if strings.HasPrefix(target, "/") {
		clean := path.Clean(target)
		return strings.TrimSuffix(clean, ".md"), true
	}
	base := path.Dir(fromConceptPath)
	resolved := path.Clean(path.Join(base, target))
	if !strings.HasPrefix(resolved, "/") {
		resolved = "/" + resolved
	}
	return strings.TrimSuffix(resolved, ".md"), true
}
