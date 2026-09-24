package provider

import (
	"path/filepath"
	"strings"
)

// SetEnv returns a copy of env with key set to value, replacing any existing
// entry. The input slice is never modified, so callers can reuse a base
// environment across launches.
func SetEnv(env []string, key, value string) []string {
	out := UnsetEnv(env, key)
	return append(out, key+"="+value)
}

// UnsetEnv returns a copy of env without key.
func UnsetEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return out
}

// HomeEnv applies a provider's "which config dir" variable (e.g.
// CLAUDE_CONFIG_DIR, CODEX_HOME): set to home when home differs from the
// provider's default, and actively removed otherwise.
//
// Why remove rather than leave alone: a shell may have the variable exported
// from an earlier session, and for Claude Code even setting it to the default
// dir switches the keychain service name and makes the account look logged
// out. The default home must therefore run with the variable absent.
func HomeEnv(env []string, key, home, defaultHome string) []string {
	if filepath.Clean(home) == filepath.Clean(defaultHome) {
		return UnsetEnv(env, key)
	}
	return SetEnv(env, key, home)
}
