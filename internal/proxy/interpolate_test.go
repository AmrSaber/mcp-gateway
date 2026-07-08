package proxy

import (
	"context"
	"testing"
)

func TestInterpolateEnv(t *testing.T) {
	Env := []string{"TOKEN=abc", "TOKEN=xyz", "EMPTY="}

	Cases := []struct {
		In   string
		Want string
	}{
		{"Bearer {env:TOKEN}", "Bearer xyz"}, // last-wins (map over process)
		{"{env:EMPTY}", ""},
		{"no directive here", "no directive here"},
		{"{env:TOKEN}-{env:TOKEN}", "xyz-xyz"}, // multiple directives
		{"{notadirective}", "{notadirective}"}, // unknown → literal
		{"prefix {env:TOKEN} suffix", "prefix xyz suffix"},
		{"{env: TOKEN}", "xyz"},   // whitespace around value trimmed
		{"{env:TOKEN }", "xyz"},   // trailing whitespace trimmed
		{"{ env:TOKEN }", "xyz"},  // whitespace around directive trimmed
	}

	for _, C := range Cases {
		Got, Err := interpolate(context.Background(), C.In, Env)
		if Err != nil {
			t.Fatalf("interpolate(%q): %v", C.In, Err)
		}
		if Got != C.Want {
			t.Errorf("interpolate(%q) = %q, want %q", C.In, Got, C.Want)
		}
	}
}

func TestInterpolateEnvUnset(t *testing.T) {
	if _, Err := interpolate(context.Background(), "{env:MISSING}", nil); Err == nil {
		t.Fatal("expected error for unset env var, got nil")
	}
}

func TestInterpolateCmd(t *testing.T) {
	Got, Err := interpolate(context.Background(), "Bearer {cmd:printf secret}", nil)
	if Err != nil {
		t.Fatalf("interpolate cmd: %v", Err)
	}
	if Got != "Bearer secret" {
		t.Errorf("got %q, want %q", Got, "Bearer secret")
	}

	// Trailing whitespace/newline is trimmed.
	Got, Err = interpolate(context.Background(), "{cmd:echo trimmed}", nil)
	if Err != nil {
		t.Fatalf("interpolate cmd trim: %v", Err)
	}
	if Got != "trimmed" {
		t.Errorf("got %q, want %q", Got, "trimmed")
	}

	// Whitespace around the command body is trimmed.
	Got, Err = interpolate(context.Background(), "{cmd: printf spaced }", nil)
	if Err != nil {
		t.Fatalf("interpolate cmd spaced: %v", Err)
	}
	if Got != "spaced" {
		t.Errorf("got %q, want %q", Got, "spaced")
	}
}

func TestInterpolateCmdFailure(t *testing.T) {
	if _, Err := interpolate(context.Background(), "{cmd:exit 1}", nil); Err == nil {
		t.Fatal("expected error for failing command, got nil")
	}
}

func TestInterpolateCmdSeesEnv(t *testing.T) {
	Got, Err := interpolate(context.Background(), "{cmd:printf %s \"$SECRET\"}", []string{"SECRET=fromenv"})
	if Err != nil {
		t.Fatalf("interpolate cmd env: %v", Err)
	}
	if Got != "fromenv" {
		t.Errorf("got %q, want %q", Got, "fromenv")
	}
}

// TestTwoPhaseResolution mirrors transportFor's ordering: an environment value
// computed by {cmd:...} is resolved first, then a header {env:...} reads it.
func TestTwoPhaseResolution(t *testing.T) {
	Ctx := context.Background()

	ResolvedEnv, Err := interpolateMap(Ctx, map[string]string{"TOKEN": "{cmd:printf computed}"}, nil)
	if Err != nil {
		t.Fatalf("phase 1: %v", Err)
	}
	if ResolvedEnv["TOKEN"] != "computed" {
		t.Fatalf("phase 1 TOKEN = %q, want computed", ResolvedEnv["TOKEN"])
	}

	Merged := mergeEnv(ResolvedEnv)
	Header, Err := interpolate(Ctx, "Bearer {env:TOKEN}", Merged)
	if Err != nil {
		t.Fatalf("phase 2: %v", Err)
	}
	if Header != "Bearer computed" {
		t.Errorf("header = %q, want %q", Header, "Bearer computed")
	}
}
