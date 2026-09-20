package okfrules

import "github.com/ghchinoy/binder/pkg/okf"

// LinkGraph extracts and resolves edges from ALREADY-OKF concept bodies. This is
// the output-side graph surface (validate/graph); the converter does its own
// source-side link rewriting in pkg/convert. It is kept internal (DEFER, Phase-2
// trace): only the native codec and the bundle loader touch it, so it need not
// widen the published okf surface.
type LinkGraph interface {
	// ExtractLinks reads edges from an OKF concept body.
	ExtractLinks(fromConceptID, body string) []okf.Link
	// ResolveLink resolves a raw markdown target to a bundle-relative concept
	// ID; ok is false for external/unresolvable targets.
	ResolveLink(fromConceptID, rawTarget string) (toConceptID string, ok bool)
}
