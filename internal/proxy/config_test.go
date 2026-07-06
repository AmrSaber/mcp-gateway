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
	    "a": { "command": ["echo"], "timeout": "45s", "spawn": "lazy" },
	    "b": { "command": ["true"], "enabled": false, "description": "disabled one" }
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

	B := Cfg.Servers["b"]
	if B.IsEnabled() {
		t.Error("b should be disabled")
	}
}

func TestLoadConfigMissingCommand(t *testing.T) {
	Dir := t.TempDir()
	Path := filepath.Join(Dir, "lazy-mcp.json")
	writeFile(t, Path, `{ "servers": { "a": { "description": "no command" } } }`)

	if _, Err := LoadConfigFrom(Path); Err == nil {
		t.Fatal("expected error for missing command, got nil")
	}
}
