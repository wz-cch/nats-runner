// Package template handles loading and looking up API definitions from template TOML files.
package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nats-runner/internal/domain"

	"github.com/BurntSushi/toml"
)

// TemplateFileInfo holds basic metadata about a scanned template file.
type TemplateFileInfo struct {
	FileName   string // e.g. "base.srp.toml"
	Path       string // full path
	EntryCount int    // number of top-level TOML keys
}

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

// ScanTemplates scans dir for *.toml files and returns metadata for each.
// Returns an error if dir does not exist or a file cannot be parsed.
func ScanTemplates(dir string) ([]TemplateFileInfo, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("template directory not found: %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading template dir %q: %w", dir, err)
	}
	var list []TemplateFileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		fpath := filepath.Join(dir, e.Name())
		count, err := countEntries(fpath)
		if err != nil {
			return nil, err
		}
		list = append(list, TemplateFileInfo{
			FileName:   e.Name(),
			Path:       fpath,
			EntryCount: count,
		})
	}
	return list, nil
}

// countEntries decodes a TOML file and returns the number of top-level keys.
func countEntries(path string) (int, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return 0, fmt.Errorf("failed to count entries in %q: %w", path, err)
	}
	return len(raw), nil
}
