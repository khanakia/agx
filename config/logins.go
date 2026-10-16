package config

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// ErrAutoLogin refuses `--login auto` / `login: auto` until token usage is
// verified readable; picking blind would spend the wrong account.
var ErrAutoLogin = errors.New("--login auto is not available yet: token logins cannot be checked for usage until verified (spec §12 V2)")

// NoLoginsReason explains why a provider without provider.LoginLauncher
// cannot have logins. Codex is the case today (logins spec §10).
func NoLoginsReason(id provider.ID) string {
	if id == provider.Codex {
		return "codex has no token logins yet: a ChatGPT-plan login exists only as refreshable tokens in <home>/auth.json (copying it risks logging the CLI out), and the API-key variable interactive Codex honours is unverified (spec §12 V5)"
	}
	return string(id) + " does not support token logins"
}

// resolveLogins validates every LoginSpec (logins spec §6).
func resolveLogins(specs []LoginSpec, byID map[provider.ID]provider.Provider) ([]login.Login, error) {
	out := make([]login.Login, 0, len(specs))
	seen := map[string]bool{}
	for i, s := range specs {
		where := fmt.Sprintf("logins[%d]", i)
		if s.Name != "" {
			where += " (" + s.Name + ")"
		}
		if !namePattern.MatchString(s.Name) {
			return nil, fmt.Errorf("%w: %s: name must match %s", ErrInvalid, where, namePattern)
		}
		if s.Name == login.Home || s.Name == login.Auto {
			return nil, fmt.Errorf("%w: %s: %q is reserved", ErrInvalid, where, s.Name)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("%w: %s: duplicate name", ErrInvalid, where)
		}
		seen[s.Name] = true
		l, err := resolveLogin(s, byID)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, where, err)
		}
		out = append(out, l)
	}
	return out, nil
}

func resolveLogin(s LoginSpec, byID map[provider.ID]provider.Provider) (login.Login, error) {
	id := provider.ID(s.Provider)
	if id == "" {
		id = provider.Claude
	}
	prov, ok := byID[id]
	if !ok {
		return login.Login{}, fmt.Errorf("provider: unknown provider %q (known: %s)", id, knownIDsMap(byID))
	}
	ll, ok := prov.(provider.LoginLauncher)
	if !ok {
		return login.Login{}, fmt.Errorf("provider: %s", NoLoginsReason(id))
	}
	kind, err := KindFor(ll, s.Kind)
	if err != nil {
		return login.Login{}, err
	}
	l := login.Login{Name: s.Name, Provider: id, Kind: kind, Account: s.Account}

	sources := 0
	if s.Token != "" {
		sources++
		if s.Token != login.StoredMarker {
			// Never echo s.Token: it is a secret in the wrong file.
			return login.Login{}, fmt.Errorf("token: a raw token does not belong in config.yaml (only %q is accepted) — delete this login from config.yaml and move the token to tokens.yaml: `agx login add %s --stdin`", login.StoredMarker, s.Name)
		}
		l.Source = login.SourceStored
	}
	if s.TokenCmd != "" {
		sources++
		l.Source, l.Cmd = login.SourceCmd, s.TokenCmd
	}
	if s.TokenEnv != "" {
		sources++
		l.Source, l.Env = login.SourceEnv, s.TokenEnv
	}
	if sources != 1 {
		return login.Login{}, errors.New("set exactly one of token: stored, token_cmd, token_env")
	}
	if s.Added != "" {
		t, err := time.Parse(time.DateOnly, s.Added)
		if err != nil {
			return login.Login{}, fmt.Errorf("added: want YYYY-MM-DD, got %q", s.Added)
		}
		l.Added = t
	}
	return l, nil
}

// KindFor validates raw against what ll supports; "" is the provider's
// default (first) kind.
func KindFor(ll provider.LoginLauncher, raw string) (provider.LoginKind, error) {
	kinds := ll.LoginKinds()
	if raw == "" {
		return kinds[0], nil
	}
	k := provider.LoginKind(raw)
	if !slices.Contains(kinds, k) {
		return "", fmt.Errorf("kind: %q is not supported (want one of %v)", raw, kinds)
	}
	return k, nil
}

// checkLoginRef validates a `login:` reference at where: an existing login
// of provider id. "home" is checked by the caller.
func checkLoginRef(where, name string, id provider.ID, logins []login.Login) error {
	if name == login.Auto {
		return fmt.Errorf("%w: %s: %w", ErrInvalid, where, ErrAutoLogin)
	}
	i := slices.IndexFunc(logins, func(l login.Login) bool { return l.Name == name })
	if i < 0 {
		return fmt.Errorf("%w: %s: unknown login %q (see `agx login ls`)", ErrInvalid, where, name)
	}
	if logins[i].Provider != id {
		return fmt.Errorf("%w: %s: login %q is a %s login, not %s", ErrInvalid, where, name, logins[i].Provider, id)
	}
	return nil
}

// checkProfileLogin validates a profile's own `login:`.
func checkProfileLogin(p provider.Profile, byID map[provider.ID]provider.Provider, logins []login.Login) error {
	if p.Login == "" || p.Login == login.Home {
		return nil
	}
	if p.Login == login.Auto {
		return fmt.Errorf("login: %w", ErrAutoLogin)
	}
	i := slices.IndexFunc(logins, func(l login.Login) bool { return l.Name == p.Login })
	if i < 0 {
		return fmt.Errorf("login: unknown login %q (see `agx login ls`)", p.Login)
	}
	l := logins[i]
	if l.Provider != p.Provider {
		return fmt.Errorf("login: %q is a %s login, not %s", p.Login, l.Provider, p.Provider)
	}
	ll, ok := byID[p.Provider].(provider.LoginLauncher)
	if !ok {
		return fmt.Errorf("login: %s", NoLoginsReason(p.Provider))
	}
	if err := login.Compatible(p, ll, l.Kind); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}

// LoginByName returns the login named name.
func (c Config) LoginByName(name string) (login.Login, bool) {
	i := slices.IndexFunc(c.Logins, func(l login.Login) bool { return l.Name == name })
	if i < 0 {
		return login.Login{}, false
	}
	return c.Logins[i], true
}

// UsedBy lists what points at login name: profile names, then
// "providers.<id>" for a provider default — what breaks if it is removed.
func (c Config) UsedBy(name string) []string {
	var out []string
	for _, p := range c.Profiles {
		if p.Login == name {
			out = append(out, p.Name)
		}
	}
	ids := make([]string, 0, len(c.ProviderLogins))
	for id, l := range c.ProviderLogins {
		if l == name {
			ids = append(ids, "providers."+string(id))
		}
	}
	slices.Sort(ids)
	return append(out, ids...)
}

// ValidLoginName checks a new login's name: the profile-name pattern and
// not a reserved word.
func ValidLoginName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("login name must match %s", namePattern)
	}
	if name == login.Home || name == login.Auto {
		return fmt.Errorf("login name %q is reserved", name)
	}
	return nil
}
