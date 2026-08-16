// Package config is binder's viper-backed configuration substrate (issue #10).
// It resolves configurable defaults (the actor identity for --verified-by, the
// default concept type) once, with the precedence flag > env > config file >
// built-in default, and exposes the resolved values (and, where cheap, the
// source of each) to the command tree. It underpins the #7 --verified-by stamp:
// the default actor is an explicit, user-configured assertion, never invented.
//
// Absence of any config file is normal — defaults apply and loading NEVER errors
// for a missing file. A malformed config `verified_by` fails fast at load with a
// usage error (exit 2), not deferred to first use (design §3.1 / option (a)).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
)

// SchemaVersion identifies the `binder config` JSON report contract. It is
// distinct from clijson.SchemaVersion (the report envelope): config has its own
// shape, versioned independently (design §4.2 forward-compat).
const SchemaVersion = "binder.config/v1"

// Config keys (namespaced, extensible). New defaults are added here without
// breaking the envelope shape.
const (
	KeyVerifiedBy     = "verified_by"
	KeyDefaultType    = "default_type"
	KeyGeminiModel    = "gemini_model"
	KeyGeminiLocation = "gemini_location"
	KeyGeminiProject  = "gemini_project"
	KeyGeminiBackend  = "gemini_backend"
)

// Default values for Gemini configuration.
const (
	DefaultGeminiModel    = "gemini-3.5-flash-lite"
	DefaultGeminiLocation = "global"
	DefaultGeminiBackend  = "auto"
)

// EnvPrefix is prepended (with an underscore) to an upper-cased key to form the
// environment variable name, e.g. verified_by → BINDER_VERIFIED_BY.
const EnvPrefix = "BINDER"

// LocalConfigName is the repo-local config file, consulted FIRST in the search
// order (findConfigFile). Per the owner ruling (Option A) a verified_by set here
// does NOT satisfy the user-set stamping exception — see PermitsStampWithoutFlag.
const LocalConfigName = ".binder.yaml"

// defaultType is the built-in default for the default_type key; it mirrors the
// convert command's historical --default-type default so behavior is unchanged.
const defaultType = "Note"

// ActorFormsHint lists the valid actor forms for --verified-by / verified_by.
// It is shared by the flag validator and the config-load validator so the two
// surfaces emit an identical, helpful message (design option (a)).
const ActorFormsHint = "valid forms: human:<id>, process:<id>, team:<id>, or <producer>/<version> (e.g. binder/0.3.0)"

// InvalidActorError returns a usage error (exit 2) for an actor value that does
// not satisfy okf.IsValidActor, listing the valid forms.
func InvalidActorError(actor string) error {
	return clijson.Usage(fmt.Errorf("invalid actor %q; %s", actor, ActorFormsHint))
}

// Config holds a resolved viper instance and the config file it read (if any).
// It is created empty and populated by Load, which the root command runs in its
// PersistentPreRunE so every subcommand shares the same resolved configuration.
type Config struct {
	v          *viper.Viper
	configFile string                 // path of the config file read, "" if none
	boundFlags map[string]*pflag.Flag // key → flag bound via BindFlag (for source attribution)
}

// Load resolves configuration from (in precedence order) env and config file
// over built-in defaults. Flag binding is layered on later, per command, via
// BindFlag. A missing config file is not an error. A malformed `verified_by`
// coming from env/file is a usage error (exit 2), surfaced here so it fails fast
// at config-load rather than at first use.
func (c *Config) Load() error {
	v := viper.New()
	v.SetDefault(KeyVerifiedBy, "")
	v.SetDefault(KeyDefaultType, defaultType)
	v.SetDefault(KeyGeminiModel, DefaultGeminiModel)
	v.SetDefault(KeyGeminiLocation, DefaultGeminiLocation)
	v.SetDefault(KeyGeminiProject, "")
	v.SetDefault(KeyGeminiBackend, DefaultGeminiBackend)

	v.SetEnvPrefix(EnvPrefix)
	v.AutomaticEnv()

	if path := findConfigFile(); path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			// A file we located but cannot read/parse is a real problem (IO/parse),
			// distinct from "no file present" which is normal. Report it.
			return fmt.Errorf("reading config file %q: %w", path, err)
		}
		c.configFile = path
	}
	c.v = v

	// Fail fast: a config-supplied default actor must itself be well-formed
	// (design option (a)). Flag values are validated separately at use.
	if vb := strings.TrimSpace(v.GetString(KeyVerifiedBy)); vb != "" && !okf.IsValidActor(vb) {
		return InvalidActorError(vb)
	}
	return nil
}

