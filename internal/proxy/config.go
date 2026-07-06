package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
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

// ServerConfig is one gated downstream MCP server.
//
// Only Command is required. Everything else has a documented default (see
// Normalize). Tool allow/deny scoping is intentionally NOT modelled yet — it is
// documented as future work in the README.
type ServerConfig struct {
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment,omitempty"`
	Spawn       SpawnMode         `json:"spawn,omitempty"`
	Timeout     Duration          `json:"timeout,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
	Description string            `json:"description,omitempty"`
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

// ConfigPath is the full path to lazy-mcp.json.
func ConfigPath() (string, error) {
	Dir, Err := ConfigDir()
	if Err != nil {
		return "", Err
	}
	return filepath.Join(Dir, "lazy-mcp.json"), nil
}

// LoadConfig reads and parses lazy-mcp.json from the resolved config dir.
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
	if Err := json.Unmarshal(Raw, &Cfg); Err != nil {
		return nil, fmt.Errorf("parsing %s: %w", Path, Err)
	}

	for Name, Srv := range Cfg.Servers {
		if len(Srv.Command) == 0 {
			return nil, fmt.Errorf("server %q: command is required", Name)
		}
	}

	return &Cfg, nil
}
