package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// fakeProvider is the minimal provider.Provider config needs: an id, a
// default home and a fixed discovery result.
type fakeProvider struct {
	id   provider.ID
	disc []provider.Profile
}

func (f fakeProvider) ID() provider.ID                             { return f.id }
func (f fakeProvider) Binary() string                              { return string(f.id) }
func (f fakeProvider) DefaultHome(home string) string              { return filepath.Join(home, "."+string(f.id)) }
func (f fakeProvider) Discover(string) ([]provider.Profile, error) { return f.disc, nil }
func (f fakeProvider) Identity(provider.Profile) (provider.Identity, error) {
	return provider.Identity{}, nil
}
func (f fakeProvider) Usage(context.Context, provider.Profile) (provider.Usage, error) {
	return provider.Usage{}, nil
}
func (f fakeProvider) Launch(provider.Profile, provider.LaunchRequest) (provider.Command, error) {
	return provider.Command{}, nil
}

const userHome = "/home/me"

func providers() []provider.Provider {
	return []provider.Provider{
		fakeProvider{id: provider.Claude, disc: []provider.Profile{
			{Name: "personal", Provider: provider.Claude, Home: "/home/me/.claude", Billing: provider.BillingPlan, Default: true, Source: provider.SourceDiscovered},
			{Name: "work", Provider: provider.Claude, Home: "/home/me/.claude-work", Billing: provider.BillingPlan, Source: provider.SourceDiscovered},
		}},
		fakeProvider{id: provider.Codex, disc: []provider.Profile{
			{Name: "codex", Provider: provider.Codex, Home: "/home/me/.codex", Billing: provider.BillingPlan, Default: true, Source: provider.SourceDiscovered},
		}},
	}
}

func resolve(t *testing.T, yaml string) (Config, error) {
	t.Helper()
	f, err := Decode([]byte(yaml))
	if err != nil {
		return Config{}, err
	}
	return Resolve(f, userHome, providers())
}

func names(ps []provider.Profile) string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return strings.Join(out, ",")
}

