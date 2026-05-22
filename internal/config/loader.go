// Package config handles loading and persisting configuration files.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"nats-runner/internal/domain"
)

// LoadAppConfig decodes a config.toml at the given path into an AppConfig.
func LoadAppConfig(path string) (*domain.AppConfig, error) {
	var cfg domain.AppConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to load config %q: %w", path, err)
	}
	return &cfg, nil
}

// ResolveConfigPath returns the effective config path.
// Priority: explicit flagPath > default stored in ~/.nats-runner.toml.
func ResolveConfigPath(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	gc, err := LoadGlobalConfig()
	if err != nil {
		return "", fmt.Errorf(
			"no config file specified. Use -c <path> or run \"nats-runner config set <path>\" first",
		)
	}
	return gc.DefaultConfigPath, nil
}

// LoadGlobalConfig reads ~/.nats-runner.toml and returns the stored settings.
func LoadGlobalConfig() (*domain.GlobalConfig, error) {
	path, err := globalConfigFile()
	if err != nil {
		return nil, err
	}
	var gc domain.GlobalConfig
	if _, err := toml.DecodeFile(path, &gc); err != nil {
		return nil, err
	}
	if gc.DefaultConfigPath == "" {
		return nil, fmt.Errorf("default_config_path not set in %s", path)
	}
	return &gc, nil
}

// SaveGlobalConfig writes the given absolute path as the default config path
// to ~/.nats-runner.toml (file mode 0600).
func SaveGlobalConfig(absPath string) error {
	path, err := globalConfigFile()
	if err != nil {
		return err
	}
	content := fmt.Sprintf("default_config_path = %q\n", absPath)
	return os.WriteFile(path, []byte(content), 0600)
}

// globalConfigFile returns the absolute path of ~/.nats-runner.toml.
func globalConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nats-runner.toml"), nil
}
