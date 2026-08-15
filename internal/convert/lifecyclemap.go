package convert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ghchinoy/binder/internal/okf"
)

// statusDefaultKey is the special --status-map key applied when a file's
// directory matches no other prefix (design §3.2 point 1).
const statusDefaultKey = "default"

// staleAfterRE pins the relative-date grammar for --stale-after-map values:
// "+<N><unit>" where unit is d (days), m (months), or y (years). It is validated
// at parse time so the deterministic date computation at convert time cannot fail.
var staleAfterRE = regexp.MustCompile(`^\+(\d+)([dmy])$`)

// parseKVPairs parses a "k1=v1,k2=v2" flag value into a map. Empty input yields
// a nil map (the flag is off). A malformed entry is an error carrying label so
// the caller can surface it as a usage error (exit 2). Keys and values are
// trimmed; both must be non-empty.
func parseKVPairs(s, label string) (map[string]string, error) {
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
			return nil, fmt.Errorf("invalid %s entry %q (want dir=value)", label, pair)
		}
		out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return out, nil
}

// ParseStatusMap parses a --status-map value of the form
// "archive=deprecated,drafts=draft,default=active". It returns the per-directory
// prefix map (excluding the special "default" key) and the default value applied
// when no prefix matches ("" if no default= key is present). Status VALUES are
// user-chosen and NOT validated here (unknown values remain a downstream
// advisory, never-reject); only the map SHAPE is validated → usage error (exit 2).
func ParseStatusMap(s string) (prefixes map[string]string, def string, err error) {
	m, err := parseKVPairs(s, "--status-map")
	if err != nil {
		return nil, "", err
	}
	if len(m) == 0 {
		return nil, "", nil
	}
	def = m[statusDefaultKey]
	delete(m, statusDefaultKey)
	return m, def, nil
}

// ParseStaleAfterMap parses a --stale-after-map value of the form
// "07-benchmarks=+6m,03-transcription=+1y,legacy=+0d". Every value must match the
// relative-date grammar ^\+(\d+)([dmy])$; a malformed map or date is a usage
// error (exit 2). The returned map holds validated specs, resolved to absolute
// dates at convert time against opts.Now (SOURCE_DATE_EPOCH-aware, deterministic).
func ParseStaleAfterMap(s string) (map[string]string, error) {
	m, err := parseKVPairs(s, "--stale-after-map")
	if err != nil {
		return nil, err
	}
	for k, v := range m {
		if !staleAfterRE.MatchString(v) {
			return nil, fmt.Errorf("invalid --stale-after-map value %q for %q (want +<N>d, +<N>m, or +<N>y, e.g. +6m)", v, k)
		}
	}
	return m, nil
}

// relativeDate resolves a validated relative-date spec ("+<N><unit>") to an
// absolute YYYY-MM-DD date relative to now, using UTC calendar arithmetic
// (d→days, m→months, y→years). +0d is today. It assumes spec already matched
// staleAfterRE (ParseStaleAfterMap enforces this), so it cannot fail.
func relativeDate(spec string, now time.Time) string {
	m := staleAfterRE.FindStringSubmatch(spec)
	n, _ := strconv.Atoi(m[1])
	base := now.UTC()
	switch m[2] {
	case "d":
		base = base.AddDate(0, 0, n)
	case "m":
		base = base.AddDate(0, n, 0)
	case "y":
		base = base.AddDate(n, 0, 0)
	}
	return base.Format("2006-01-02")
}

// applyLifecycleMaps sets status and stale_after from the declarative --status-map
// and --stale-after-map, matched against relPath's directory prefix. Each is set
// ONLY when the frontmatter key is absent (never clobbers authored values). With
// neither map configured this is a no-op, so default-off output is byte-identical.
func applyLifecycleMaps(c *okf.Concept, relPath string, opts Options) {
	fm := c.Frontmatter

	// status: longest-prefix match, falling back to the special default= value.
	if (len(opts.StatusMap) > 0 || opts.StatusDefault != "") && !hasNonEmpty(fm, "status") {
		status := lookupPrefixMap(opts.StatusMap, relPath)
		if status == "" {
			status = opts.StatusDefault
		}
		if status != "" {
			fm.Set("status", status)
		}
	}

	// stale_after: longest-prefix match, resolved to an absolute date at opts.Now.
	if len(opts.StaleAfterMap) > 0 && !hasNonEmpty(fm, "stale_after") {
		if spec := lookupPrefixMap(opts.StaleAfterMap, relPath); spec != "" {
			fm.Set("stale_after", relativeDate(spec, opts.Now))
		}
	}
}
