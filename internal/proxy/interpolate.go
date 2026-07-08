package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// interpolate expands {env:NAME} and {cmd:...} directives in a config value,
// resolving against Env (a process-style KEY=VALUE slice). Literal text passes
// through untouched, and a value may contain multiple directives.
//
//   - {env:NAME} is replaced with NAME's value in Env; an unset NAME is an error
//     (fail fast rather than silently emit an empty secret).
//   - {cmd:...} runs the body via `sh -c` with Env as its environment and is
//     replaced with the command's trimmed stdout; a non-zero exit is an error.
//
// This is the seam that keeps secret stores out of lazy-mcp: the author writes
// e.g. {cmd:kv get github:pat}, and lazy-mcp only knows "run this and use the
// output" — never anything about kv itself.
func interpolate(Ctx context.Context, Value string, Env []string) (string, error) {
	var Out strings.Builder

	for len(Value) > 0 {
		Open := strings.Index(Value, "{")
		if Open == -1 {
			Out.WriteString(Value)
			break
		}
		Out.WriteString(Value[:Open])
		Value = Value[Open:]

		Close := strings.Index(Value, "}")
		if Close == -1 {
			// Unterminated brace — treat the rest as literal.
			Out.WriteString(Value)
			break
		}
		Directive := Value[1:Close]
		Value = Value[Close+1:]

		Resolved, Ok, Err := resolveDirective(Ctx, Directive, Env)
		if Err != nil {
			return "", Err
		}
		if !Ok {
			// Not a recognised directive — keep the braces verbatim.
			Out.WriteString("{" + Directive + "}")
			continue
		}
		Out.WriteString(Resolved)
	}

	return Out.String(), nil
}

// resolveDirective resolves a single {env:...}/{cmd:...} body. Ok is false when
// the body is not a recognised directive (caller keeps it literal). Surrounding
// whitespace is trimmed, so {env: NAME} and {env:NAME} are equivalent.
func resolveDirective(Ctx context.Context, Directive string, Env []string) (string, bool, error) {
	Directive = strings.TrimSpace(Directive)

	switch {
	case strings.HasPrefix(Directive, "env:"):
		Name := strings.TrimSpace(Directive[len("env:"):])
		if Val, Found := lookupEnv(Env, Name); Found {
			return Val, true, nil
		}
		return "", true, fmt.Errorf("env var %q is not set", Name)

	case strings.HasPrefix(Directive, "cmd:"):
		Body := strings.TrimSpace(Directive[len("cmd:"):])
		Cmd := exec.CommandContext(Ctx, "sh", "-c", Body)
		Cmd.Env = Env
		Out, Err := Cmd.Output()
		if Err != nil {
			return "", true, fmt.Errorf("running {cmd:%s}: %w", Body, Err)
		}
		return strings.TrimSpace(string(Out)), true, nil

	default:
		return "", false, nil
	}
}

// lookupEnv finds Name in a process-style KEY=VALUE slice, scanning in reverse
// so the last entry wins — matching exec's duplicate-key semantics, which is how
// the environment map overrides the inherited process env (see mergeEnv).
func lookupEnv(Env []string, Name string) (string, bool) {
	Prefix := Name + "="
	for I := len(Env) - 1; I >= 0; I-- {
		if strings.HasPrefix(Env[I], Prefix) {
			return Env[I][len(Prefix):], true
		}
	}
	return "", false
}

// interpolateSlice interpolates every element of a slice (e.g. command args).
func interpolateSlice(Ctx context.Context, Values []string, Env []string) ([]string, error) {
	if len(Values) == 0 {
		return Values, nil
	}
	Out := make([]string, len(Values))
	for I, V := range Values {
		Resolved, Err := interpolate(Ctx, V, Env)
		if Err != nil {
			return nil, Err
		}
		Out[I] = Resolved
	}
	return Out, nil
}

// interpolateMap interpolates every value of a map (e.g. headers, environment
// values), leaving keys untouched.
func interpolateMap(Ctx context.Context, Values map[string]string, Env []string) (map[string]string, error) {
	if len(Values) == 0 {
		return Values, nil
	}
	Out := make(map[string]string, len(Values))
	for K, V := range Values {
		Resolved, Err := interpolate(Ctx, V, Env)
		if Err != nil {
			return nil, Err
		}
		Out[K] = Resolved
	}
	return Out, nil
}
