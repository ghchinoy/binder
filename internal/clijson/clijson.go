// Package clijson is binder's shared machine-readable output + exit-code helper.
// It renders the existing command report structs as a deterministic JSON envelope
// (issue #13) and maps typed command errors onto the stable exit-code contract so
// every command behaves identically. It adds NO fabricated report fields: the
// envelope carries only provenance already present (the binder version) plus a
// schema tag consumers branch on for forward-compat (design §4.2).
package clijson

import (
	"encoding/json"
	"errors"
	"io"
)

// SchemaVersion identifies the JSON report contract. Bump ONLY on a breaking
// change to a report payload's shape or field names.
const SchemaVersion = "binder.report/v1"

// Exit codes — the stable contract (design §5). Documented in README and
// docs/user_guide.md. The same code applies in prose and --json mode: the
// contract is about the run, not the output format.
const (
	ExitSuccess  = 0 // completed; no gating findings (advisories may be present)
	ExitFindings = 1 // a gating condition (validate non-conformance; or advisories under --strict, wired in #7)
	ExitUsage    = 2 // bad flags/args (unknown flag, missing/conflicting args)
	ExitIO       = 3 // cannot read/write, or any other internal error
)

// Envelope wraps a command's report struct with the provenance and schema tag a
// machine consumer needs to parse it safely. Result is the existing report
// struct, untouched in shape.
type Envelope struct {
	Binder  string `json:"binder"`  // "binder/<version>"
	Command string `json:"command"` // "convert" | "validate" | "review"
	Schema  string `json:"schema"`  // SchemaVersion
	Result  any    `json:"result"`  // the existing report struct, unmodified
}

// Encode writes the deterministic JSON envelope for a command's result to w:
// fixed 2-space indent, HTML escaping OFF, sorted map keys (encoding/json's
// default), and a trailing newline. binderVersion is the bare version string
// (e.g. "0.1.0"); it is prefixed with "binder/" for the envelope.
func Encode(w io.Writer, binderVersion, command string, result any) error {
	return EncodeSchema(w, binderVersion, command, SchemaVersion, result)
}

// EncodeSchema is Encode with a caller-supplied schema tag. Commands whose
// report has its own contract (e.g. `binder config` → "binder.config/v1") use
// this so their envelope carries the right schema while sharing the identical
// deterministic encoding (2-space indent, HTML escaping off, sorted keys,
// trailing newline).
func EncodeSchema(w io.Writer, binderVersion, command, schema string, result any) error {
	env := Envelope{
		Binder:  "binder/" + binderVersion,
		Command: command,
		Schema:  schema,
		Result:  result,
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

// UsageError marks an invalid invocation (unknown flag, missing/conflicting
// args). It maps to ExitUsage. Cobra flag-parse and arg-count errors are wrapped
// into this type so they are distinguishable from IO/internal failures.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// Usage wraps err as a UsageError (exit 2). A nil err returns nil.
func Usage(err error) error {
	if err == nil {
		return nil
	}
	return &UsageError{Err: err}
}

// FindingsError marks a gating condition: a run that completed but produced
// findings that must fail the command (validate non-conformance; or advisories
// under --strict once #7 wires it). It maps to ExitFindings.
type FindingsError struct{ Msg string }

func (e *FindingsError) Error() string { return e.Msg }

// Gate returns a FindingsError (exit 1) when a gating condition is present, else
// nil. hardNonConformance always gates. advisoriesPresent gates only under
// strict. strict is the seam reserved for #7's --strict flag; callers hardwire
// it false until then, so #7 is a one-line wiring change, not a retrofit.
func Gate(strict, hardNonConformance, advisoriesPresent bool, msg string) error {
	if hardNonConformance || (strict && advisoriesPresent) {
		return &FindingsError{Msg: msg}
	}
	return nil
}

// ExitCode maps a command error onto the stable exit-code contract:
// nil→0, UsageError→2, FindingsError→1, anything else→3.
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var ue *UsageError
	if errors.As(err, &ue) {
		return ExitUsage
	}
	var fe *FindingsError
	if errors.As(err, &fe) {
		return ExitFindings
	}
	return ExitIO
}
