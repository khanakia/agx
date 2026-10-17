package config

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// commented is a config written by hand: comments in every position the
// editor must keep, and keys in a non-schema order.
const commented = `# my agx config — comments must survive edits
profiles: # the profiles
  # personal is the default home
  - name: personal
    provider: claude
    home: ~/.claude # the default home
  - name: work
    provider: claude
    home: ~/.claude-work
shell:
  aliases:
    cl: run -p personal # plain alias
sessions:
  root: ~/s
`

// commentLines are the comment texts that must survive every edit.
var commentLines = []string{"# my agx config — comments must survive edits", "# the profiles", "# personal is the default home", "# the default home", "# plain alias"}

func newEditor(t *testing.T, body string, perm os.FileMode) Editor {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agx", "config.yaml")
	if body != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), perm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, perm); err != nil {
			t.Fatal(err)
		}
	}
	return Editor{Path: path, UserHome: userHome, Providers: loginProviders()}
}

func (e Editor) load(t *testing.T) Config {
	t.Helper()
	cfg, err := Load(e.Path, e.UserHome, e.Providers)
	if err != nil {
		t.Fatalf("edited config does not load: %v", err)
	}
	return cfg
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// keyOrder returns the top-level keys of a YAML file in order.
func keyOrder(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if line != "" && line[0] != ' ' && line[0] != '#' && line[0] != '-' {
			k, _, _ := strings.Cut(line, ":")
			out = append(out, k)
		}
	}
	return out
}

// TestEditor_RoundTrip runs every edit in sequence on a commented file and
// checks after each that comments, key order and mode survive and that
// the result loads.
func TestEditor_RoundTrip(t *testing.T) {
	t.Parallel()
	e := newEditor(t, commented, 0o644)
	steps := []struct {
		name string
		do   func(Config) (Change, error)
		want Change
	}{
		{"add personal", func(Config) (Change, error) {
			return Change{}, e.AddLogin(LoginSpec{Name: "personal", Provider: "claude", Token: "stored", Account: "me@example.com", Added: "2026-10-16"}, false)
		}, Change{}},
		{"add work", func(Config) (Change, error) {
			return Change{}, e.AddLogin(LoginSpec{Name: "work", Provider: "claude", TokenCmd: "gopass show -o x"}, false)
		}, Change{}},
		{"use on profile", func(c Config) (Change, error) {
			p, _ := c.ByName("work")
			return e.SetProfileLogin(c, p, "personal")
		}, Change{Where: "profiles[work].login", Before: unsetValue, After: "personal"}},
		{"re-point profile", func(c Config) (Change, error) {
			p, _ := c.ByName("work")
			return e.SetProfileLogin(c, p, "work")
		}, Change{Where: "profiles[work].login", Before: "personal", After: "work"}},
		{"pin home", func(c Config) (Change, error) {
			p, _ := c.ByName("personal")
			return e.SetProfileLogin(c, p, "home")
		}, Change{Where: "profiles[personal].login", Before: unsetValue, After: "home"}},
		{"provider default", func(Config) (Change, error) {
			return e.SetProviderLogin(provider.Claude, "personal")
		}, Change{Where: "providers.claude.login", Before: unsetValue, After: "personal"}},
		{"provider home", func(Config) (Change, error) {
			return e.SetProviderLogin(provider.Claude, "home")
		}, Change{Where: "providers.claude.login", Before: "personal", After: unsetValue}},
		{"replace work", func(Config) (Change, error) {
			return Change{}, e.AddLogin(LoginSpec{Name: "work", Provider: "claude", TokenEnv: "WORK_TOKEN"}, true)
		}, Change{}},
	}
	for _, s := range steps {
		ch, err := s.do(e.load(t))
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if ch.Where != s.want.Where || ch.Before != s.want.Before || ch.After != s.want.After {
			t.Errorf("%s: change = %+v, want %+v", s.name, ch, s.want)
		}
		body := read(t, e.Path)
		for _, c := range commentLines {
			if !strings.Contains(body, c) {
				t.Errorf("%s: lost comment %q\n%s", s.name, c, body)
			}
		}
		order := keyOrder(body)
		if !slices.Equal(order[:3], []string{"profiles", "shell", "sessions"}) {
			t.Errorf("%s: key order %v", s.name, order)
		}
		if got := mode(t, e.Path); got != 0o644 {
			t.Errorf("%s: mode %04o, want the user's 0644 kept", s.name, got)
		}
	}
	cfg := e.load(t)
	if l, _ := cfg.LoginByName("work"); l.Env != "WORK_TOKEN" || l.Cmd != "" {
		t.Errorf("replaced login = %+v", l)
	}
	if p, _ := cfg.ByName("work"); p.Login != "work" {
		t.Errorf("work.login = %q", p.Login)
	}
	if len(cfg.ProviderLogins) != 0 {
		t.Errorf("provider logins = %v", cfg.ProviderLogins)
	}
	entries, _ := os.ReadDir(filepath.Dir(e.Path))
	if len(entries) != 1 {
		t.Errorf("temp files left: %v", entries)
	}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// TestEditor_RemoveClearsReferences: `login rm --force` removes the login
// and every profile/provider `login:` naming it, and leaves the others.
func TestEditor_RemoveClearsReferences(t *testing.T) {
	t.Parallel()
	e := newEditor(t, `logins:
  - {name: a, token: stored}
  - {name: b, token_env: B}
providers:
  claude: {login: a, args: [--verbose]}
profiles:
  - {name: personal, provider: claude, home: ~/.claude, login: a}
  - {name: work, provider: claude, home: ~/.claude-work, login: b}
  - {name: x, provider: claude, home: ~/.claude-x, login: a}
`, 0o600)
	cleared, err := e.RemoveLogin("a")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"profiles[personal].login", "profiles[x].login", "providers.claude.login"}; !slices.Equal(cleared, want) {
		t.Errorf("cleared = %v, want %v", cleared, want)
	}
	cfg := e.load(t)
	if _, ok := cfg.LoginByName("a"); ok || len(cfg.Logins) != 1 {
		t.Errorf("logins = %+v", cfg.Logins)
	}
	for name, want := range map[string]string{"personal": "", "work": "b", "x": ""} {
		if p, _ := cfg.ByName(name); p.Login != want {
			t.Errorf("%s.login = %q, want %q", name, p.Login, want)
		}
	}
	if p, _ := cfg.ByName("personal"); !slices.Equal(p.Args, []string{"--verbose"}) {
		t.Errorf("provider args lost: %v", p.Args)
	}
	if _, err := e.RemoveLogin("a"); err == nil || !strings.Contains(err.Error(), `no login "a"`) {
		t.Errorf("second remove: %v", err)
	}
}

