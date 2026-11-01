package codex

import (
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// TestHomeSetup: Codex shares nothing (no file has been checked as safe to
// link), and its login command selects the home through CODEX_HOME, absent
// for the default home.
func TestHomeSetup(t *testing.T) {
	t.Parallel()
	p := New("/home/me")
	if got := p.SharedHomeFiles(); got != nil {
		t.Errorf("shared = %v, want nil", got)
	}
	for home, want := range map[string]string{
		"/home/me/.codex-client": "/home/me/.codex-client",
		"/home/me/.codex":        "",
		"/home/me/.codex/":       "",
	} {
		cmd := p.LoginCommand(provider.Profile{Name: "c", Provider: provider.Codex, Home: home}, []string{"PATH=/bin", HomeEnv + "=/stale"})
		if cmd.Path != Binary || !slices.Equal(cmd.Args, []string{"login"}) {
			t.Errorf("%s: command = %s %v", home, cmd.Path, cmd.Args)
		}
		got := ""
		for _, kv := range cmd.Env {
			if v, ok := strings.CutPrefix(kv, HomeEnv+"="); ok {
				got = v
			}
		}
		if got != want {
			t.Errorf("%s: %s = %q, want %q", home, HomeEnv, got, want)
		}
	}
}
