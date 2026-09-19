package binder

import (
	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/convert"
)

// resolveStatusMap parses and vocabulary-checks a --status-map value, shared
// verbatim by Service.Convert and Service.Enrich so the two surfaces behave
// identically (issue #23). It is the ONE place the status-vocabulary policy is
// applied — moved here from cmd/statusmap.go so the strict pre-write gate is part of
// the service's option-assembly rather than the CLI adapter's:
//
//   - A malformed argument (not key=value) is a usage error → exit 2.
//   - A well-formed value outside OKF §5.4 (draft|stable|deprecated) is, on the
//     DEFAULT path, a warning surfaced in the report; the value is written UNCHANGED
//     (binder never guesses intent). Under strict it escalates to a pre-write gate →
//     exit 1, so a corpus that would warn is never half-written.
//   - Canonicalization of the fixed alias set is OPT-IN via canonicalize; off by
//     default. Each rewrite it performs is reported (never silent).
//
// It returns the resolved prefix map, default value, and the ordered report notes.
// The strict gate happens here, BEFORE any file is written. Both parse errors and the
// gate error are returned as clijson error types (Usage → exit 2, Findings → exit 1);
// the MCP surface passes strict=false and returns the raw error text (identical, since
// clijson.UsageError.Error() returns the inner message).
func resolveStatusMap(raw string, canonicalize, strict bool) (prefixes map[string]string, def string, notes []string, err error) {
	prefixes, def, err = convert.ParseStatusMap(raw)
	if err != nil {
		// Malformed shape/value → usage error (exit 2).
		return nil, "", nil, clijson.Usage(err)
	}
	prefixes, def, vocab := convert.ResolveStatusVocabulary(prefixes, def, canonicalize)
	// strict escalates a non-conformant status value to a gate BEFORE the run,
	// mirroring the --verified-by (#7) fail-fast posture. Without strict the run
	// proceeds and the notes are surfaced advisorily in the report.
	if gateErr := clijson.Gate(strict, false, vocab.NonConformant(), vocab.GateMessage()); gateErr != nil {
		return nil, "", nil, gateErr
	}
	return prefixes, def, vocab.Notes, nil
}
