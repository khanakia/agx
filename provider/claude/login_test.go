package claude

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// credentialVars are every variable Claude Code reads a credential from.
var credentialVars = []string{OAuthTokenEnv, APIKeyEnv, AuthTokenEnv}

// envValue returns key's value in env and whether it is present.
func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// TestLoginEnv_SetsOneRemovesTheRest: per kind, exactly one credential
// variable is set and the other two are removed, so a leftover API key or
// auth token can never take precedence over the login (spec §7).
func TestLoginEnv_SetsOneRemovesTheRest(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	want := map[provider.LoginKind]string{provider.LoginOAuthToken: OAuthTokenEnv, provider.LoginAPIKey: APIKeyEnv}
	if got := p.LoginKinds(); !slices.Equal(got, []provider.LoginKind{provider.LoginOAuthToken, provider.LoginAPIKey}) {
		t.Errorf("LoginKinds = %v (oauth-token must stay first: it is the default)", got)
	}
	for kind, setVar := range want {
		set, unset := p.LoginEnv(kind, "v")
		if !maps.Equal(set, map[string]string{setVar: "v"}) {
			t.Errorf("%s: set = %v", kind, set)
		}
		var others []string
		for _, v := range credentialVars {
			if v != setVar {
				others = append(others, v)
			}
		}
		slices.Sort(unset)
		slices.Sort(others)
		if !slices.Equal(unset, others) {
			t.Errorf("%s: unset = %v, want %v", kind, unset, others)
		}
		// An empty value still names every variable (conflict checks and
		// --dry-run rely on it).
		if set, unset := p.LoginEnv(kind, ""); len(set) != 1 || len(unset) != 2 {
			t.Errorf("%s with no value: %v %v", kind, set, unset)
		}
	}
	if set, unset := p.LoginEnv("password", "v"); set != nil || unset != nil {
		t.Errorf("unknown kind: %v %v", set, unset)
	}
	if p.LoginLifetime(provider.LoginOAuthToken) != SetupTokenLifetime || p.LoginLifetime(provider.LoginAPIKey) != 0 {
		t.Error("lifetimes changed")
	}
}

// TestLaunchWithLogin_Matrix: home (default, other) × kind × inherited
// credential variables. The login must not change CLAUDE_CONFIG_DIR
// handling (default home → variable absent, even when inherited), and
// every inherited credential variable but the login's own is gone.
func TestLaunchWithLogin_Matrix(t *testing.T) {
	t.Parallel()
	p := &Provider{UserHome: "/u"}
	inherited := []string{"PATH=/bin", ConfigDirEnv + "=/stale", OAuthTokenEnv + "=old-oauth", APIKeyEnv + "=old-key", AuthTokenEnv + "=old-auth"}
	for _, home := range []string{"/u/.claude", "/u/.claude-work"} {
		for _, kind := range provider.LoginKindValues {
			base := slices.Clone(inherited)
			cmd, err := p.Launch(provider.Profile{Name: "x", Home: home}, provider.LaunchRequest{Env: base})
			if err != nil {
				t.Fatal(err)
			}
			set, unset := p.LoginEnv(kind, "new-token")
			env := provider.ApplyLoginEnv(cmd.Env, set, unset)
			if !slices.Equal(base, inherited) {
				t.Errorf("Launch/ApplyLoginEnv modified the caller's env")
			}
			dir, has := envValue(env, ConfigDirEnv)
			if home == "/u/.claude" && has {
				t.Errorf("%s/%s: default home must leave %s absent, got %q", home, kind, ConfigDirEnv, dir)
			}
			if home != "/u/.claude" && dir != home {
				t.Errorf("%s/%s: %s = %q", home, kind, ConfigDirEnv, dir)
			}
			for _, v := range credentialVars {
				got, ok := envValue(env, v)
				if _, isSet := set[v]; isSet {
					if got != "new-token" {
						t.Errorf("%s/%s: %s = %q, want the login's token", home, kind, v, got)
					}
				} else if ok {
					t.Errorf("%s/%s: %s survived (%q)", home, kind, v, got)
				}
			}
			if v, _ := envValue(env, "PATH"); v != "/bin" {
				t.Errorf("%s/%s: unrelated variable lost", home, kind)
			}
		}
	}
}
