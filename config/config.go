// Package config loads agx's optional YAML config and merges it with what
// each provider discovers on disk, producing the final list of profiles and
// settings every command works from.
//
// The file is optional (spec §5): with no file, every provider's discovered
// profiles are used and built-in defaults apply. When a provider has at least
// one configured profile, its discovered profiles are NOT added — the config
// is then the complete, intentional list for that provider.
//
// This package knows providers only through the provider.Provider interface,
// so it never imports a concrete provider.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/secret"
)

// FileName is the config file name inside agx's config dir.
const FileName = "config.yaml"

// AutoProfile is the reserved profile name that asks for auto-pick (spec §7).
const AutoProfile = "auto"

// Defaults applied when the file leaves a setting empty.
const (
	// defaultSessionsDirName is created under the user's home when
	// sessions.root is unset. Deliberately visible (not a dotdir): session
	// folders are work the user opens.
	defaultSessionsDirName = "agx-sessions"
)

// namePattern constrains profile names: they become CLI arguments and alias
// targets, so keep them shell-safe.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// aliasPattern constrains shell alias names: they become zsh function names.
var aliasPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// aliasArgsPattern constrains what an alias runs. The value is pasted into
// generated shell code by `agx shell-init`, so anything that could break out
// of a word ($, quotes, ;, |, &, backticks, newlines) is rejected here rather
// than escaped there.
var aliasArgsPattern = regexp.MustCompile(`^[A-Za-z0-9 _./:=@+-]+$`)

// File is the on-disk schema (spec §5.2). Unknown keys are rejected.
type File struct {
	Sessions  SessionsSpec                    `yaml:"sessions"`
	Providers map[string]ProviderDefaultsSpec `yaml:"providers"`
	Profiles  []ProfileSpec                   `yaml:"profiles"`
	Shell     ShellSpec                       `yaml:"shell"`
}

// SessionsSpec configures session folders.
type SessionsSpec struct {
	Root        string `yaml:"root"`
	PromoteRoot string `yaml:"promote_root"`
}

// ProviderDefaultsSpec is merged under every profile of that provider.
type ProviderDefaultsSpec struct {
	Args []string `yaml:"args"`
}

// ProfileSpec is one configured profile.
type ProfileSpec struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	// Home defaults to the provider's default home.
	Home string `yaml:"home"`
	// Billing is "plan" (default) or "api".
	Billing string   `yaml:"billing"`
	Default bool     `yaml:"default"`
	Args    []string `yaml:"args"`
	// ArgsReplace makes Args replace, not extend, the provider defaults.
	ArgsReplace bool              `yaml:"args_replace"`
	Env         map[string]string `yaml:"env"`
	// Secrets maps env var name → "scheme:ref" (package secret).
	Secrets map[string]string `yaml:"secrets"`
}

// ShellSpec configures `agx shell-init`.
type ShellSpec struct {
	// Aliases maps a shell function name to the agx arguments it runs,
	// e.g. cl: "run -p personal".
	Aliases map[string]string `yaml:"aliases"`
}

// Alias is one resolved shell alias.
type Alias struct {
	Name string
	Args string
}

// Config is the resolved configuration.
type Config struct {
	// Path is where the file is (or would be); Found says whether it exists.
	Path  string
	Found bool
	// SessionsRoot is where `agx new` creates folders (absolute).
	SessionsRoot string
	// PromoteRoot is the default destination of `sessions promote`.
	PromoteRoot string
	// Profiles are in config order: configured providers first as written,
	// then discovered ones.
	Profiles []provider.Profile
	// Aliases are sorted by name for deterministic shell output.
	Aliases []Alias
}

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("config: invalid")

// Load reads path (a missing file is not an error) and resolves profiles
// against providers. userHome expands "~" and anchors discovery.
func Load(path, userHome string, providers []provider.Provider) (Config, error) {
	raw, err := os.ReadFile(path)
	found := true
	if errors.Is(err, os.ErrNotExist) {
		found, raw = false, nil
	} else if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	f, err := Decode(raw)
	if err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	cfg, err := Resolve(f, userHome, providers)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Path, cfg.Found = path, found
	return cfg, nil
}

