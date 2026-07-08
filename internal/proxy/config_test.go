package proxy

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDurationUnmarshal(t *testing.T) {
	Cases := []struct {
		In   string
		Want time.Duration
	}{
		{`"1h30m12s"`, time.Hour + 30*time.Minute + 12*time.Second},
		{`30`, 30 * time.Second},
		{`0.5`, 500 * time.Millisecond},
		{`"250ms"`, 250 * time.Millisecond},
	}

	for _, C := range Cases {
		var D Duration
		if Err := D.UnmarshalJSON([]byte(C.In)); Err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", C.In, Err)
		}
		if time.Duration(D) != C.Want {
			t.Errorf("UnmarshalJSON(%s) = %v, want %v", C.In, time.Duration(D), C.Want)
		}
	}
}

func TestDurationOrDefault(t *testing.T) {
	var Zero Duration
	if Zero.OrDefault() != DefaultTimeout {
		t.Errorf("zero OrDefault = %v, want %v", Zero.OrDefault(), DefaultTimeout)
	}

	Set := Duration(5 * time.Second)
	if Set.OrDefault() != 5*time.Second {
		t.Errorf("set OrDefault = %v, want 5s", Set.OrDefault())
	}
}

func TestServerConfigIsEnabled(t *testing.T) {
	True := true
	False := false

	if !(ServerConfig{}).IsEnabled() {
		t.Error("omitted enabled should default to true")
	}
	if !(ServerConfig{Enabled: &True}).IsEnabled() {
		t.Error("explicit true should be enabled")
	}
	if (ServerConfig{Enabled: &False}).IsEnabled() {
		t.Error("explicit false should be disabled")
	}
}

func TestLoadConfigFrom(t *testing.T) {
	Dir := t.TempDir()
	Path := filepath.Join(Dir, "lazy-mcp.json")

	writeFile(t, Path, `{
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

	Cfg, Err := LoadConfigFrom(Path)
	if Err != nil {
		t.Fatalf("LoadConfigFrom: %v", Err)
	}

	A := Cfg.Servers["a"]
	if time.Duration(A.Timeout) != 45*time.Second {
		t.Errorf("a.timeout = %v, want 45s", time.Duration(A.Timeout))
	}
	if A.Spawn != SpawnLazy {
		t.Errorf("a.spawn = %q, want lazy", A.Spawn)
	}
	if !A.IsEnabled() {
		t.Error("a should be enabled by default")
	}
	if A.Server.IsRemote() {
		t.Error("a should be local")
	}

	B := Cfg.Servers["b"]
	if B.IsEnabled() {
		t.Error("b should be disabled")
	}

	C := Cfg.Servers["c"]
	if !C.Server.IsRemote() {
		t.Error("c should be remote")
	}
	if C.Server.Headers["Authorization"] != "Bearer x" {
		t.Errorf("c.headers[Authorization] = %q, want Bearer x", C.Server.Headers["Authorization"])
	}
	if C.Server.Environment["REGION"] != "us-east-1" {
		t.Errorf("c.environment[REGION] = %q, want us-east-1 (environment valid on remote)", C.Server.Environment["REGION"])
	}
}

func TestLoadConfigJSONC(t *testing.T) {
	Dir := t.TempDir()
	Path := filepath.Join(Dir, "lazy-mcp.jsonc")

	writeFile(t, Path, `{
	  // a leading comment
	  "servers": {
	    "a": {
	      "description": "local one", /* inline */
	      "server": { "command": ["echo"] }, // trailing comma below
	    },
	  }
	}`)

	Cfg, Err := LoadConfigFrom(Path)
	if Err != nil {
		t.Fatalf("LoadConfigFrom jsonc: %v", Err)
	}
	if Cfg.Servers["a"].Description != "local one" {
		t.Errorf("a.description = %q, want %q", Cfg.Servers["a"].Description, "local one")
	}
}

func TestLoadConfigValidation(t *testing.T) {
	Cases := []struct {
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

	for _, C := range Cases {
		t.Run(C.Name, func(t *testing.T) {
			Dir := t.TempDir()
			Path := filepath.Join(Dir, "lazy-mcp.json")
			writeFile(t, Path, C.JSON)
			if _, Err := LoadConfigFrom(Path); Err == nil {
				t.Fatalf("expected error for %q, got nil", C.Name)
			}
		})
	}
}
