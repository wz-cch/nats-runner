// Package domain defines the core business types shared across all layers.
// This package has no dependencies on external libraries or other internal packages.
package domain

// AppConfig is the runtime-assembled application configuration passed to the
// NATS connection layer. Shell functions are resolved separately via the vars
// package and are not part of this struct.
type AppConfig struct {
	Connection ConnectionConfig
}

// ConnectionConfig holds NATS connection and authentication settings.
// It is decoded from the [connection] section of a configs/<name>.toml file.
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
	DefaultConnection string `toml:"default_connection"`
	TemplateDir       string `toml:"template_dir"`
	FuncsDir          string `toml:"funcs_dir"`
	ValuesDir         string `toml:"values_dir"`
}

// FuncConfig corresponds to a single funcs/<name>.toml file.
type FuncConfig struct {
	Command string `toml:"command"`
	Desc    string `toml:"desc"`
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
