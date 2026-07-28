package proxy

import "os"

// mergeEnv returns the parent process environment with Overrides applied on
// top. Inheriting the parent env is essential: fronted servers rely on PATH (to
// find the binaries they launch) and on ambient auth (tokens, cookies, and
// other credentials) that live in the environment mcp-gateway was launched with.
func mergeEnv(overrides map[string]string) []string {
	if len(overrides) == 0 {
		return os.Environ()
	}

	env := os.Environ()
	for key, val := range overrides {
		env = append(env, key+"="+val)
	}
	return env
}