// TestEditor_DiscoveredProfiles: with no config file, `login use -p work`
// writes every discovered profile of that provider (configuring one hides
// the rest), not other providers', creates the file 0600 in a 0700 dir,
// and keeps the default.
func TestEditor_DiscoveredProfiles(t *testing.T) {
	t.Parallel()
	e := newEditor(t, "", 0)
	if err := e.AddLogin(LoginSpec{Name: "personal", Token: "stored"}, false); err != nil {
		t.Fatal(err)
	}
	if got := mode(t, e.Path); got != newConfigPerm {
		t.Errorf("new config mode %04o", got)
	}
	if got := mode(t, filepath.Dir(e.Path)); got != configDirPerm {
		t.Errorf("new config dir mode %04o", got)
	}
	cfg := e.load(t)
	work, _ := cfg.ByName("work")
	if work.Source != provider.SourceDiscovered {
		t.Fatalf("work is %s", work.Source)
	}
	ch, err := e.SetProfileLogin(cfg, work, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ch.Notes, []string{"wrote discovered profile personal into the config", "wrote discovered profile work into the config"}) {
		t.Errorf("notes = %v", ch.Notes)
	}
	cfg = e.load(t)
	for _, p := range cfg.Profiles {
		if p.Provider == provider.Claude && p.Source != provider.SourceConfig {
			t.Errorf("%s is still %s", p.Name, p.Source)
		}
	}
	if names(cfg.Profiles) != names(e.loadDiscovered(t)) {
		t.Errorf("profiles changed: %s vs %s", names(cfg.Profiles), names(e.loadDiscovered(t)))
	}
	p, _ := cfg.ByName("personal")
	if !p.Default || p.Home != "/home/me/.claude" {
		t.Errorf("personal = %+v", p)
	}
	if w, _ := cfg.ByName("work"); w.Login != "personal" {
		t.Errorf("work.login = %q", w.Login)
	}
	if strings.Contains(read(t, e.Path), "codex") {
		t.Errorf("codex profile written:\n%s", read(t, e.Path))
	}
}

// loadDiscovered resolves an empty config: what discovery alone gives.
func (e Editor) loadDiscovered(t *testing.T) []provider.Profile {
	t.Helper()
	cfg, err := Resolve(File{}, e.UserHome, e.Providers)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Profiles
}

