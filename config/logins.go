package config

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// ErrAutoLogin refuses "auto" everywhere except the --login flag of a
// launch: auto probes every login, which spends a few tokens of each
// account's quota (logins spec §14 D14), so it must never become a silent
// default in config.yaml, AGX_LOGIN or a provider default.
var ErrAutoLogin = errors.New(`"auto" is accepted only as --login on run, new, resume and ws (it probes every login, spending a few tokens of each account's quota)`)

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

// checkProfileLogin validates profiles[idx]'s own `login:` and returns a
// complete ErrInvalid error that names the field once:
// "profiles[idx] (name): login: …". login.Compatible's errors already start
// with "login:" (the login package's prefix), so they get only the profile;
// adding the field as well once printed "login: login: …".
func checkProfileLogin(idx int, p provider.Profile, byID map[provider.ID]provider.Provider, logins []login.Login) error {
	if p.Login == "" || p.Login == login.Home {
		return nil
	}
	prof := fmt.Sprintf("profiles[%d] (%s)", idx, p.Name)
	field := prof + ": " + keyLogin
	if err := checkLoginRef(field, p.Login, p.Provider, logins); err != nil {
		return err
	}
	ll, ok := byID[p.Provider].(provider.LoginLauncher)
	if !ok {
		return fmt.Errorf("%w: %s: %s", ErrInvalid, field, NoLoginsReason(p.Provider))
	}
	l, _ := loginNamed(logins, p.Login) // present: checkLoginRef passed
	if err := login.Compatible(p, ll, l.Kind); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalid, prof, err)
	}
	return nil
}

// loginNamed returns the login called name from logins.
func loginNamed(logins []login.Login, name string) (login.Login, bool) {
	i := slices.IndexFunc(logins, func(l login.Login) bool { return l.Name == name })
	if i < 0 {
		return login.Login{}, false
	}
	return logins[i], true
}

// LoginByName returns the login named name.
func (c Config) LoginByName(name string) (login.Login, bool) {
	return loginNamed(c.Logins, name)
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
