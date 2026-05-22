// Package nats provides NATS connection management and message execution.
package nats

import (
	"crypto/tls"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"nats-runner/internal/domain"
)

// BuildOptions constructs a slice of nats.Option from the AppConfig,
// covering authentication (creds / token / nkey) and optional TLS.
func BuildOptions(cfg *domain.AppConfig) ([]natsgo.Option, error) {
	var opts []natsgo.Option
	conn := cfg.Connection

	switch conn.AuthMode {
	case "creds":
		if conn.CredsFile == "" {
			return nil, fmt.Errorf("auth_mode is 'creds' but creds_file is not set")
		}
		opts = append(opts, natsgo.UserCredentials(conn.CredsFile))

	case "token":
		if conn.Token == "" {
			return nil, fmt.Errorf("auth_mode is 'token' but token is not set")
		}
		opts = append(opts, natsgo.Token(conn.Token))

	case "nkey":
		if conn.NKeySeedFile == "" {
			return nil, fmt.Errorf("auth_mode is 'nkey' but nkey_seed_file is not set")
		}
		// NkeyOptionFromSeed reads the .nk seed file and automatically derives the
		// public key for signing — no separate public key file is required.
		opt, err := natsgo.NkeyOptionFromSeed(conn.NKeySeedFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load nkey seed: %w", err)
		}
		opts = append(opts, opt)

	case "none", "":
		// no authentication required

	default:
		return nil, fmt.Errorf("unknown auth_mode %q: expected creds, token, nkey, or none", conn.AuthMode)
	}

	// TLS — only applied when at least one field is configured.
	tlsCfg := conn.TLS
	if tlsCfg.CACert != "" {
		opts = append(opts, natsgo.RootCAs(tlsCfg.CACert))
	}
	if tlsCfg.ClientCert != "" && tlsCfg.ClientKey != "" {
		opts = append(opts, natsgo.ClientCert(tlsCfg.ClientCert, tlsCfg.ClientKey))
	}
	if tlsCfg.InsecureSkipVerify {
		opts = append(opts, natsgo.Secure(&tls.Config{InsecureSkipVerify: true})) //nolint:gosec // intentional, only for local testing
	}

	return opts, nil
}