// TestEditor_RefusesInvalidResult: an edit that would produce a config agx
// cannot load is refused and the file is left byte-for-byte unchanged.
func TestEditor_RefusesInvalidResult(t *testing.T) {
	t.Parallel()
	const body = "# keep me\nlogins:\n  - {name: a, token: stored}\nprofiles:\n  - {name: personal, provider: claude, home: ~/.claude}\n  - {name: kimi, provider: claude, home: ~/.kimi, billing: api}\n"
	e := newEditor(t, body, 0o600)
	cfg := e.load(t)
	personal, _ := cfg.ByName("personal")
	kimi, _ := cfg.ByName("kimi")
	for name, edit := range map[string]func() error{
		"unknown login":     func() error { _, err := e.SetProfileLogin(cfg, personal, "nosuch"); return err },
		"auto":              func() error { _, err := e.SetProfileLogin(cfg, personal, "auto"); return err },
		"api-billed":        func() error { _, err := e.SetProfileLogin(cfg, kimi, "a"); return err },
		"provider unknown":  func() error { _, err := e.SetProviderLogin(provider.Claude, "nosuch"); return err },
		"provider no login": func() error { _, err := e.SetProviderLogin(provider.Codex, "a"); return err },
		"raw token":         func() error { return e.AddLogin(LoginSpec{Name: "b", Token: fakeSecret}, false) },
		"no source":         func() error { return e.AddLogin(LoginSpec{Name: "b"}, false) },
		"reserved name":     func() error { return e.AddLogin(LoginSpec{Name: "home", Token: "stored"}, false) },
		"duplicate":         func() error { return e.AddLogin(LoginSpec{Name: "a", TokenEnv: "X"}, false) },
		"remove missing":    func() error { _, err := e.RemoveLogin("zzz"); return err },
		"unknown profile": func() error {
			_, err := e.SetProfileLogin(cfg, provider.Profile{Name: "ghost", Source: provider.SourceConfig}, "a")
			return err
		},
	} {
		err := edit()
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(err.Error(), fakeSecret) {
			t.Errorf("%s: error echoes the token: %v", name, err)
		}
		if got := read(t, e.Path); got != body {
			t.Errorf("%s: file changed:\n%s", name, got)
		}
	}
	// Removing a login a profile uses clears the reference, so the result
	// still loads (the CLI refuses without --force before calling this).
	e2 := newEditor(t, "logins: [{name: a, token: stored}]\nprofiles: [{name: w, provider: claude, home: ~/.w, login: a}]\n", 0o600)
	if _, err := e2.RemoveLogin("a"); err != nil {
		t.Errorf("remove used login: %v", err)
	}
}

func TestEditor_BadDocuments(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"list":      "- a\n- b\n",
		"scalar":    "hello\n",
		"bad yaml":  "profiles: [\n",
		"providers": "providers: [claude]\n",
	} {
		e := newEditor(t, body, 0o600)
		_, err := e.SetProviderLogin(provider.Claude, "home")
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
		if !bytes.Equal([]byte(read(t, e.Path)), []byte(body)) {
			t.Errorf("%s: file changed", name)
		}
	}
	// A comment-only file is an empty mapping.
	e := newEditor(t, "# nothing yet\n", 0o600)
	if err := e.AddLogin(LoginSpec{Name: "a", TokenEnv: "A"}, false); err != nil {
		t.Errorf("comment-only file: %v", err)
	}
}

func TestTildePath(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"/home/me": "~", "/home/me/.claude": "~/.claude", "/home/me/a/b": "~/a/b", "/home/meow": "/home/meow", "/opt/x": "/opt/x", "/home/..me/x": "/home/..me/x"} {
		if got := tildePath(in, "/home/me"); got != want {
			t.Errorf("tildePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEditor_PrunesEmptyProviders: removing the last key of providers.<id>
// (login use home, login rm) drops the empty entry and an empty providers:,
// while other providers, other keys and the rest of the file survive.
func TestEditor_PrunesEmptyProviders(t *testing.T) {
	t.Parallel()
	const head = "logins: [{name: a, token: stored}]\nprofiles: [{name: personal, provider: claude, home: ~/.claude}]\n"
	for _, c := range []struct {
		name      string
		providers string
		do        func(Editor) error
		want      []string // substrings that must remain
		gone      []string // substrings that must be gone
	}{
		{name: "use-home-last-key", providers: "providers:\n  claude: {login: a}\n",
			do:   func(e Editor) error { _, err := e.SetProviderLogin(provider.Claude, "home"); return err },
			gone: []string{"providers", "claude: {}"}},
		{name: "use-home-other-key", providers: "providers:\n  claude: {login: a, args: [--verbose]}\n",
			do:   func(e Editor) error { _, err := e.SetProviderLogin(provider.Claude, "home"); return err },
			want: []string{"providers:", "--verbose"}, gone: []string{"login: a"}},
		{name: "use-home-other-provider", providers: "providers:\n  claude: {login: a}\n  codex: {args: [--x]}\n",
			do:   func(e Editor) error { _, err := e.SetProviderLogin(provider.Claude, "home"); return err },
			want: []string{"providers:", "codex:"}, gone: []string{"claude:"}},
		{name: "use-home-never-set", providers: "",
			do:   func(e Editor) error { _, err := e.SetProviderLogin(provider.Claude, "home"); return err },
			gone: []string{"providers"}},
		{name: "rm-login-last-key", providers: "providers:\n  claude: {login: a}\n",
			do:   func(e Editor) error { _, err := e.RemoveLogin("a"); return err },
			gone: []string{"providers", "claude: {}"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newEditor(t, head+c.providers, 0o644)
			if err := c.do(e); err != nil {
				t.Fatal(err)
			}
			e.load(t)
			body := read(t, e.Path)
			for _, w := range append([]string{"personal"}, c.want...) {
				if !strings.Contains(body, w) {
					t.Errorf("lost %q:\n%s", w, body)
				}
			}
			for _, g := range c.gone {
				if strings.Contains(body, g) {
					t.Errorf("left %q:\n%s", g, body)
				}
			}
		})
	}
}
