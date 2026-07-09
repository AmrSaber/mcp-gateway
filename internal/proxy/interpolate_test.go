package proxy

import (
	"context"
	"testing"
)

func TestInterpolateEnv(t *testing.T) {
	env := []string{"TOKEN=abc", "TOKEN=xyz", "EMPTY="}

	cases := []struct {
		In   string
		Want string
	}{
		{"Bearer {env:TOKEN}", "Bearer xyz"}, // last-wins (map over process)
		{"{env:EMPTY}", ""},
		{"no directive here", "no directive here"},
		{"{env:TOKEN}-{env:TOKEN}", "xyz-xyz"}, // multiple directives
		{"{notadirective}", "{notadirective}"}, // unknown → literal
		{"prefix {env:TOKEN} suffix", "prefix xyz suffix"},
		{"{env: TOKEN}", "xyz"},  // whitespace around value trimmed
		{"{env:TOKEN }", "xyz"},  // trailing whitespace trimmed
		{"{ env:TOKEN }", "xyz"}, // whitespace around directive trimmed
	}

	for _, c := range cases {
		got, err := interpolate(context.Background(), c.In, env)
		if err != nil {
			t.Fatalf("interpolate(%q): %v", c.In, err)
		}
		if got != c.Want {
			t.Errorf("interpolate(%q) = %q, want %q", c.In, got, c.Want)
		}
	}
}

func TestInterpolateEnvUnset(t *testing.T) {
	if _, err := interpolate(context.Background(), "{env:MISSING}", nil); err == nil {
		t.Fatal("expected error for unset env var, got nil")
	}
}

func TestInterpolateCmd(t *testing.T) {
	got, err := interpolate(context.Background(), "Bearer {cmd:printf secret}", nil)
	if err != nil {
		t.Fatalf("interpolate cmd: %v", err)
	}
	if got != "Bearer secret" {
		t.Errorf("got %q, want %q", got, "Bearer secret")
	}

	// Trailing whitespace/newline is trimmed.
	got, err = interpolate(context.Background(), "{cmd:echo trimmed}", nil)
	if err != nil {
		t.Fatalf("interpolate cmd trim: %v", err)
	}
	if got != "trimmed" {
		t.Errorf("got %q, want %q", got, "trimmed")
	}

	// Whitespace around the command body is trimmed.
	got, err = interpolate(context.Background(), "{cmd: printf spaced }", nil)
	if err != nil {
		t.Fatalf("interpolate cmd spaced: %v", err)
	}
	if got != "spaced" {
		t.Errorf("got %q, want %q", got, "spaced")
	}
}

func TestInterpolateCmdFailure(t *testing.T) {
	if _, err := interpolate(context.Background(), "{cmd:exit 1}", nil); err == nil {
		t.Fatal("expected error for failing command, got nil")
	}
}

func TestInterpolateCmdSeesEnv(t *testing.T) {
	got, err := interpolate(context.Background(), "{cmd:printf %s \"$SECRET\"}", []string{"SECRET=fromenv"})
	if err != nil {
		t.Fatalf("interpolate cmd env: %v", err)
	}
	if got != "fromenv" {
		t.Errorf("got %q, want %q", got, "fromenv")
	}
}

// TestTwoPhaseResolution mirrors transportFor's ordering: an environment value
// computed by {cmd:...} is resolved first, then a header {env:...} reads it.
func TestTwoPhaseResolution(t *testing.T) {
	ctx := context.Background()

	resolvedEnv, err := interpolateMap(ctx, map[string]string{"TOKEN": "{cmd:printf computed}"}, nil)
	if err != nil {
		t.Fatalf("phase 1: %v", err)
	}
	if resolvedEnv["TOKEN"] != "computed" {
		t.Fatalf("phase 1 TOKEN = %q, want computed", resolvedEnv["TOKEN"])
	}

	merged := mergeEnv(resolvedEnv)
	header, err := interpolate(ctx, "Bearer {env:TOKEN}", merged)
	if err != nil {
		t.Fatalf("phase 2: %v", err)
	}
	if header != "Bearer computed" {
		t.Errorf("header = %q, want %q", header, "Bearer computed")
	}
}
