package proxy

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDurationUnmarshal(t *testing.T) {
	cases := []struct {
		In   string
		Want time.Duration
	}{
		{`"1h30m12s"`, time.Hour + 30*time.Minute + 12*time.Second},
		{`30`, 30 * time.Second},
		{`0.5`, 500 * time.Millisecond},
		{`"250ms"`, 250 * time.Millisecond},
	}

	for _, c := range cases {
		var d Duration
		if err := d.UnmarshalJSON([]byte(c.In)); err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", c.In, err)
		}
		if time.Duration(d) != c.Want {
			t.Errorf("UnmarshalJSON(%s) = %v, want %v", c.In, time.Duration(d), c.Want)
		}
	}
}

func TestDurationOrDefault(t *testing.T) {
	var zero Duration
	if zero.OrDefault() != DefaultTimeout {
		t.Errorf("zero OrDefault = %v, want %v", zero.OrDefault(), DefaultTimeout)
	}

	set := Duration(5 * time.Second)
	if set.OrDefault() != 5*time.Second {
		t.Errorf("set OrDefault = %v, want 5s", set.OrDefault())
	}
}

func TestServerConfigIsEnabled(t *testing.T) {
	true := true
	false := false

	if !(ServerConfig{}).IsEnabled() {
		t.Error("omitted enabled should default to true")
	}
	if !(ServerConfig{Enabled: &true}).IsEnabled() {
		t.Error("explicit true should be enabled")
	}
	if (ServerConfig{Enabled: &false}).IsEnabled() {
		t.Error("explicit false should be disabled")
	}
}

func TestLoadConfigFrom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lazy-mcp.json")

	writeFile(t, path, `{
	  "servers": {
	    "a": {
	      "description": "local one", "timeout": "45s", "spawn": "lazy",
	      "server": { "command": ["echo"] }
	    },
	    "b": {
	      "description": "disabled one", "enabled": false,
	      "server": { "command": ["true"] }
	    },
	    "c": {
	      "description": "remote one",
	      "server": { "url": "https://mcp.example.com/mcp", "headers": { "Authorization": "Bearer x" }, "environment": { "REGION": "us-east-1" } }
	    }
	  }
	}`)

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}

	a := cfg.Servers["a"]
	if time.Duration(a.Timeout) != 45*time.Second {
		t.Errorf("a.timeout = %v, want 45s", time.Duration(a.Timeout))
	}
	if a.Spawn != SpawnLazy {
		t.Errorf("a.spawn = %q, want lazy", a.Spawn)
	}
	if !a.IsEnabled() {
		t.Error("a should be enabled by default")
	}
	if a.Server.IsRemote() {
		t.Error("a should be local")
	}

	b := cfg.Servers["b"]
	if b.IsEnabled() {
		t.Error("b should be disabled")
	}

	c := cfg.Servers["c"]
	if !c.Server.IsRemote() {
		t.Error("c should be remote")
	}
	if c.Server.Headers["Authorization"] != "Bearer x" {
		t.Errorf("c.headers[Authorization] = %q, want Bearer x", c.Server.Headers["Authorization"])
	}
	if c.Server.Environment["REGION"] != "us-east-1" {
		t.Errorf("c.environment[REGION] = %q, want us-east-1 (environment valid on remote)", c.Server.Environment["REGION"])
	}
}

func TestLoadConfigJSONC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lazy-mcp.jsonc")

	writeFile(t, path, `{
	  // a leading comment
	  "servers": {
	    "a": {
	      "description": "local one", /* inline */
	      "server": { "command": ["echo"] }, // trailing comma below
	    },
	  }
	}`)

	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom jsonc: %v", err)
	}
	if cfg.Servers["a"].Description != "local one" {
		t.Errorf("a.description = %q, want %q", cfg.Servers["a"].Description, "local one")
	}
}

func TestLoadConfigValidation(t *testing.T) {
	cases := []struct {
		Name string
		JSON string
	}{
		{"missing description", `{ "servers": { "a": { "server": { "command": ["echo"] } } } }`},
		{"no transport", `{ "servers": { "a": { "description": "d", "server": {} } } }`},
		{"both transports", `{ "servers": { "a": { "description": "d", "server": { "command": ["echo"], "url": "https://x" } } } }`},
		{"local with headers", `{ "servers": { "a": { "description": "d", "server": { "command": ["echo"], "headers": { "K": "V" } } } } }`},
		{"local with oauth", `{ "servers": { "a": { "description": "d", "server": { "command": ["echo"], "oauth": { "clientId": "x", "clientSecret": "y" } } } } }`},
		{"oauth missing secret", `{ "servers": { "a": { "description": "d", "server": { "url": "https://x", "oauth": { "clientId": "x" } } } } }`},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "lazy-mcp.json")
			writeFile(t, path, c.JSON)
			if _, err := LoadConfigFrom(path); err == nil {
				t.Fatalf("expected error for %q, got nil", c.Name)
			}
		})
	}
}
