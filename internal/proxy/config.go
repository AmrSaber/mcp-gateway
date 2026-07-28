package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tidwall/jsonc"
)

// DefaultTimeout is the connect/handshake timeout applied when a server omits
// its own "timeout".
const DefaultTimeout = 30 * time.Second

// SpawnMode controls when a downstream server's subprocess is started.
type SpawnMode string

const (
	spawnEager SpawnMode = "eager"
	spawnLazy  SpawnMode = "lazy"
)

// Config is the parsed mcp-gateway.json.
type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
}

// ServerConfig is one fronted downstream MCP server: operational settings plus a
// nested Server transport block.
//
// Description and Server are required. The other settings have documented
// defaults (see the field docs). Tool allow/deny scoping is intentionally NOT
// modelled yet — it is documented as future work in the README.
type ServerConfig struct {
	Description string     `json:"description"`
	Spawn       SpawnMode  `json:"spawn,omitempty"`
	Timeout     Duration   `json:"timeout,omitempty"`
	Enabled     *bool      `json:"enabled,omitempty"`
	Server      ServerSpec `json:"server"`
}

// ServerSpec is the transport for a fronted server. The transport kind is
// inferred from which field is set: Command ⇒ local (stdio subprocess), URL ⇒
// remote (streamable HTTP). Exactly one must be set (enforced in validation).
//
// All author-supplied string values (Command args, Environment values, Headers,
// OAuth credentials) support {env:NAME} and {cmd:...} interpolation resolved at
// connect time — see interpolate.
type ServerSpec struct {
	// Command is the local (stdio subprocess) transport; Environment adds env
	// vars on top of the inherited process environment and is valid for both
	// transports (it also feeds {env:...} resolution).
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`

	// Remote (streamable HTTP).
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	OAuth   *OAuthConfig      `json:"oauth,omitempty"`
}

// isRemote reports whether this is a remote (URL) server rather than a local
// stdio one. Only meaningful after validation has confirmed exactly one of
// Command/URL is set.
func (spec ServerSpec) isRemote() bool { return spec.URL != "" }

// OAuthConfig configures pre-registered OAuth client credentials for a remote
// server. Both fields are required: mcp-gateway uses the client-credentials grant
// (service-to-service, no user interaction), which needs a confidential client.
// The token endpoint and scopes are discovered automatically from server
// metadata. The interactive (browser) authorization-code flow with token
// storage is not implemented yet — see README future work.
type OAuthConfig struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// isEnabled reports whether the server should be loaded. Enabled defaults to
// true when omitted; only an explicit false skips the server.
func (config ServerConfig) isEnabled() bool {
	return config.Enabled == nil || *config.Enabled
}

// validate enforces the config invariants: a description, and exactly one
// transport (local Command or remote URL) with only its own fields set.
// Environment is allowed on both transports (it feeds {env:...} resolution).
func (config ServerConfig) validate() error {
	if config.Description == "" {
		return fmt.Errorf("description is required")
	}

	spec := config.Server
	hasCommand := len(spec.Command) > 0
	hasURL := spec.URL != ""

	switch {
	case hasCommand && hasURL:
		return fmt.Errorf("exactly one of server.command or server.url is required, not both")
	case !hasCommand && !hasURL:
		return fmt.Errorf("exactly one of server.command or server.url is required")
	case hasCommand:
		if len(spec.Headers) > 0 || spec.OAuth != nil {
			return fmt.Errorf("server.headers and server.oauth are only valid for remote (url) servers")
		}
	case hasURL:
		if spec.OAuth != nil && (spec.OAuth.ClientID == "" || spec.OAuth.ClientSecret == "") {
			return fmt.Errorf("server.oauth requires both clientId and clientSecret")
		}
	}

	return nil
}

// Duration accepts either a Go duration string ("1h30m12s") or a bare number
// (interpreted as seconds) from JSON.
type Duration time.Duration

func (dur *Duration) UnmarshalJSON(data []byte) error {
	// Bare number → seconds.
	if num, err := strconv.ParseFloat(string(data), 64); err == nil {
		*dur = Duration(time.Duration(num * float64(time.Second)))
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("timeout must be a duration string or number of seconds: %w", err)
	}

	parsed, err := time.ParseDuration(str)
	if err != nil {
		return fmt.Errorf("invalid timeout %q: %w", str, err)
	}

	*dur = Duration(parsed)
	return nil
}

// orDefault returns the duration, or DefaultTimeout when unset (zero).
func (dur Duration) orDefault() time.Duration {
	if dur == 0 {
		return DefaultTimeout
	}
	return time.Duration(dur)
}

// configDir resolves the config directory: OPENCODE_CONFIG_DIR if set,
// else $XDG_CONFIG_HOME, else ~/.config.
func configDir() (string, error) {
	if dir := os.Getenv("OPENCODE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory: %w", err)
		}

		dir = filepath.Join(home, ".config")
	}

	return dir, nil
}

// configPath resolves the config file in the config dir, preferring
// mcp-gateway.jsonc over mcp-gateway.json. It returns an error if neither exists.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	jsonc := filepath.Join(dir, "mcp-gateway.jsonc")
	json := filepath.Join(dir, "mcp-gateway.json")
	for _, path := range []string{jsonc, json} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no config found: expected %s or %s", jsonc, json)
}

// LoadConfig reads and parses the config from the resolved config dir.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return loadConfigFrom(path)
}

// loadConfigFrom reads and parses mcp-gateway.json from an explicit path.
func loadConfigFrom(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(jsonc.ToJSON(raw), &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	for name, srv := range cfg.Servers {
		if err := srv.validate(); err != nil {
			return nil, fmt.Errorf("server %q: %w", name, err)
		}
	}

	return &cfg, nil
}
