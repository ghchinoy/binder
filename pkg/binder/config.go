package binder

import (
	"io"

	"github.com/ghchinoy/binder/pkg/clijson"
)

// The config capability is a CLI/adapter SUBSTRATE (design §2 Non-Goal, §6 Phase
// 4): the viper/pflag machinery, the CWD/$HOME/BINDER_* ambient discovery, and the
// canonicalize/validate/read/write file operations all stay in internal/config and
// its cmd/ adapter. Only RESOLVED values cross this seam — no pflag.Flag, *viper.Viper,
// or cobra type appears in any signature here (design AC5 / Residual Risk 8).
//
// What the service owns is the `binder config` get/set/unset WIRE CONTRACT: the
// three JSON payloads that used to be anonymous map[string]any literals in
// cmd/config.go (survey §4.9) are typed result structs here, and the status
// vocabulary ("updated" / "removed" / "noop") is defined once. show/list are
// already covered by internal/config.Resolved and are not part of this seam.
const (
	configGetCommand   = "config get"
	configSetCommand   = "config set"
	configUnsetCommand = "config unset"

	// configSchemaVersion is the `binder config` wire-contract schema tag. It is
	// held locally (mirroring internal/config.SchemaVersion) so this service does
	// NOT import the viper/pflag config substrate merely to name its own contract
	// (design Residual Risk 8). Byte-identity goldens and the plugindocs drift gate
	// pin the two to the same value.
	//
	// NOTE: this literal is INTENTIONALLY duplicated with internal/config.SchemaVersion
	// — a knowing DRY trade-off to keep viper/pflag off the service seam, not an
	// oversight. It is drift-gated: config_test.go goldens and
	// internal/plugindocs/drift_proving_test.go both pin this value, so any
	// divergence fails the test gate. See phase4/infer-config-decisions.md §2.
	configSchemaVersion = "binder.config/v1"
)

// ConfigGetRequest carries the resolved lookup of a single config key.
type ConfigGetRequest struct {
	Version string // binder version for the envelope's `binder` field
	Key     string // canonical key name
	Value   string // resolved value
	Source  string // resolved source (flag | env | file | default)
}

// ConfigGetResult is the typed `config get` payload. Field order matches the
// deterministic wire contract (Go marshals struct fields in declaration order; the
// former map marshaled keys sorted: key, source, value).
type ConfigGetResult struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Value  string `json:"value"`

	version string
}

// ConfigGet builds the typed `config get` result from resolved values.
func (s *Service) ConfigGet(req ConfigGetRequest) ConfigGetResult {
	return ConfigGetResult{
		Key:     req.Key,
		Source:  req.Source,
		Value:   req.Value,
		version: req.Version,
	}
}

// EncodeJSON writes the binder.config/v1 `config get` envelope to w — byte-identical
// to `binder config get --json`.
func (r ConfigGetResult) EncodeJSON(w io.Writer) error {
	return clijson.EncodeSchema(w, r.version, configGetCommand, configSchemaVersion, r)
}

// ConfigSetRequest carries the resolved outcome of persisting a config key.
type ConfigSetRequest struct {
	Version string // binder version for the envelope's `binder` field
	Key     string // canonical key name
	Value   string // value written
	File    string // target file the value was written to
}

// ConfigSetResult is the typed `config set` payload. Field order matches the
// deterministic wire contract (former sorted map keys: file, key, status, value).
type ConfigSetResult struct {
	File   string `json:"file"`
	Key    string `json:"key"`
	Status string `json:"status"`
	Value  string `json:"value"`

	version string
}

// ConfigSet builds the typed `config set` result. A successful set is always
// reported with status "updated" — the status vocabulary lives here, once.
func (s *Service) ConfigSet(req ConfigSetRequest) ConfigSetResult {
	return ConfigSetResult{
		File:    req.File,
		Key:     req.Key,
		Status:  "updated",
		Value:   req.Value,
		version: req.Version,
	}
}

// EncodeJSON writes the binder.config/v1 `config set` envelope to w — byte-identical
// to `binder config set --json`.
func (r ConfigSetResult) EncodeJSON(w io.Writer) error {
	return clijson.EncodeSchema(w, r.version, configSetCommand, configSchemaVersion, r)
}

// ConfigUnsetRequest carries the resolved outcome of removing a config key.
type ConfigUnsetRequest struct {
	Version string // binder version for the envelope's `binder` field
	Key     string // canonical key name
	File    string // target file the key was removed from
	Existed bool   // whether the key was present before removal
}

// ConfigUnsetResult is the typed `config unset` payload. Field order matches the
// deterministic wire contract (former sorted map keys: file, key, status).
type ConfigUnsetResult struct {
	File   string `json:"file"`
	Key    string `json:"key"`
	Status string `json:"status"`

	version string
}

// ConfigUnset builds the typed `config unset` result. The status vocabulary
// ("removed" when the key existed, "noop" when it did not) is defined here, once.
func (s *Service) ConfigUnset(req ConfigUnsetRequest) ConfigUnsetResult {
	status := "removed"
	if !req.Existed {
		status = "noop"
	}
	return ConfigUnsetResult{
		File:    req.File,
		Key:     req.Key,
		Status:  status,
		version: req.Version,
	}
}

// EncodeJSON writes the binder.config/v1 `config unset` envelope to w —
// byte-identical to `binder config unset --json`.
func (r ConfigUnsetResult) EncodeJSON(w io.Writer) error {
	return clijson.EncodeSchema(w, r.version, configUnsetCommand, configSchemaVersion, r)
}
