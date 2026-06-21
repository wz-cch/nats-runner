package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nats-runner/internal/domain"
)

func TestResolveConnPath(t *testing.T) {
	if got := ResolveConnPath("local"); got != filepath.Join("configs", "local.toml") {
		t.Errorf("name should expand to configs/<name>.toml, got %q", got)
	}
	if got := ResolveConnPath("/abs/path.toml"); got != "/abs/path.toml" {
		t.Errorf("explicit path should pass through, got %q", got)
	}
}

func TestLoadConnectionFile_AuthValidation(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		wantErr string // substring; "" = expect success
	}{
		{"creds_missing_file", `[connection]` + "\n" + `auth_mode = "creds"`, "creds_file"},
		{"token_missing", `[connection]` + "\n" + `auth_mode = "token"`, "token"},
		{"nkey_missing", `[connection]` + "\n" + `auth_mode = "nkey"`, "nkey_seed_file"},
		{"unknown_mode", `[connection]` + "\n" + `auth_mode = "bogus"`, "unknown auth_mode"},
		{"none_ok", `[connection]` + "\n" + `url = "nats://localhost:4222"` + "\n" + `auth_mode = "none"`, ""},
		{"empty_mode_ok", `[connection]` + "\n" + `url = "nats://localhost:4222"`, ""},
		{"creds_ok", `[connection]` + "\n" + `auth_mode = "creds"` + "\n" + `creds_file = "x.creds"`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.name+".toml")
			if err := os.WriteFile(path, []byte(c.content), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadConnectionFile(path)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", c.wantErr, err)
			}
		})
	}
}

func TestSaveAndLoadGlobalConfig_RoundTripAndMerge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// First write sets the default connection only.
	if err := SaveGlobalConfig(&domain.GlobalConfig{DefaultConnection: "local"}); err != nil {
		t.Fatalf("save 1: %v", err)
	}
	gc, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("load 1: %v", err)
	}
	if gc.DefaultConnection != "local" {
		t.Fatalf("expected default_connection=local, got %q", gc.DefaultConnection)
	}

	// Second write sets a directory; merge semantics must preserve the connection.
	if err := SaveGlobalConfig(&domain.GlobalConfig{TemplateDir: "tmpl"}); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	gc, err = LoadGlobalConfig()
	if err != nil {
		t.Fatalf("load 2: %v", err)
	}
	if gc.DefaultConnection != "local" {
		t.Errorf("merge lost default_connection: %q", gc.DefaultConnection)
	}
	if gc.TemplateDir != "tmpl" {
		t.Errorf("expected template_dir=tmpl, got %q", gc.TemplateDir)
	}
}

// The TOML encoder must round-trip values containing special characters
// (a backslash path is the classic case where hand-rolled %q diverges).
func TestSaveGlobalConfig_SpecialCharsRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	weird := `C:\nats\templates`
	if err := SaveGlobalConfig(&domain.GlobalConfig{TemplateDir: weird}); err != nil {
		t.Fatalf("save: %v", err)
	}
	gc, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if gc.TemplateDir != weird {
		t.Errorf("backslash path did not round-trip: got %q want %q", gc.TemplateDir, weird)
	}
}

func TestLoadGlobalConfig_MissingFileIsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	gc, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if gc.DefaultConnection != "" || gc.TemplateDir != "" {
		t.Errorf("expected empty config, got %+v", gc)
	}
}