func TestResolve_NoConfigUsesDiscovery(t *testing.T) {
	t.Parallel()
	cfg, err := resolve(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if names(cfg.Profiles) != "personal,work,codex" {
		t.Errorf("profiles = %s", names(cfg.Profiles))
	}
	if cfg.SessionsRoot != "/home/me/agx-sessions" || cfg.PromoteRoot != "/home/me" {
		t.Errorf("roots = %s, %s", cfg.SessionsRoot, cfg.PromoteRoot)
	}
	if p, _ := cfg.Default(provider.Claude); p.Name != "personal" {
		t.Errorf("claude default = %s", p.Name)
	}
}

func TestResolve_FullConfig(t *testing.T) {
	t.Parallel()
	cfg, err := resolve(t, `
sessions:
  root: ~/s
providers:
  claude:
    args: [--a, --b]
profiles:
  - name: work
    provider: claude
    home: ~/.claude-work
  - name: personal
    provider: claude
    default: true
    args: [--c]
  - name: kimi
    provider: claude
    billing: api
    args_replace: true
    args: [--only]
    env: {BASE: x}
    secrets: {TOKEN: "gopass:a/b"}
shell:
  aliases:
    clw: run -p work
    cl: run -p personal
`)
	if err != nil {
		t.Fatal(err)
	}
	// Configured claude profiles replace discovery; codex is still discovered.
	if names(cfg.Profiles) != "work,personal,kimi,codex" {
		t.Errorf("profiles = %s", names(cfg.Profiles))
	}
	work, _ := cfg.ByName("work")
	personal, _ := cfg.ByName("personal")
	kimi, _ := cfg.ByName("kimi")
	if work.Home != "/home/me/.claude-work" || !slices.Equal(work.Args, []string{"--a", "--b"}) || work.Source != provider.SourceConfig {
		t.Errorf("work = %+v", work)
	}
	if personal.Home != "/home/me/.claude" || !personal.Default || !slices.Equal(personal.Args, []string{"--a", "--b", "--c"}) {
		t.Errorf("personal = %+v", personal)
	}
	if kimi.Billing != provider.BillingAPI || !slices.Equal(kimi.Args, []string{"--only"}) || kimi.Secrets["TOKEN"] != "gopass:a/b" {
		t.Errorf("kimi = %+v", kimi)
	}
	if cfg.SessionsRoot != "/home/me/s" || cfg.PromoteRoot != "/home/me" {
		t.Errorf("roots = %s, %s", cfg.SessionsRoot, cfg.PromoteRoot)
	}
	if len(cfg.Aliases) != 2 || cfg.Aliases[0].Name != "cl" {
		t.Errorf("aliases not sorted: %+v", cfg.Aliases)
	}
	// ForHome prefers the plan-billed default over kimi on the shared home.
	if p, _ := cfg.ForHome(provider.Claude, "/home/me/.claude"); p.Name != "personal" {
		t.Errorf("ForHome = %s", p.Name)
	}
	// Homes dedupes the shared home.
	if names(cfg.Homes()) != "work,personal,codex" {
		t.Errorf("Homes = %s", names(cfg.Homes()))
	}
}

func TestResolve_FirstProfileBecomesDefault(t *testing.T) {
	t.Parallel()
	cfg, err := resolve(t, "profiles:\n  - {name: b, provider: claude}\n  - {name: a, provider: claude}\n")
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := cfg.Default(provider.Claude); p.Name != "b" {
		t.Errorf("default = %s", p.Name)
	}
	var defaults int
	for _, p := range cfg.Profiles {
		if p.Provider == provider.Claude && p.Default {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("%d claude defaults", defaults)
	}
}

func TestResolve_DiscoveredNameCollision(t *testing.T) {
	t.Parallel()
	cfg, err := resolve(t, "profiles:\n  - {name: codex, provider: claude}\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.ByName("codex-codex"); !ok {
		t.Errorf("colliding discovered profile not namespaced: %s", names(cfg.Profiles))
	}
}

func TestResolve_Invalid(t *testing.T) {
	t.Parallel()
	for name, yaml := range map[string]string{
		"unknown key":        "sesions: {}",
		"bad name":           "profiles: [{name: Bad_Name, provider: claude}]",
		"reserved auto":      "profiles: [{name: auto, provider: claude}]",
		"unknown provider":   "profiles: [{name: x, provider: gemini}]",
		"unknown providers":  "providers: {gemini: {args: []}}",
		"bad billing":        "profiles: [{name: x, provider: claude, billing: free}]",
		"duplicate":          "profiles: [{name: x, provider: claude}, {name: x, provider: claude}]",
		"two defaults":       "profiles: [{name: x, provider: claude, default: true}, {name: y, provider: claude, default: true}]",
		"bad secret scheme":  "profiles: [{name: x, provider: claude, secrets: {T: 'vault:a'}}]",
		"secret without ref": "profiles: [{name: x, provider: claude, secrets: {T: 'gopass'}}]",
		"env+secret clash":   "profiles: [{name: x, provider: claude, env: {T: v}, secrets: {T: 'env:Y'}}]",
		"bad alias name":     "shell: {aliases: {'bad name': run}}",
		"empty alias":        "shell: {aliases: {cl: ' '}}",
		"alias injection":    "shell: {aliases: {cl: 'run; rm -rf ~'}}",
		"alias subshell":     "shell: {aliases: {cl: 'run $(id)'}}",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := resolve(t, yaml); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestLoad_MissingAndPresentFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	cfg, err := Load(path, userHome, providers())
	if err != nil || cfg.Found || cfg.Path != path {
		t.Fatalf("missing file: %+v, %v", cfg, err)
	}
	if err := os.WriteFile(path, []byte("sessions: {root: /tmp/x}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path, userHome, providers())
	if err != nil || !cfg.Found || cfg.SessionsRoot != "/tmp/x" {
		t.Fatalf("present file: %+v, %v", cfg, err)
	}
	if err := os.WriteFile(path, []byte("profiles: [{name: x}]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, userHome, providers()); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("invalid file error should name the path: %v", err)
	}
}

func TestExpand(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":          "",
		"~":         "/home/me",
		"~/a/../b/": "/home/me/b",
		"/abs/x/":   "/abs/x",
	} {
		if got := expand(in, userHome); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
}
