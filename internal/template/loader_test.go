package template

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_OK(t *testing.T) {
	const content = `
[create_user]
subject  = "users.create"
mode     = "req"
defaults = { role = "member" }
body     = """{"id": "{{id}}"}"""
`
	f := writeTempTOML(t, content)

	tmpl, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry, ok := tmpl["create_user"]
	if !ok {
		t.Fatal("expected create_user entry")
	}
	if entry.Subject != "users.create" {
		t.Errorf("unexpected subject: %s", entry.Subject)
	}
	if entry.Defaults["role"] != "member" {
		t.Errorf("unexpected default role: %s", entry.Defaults["role"])
	}
}

func TestLoad_WithStream(t *testing.T) {
	const content = `
[publish_order]
subject = "orders.created"
mode    = "js"
body    = """{"id": "{{id}}"}"""

[publish_order.stream]
create   = true
name     = "ORDERS"
subjects = ["orders.*"]
storage  = "file"
`
	f := writeTempTOML(t, content)

	tmpl, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry := tmpl["publish_order"]
	if entry.Stream == nil {
		t.Fatal("expected stream config to be parsed")
	}
	if !entry.Stream.Create {
		t.Error("expected stream.create = true")
	}
	if entry.Stream.Name != "ORDERS" {
		t.Errorf("unexpected stream name: %s", entry.Stream.Name)
	}
	if len(entry.Stream.Subjects) != 1 || entry.Stream.Subjects[0] != "orders.*" {
		t.Errorf("unexpected subjects: %v", entry.Stream.Subjects)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/tmpl.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error message should mention 'not found': %v", err)
	}
}

func TestGetEntry_Found(t *testing.T) {
	const content = `
[get_user]
subject = "users.get"
mode    = "req"
body    = """{"id": "{{id}}"}"""
`
	f := writeTempTOML(t, content)
	tmpl, err := Load(f)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	entry, err := GetEntry(tmpl, "get_user", f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Subject != "users.get" {
		t.Errorf("unexpected subject: %s", entry.Subject)
	}
}

func TestGetEntry_NotFound(t *testing.T) {
	const content = `
[existing]
subject = "test"
mode    = "req"
body    = "{}"
`
	f := writeTempTOML(t, content)
	tmpl, _ := Load(f)

	_, err := GetEntry(tmpl, "nonexistent", f)
	if err == nil {
		t.Fatal("expected error for missing entry name")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the missing name: %v", err)
	}
}

// writeTempTOML writes content to a temporary .toml file and returns its path.
func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "tmpl-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