// Decode parses YAML strictly. Empty input yields a zero File.
func Decode(raw []byte) (File, error) {
	var f File
	if len(bytes.TrimSpace(raw)) == 0 {
		return f, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return File{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return f, nil
}

// Resolve validates f and merges it with discovery.
func Resolve(f File, userHome string, providers []provider.Provider) (Config, error) {
	byID := make(map[provider.ID]provider.Provider, len(providers))
	for _, p := range providers {
		byID[p.ID()] = p
	}
	for id := range f.Providers {
		if _, ok := byID[provider.ID(id)]; !ok {
			return Config{}, fmt.Errorf("%w: providers.%s: unknown provider (known: %s)", ErrInvalid, id, knownIDs(providers))
		}
	}

	cfg := Config{}
	cfg.SessionsRoot = expand(f.Sessions.Root, userHome)
	if cfg.SessionsRoot == "" {
		cfg.SessionsRoot = filepath.Join(userHome, defaultSessionsDirName)
	}
	cfg.PromoteRoot = expand(f.Sessions.PromoteRoot, userHome)
	if cfg.PromoteRoot == "" {
		cfg.PromoteRoot = filepath.Dir(cfg.SessionsRoot)
	}

	seen := map[string]bool{}
	configured := map[provider.ID]bool{}
	for i, s := range f.Profiles {
		p, err := resolveProfile(i, s, f.Providers, byID, userHome)
		if err != nil {
			return Config{}, err
		}
		if seen[p.Name] {
			return Config{}, fmt.Errorf("%w: profiles[%d]: duplicate name %q", ErrInvalid, i, p.Name)
		}
		seen[p.Name] = true
		configured[p.Provider] = true
		cfg.Profiles = append(cfg.Profiles, p)
	}

	for _, prov := range providers {
		if configured[prov.ID()] {
			continue
		}
		disc, err := prov.Discover(userHome)
		if err != nil {
			return Config{}, fmt.Errorf("config: discover %s: %w", prov.ID(), err)
		}
		for _, p := range disc {
			if seen[p.Name] {
				// A discovered name colliding with a configured one (e.g. a
				// codex profile named "personal") is namespaced, not dropped.
				p.Name = string(p.Provider) + "-" + p.Name
			}
			seen[p.Name] = true
			p.Args = append(append([]string{}, f.Providers[string(prov.ID())].Args...), p.Args...)
			cfg.Profiles = append(cfg.Profiles, p)
		}
	}

	if err := settleDefaults(cfg.Profiles); err != nil {
		return Config{}, err
	}

	for name, args := range f.Shell.Aliases {
		if !aliasPattern.MatchString(name) {
			return Config{}, fmt.Errorf("%w: shell.aliases.%s: not a valid shell function name", ErrInvalid, name)
		}
		if strings.TrimSpace(args) == "" {
			return Config{}, fmt.Errorf("%w: shell.aliases.%s: empty command", ErrInvalid, name)
		}
		if !aliasArgsPattern.MatchString(args) {
			return Config{}, fmt.Errorf("%w: shell.aliases.%s: only letters, digits, spaces and _./:=@+- are allowed", ErrInvalid, name)
		}
		cfg.Aliases = append(cfg.Aliases, Alias{Name: name, Args: strings.TrimSpace(args)})
	}
	sort.Slice(cfg.Aliases, func(i, j int) bool { return cfg.Aliases[i].Name < cfg.Aliases[j].Name })
	return cfg, nil
}

// resolveProfile validates one ProfileSpec.
func resolveProfile(i int, s ProfileSpec, defaults map[string]ProviderDefaultsSpec, byID map[provider.ID]provider.Provider, userHome string) (provider.Profile, error) {
	where := fmt.Sprintf("profiles[%d]", i)
	if s.Name != "" {
		where += " (" + s.Name + ")"
	}
	if !namePattern.MatchString(s.Name) {
		return provider.Profile{}, fmt.Errorf("%w: %s: name must match %s", ErrInvalid, where, namePattern)
	}
	if s.Name == AutoProfile {
		return provider.Profile{}, fmt.Errorf("%w: %s: %q is reserved for auto-pick", ErrInvalid, where, AutoProfile)
	}
	prov, ok := byID[provider.ID(s.Provider)]
	if !ok {
		return provider.Profile{}, fmt.Errorf("%w: %s: unknown provider %q (known: %s)", ErrInvalid, where, s.Provider, knownIDsMap(byID))
	}
	billing := provider.Billing(s.Billing)
	if billing == "" {
		billing = provider.BillingPlan
	}
	if !validBilling(billing) {
		return provider.Profile{}, fmt.Errorf("%w: %s: billing must be one of %v", ErrInvalid, where, provider.BillingValues)
	}
	for envName, ref := range s.Secrets {
		if _, err := secret.Parse(ref); err != nil {
			return provider.Profile{}, fmt.Errorf("%w: %s: secrets.%s: %w", ErrInvalid, where, envName, err)
		}
		if _, clash := s.Env[envName]; clash {
			return provider.Profile{}, fmt.Errorf("%w: %s: %s is set in both env and secrets", ErrInvalid, where, envName)
		}
	}
	home := expand(s.Home, userHome)
	if home == "" {
		home = prov.DefaultHome(userHome)
	}
	var args []string
	if !s.ArgsReplace {
		args = append(args, defaults[s.Provider].Args...)
	}
	args = append(args, s.Args...)

	return provider.Profile{
		Name: s.Name, Provider: prov.ID(), Home: home, Billing: billing,
		Args: args, Env: s.Env, Secrets: s.Secrets, Default: s.Default,
		Source: provider.SourceConfig,
	}, nil
}

// settleDefaults ensures exactly one default per provider: an explicit
// default wins (two explicit defaults is an error); otherwise the first
// profile of that provider becomes the default.
func settleDefaults(ps []provider.Profile) error {
	explicit := map[provider.ID]string{}
	for _, p := range ps {
		if !p.Default || p.Source != provider.SourceConfig {
			continue
		}
		if other, dup := explicit[p.Provider]; dup {
			return fmt.Errorf("%w: profiles %q and %q are both default for %s", ErrInvalid, other, p.Name, p.Provider)
		}
		explicit[p.Provider] = p.Name
	}
	chosen := map[provider.ID]bool{}
	for i := range ps {
		id := ps[i].Provider
		if name, ok := explicit[id]; ok {
			ps[i].Default = ps[i].Name == name
			continue
		}
		// No explicit default: keep a discovered default flag if present,
		// else the first profile of the provider.
		ps[i].Default = !chosen[id] && (ps[i].Default || firstOf(ps, id) == i)
		if ps[i].Default {
			chosen[id] = true
		}
	}
	return nil
}

// firstOf returns the index of the first profile of provider id, or -1.
func firstOf(ps []provider.Profile, id provider.ID) int {
	for i, p := range ps {
		if p.Provider == id {
			return i
		}
	}
	return -1
}

func validBilling(b provider.Billing) bool {
	for _, v := range provider.BillingValues {
		if b == v {
			return true
		}
	}
	return false
}

// expand resolves a leading "~" and makes the path absolute and clean.
// Empty stays empty.
func expand(p, userHome string) string {
	switch {
	case p == "":
		return ""
	case p == "~":
		p = userHome
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(userHome, p[2:])
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

func knownIDs(ps []provider.Provider) string {
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, string(p.ID()))
	}
	return strings.Join(ids, ", ")
}

func knownIDsMap(m map[provider.ID]provider.Provider) string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

// ByName returns the profile named name.
func (c Config) ByName(name string) (provider.Profile, bool) {
	for _, p := range c.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return provider.Profile{}, false
}

// Default returns the default profile of provider id.
func (c Config) Default(id provider.ID) (provider.Profile, bool) {
	for _, p := range c.Profiles {
		if p.Provider == id && p.Default {
			return p, true
		}
	}
	return provider.Profile{}, false
}

// ForHome returns the profile to resume a conversation stored in home:
// plan-billed before api-billed, then the provider default, then config
// order. Why plan first: a home shared by `personal` and `kimi` holds
// conversations made on either, and the subscription login is the one that
// can always open them.
func (c Config) ForHome(id provider.ID, home string) (provider.Profile, bool) {
	var best provider.Profile
	found := false
	rank := func(p provider.Profile) int {
		r := 0
		if p.Billing == provider.BillingPlan {
			r += 2
		}
		if p.Default {
			r++
		}
		return r
	}
	for _, p := range c.Profiles {
		if p.Provider != id || filepath.Clean(p.Home) != filepath.Clean(home) {
			continue
		}
		if !found || rank(p) > rank(best) {
			best, found = p, true
		}
	}
	return best, found
}

// Homes returns each distinct (provider, home) pair once, in profile order,
// represented by the profile ForHome would choose. Used wherever work is per
// login rather than per profile (usage, conversation listing).
func (c Config) Homes() []provider.Profile {
	type key struct {
		id   provider.ID
		home string
	}
	seen := map[key]bool{}
	var out []provider.Profile
	for _, p := range c.Profiles {
		k := key{p.Provider, filepath.Clean(p.Home)}
		if seen[k] {
			continue
		}
		seen[k] = true
		best, _ := c.ForHome(p.Provider, p.Home)
		out = append(out, best)
	}
	return out
}