// findConfigFile returns the first existing config file in the documented search
// order, or "" if none is found:
//  1. ./.binder.yaml
//  2. $XDG_CONFIG_HOME/binder/config.yaml (fallback $HOME/.config/binder/config.yaml)
func findConfigFile() string {
	candidates := []string{filepath.Join(".", ".binder.yaml")}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "binder", "config.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".config", "binder", "config.yaml"))
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// BindFlag ties a command flag to a config key so a flag that was explicitly set
// takes precedence over env/file/default (viper honors flag.Changed). A nil flag
// is ignored. Must be called after Load.
func (c *Config) BindFlag(key string, flag *pflag.Flag) {
	if c.v == nil || flag == nil {
		return
	}
	if c.boundFlags == nil {
		c.boundFlags = map[string]*pflag.Flag{}
	}
	c.boundFlags[key] = flag
	_ = c.v.BindPFlag(key, flag)
}

// GetString returns the resolved string value for key (flag > env > file >
// default). It is safe to call before Load (returns "").
func (c *Config) GetString(key string) string {
	if c.v == nil {
		return ""
	}
	return c.v.GetString(key)
}

// ConfigFile returns the config file path that was read, or "" if none.
func (c *Config) ConfigFile() string { return c.configFile }

// Source reports where the resolved value for key came from: "flag", "env",
// "file", or "default". It is a best-effort attribution used by `binder config`.
func (c *Config) Source(key string) string {
	if c.v == nil {
		return "default"
	}
	// A bound flag that was explicitly set on the command line takes precedence.
	if f, ok := c.boundFlags[key]; ok && f != nil && f.Changed {
		return "flag"
	}
	if _, ok := os.LookupEnv(envKey(key)); ok {
		return "env"
	}
	if c.v.InConfig(key) {
		return "file"
	}
	return "default"
}

func envKey(key string) string {
	return EnvPrefix + "_" + strings.ToUpper(key)
}

// VerifiedByOrigin classifies where the resolved verified_by value came from, at
// the granularity the never-fabricate-trust stamping decision needs. Source()
// collapses a repo-local .binder.yaml and a global user config into "file"; the
// user-set stamping exception (PermitsStampWithoutFlag) must tell them apart, so
// this is a separate, finer classification used only by the stamp gate.
type VerifiedByOrigin int

const (
	// OriginNone: no verifier was determined (default/empty).
	OriginNone VerifiedByOrigin = iota
	// OriginFlag: an explicit --verified-by on THIS invocation.
	OriginFlag
	// OriginEnv: the BINDER_VERIFIED_BY environment variable.
	OriginEnv
	// OriginRepoConfig: a repo-local ./.binder.yaml.
	OriginRepoConfig
	// OriginGlobalConfig: the global user config (XDG/HOME).
	OriginGlobalConfig
)

// String renders the origin for trust disclosure (Residual B): the token that
// appears as the "source" of a written stamp. Repo-local config never authorizes
// a stamp on its own, so it never surfaces here as a write source.
func (o VerifiedByOrigin) String() string {
	switch o {
	case OriginFlag:
		return "flag"
	case OriginEnv:
		return "env"
	case OriginGlobalConfig, OriginRepoConfig:
		return "config"
	default:
		return "none"
	}
}

// VerifiedByOrigin reports the origin of the resolved verified_by actor, using
// the same precedence viper applies (flag > env > file > default) but keeping the
// repo-local vs global config distinction the stamp gate depends on. It returns
// OriginNone when the resolved value is empty.
func (c *Config) VerifiedByOrigin() VerifiedByOrigin {
	if c.v == nil || strings.TrimSpace(c.v.GetString(KeyVerifiedBy)) == "" {
		return OriginNone
	}
	if f, ok := c.boundFlags[KeyVerifiedBy]; ok && f != nil && f.Changed {
		return OriginFlag
	}
	if _, ok := os.LookupEnv(envKey(KeyVerifiedBy)); ok {
		return OriginEnv
	}
	if c.v.InConfig(KeyVerifiedBy) {
		if c.configFile == filepath.Join(".", LocalConfigName) {
			return OriginRepoConfig
		}
		return OriginGlobalConfig
	}
	return OriginNone
}

// PermitsStampWithoutFlag is THE single predicate for the owner's user-set
// stamping exception: does a verified_by resolved from this origin count as the
// user having decided on a default, permitting a `verified` stamp WITHOUT a
// per-invocation --verified-by?
//
// This is deliberately the one place the exception is decided, so implementing
// (or later revising) the ruling is a one-line change here rather than logic
// scattered across the command tree. The reviewer checks the ruling here.
//
// OWNER RULING (was HELD, decided 2026-08-16 — Option A):
//   - OriginGlobalConfig → YES. A config in the user's OWN home directory
//     evidences a decision by THIS user.
//   - OriginRepoConfig   → NO. A repo-local ./.binder.yaml can arrive inside a
//     git clone somebody else authored, so it cannot evidence a decision by this
//     user. It is treated the same as no config: no flag, no stamp. Flip THIS
//     case to change that answer.
//   - OriginEnv → YES. A deliberate per-session act by whoever runs the command,
//     like global config and unlike an inherited file. (binder-030-em's call, not
//     the owner's; flagged for revisit.)
//
// OriginFlag is handled by the caller (an explicit flag always stamps) and is not
// part of the "without flag" question, so it returns false here.
func PermitsStampWithoutFlag(o VerifiedByOrigin) bool {
	switch o {
	case OriginGlobalConfig:
		return true
	case OriginEnv:
		return true
	default:
		// OriginRepoConfig (Option A), OriginFlag, OriginNone.
		return false
	}
}

// ResolvedValue is one key's resolved value plus its source, for `binder config`.
type ResolvedValue struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// Resolved is the `binder config` report: the file that was read (if any) and
// each configuration key's resolved value and source.
type Resolved struct {
	ConfigFile string                   `json:"config_file"`
	Values     map[string]ResolvedValue `json:"values"`
}

// Keys is the stable, ordered list of configuration keys `binder config` prints.
func Keys() []string {
	return []string{
		KeyDefaultType,
		KeyVerifiedBy,
		KeyGeminiModel,
		KeyGeminiLocation,
		KeyGeminiProject,
		KeyGeminiBackend,
	}
}

// Resolve builds the Resolved view for `binder config`.
func (c *Config) Resolve() Resolved {
	vals := make(map[string]ResolvedValue, len(Keys()))
	for _, k := range Keys() {
		vals[k] = ResolvedValue{Value: c.GetString(k), Source: c.Source(k)}
	}
	return Resolved{ConfigFile: c.configFile, Values: vals}
}
