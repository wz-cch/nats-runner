package vars

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// LoadValuesFiles loads and merges value files in order.
// Later files take priority over earlier ones.
// Supports .toml and .json extensions (detected automatically).
// An empty paths slice returns an empty map with no error.
func LoadValuesFiles(paths []string) (map[string]any, error) {
	merged := make(map[string]any)
	for _, p := range paths {
		m, err := loadValuesFile(p)
		if err != nil {
			return nil, err
		}
		mergeMaps(merged, m)
	}
	return merged, nil
}

func loadValuesFile(path string) (map[string]any, error) {
	switch {
	case strings.HasSuffix(path, ".toml"):
		return loadTOML(path)
	case strings.HasSuffix(path, ".json"):
		return loadJSON(path)
	default:
		return nil, fmt.Errorf("unsupported values file extension: %q (expected .toml or .json)", path)
	}
}

func loadTOML(path string) (map[string]any, error) {
	var m map[string]any
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return nil, fmt.Errorf("failed to load values file %q: %w", path, err)
	}
	return m, nil
}

func loadJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read values file %q: %w", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse values file %q: %w", path, err)
	}
	return m, nil
}

// mergeMaps merges src into dst; src values overwrite dst values.
func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}
