package vars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B1: JSON values must keep integer fidelity. Without UseNumber, a JSON number
// like 1000000 unmarshals to float64 and renders as "1e+06" in the payload.
func TestLoadValuesFiles_JSONIntegerFidelity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vals.json")
	if err := os.WriteFile(path, []byte(`{"count": 1000000, "big": 12345678, "ratio": 1.5}`), 0600); err != nil {
		t.Fatal(err)
	}

	vals, err := LoadValuesFiles([]string{path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := `{"count": {{ .count }}, "big": {{ .big }}, "ratio": {{ .ratio }}}`
	out, err := Resolve(body, ctx(nil, vals, nil, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{`"count": 1000000`, `"big": 12345678`, `"ratio": 1.5`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
	if strings.Contains(out, "e+") {
		t.Errorf("integers must not render in scientific notation: %s", out)
	}
}

func TestLoadValuesFiles_LaterFileWins(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.toml")
	b := filepath.Join(dir, "b.json")
	if err := os.WriteFile(a, []byte(`role = "member"`+"\n"+`team = "core"`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`{"role": "admin"}`), 0600); err != nil {
		t.Fatal(err)
	}
	vals, err := LoadValuesFiles([]string{a, b})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals["role"] != "admin" {
		t.Errorf("later file should win for role: got %v", vals["role"])
	}
	if vals["team"] != "core" {
		t.Errorf("unoverridden key should survive: got %v", vals["team"])
	}
}

func TestLoadValuesFiles_UnsupportedExtension(t *testing.T) {
	_, err := LoadValuesFiles([]string{"vals.yaml"})
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

// ResolveWithValues returns the effective value of every referenced variable.
func TestResolveWithValues_ReturnsData(t *testing.T) {
	body := `{"id": "{{ .id }}", "role": "{{ .role }}"}`
	out, values, err := ResolveWithValues(body, ctx(
		map[string]string{"id": "42"},
		nil,
		map[string]string{"role": "member"},
		nil,
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"42"`) {
		t.Errorf("unexpected output: %s", out)
	}
	if values["id"] != "42" || values["role"] != "member" {
		t.Errorf("resolved values incomplete: %v", values)
	}
}
