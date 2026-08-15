package okf

// SpecVersion registry (design-v2 §2.5). v0.2 is the only entry today. Adding a
// future v0.3 = one entry here plus TrustSignals/validation extensions; the
// converter, CLI, and codecs are untouched. Unknown/forward frontmatter keys
// round-trip losslessly regardless, because Frontmatter is authoritative and
// TrustSignals is only a view.

// SpecV02 is the current default target spec version.
const SpecV02 SpecVersion = "0.2"

// DefaultSpecVersion is the version binder emits.
const DefaultSpecVersion = SpecV02

// SpecRules declares the per-version conventions binder enforces on emit and
// checks (advisory) on validate.
type SpecRules struct {
	Version        SpecVersion
	RequiredFields []string // hard-required frontmatter keys (spec §11)
	ReservedFiles  []string // reserved filenames (spec §3.1)
	TrustFields    []string // the trust/provenance/lifecycle family (spec §5/§10)
	// ProvenanceStamp/VersionKey record the emit rules that distinguish v0.2
	// from v0.1 (spec §13.1): use generated.at not timestamp; declare
	// okf_version in the root index.md.
	ProvenanceStamp string
	VersionKey      string
}

var registry = map[SpecVersion]SpecRules{
	SpecV02: {
		Version:        SpecV02,
		RequiredFields: []string{"type"},
		ReservedFiles:  []string{"index.md", "log.md"},
		TrustFields: []string{
			"sources", "usage_window", "generated", "verified",
			"status", "stale_after",
			"runtime", "parameters", "computation", "executor", "attester",
		},
		ProvenanceStamp: "generated",
		VersionKey:      "okf_version",
	},
}

// Rules returns the SpecRules for v and whether the version is known.
func Rules(v SpecVersion) (SpecRules, bool) {
	r, ok := registry[v]
	return r, ok
}

// KnownVersion reports whether v is a version binder understands.
func KnownVersion(v SpecVersion) bool {
	_, ok := registry[v]
	return ok
}
