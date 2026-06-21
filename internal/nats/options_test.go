package nats

import (
	"strings"
	"testing"

	"nats-runner/internal/domain"
)

func TestBuildOptions_AuthModes(t *testing.T) {
	cases := []struct {
		name     string
		conn     domain.ConnectionConfig
		wantOpts int
		wantErr  string
	}{
		{"none", domain.ConnectionConfig{AuthMode: "none"}, 0, ""},
		{"empty", domain.ConnectionConfig{AuthMode: ""}, 0, ""},
		{"creds", domain.ConnectionConfig{AuthMode: "creds", CredsFile: "x.creds"}, 1, ""},
		{"token", domain.ConnectionConfig{AuthMode: "token", Token: "secret"}, 1, ""},
		{"unknown", domain.ConnectionConfig{AuthMode: "bogus"}, 0, "unknown auth_mode"},
		{"nkey_bad_seed", domain.ConnectionConfig{AuthMode: "nkey", NKeySeedFile: "/nonexistent.nk"}, 0, "nkey seed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, err := BuildOptions(&c.conn)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(opts) != c.wantOpts {
				t.Errorf("expected %d options, got %d", c.wantOpts, len(opts))
			}
		})
	}
}

func TestBuildOptions_TLS(t *testing.T) {
	conn := domain.ConnectionConfig{
		AuthMode: "none",
		TLS: domain.TLSConfig{
			CACert:             "ca.pem",
			InsecureSkipVerify: true,
		},
	}
	opts, err := BuildOptions(&conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RootCAs + Secure(InsecureSkipVerify).
	if len(opts) != 2 {
		t.Errorf("expected 2 TLS options, got %d", len(opts))
	}
}
