package okf

// Codec parses and serializes OKF concept documents and maps between
// bundle-relative paths and concept IDs. Implementations (factileadapter today,
// native later) satisfy this interface; the converter and CLI depend only on it.
type Codec interface {
	// ParseConcept splits frontmatter/body for the file at relPath, preserving
	// key order and unknown keys, maps RelPath→ConceptID, and projects
	// TrustSignals. It returns an error only for structural problems the spec
	// treats as non-conformant (missing/unterminated/invalid frontmatter).
	ParseConcept(relPath string, raw []byte) (*Concept, error)

	// Serialize is the inverse of ParseConcept: order-stable and
	// byte-deterministic for a given Concept.
	Serialize(c *Concept) ([]byte, error)

	// ConceptIDFromRel maps a bundle-relative path to a concept ID (path minus
	// ".md"); ok is false for reserved or non-concept paths.
	ConceptIDFromRel(rel string) (id string, ok bool)

	// RelFromConceptID is the inverse of ConceptIDFromRel.
	RelFromConceptID(id string) (rel string, err error)

	// IsReservedFile reports whether name is a reserved filename (index.md /
	// log.md, spec §3.1).
	IsReservedFile(name string) bool
}

// LinkGraph extracts and resolves edges from ALREADY-OKF concept bodies. This
// is the output-side graph surface (validate/graph); the converter does its own
// source-side link rewriting in internal/convert.
type LinkGraph interface {
	// ExtractLinks reads edges from an OKF concept body.
	ExtractLinks(fromConceptID, body string) []Link
	// ResolveLink resolves a raw markdown target to a bundle-relative concept
	// ID; ok is false for external/unresolvable targets.
	ResolveLink(fromConceptID, rawTarget string) (toConceptID string, ok bool)
}
