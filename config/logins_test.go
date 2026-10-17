package config

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// fakeSecret is a token-shaped value; validation must never echo it.
const fakeSecret = "sk-ant-oat01-FAKE-config-test-0000"

// loginProvider is a fakeProvider that can run on logins, shaped like
// Claude: oauth-token (default) and api-key, each setting one variable and
// removing the other two.
type loginProvider struct{ fakeProvider }

func (loginProvider) LoginKinds() []provider.LoginKind {
	return []provider.LoginKind{provider.LoginOAuthToken, provider.LoginAPIKey}
}

func (loginProvider) LoginEnv(kind provider.LoginKind, v string) (map[string]string, []string) {
	if kind == provider.LoginAPIKey {
		return map[string]string{"ANTHROPIC_API_KEY": v}, []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_AUTH_TOKEN"}
	}
	return map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": v}, []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}
}

// loginProviders is providers() with Claude able to run on logins; Codex
// still cannot, as in the real build.
func loginProviders() []provider.Provider {
	ps := providers()
	ps[0] = loginProvider{ps[0].(fakeProvider)}
	return ps
}

func resolveLoginsYAML(yaml string) (Config, error) {
	f, err := Decode([]byte(yaml))
	if err != nil {
		return Config{}, err
	}
	return Resolve(f, userHome, loginProviders())
}

