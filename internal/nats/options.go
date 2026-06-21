// Package nats provides NATS connection management and message execution.
package nats

import (
	"crypto/tls"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"nats-runner/internal/domain"
)

// BuildOptions constructs a slice of nats.Option from a ConnectionConfig,
// covering authentication (creds / token / nkey) and optional TLS.
//
// Required-field presence (e.g. creds_file for auth_mode "creds") is validated
// once at load time by config.validateAuthMode; this function trusts that and
// only translates the settings into nats.Option values.
func BuildOptions(conn *domain.ConnectionConfig) ([]natsgo.Option, error) {
	var opts []natsgo.Option

	switch conn.AuthMode {
	case "creds":
		opts = append(opts, natsgo.UserCredentials(conn.CredsFile))

	case "token":
		opts = append(opts, natsgo.Token(conn.Token))

	case "nkey":
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
