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
func interpolate(ctx context.Context, value string, env []string) (string, error) {
	var out strings.Builder

	for len(value) > 0 {
		open := strings.Index(value, "{")
		if open == -1 {
			out.WriteString(value)
			break
		}
		out.WriteString(value[:open])
		value = value[open:]

		close := strings.Index(value, "}")
		if close == -1 {
			// Unterminated brace — treat the rest as literal.
			out.WriteString(value)
			break
		}
		directive := value[1:close]
		value = value[close+1:]

		resolved, ok, err := resolveDirective(ctx, directive, env)
		if err != nil {
			return "", err
		}
		if !ok {
			// Not a recognised directive — keep the braces verbatim.
			out.WriteString("{" + directive + "}")
			continue
		}
		out.WriteString(resolved)
	}

	return out.String(), nil
}

// resolveDirective resolves a single {env:...}/{cmd:...} body. Ok is false when
// the body is not a recognised directive (caller keeps it literal). Surrounding
// whitespace is trimmed, so {env: NAME} and {env:NAME} are equivalent.
func resolveDirective(ctx context.Context, directive string, env []string) (string, bool, error) {
	directive = strings.TrimSpace(directive)

	switch {
	case strings.HasPrefix(directive, "env:"):
		name := strings.TrimSpace(directive[len("env:"):])
		if val, found := lookupEnv(env, name); found {
			return val, true, nil
		}
		return "", true, fmt.Errorf("env var %q is not set", name)

	case strings.HasPrefix(directive, "cmd:"):
		body := strings.TrimSpace(directive[len("cmd:"):])
		cmd := exec.CommandContext(ctx, "sh", "-c", body)
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			return "", true, fmt.Errorf("running {cmd:%s}: %w", body, err)
		}
		return strings.TrimSpace(string(out)), true, nil

	default:
		return "", false, nil
	}
}

// lookupEnv finds Name in a process-style KEY=VALUE slice, scanning in reverse
// so the last entry wins — matching exec's duplicate-key semantics, which is how
// the environment map overrides the inherited process env (see mergeEnv).
func lookupEnv(env []string, name string) (string, bool) {
	prefix := name + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}

// interpolateSlice interpolates every element of a slice (e.g. command args).
func interpolateSlice(ctx context.Context, values []string, env []string) ([]string, error) {
	if len(values) == 0 {
		return values, nil
	}
	out := make([]string, len(values))
	for i, v := range values {
		resolved, err := interpolate(ctx, v, env)
		if err != nil {
			return nil, err
		}
		out[i] = resolved
	}
	return out, nil
}

// interpolateMap interpolates every value of a map (e.g. headers, environment
// values), leaving keys untouched.
func interpolateMap(ctx context.Context, values map[string]string, env []string) (map[string]string, error) {
	if len(values) == 0 {
		return values, nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		resolved, err := interpolate(ctx, v, env)
		if err != nil {
			return nil, err
		}
		out[k] = resolved
	}
	return out, nil
}
