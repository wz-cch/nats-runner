package template_test

import (
	"testing"

	"nats-runner/internal/template"
	"nats-runner/internal/vars"
)

// TestShippedTemplatesParse guards against TOML or Go-template syntax errors in
// the templates shipped under templates/. It loads every entry and parses its
// body (via ReferencedVars) without executing or sending anything.
func TestShippedTemplatesParse(t *testing.T) {
	files, err := template.ScanTemplates("../../templates")
	if err != nil {
		t.Fatalf("scanning templates: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no templates found")
	}
	for _, f := range files {
		tmpl, err := template.Load(f.Path)
		if err != nil {
			t.Errorf("%s: load failed: %v", f.FileName, err)
			continue
		}
		for name, entry := range tmpl {
			if _, err := vars.ReferencedVars(entry.Body); err != nil {
				t.Errorf("%s/%s: template parse error: %v", f.FileName, name, err)
			}
			switch entry.Mode {
			case "req", "pub", "js":
			default:
				t.Errorf("%s/%s: invalid mode %q", f.FileName, name, entry.Mode)
			}
		}
	}
}
