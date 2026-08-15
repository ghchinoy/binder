package convert

import (
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// SourceFacts records the AUTHORED state of one source file, captured during
// Analyze at the exact point ensureType/ensureTitle run — i.e. BEFORE their
// defaulting masks it. It is the only surface that sees the corpus as authored:
// once convert defaults a missing title/type, the emitted bundle no longer shows
// that the source lacked them. `binder lint` (issue #8) reads these facts to
// report missing titles and schema violations. It is purely descriptive: Analyze
// never rejects or mutates the source based on it (never-reject, spec §11).
type SourceFacts struct {
	RelPath      string `json:"rel_path"`      // source-relative path, e.g. "notes/a.md"
	ConceptID    string `json:"concept_id"`    // resolved bundle concept id (== okf.Concept.ID)
	TitlePresent bool   `json:"title_present"` // authored title: in frontmatter OR a first H1 in the body
	TypePresent  bool   `json:"type_present"`  // authored type: in frontmatter (before defaulting)
	Recovered    bool   `json:"recovered"`     // frontmatter did not parse (invalid YAML) — preserved as body
	RecoverErr   string `json:"recover_err"`   // the frontmatter parse error, when Recovered (else empty)
}

// authoredTitlePresent reports whether the file carries a title as authored: a
// non-empty frontmatter title: OR a first-level ATX heading in the body. It
// mirrors ensureTitle's precedence for the "present" question, so a file lint
// flags as title-less is exactly one ensureTitle would have to synthesize a
// humanized-filename title for.
func authoredTitlePresent(fm *okf.OrderedMap, body string) bool {
	if v, ok := fm.Get("title"); ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return true
		}
	}
	return firstH1(body) != ""
}

// authoredTypePresent reports whether the file carries a non-empty frontmatter
// type: as authored (before ensureType applies the type-map / default-type
// precedence). A file lint flags as type-less is one convert would default.
func authoredTypePresent(fm *okf.OrderedMap) bool {
	if v, ok := fm.Get("type"); ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}
