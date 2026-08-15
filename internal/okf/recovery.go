package okf

// RecoveryMarkerKey is the binder-namespaced frontmatter key stamped on a concept
// whose original frontmatter could not be parsed and was preserved verbatim as
// body (never-reject, design-v2 §4.6). It is binder-owned (no OKF-vocabulary
// collision), round-trips as an unknown key (spec §11 — preserved, never a
// rejection reason), and is the single authoritative signal both
// `binder convert --report` and `binder review` read to report recovery, so the
// two surfaces can never disagree. A body-shape heuristic cannot separate a
// recovered file's inert "---"+mapping body from a legitimate thematic-break body
// that happens to open with a "key:"-shaped line; an explicit persisted marker can.
const RecoveryMarkerKey = "x_binder"

// MarkRecovered stamps the recovery marker on a concept's frontmatter. Its value
// is a mapping { recovered: true, reason: <reason> }, so it doubles as honest,
// self-describing provenance rather than an opaque flag.
func MarkRecovered(fm *OrderedMap, reason string) {
	if fm == nil {
		return
	}
	fm.Set(RecoveryMarkerKey, map[string]any{
		"recovered": true,
		"reason":    reason,
	})
}

// IsRecovered reports whether frontmatter carries the recovery marker with
// recovered: true. It reads the same key MarkRecovered writes, whether the value
// is a freshly-built Go map (convert) or one re-parsed from persisted YAML (read
// side), so writer and reader agree.
func IsRecovered(fm *OrderedMap) bool {
	m := asStringMap(mapGet(fm, RecoveryMarkerKey))
	if m == nil {
		return false
	}
	b, _ := m["recovered"].(bool)
	return b
}