// TestResolve_Logins_Valid covers each accepted shape and the defaults.
func TestResolve_Logins_Valid(t *testing.T) {
	t.Parallel()
	cfg, err := resolveLoginsYAML(`
logins:
  - {name: personal, token: stored, account: me@example.com, added: 2026-10-16}
  - {name: work, provider: claude, kind: oauth-token, token_cmd: "gopass show -o x | head -1"}
  - {name: ci, kind: api-key, token_env: CI_KEY}
providers:
  claude: {login: work}
profiles:
  - {name: personal, provider: claude, home: ~/.claude}
  - {name: work, provider: claude, home: ~/.claude-work, login: personal}
  - {name: pinned, provider: claude, home: ~/.claude-x, login: home}
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []login.Login{
		{Name: "personal", Provider: provider.Claude, Kind: provider.LoginOAuthToken, Source: login.SourceStored, Account: "me@example.com"},
		{Name: "work", Provider: provider.Claude, Kind: provider.LoginOAuthToken, Source: login.SourceCmd, Cmd: "gopass show -o x | head -1"},
		{Name: "ci", Provider: provider.Claude, Kind: provider.LoginAPIKey, Source: login.SourceEnv, Env: "CI_KEY"},
	}
	for i, w := range want {
		g := cfg.Logins[i]
		g.Added = w.Added
		if g != w {
			t.Errorf("logins[%d] = %+v, want %+v", i, g, w)
		}
	}
	if cfg.Logins[0].Added.Format("2006-01-02") != "2026-10-16" || !cfg.Logins[1].Added.IsZero() {
		t.Errorf("added dates: %v %v", cfg.Logins[0].Added, cfg.Logins[1].Added)
	}
	if cfg.ProviderLogins[provider.Claude] != "work" {
		t.Errorf("provider logins = %v", cfg.ProviderLogins)
	}
	if p, _ := cfg.ByName("work"); p.Login != "personal" {
		t.Errorf("work.login = %q", p.Login)
	}
	if l, ok := cfg.LoginByName("ci"); !ok || l.Env != "CI_KEY" {
		t.Errorf("LoginByName(ci) = %+v %v", l, ok)
	}
	if _, ok := cfg.LoginByName("nosuch"); ok {
		t.Error("LoginByName(nosuch) found something")
	}
	if got := cfg.UsedBy("personal"); !slices.Equal(got, []string{"work"}) {
		t.Errorf("UsedBy(personal) = %v", got)
	}
	if got := cfg.UsedBy("work"); !slices.Equal(got, []string{"providers.claude"}) {
		t.Errorf("UsedBy(work) = %v", got)
	}
	if got := cfg.UsedBy("ci"); len(got) != 0 {
		t.Errorf("UsedBy(ci) = %v", got)
	}

	// `login: home` on a provider is the same as no default.
	cfg, err = resolveLoginsYAML("providers:\n  claude: {login: home}\n")
	if err != nil || len(cfg.ProviderLogins) != 0 {
		t.Errorf("providers.claude.login: home → %v %v", cfg.ProviderLogins, err)
	}
}

// TestResolve_Logins_Invalid walks every §6 rule and the REPORT deltas. Each
// error must be ErrInvalid, name the field and never echo a token.
func TestResolve_Logins_Invalid(t *testing.T) {
	t.Parallel()
	const p = "profiles:\n  - {name: personal, provider: claude, home: ~/.claude}\n"
	for _, c := range []struct {
		name, yaml, want string
	}{
		// Names.
		{"name-empty", "logins: [{token: stored}]", "logins[0]: name must match"},
		{"name-upper", "logins: [{name: Personal, token: stored}]", "name must match"},
		{"name-leading-dash", "logins: [{name: -x, token: stored}]", "name must match"},
		{"name-space", "logins: [{name: 'a b', token: stored}]", "name must match"},
		{"name-unicode", "logins: [{name: wörk, token: stored}]", "name must match"},
		{"name-home", "logins: [{name: home, token: stored}]", `"home" is reserved`},
		{"name-auto", "logins: [{name: auto, token: stored}]", `"auto" is reserved`},
		{"name-duplicate", "logins: [{name: a, token: stored}, {name: a, token_env: X}]", "logins[1] (a): duplicate name"},
		// Provider and kind.
		{"provider-unknown", "logins: [{name: a, provider: gemini, token: stored}]", `unknown provider "gemini"`},
		{"provider-no-logins", "logins: [{name: a, provider: codex, token: stored}]", "codex has no token logins yet"},
		{"kind-unknown", "logins: [{name: a, kind: password, token: stored}]", `kind: "password" is not supported`},
		// Sources.
		{"no-source", "logins: [{name: a}]", "set exactly one of"},
		{"stored-and-cmd", "logins: [{name: a, token: stored, token_cmd: x}]", "set exactly one of"},
		{"cmd-and-env", "logins: [{name: a, token_cmd: x, token_env: Y}]", "set exactly one of"},
		{"all-three", "logins: [{name: a, token: stored, token_cmd: x, token_env: Y}]", "set exactly one of"},
		{"raw-token", "logins: [{name: a, token: " + fakeSecret + "}]", "a raw token does not belong in config.yaml"},
		{"raw-token-and-cmd", "logins: [{name: a, token: " + fakeSecret + ", token_cmd: x}]", "a raw token does not belong in config.yaml"},
		{"raw-token-Stored", "logins: [{name: a, token: Stored}]", "a raw token does not belong in config.yaml"},
		{"added-bad", "logins: [{name: a, token: stored, added: 16/10/2026}]", "added: want YYYY-MM-DD"},
		// References.
		{"profile-unknown-login", "logins: [{name: a, token: stored}]\nprofiles: [{name: w, provider: claude, home: ~/.w, login: nosuch}]", `profiles[0] (w): login: unknown login "nosuch"`},
		{"profile-auto", "profiles: [{name: w, provider: claude, home: ~/.w, login: auto}]", `"auto" is accepted only as --login`},
		{"profile-wrong-provider", "logins: [{name: a, token: stored}]\nprofiles: [{name: c, provider: codex, home: ~/.codex, login: a}]", `"a" is a claude login, not codex`},
		{"profile-api-billed", "logins: [{name: a, token: stored}]\nprofiles: [{name: k, provider: claude, home: ~/.k, billing: api, login: a}]", "billed per token"},
		{"profile-own-env-set", "logins: [{name: a, token: stored}]\nprofiles: [{name: k, provider: claude, home: ~/.k, env: {CLAUDE_CODE_OAUTH_TOKEN: x}, login: a}]", "sets CLAUDE_CODE_OAUTH_TOKEN in env"},
		{"profile-own-env-unset", "logins: [{name: a, token: stored}]\nprofiles: [{name: k, provider: claude, home: ~/.k, env: {ANTHROPIC_AUTH_TOKEN: x}, login: a}]", "sets ANTHROPIC_AUTH_TOKEN in env"},
		{"profile-own-secret", "logins: [{name: a, kind: api-key, token: stored}]\nprofiles: [{name: k, provider: claude, home: ~/.k, secrets: {ANTHROPIC_API_KEY: 'gopass:x'}, login: a}]", "sets ANTHROPIC_API_KEY in secrets"},
		{"provider-unknown-login", "providers:\n  claude: {login: nosuch}\n", `providers.claude.login: unknown login "nosuch"`},
		{"provider-auto", "providers:\n  claude: {login: auto}\n", `providers.claude.login: "auto" is accepted only as --login`},
		{"provider-wrong-provider", "logins: [{name: a, token: stored}]\nproviders:\n  codex: {login: a}\n", `providers.codex.login: login "a" is a claude login, not codex`},
		{"unknown-key", "logins: [{name: a, token: stored, value: x}]", "value"},
	} {
		y := c.yaml
		if !strings.Contains(y, "profiles:") {
			y += "\n" + p
		}
		_, err := resolveLoginsYAML(y)
		if err == nil {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
		if !errors.Is(err, ErrInvalid) && c.name != "unknown-key" {
			t.Errorf("%s: %v is not ErrInvalid", c.name, err)
		}
		if strings.Contains(err.Error(), fakeSecret) {
			t.Errorf("%s: error echoes the token: %v", c.name, err)
		}
	}
}

func TestKindForAndValidLoginName(t *testing.T) {
	t.Parallel()
	ll := loginProvider{}
	for raw, want := range map[string]provider.LoginKind{"": provider.LoginOAuthToken, "oauth-token": provider.LoginOAuthToken, "api-key": provider.LoginAPIKey} {
		if got, err := KindFor(ll, raw); err != nil || got != want {
			t.Errorf("KindFor(%q) = %v %v", raw, got, err)
		}
	}
	for _, bad := range []string{"OAUTH-TOKEN", "apikey", " api-key"} {
		if _, err := KindFor(ll, bad); err == nil {
			t.Errorf("KindFor(%q) accepted", bad)
		}
	}
	for name, ok := range map[string]bool{"a": true, "work-2": true, "0": true, "": false, "home": false, "auto": false, "A": false, "-a": false, "a_b": false, "a.b": false} {
		if err := ValidLoginName(name); (err == nil) != ok {
			t.Errorf("ValidLoginName(%q) = %v", name, err)
		}
	}
	if !strings.Contains(NoLoginsReason(provider.Codex), "spec §12 V5") || !strings.Contains(NoLoginsReason("gemini"), "gemini does not support") {
		t.Error("NoLoginsReason wording")
	}
}
