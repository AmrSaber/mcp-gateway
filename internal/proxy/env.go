package proxy

import "os"

// mergeEnv returns the parent process environment with Overrides applied on
// top. Inheriting the parent env is essential: gated servers rely on PATH (to
// find the binaries they launch) and on ambient auth (tokens, cookies, and
// other credentials) that live in the environment lazy-mcp was launched with.
func mergeEnv(Overrides map[string]string) []string {
	if len(Overrides) == 0 {
		return os.Environ()
	}

	Env := os.Environ()
	for Key, Val := range Overrides {
		Env = append(Env, Key+"="+Val)
	}
	return Env
}
