// Package config handles loading and persisting configuration files.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nats-runner/internal/domain"

	"github.com/BurntSushi/toml"
)

// ConnectionInfo holds display information for a single connection.
type ConnectionInfo struct {
	Name string
	Path string
	URL  string
}

// ResolveConnection resolves the effective connection configuration.
// Priority: flagVal (name or path) > gc.DefaultConnection > error.
// gc is the already-loaded global config (pass the result of LoadGlobalConfig);
// it is used only to look up the default connection when flagVal is empty.
// Returns the config and a human-readable source label for config show.
func ResolveConnection(flagVal string, gc *domain.GlobalConfig) (*domain.ConnectionConfig, string, error) {
	name := flagVal
	if name == "" {
		if gc == nil || gc.DefaultConnection == "" {
			return nil, "", fmt.Errorf(
				"no connection specified; use -c <name> or run \"nats-runner config set <name>\" first",
			)
		}
		name = gc.DefaultConnection
	}
	path := ResolveConnPath(name)
	cfg, err := LoadConnectionFile(path)
	if err != nil {
		return nil, "", err
	}
	label := fmt.Sprintf("%s (%s)", name, path)
	return cfg, label, nil
}

// LoadConnectionFile reads and validates a configs/<name>.toml file.
// It decodes the [connection] section and validates auth_mode requirements.
func LoadConnectionFile(path string) (*domain.ConnectionConfig, error) {
	var wrapper struct {
		Connection domain.ConnectionConfig `toml:"connection"`
	}
	if _, err := toml.DecodeFile(path, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to load connection %q: %w", path, err)
	}
	cfg := &wrapper.Connection
	if err := validateAuthMode(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// validateAuthMode enforces required fields for each auth_mode value.
func validateAuthMode(cfg *domain.ConnectionConfig) error {
	switch cfg.AuthMode {
	case "creds":
		if cfg.CredsFile == "" {
			return fmt.Errorf("auth_mode \"creds\" requires creds_file")
		}
	case "token":
		if cfg.Token == "" {
			return fmt.Errorf("auth_mode \"token\" requires token")
		}
	case "nkey":
		if cfg.NKeySeedFile == "" {
			return fmt.Errorf("auth_mode \"nkey\" requires nkey_seed_file")
		}
	case "none", "":
		// OK — no required fields
	default:
		return fmt.Errorf("unknown auth_mode %q; valid values: creds, token, nkey, none", cfg.AuthMode)
	}
	return nil
}

// ResolveConnPath converts a connection name or explicit path to a file path.
// Names without path separators are expanded to configs/<name>.toml.
func ResolveConnPath(name string) string {
	if strings.ContainsAny(name, "/\\") {
		return name
	}
	return filepath.Join("configs", name+".toml")
}

// LoadGlobalConfig reads ~/.nats-runner.toml.
// Returns an empty GlobalConfig (no error) when the file does not exist.
func LoadGlobalConfig() (*domain.GlobalConfig, error) {
	path, err := globalConfigPath()
	if err != nil {
		return nil, err
	}
	var gc domain.GlobalConfig
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return &gc, nil
	}
	if _, err := toml.DecodeFile(path, &gc); err != nil {
		return nil, fmt.Errorf("failed to load global config %q: %w", path, err)
	}
	return &gc, nil
}

// SaveGlobalConfig writes gc to ~/.nats-runner.toml (mode 0600).
// Non-empty fields in gc overwrite the current on-disk values; empty fields
// retain their existing values (merge semantics).
func SaveGlobalConfig(gc *domain.GlobalConfig) error {
	existing, err := LoadGlobalConfig()
	if err != nil {
		return err
	}
	if gc.DefaultConnection != "" {
		existing.DefaultConnection = gc.DefaultConnection
	}
	if gc.TemplateDir != "" {
		existing.TemplateDir = gc.TemplateDir
	}
	if gc.FuncsDir != "" {
		existing.FuncsDir = gc.FuncsDir
	}
	if gc.ValuesDir != "" {
		existing.ValuesDir = gc.ValuesDir
	}
	path, err := globalConfigPath()
	if err != nil {
		return err
	}
	var lines []string
	if existing.DefaultConnection != "" {
		lines = append(lines, fmt.Sprintf("default_connection = %q", existing.DefaultConnection))
	}
	if existing.TemplateDir != "" {
		lines = append(lines, fmt.Sprintf("template_dir = %q", existing.TemplateDir))
	}
	if existing.FuncsDir != "" {
		lines = append(lines, fmt.Sprintf("funcs_dir = %q", existing.FuncsDir))
	}
	if existing.ValuesDir != "" {
		lines = append(lines, fmt.Sprintf("values_dir = %q", existing.ValuesDir))
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0600)
}

// globalConfigPath returns the absolute path of ~/.nats-runner.toml.
func globalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nats-runner.toml"), nil
}

// ScanFunctions reads all *.toml files in funcsDir and returns a map keyed by
// filename without extension. Returns an empty map when funcsDir does not exist.
func ScanFunctions(funcsDir string) (map[string]domain.FuncConfig, error) {
	result := make(map[string]domain.FuncConfig)
	if _, err := os.Stat(funcsDir); os.IsNotExist(err) {
		return result, nil
	}
	entries, err := os.ReadDir(funcsDir)
	if err != nil {
		return nil, fmt.Errorf("reading funcs dir %q: %w", funcsDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		fpath := filepath.Join(funcsDir, e.Name())
		var fc domain.FuncConfig
		if _, err := toml.DecodeFile(fpath, &fc); err != nil {
			return nil, fmt.Errorf("parsing func config %q: %w", fpath, err)
		}
		if fc.Command == "" {
			return nil, fmt.Errorf("func config %q missing required field 'command'", fpath)
		}
		key := strings.TrimSuffix(e.Name(), ".toml")
		result[key] = fc
	}
	return result, nil
}

// ListConnections scans configsDir and returns a summary of each connection.
// Used by "config list".
func ListConnections(configsDir string) ([]ConnectionInfo, error) {
	entries, err := os.ReadDir(configsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading configs dir %q: %w", configsDir, err)
	}
	var list []ConnectionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		fpath := filepath.Join(configsDir, e.Name())
		name := strings.TrimSuffix(e.Name(), ".toml")
		url := ""
		if cfg, err := LoadConnectionFile(fpath); err == nil {
			url = cfg.URL
		}
		list = append(list, ConnectionInfo{Name: name, Path: fpath, URL: url})
	}
	return list, nil
}
