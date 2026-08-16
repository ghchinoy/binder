package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ghchinoy/binder/internal/clijson"
	"github.com/ghchinoy/binder/internal/okf"
)

var knownKeys = map[string]string{
	"default_type":    KeyDefaultType,
	"default.type":    KeyDefaultType,
	"verified_by":     KeyVerifiedBy,
	"verified.by":     KeyVerifiedBy,
	"gemini_model":    KeyGeminiModel,
	"gemini.model":    KeyGeminiModel,
	"gemini_location": KeyGeminiLocation,
	"gemini.location": KeyGeminiLocation,
	"gemini_project":  KeyGeminiProject,
	"gemini.project":  KeyGeminiProject,
	"gemini_backend":  KeyGeminiBackend,
	"gemini.backend":  KeyGeminiBackend,
}

// CanonicalKey normalizes a user-supplied key (e.g. "gemini.project" -> "gemini_project")
// and verifies that it is a known configuration key. Returns a usage error if unknown.
func CanonicalKey(k string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(k))
	if canonical, ok := knownKeys[norm]; ok {
		return canonical, nil
	}
	validList := strings.Join(Keys(), ", ")
	return "", clijson.Usage(fmt.Errorf("unknown configuration key %q (valid keys: %s)", k, validList))
}

// ValidateValue checks that a key's proposed value conforms to its grammar/schema.
func ValidateValue(key, value string) error {
	canonical, err := CanonicalKey(key)
	if err != nil {
		return err
	}
	val := strings.TrimSpace(value)

	switch canonical {
	case KeyVerifiedBy:
		if val != "" && !okf.IsValidActor(val) {
			return InvalidActorError(val)
		}
	case KeyGeminiBackend:
		switch strings.ToLower(val) {
		case "auto", "api", "vertex", "":
			return nil
		default:
			return clijson.Usage(fmt.Errorf("invalid gemini_backend %q: must be auto, api, or vertex", value))
		}
	case KeyDefaultType:
		if val == "" {
			return clijson.Usage(fmt.Errorf("default_type cannot be empty"))
		}
	}
	return nil
}

// TargetFilePath determines where to write settings:
// - global: $XDG_CONFIG_HOME/binder/config.yaml (or ~/.config/binder/config.yaml)
// - local: ./.binder.yaml
func TargetFilePath(global bool) (string, error) {
	if !global {
		return filepath.Join(".", ".binder.yaml"), nil
	}
	var dir string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dir = filepath.Join(xdg, "binder")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine user home directory: %w", err)
		}
		dir = filepath.Join(home, ".config", "binder")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("could not create config directory %q: %w", dir, err)
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// ReadConfigFile reads the raw persisted config as a map. A missing or empty
// file yields an empty map. This reads ONLY the file on disk — it does not merge
// flags, environment variables, or built-in defaults.
func ReadConfigFile(path string) (map[string]any, error) {
	m := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return m, nil
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// WriteConfigFile persists the settings map as clean, indented YAML. The write
// is atomic: the YAML is encoded to a temp file in the same directory and then
// renamed over the target (rename is atomic on the same filesystem), so a crash
// or disk-full mid-write cannot leave a partially written config file. The mode
// of an existing target is preserved; new files default to 0o644.
func WriteConfigFile(path string, m map[string]any) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("encoding config YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("closing YAML encoder: %w", err)
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".binder-config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp config file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we fail before the rename succeeds.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp config file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp config file %q: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("setting mode on temp config file %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming temp config file over %q: %w", path, err)
	}
	return nil
}

// SetKeyInFile sets a key/value in the specified config file.
func SetKeyInFile(filePath, key, value string) (string, error) {
	canonical, err := CanonicalKey(key)
	if err != nil {
		return "", err
	}
	if err := ValidateValue(canonical, value); err != nil {
		return "", err
	}

	settings, err := ReadConfigFile(filePath)
	if err != nil {
		return "", err
	}

	// All known config keys are string-typed, so store the raw string value.
	// This avoids surprising YAML type coercion (e.g. a numeric GCP project id
	// becoming a YAML int, or "true" becoming a YAML bool).
	settings[canonical] = value

	if err := WriteConfigFile(filePath, settings); err != nil {
		return "", err
	}
	return canonical, nil
}

// UnsetKeyInFile removes a key from the specified config file.
// Returns canonical key, whether it was present, and error.
func UnsetKeyInFile(filePath, key string) (string, bool, error) {
	canonical, err := CanonicalKey(key)
	if err != nil {
		return "", false, err
	}

	settings, err := ReadConfigFile(filePath)
	if err != nil {
		return "", false, err
	}

	existed := false
	if _, ok := settings[canonical]; ok {
		delete(settings, canonical)
		existed = true
	}
	// Also check dotted or alternative representations if any
	altKey := strings.ReplaceAll(canonical, "_", ".")
	if _, ok := settings[altKey]; ok {
		delete(settings, altKey)
		existed = true
	}

	if existed {
		if len(settings) == 0 {
			// Removing the last key would leave an ugly "{}" in the file;
			// delete the file instead so an empty config is genuinely absent.
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				return canonical, false, fmt.Errorf("removing empty config file %q: %w", filePath, err)
			}
		} else if err := WriteConfigFile(filePath, settings); err != nil {
			return canonical, false, err
		}
	}
	return canonical, existed, nil
}
