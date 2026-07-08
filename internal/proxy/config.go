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
	SpawnEager SpawnMode = "eager"
	SpawnLazy  SpawnMode = "lazy"
)

// Config is the parsed lazy-mcp.json.
type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
}

// ServerConfig is one gated downstream MCP server: operational settings plus a
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

// ServerSpec is the transport for a gated server. The transport kind is
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

// IsRemote reports whether this is a remote (URL) server rather than a local
// stdio one. Only meaningful after validation has confirmed exactly one of
// Command/URL is set.
func (Spec ServerSpec) IsRemote() bool { return Spec.URL != "" }

// OAuthConfig configures pre-registered OAuth client credentials for a remote
// server. Both fields are required: lazy-mcp uses the client-credentials grant
// (service-to-service, no user interaction), which needs a confidential client.
// The token endpoint and scopes are discovered automatically from server
// metadata. The interactive (browser) authorization-code flow with token
// storage is not implemented yet — see README future work.
type OAuthConfig struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// IsEnabled reports whether the server should be loaded. Enabled defaults to
// true when omitted; only an explicit false skips the server.
func (Config ServerConfig) IsEnabled() bool {
	return Config.Enabled == nil || *Config.Enabled
}

// Duration accepts either a Go duration string ("1h30m12s") or a bare number
// (interpreted as seconds) from JSON.
type Duration time.Duration

func (Dur *Duration) UnmarshalJSON(Data []byte) error {
	// Bare number → seconds.
	if Num, Err := strconv.ParseFloat(string(Data), 64); Err == nil {
		*Dur = Duration(time.Duration(Num * float64(time.Second)))
		return nil
	}

	var Str string
	if Err := json.Unmarshal(Data, &Str); Err != nil {
		return fmt.Errorf("timeout must be a duration string or number of seconds: %w", Err)
	}

	Parsed, Err := time.ParseDuration(Str)
	if Err != nil {
		return fmt.Errorf("invalid timeout %q: %w", Str, Err)
	}

	*Dur = Duration(Parsed)
	return nil
}

// OrDefault returns the duration, or DefaultTimeout when unset (zero).
func (Dur Duration) OrDefault() time.Duration {
	if Dur == 0 {
		return DefaultTimeout
	}
	return time.Duration(Dur)
}

// ConfigDir resolves opencode's config directory: OPENCODE_CONFIG_DIR if set,
// else $XDG_CONFIG_HOME/opencode, else ~/.config/opencode. Mirrors opencode's
// own resolution for the common case.
func ConfigDir() (string, error) {
	if Dir := os.Getenv("OPENCODE_CONFIG_DIR"); Dir != "" {
		return Dir, nil
	}

	Base := os.Getenv("XDG_CONFIG_HOME")
	if Base == "" {
		Home, Err := os.UserHomeDir()
		if Err != nil {
			return "", fmt.Errorf("resolving home directory: %w", Err)
		}
		Base = filepath.Join(Home, ".config")
	}

	return filepath.Join(Base, "opencode"), nil
}

// ConfigPath resolves the config file in the config dir, preferring
// lazy-mcp.jsonc over lazy-mcp.json. It returns an error if neither exists.
func ConfigPath() (string, error) {
	Dir, Err := ConfigDir()
	if Err != nil {
		return "", Err
	}

	Jsonc := filepath.Join(Dir, "lazy-mcp.jsonc")
	Json := filepath.Join(Dir, "lazy-mcp.json")
	for _, Path := range []string{Jsonc, Json} {
		if _, Err := os.Stat(Path); Err == nil {
			return Path, nil
		}
	}

	return "", fmt.Errorf("no config found: expected %s or %s", Jsonc, Json)
}

// LoadConfig reads and parses the config from the resolved config dir.
func LoadConfig() (*Config, error) {
	Path, Err := ConfigPath()
	if Err != nil {
		return nil, Err
	}
	return LoadConfigFrom(Path)
}

// LoadConfigFrom reads and parses lazy-mcp.json from an explicit path.
func LoadConfigFrom(Path string) (*Config, error) {
	Raw, Err := os.ReadFile(Path)
	if Err != nil {
		return nil, fmt.Errorf("reading %s: %w", Path, Err)
	}

	var Cfg Config
	if Err := json.Unmarshal(jsonc.ToJSON(Raw), &Cfg); Err != nil {
		return nil, fmt.Errorf("parsing %s: %w", Path, Err)
	}

	for Name, Srv := range Cfg.Servers {
		if Err := Srv.validate(); Err != nil {
			return nil, fmt.Errorf("server %q: %w", Name, Err)
		}
	}

	return &Cfg, nil
}

// validate enforces the config invariants: a description, and exactly one
// transport (local Command or remote URL) with only its own fields set.
// Environment is allowed on both transports (it feeds {env:...} resolution).
func (Cfg ServerConfig) validate() error {
	if Cfg.Description == "" {
		return fmt.Errorf("description is required")
	}

	Spec := Cfg.Server
	HasCommand := len(Spec.Command) > 0
	HasURL := Spec.URL != ""

	switch {
	case HasCommand && HasURL:
		return fmt.Errorf("exactly one of server.command or server.url is required, not both")
	case !HasCommand && !HasURL:
		return fmt.Errorf("exactly one of server.command or server.url is required")
	case HasCommand:
		if len(Spec.Headers) > 0 || Spec.OAuth != nil {
			return fmt.Errorf("server.headers and server.oauth are only valid for remote (url) servers")
		}
	case HasURL:
		if Spec.OAuth != nil && (Spec.OAuth.ClientID == "" || Spec.OAuth.ClientSecret == "") {
			return fmt.Errorf("server.oauth requires both clientId and clientSecret")
		}
	}

	return nil
}
