// Package domain defines the core business types shared across all layers.
// This package has no dependencies on external libraries or other internal packages.
package domain

// AppConfig is the top-level configuration decoded from config.toml.
type AppConfig struct {
	Connection ConnectionConfig  `toml:"connection"`
	Functions  map[string]string `toml:"functions"`
}

// ConnectionConfig holds NATS connection and authentication settings.
type ConnectionConfig struct {
	URL          string    `toml:"url"`
	AuthMode     string    `toml:"auth_mode"`
	CredsFile    string    `toml:"creds_file"`
	Token        string    `toml:"token"`
	NKeySeedFile string    `toml:"nkey_seed_file"`
	TimeoutMs    int       `toml:"timeout_ms"`
	TLS          TLSConfig `toml:"tls"`
}

// TLSConfig holds optional TLS settings under [connection.tls].
// All fields are optional; an empty struct means TLS is not applied.
type TLSConfig struct {
	CACert             string `toml:"ca_cert"`
	ClientCert         string `toml:"client_cert"`
	ClientKey          string `toml:"client_key"`
	InsecureSkipVerify bool   `toml:"insecure_skip_verify"`
}

// GlobalConfig is decoded from ~/.nats-runner.toml.
type GlobalConfig struct {
	DefaultConfigPath string `toml:"default_config_path"`
}

// TemplateEntry represents a single API definition within a template TOML file.
type TemplateEntry struct {
	Subject  string            `toml:"subject"`
	Mode     string            `toml:"mode"`
	Defaults map[string]string `toml:"defaults"`
	Body     string            `toml:"body"`
	Stream   *StreamConfig     `toml:"stream"`
}

// StreamConfig holds JetStream stream auto-creation settings declared under [name.stream].
type StreamConfig struct {
	Create        bool     `toml:"create"`
	Name          string   `toml:"name"`
	Subjects      []string `toml:"subjects"`
	Storage       string   `toml:"storage"` // "file" or "memory"
	MaxAgeSeconds int64    `toml:"max_age_seconds"`
	MaxMsgs       int64    `toml:"max_msgs"`
}
