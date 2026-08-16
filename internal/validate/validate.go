// Package validate checks an OKF bundle for spec §11 conformance using a
// binder-owned Codec. It enforces exactly the hard rules (parseable frontmatter
// + non-empty type on every non-reserved .md) and reports trust well-formedness
// as advisories. It MUST NEVER reject a bundle for missing optional fields,
// unknown keys, unknown type values, broken cross-links, or trust-family absence
// (spec §11).
package validate

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ghchinoy/binder/internal/okf"
)

// Result is the outcome of validating a bundle.
type Result struct {
	Root        string        `json:"root"`
	NumConcepts int           `json:"num_concepts"`
	NumReserved int           `json:"num_reserved"`
	Findings    []okf.Finding `json:"findings"`

	// ReservedStructureChecked reports whether the structure of reserved files
	// (index.md, log.md; spec §8/§9) was examined. It is false in this release:
	// structural validation of reserved files is not yet implemented (#77 item
	// 2, deferred). It is emitted so the verdict's scope is explicit — a
	// `conformant` result must not be read as covering a surface the validator
	// never examined (invariant: never fabricate trust). num_reserved reports
	// how many such files were present but structurally unexamined.
	ReservedStructureChecked bool `json:"reserved_structure_checked"`
}

// Conformant reports whether the bundle satisfies the hard conformance rules
// (no error-severity findings).
func (r *Result) Conformant() bool {
	for _, f := range r.Findings {
		if f.Severity == okf.SeverityError {
			return false
		}
	}
	return true
}

// Errors returns only the hard-violation findings.
func (r *Result) Errors() []okf.Finding {
	var out []okf.Finding
	for _, f := range r.Findings {
		if f.Severity == okf.SeverityError {
			out = append(out, f)
		}
	}
	return out
}

// Advisories returns only the advisory findings.
func (r *Result) Advisories() []okf.Finding {
	var out []okf.Finding
	for _, f := range r.Findings {
		if f.Severity == okf.SeverityAdvisory {
			out = append(out, f)
		}
	}
	return out
}

// Bundle validates the bundle rooted at root.
func Bundle(root string, codec okf.Codec, spec okf.SpecVersion) (*Result, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "validate", Path: root, Err: os.ErrInvalid}
	}

	var files []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	// Initialize Findings so a conformant bundle serializes to [] not null (#13).
	result := &Result{Root: root, Findings: []okf.Finding{}}
	for _, rel := range files {
		if codec.IsReservedFile(rel) {
			// Reserved files (index.md/log.md) are not concepts and are not
			// required to carry type; structural validation of §8/§9 is Phase 2.
			result.NumReserved++
			continue
		}
		result.NumConcepts++

		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		c, err := codec.ParseConcept(rel, raw)
		if err != nil {
			result.Findings = append(result.Findings, okf.Finding{
				ConceptID: strings.TrimSuffix(rel, ".md"),
				Severity:  okf.SeverityError,
				Message:   "unparseable or missing YAML frontmatter (spec §11.1): " + err.Error(),
			})
			continue
		}
		if strings.TrimSpace(c.Type) == "" {
			result.Findings = append(result.Findings, okf.Finding{
				ConceptID: c.ID,
				Severity:  okf.SeverityError,
				Message:   "missing non-empty 'type' (spec §11.2)",
			})
		}
		// Trust well-formedness is advisory only; never a rejection reason.
		result.Findings = append(result.Findings, okf.ValidateTrust(c, spec)...)
	}
	return result, nil
}
