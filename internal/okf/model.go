// Package okf is the binder-OWNED OKF domain model and boundary.
//
// It defines the typed domain (Bundle, Concept, Link, TrustSignals), the two
// interfaces the rest of binder depends on (Codec, LinkGraph), the trust
// vocabulary logic (ValidateTrust/TrustTier/IsStale), and the spec-version
// registry. Nothing above this package (internal/convert, cmd) may import a
// concrete codec (factileadapter) or factile directly; they depend only on the
// interfaces declared here. See design-v2 §2.2/§2.3.
package okf

// OrderedMap is an insertion-order-preserving string-keyed map. It is the
// authoritative representation of a concept's frontmatter: every key present in
// the source survives round-trip, in order, including keys binder does not
// understand (spec §4 "consumers SHOULD preserve unknown keys").
type OrderedMap struct {
	keys []string
	vals map[string]any
}

// NewOrderedMap returns an empty OrderedMap.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{vals: map[string]any{}}
}

// Get returns the value for key and whether it was present.
func (m *OrderedMap) Get(key string) (any, bool) {
	if m == nil || m.vals == nil {
		return nil, false
	}
	v, ok := m.vals[key]
	return v, ok
}

// Set inserts or updates key, preserving first-insertion order.
func (m *OrderedMap) Set(key string, value any) {
	if m.vals == nil {
		m.vals = map[string]any{}
	}
	if _, exists := m.vals[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = value
}

// Has reports whether key is present.
func (m *OrderedMap) Has(key string) bool {
	if m == nil || m.vals == nil {
		return false
	}
	_, ok := m.vals[key]
	return ok
}

// Keys returns the keys in insertion order (a copy).
func (m *OrderedMap) Keys() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.keys...)
}

// Len returns the number of keys.
func (m *OrderedMap) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

// SpecVersion identifies an OKF spec revision (e.g. "0.2"). Registry-driven; see spec.go.
type SpecVersion string

// Bundle is a converted/loaded OKF bundle rooted at a filesystem directory.
type Bundle struct {
	Root       string
	OKFVersion SpecVersion // declared ONLY in root index.md (spec §12)
	Concepts   []*Concept
}

// Concept is a single OKF concept document.
//
// Frontmatter is authoritative: Type and Trust are typed projections derived
// from it, never a competing source of truth. Unknown/forward keys live only in
// Frontmatter and are never dropped.
type Concept struct {
	ID          string      // bundle-relative path minus ".md" (spec §2)
	RelPath     string      // bundle-relative path, e.g. "tables/orders.md"
	Type        string      // REQUIRED, non-empty (the only hard field, spec §11)
	Frontmatter *OrderedMap // ALL keys preserved, order-stable (spec §4/§11)
	Body        string      // markdown after frontmatter
	Links       []Link      // extracted edges (spec §6)
	Trust       TrustSignals
}

// Link is a directed edge from one concept to a link target (spec §6).
type Link struct {
	RawTarget string // target exactly as written in the source
	TargetID  string // resolved bundle-relative concept ID, or "" if unresolved
	Text      string // link text (relationship label by convention)
	Resolved  bool   // whether TargetID names a concept in the bundle
}

// TrustSignals is a typed projection of the v0.2 trust vocabulary (spec §5/§10).
// It is derived from Frontmatter on parse; Frontmatter stays authoritative so
// unknown/forward keys are never lost.
type TrustSignals struct {
	Sources     []Source
	UsageWindow *DateRange
	Generated   *Actorstamp
	Verified    []Actorstamp
	Status      string // draft|stable|deprecated ("" ⇒ stable, spec §5.4)
	StaleAfter  string // YYYY-MM-DD (spec §5.5), "" if absent
	Attested    bool   // type == "Attested Computation" (spec §10); presence only in Phase 1
}

// Actorstamp is a { by, at } pair used by generated and verified (spec §5.2).
type Actorstamp struct {
	By string // actor convention (spec §7)
	At string // ISO 8601 datetime
}

// Source is one entry of the sources provenance family (spec §5.1).
type Source struct {
	ID           string
	Resource     string
	Title        string
	Author       string
	UsageCount   string
	LastModified string
}

// DateRange is a { from, to } window (spec §5.1 usage_window).
type DateRange struct {
	From string
	To   string
}
