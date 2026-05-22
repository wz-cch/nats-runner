// Package template handles loading and looking up API definitions from template TOML files.
package template

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"nats-runner/internal/domain"
)

// Load decodes a template TOML file into a map keyed by template name.
// Returns a clear error if the file does not exist.
func Load(path string) (map[string]domain.TemplateEntry, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("template file not found: %s", path)
	}
	var tmpl map[string]domain.TemplateEntry
	if _, err := toml.DecodeFile(path, &tmpl); err != nil {
		return nil, fmt.Errorf("failed to parse template %q: %w", path, err)
	}
	return tmpl, nil
}

// GetEntry looks up a named entry in a decoded template map.
// Returns a clear error if the name is not present.
func GetEntry(tmpl map[string]domain.TemplateEntry, name, path string) (*domain.TemplateEntry, error) {
	entry, ok := tmpl[name]
	if !ok {
		return nil, fmt.Errorf("template %q not found in %s", name, path)
	}
	return &entry, nil
}
