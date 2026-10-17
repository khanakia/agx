package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/khanakia/agx/provider"
)

// TestResolve_AliasArgsCharset walks the alias value space: every class the
// pattern admits is accepted (and trimmed), and every byte that could break
// out of a shell word in the generated zsh/bash code is refused — the value
// is pasted into shell code unescaped, so this check is the only guard.
func TestResolve_AliasArgsCharset(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"run", "run"},
		{"run -p personal", "run -p personal"},
		{"  run -p work  ", "run -p work"},
		{"run -p auto -- --model=opus", "run -p auto -- --model=opus"},
		{"new a.b/c:d=e@f+g_h", "new a.b/c:d=e@f+g_h"},
		{"resume --list", "resume --list"},
		{"ws ../x", "ws ../x"},
	} {
		cfg, err := Resolve(File{Shell: ShellSpec{Aliases: map[string]string{"cl": c.in}}}, userHome, providers())
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if len(cfg.Aliases) != 1 || cfg.Aliases[0].Args != c.want {
			t.Errorf("%q: aliases %+v, want args %q", c.in, cfg.Aliases, c.want)
		}
	}
	for _, bad := range []string{
		"run;id", "run|id", "run$HOME", "run`id`", "run&", "run>x", "run<x",
		"run'x'", `run"x"`, "run\nid", "run\tx", `run\x`, "run(x)", "run*",
		"run?", "run!", "run#x", "run ~", "run{a,b}", "run[x]", "run%x", "run^x", "a,b",
		"rün", "run 日本", "run x", "run\u200bx", "run\x00", "\n",
	} {
		_, err := Resolve(File{Shell: ShellSpec{Aliases: map[string]string{"cl": bad}}}, userHome, providers())
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: err = %v, want ErrInvalid", bad, err)
		}
	}
}

// TestResolve_AliasNameMatrix: alias names become shell function names.
func TestResolve_AliasNameMatrix(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"cl", "_x", "Cl-2", "a_b", "CLW"} {
		if _, err := Resolve(File{Shell: ShellSpec{Aliases: map[string]string{name: "run"}}}, userHome, providers()); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"", "2cl", "-cl", "c.l", "c l", "cl;", "ç", "c$", "c/l", "c=l"} {
		_, err := Resolve(File{Shell: ShellSpec{Aliases: map[string]string{name: "run"}}}, userHome, providers())
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: err = %v, want ErrInvalid", name, err)
		}
	}
}

// TestResolve_AliasesSorted: shell output order is by name, not map order.
func TestResolve_AliasesSorted(t *testing.T) {
	t.Parallel()
	cfg, err := Resolve(File{Shell: ShellSpec{Aliases: map[string]string{"zz": "run", "aa": "new", "Mm": "ws", "_b": "usage"}}}, userHome, providers())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range cfg.Aliases {
		got = append(got, a.Name)
	}
	if strings.Join(got, ",") != "Mm,_b,aa,zz" {
		t.Errorf("order %v", got)
	}
}

// TestResolve_RawTokenValues: `token:` accepts only the exact marker; any
// other value is a secret in the wrong file and is refused without echoing.
func TestResolve_RawTokenValues(t *testing.T) {
	t.Parallel()
	const unicodeSecret = "tök€n-FAKE-0001-xyz"
	ok, err := Resolve(File{Logins: []LoginSpec{{Name: "a", Token: "stored"}}}, userHome, loginProviders())
	if err != nil || len(ok.Logins) != 1 {
		t.Fatalf("stored marker: %+v, %v", ok.Logins, err)
	}
	for _, v := range []string{fakeSecret, unicodeSecret, "STORED", "Stored", " stored", "stored ", "stored\n", "yes", "true", "stored-x"} {
		_, err := Resolve(File{Logins: []LoginSpec{{Name: "a", Token: v}}}, userHome, loginProviders())
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "a raw token does not belong in config.yaml") {
			t.Errorf("%q: err = %v", v, err)
			continue
		}
		if len(v) > len("stored-x") && strings.Contains(err.Error(), v) {
			t.Errorf("%q: error echoes the value: %v", v, err)
		}
	}
}

// discoverErrProvider is a provider whose discovery fails.
type discoverErrProvider struct{ fakeProvider }

var errDiscover = errors.New("disk on fire")

func (discoverErrProvider) Discover(string) ([]provider.Profile, error) { return nil, errDiscover }

func TestLoad_ErrorBranches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A directory where the file should be is a read error, not "missing".
	if _, err := Load(dir, userHome, providers()); err == nil || !strings.Contains(err.Error(), "config: read") {
		t.Errorf("dir as file: %v", err)
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("profiles: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad, userHome, providers()); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), bad) {
		t.Errorf("bad yaml: %v", err)
	}
	missing := filepath.Join(dir, "none.yaml")
	if _, err := Load(missing, userHome, []provider.Provider{discoverErrProvider{fakeProvider{id: provider.Claude}}}); !errors.Is(err, errDiscover) {
		t.Errorf("discover error: %v", err)
	}
	// A configured provider is not discovered, so its discovery error is moot.
	f := File{Profiles: []ProfileSpec{{Name: "x", Provider: "claude"}}}
	if _, err := Resolve(f, userHome, []provider.Provider{discoverErrProvider{fakeProvider{id: provider.Claude}}}); err != nil {
		t.Errorf("configured provider must not be discovered: %v", err)
	}
}

func TestConfig_DefaultAndByNameMissing(t *testing.T) {
	t.Parallel()
	var empty Config
	if _, ok := empty.Default(provider.Claude); ok {
		t.Error("empty config has a default")
	}
	if _, ok := empty.ByName("personal"); ok {
		t.Error("empty config has a profile")
	}
	cfg, err := resolve(t, "profiles: [{name: x, provider: codex}]")
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := cfg.Default(provider.Codex); !ok || p.Name != "x" {
		t.Errorf("codex default = %+v, %v", p, ok)
	}
	if p, ok := cfg.Default(provider.Claude); !ok || p.Name != "personal" {
		t.Errorf("claude default = %+v, %v (discovered default kept)", p, ok)
	}
	if _, ok := cfg.Default(provider.ID("gemini")); ok {
		t.Error("unknown provider has a default")
	}
	if got := firstOf(cfg.Profiles, provider.ID("gemini")); got != -1 {
		t.Errorf("firstOf unknown = %d, want -1", got)
	}
}

func TestWriteAtomic(t *testing.T) {
	t.Parallel()
	const mode os.FileMode = 0o640
	dir := filepath.Join(t.TempDir(), "new", "deep")
	path := filepath.Join(dir, FileName)
	for _, body := range []string{"a: 1\n", "b: 2\n"} {
		if err := writeAtomic(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		if got := read(t, path); got != body {
			t.Errorf("content %q, want %q", got, body)
		}
		if got := mode0(t, path); got != mode {
			t.Errorf("mode %04o, want %04o", got, mode)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temp files left: %v", entries)
	}

	// Parent is a regular file: MkdirAll fails.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(file, FileName), []byte("x"), mode); err == nil {
		t.Error("write under a file: accepted")
	}
	// Read-only dir: the temp file cannot be created; the old file stays.
	ro := t.TempDir()
	old := filepath.Join(ro, FileName)
	if err := os.WriteFile(old, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	if err := writeAtomic(old, []byte("new\n"), mode); err == nil {
		t.Error("write in read-only dir: accepted")
	}
	if got := read(t, old); got != "old\n" {
		t.Errorf("old file changed: %q", got)
	}
}

// mode0 is mode() without clashing with edit_test's helper signature.
func mode0(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestEditor_ReadError(t *testing.T) {
	t.Parallel()
	e := Editor{Path: t.TempDir(), UserHome: userHome, Providers: loginProviders()}
	if err := e.AddLogin(LoginSpec{Name: "a", TokenEnv: "A"}, false); err == nil || !strings.Contains(err.Error(), "config: read") {
		t.Errorf("dir as config: %v", err)
	}
}

func TestMapGet(t *testing.T) {
	t.Parallel()
	m := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "a"}, {Kind: yaml.ScalarNode, Value: "1"},
		{Kind: yaml.ScalarNode, Value: "ü"}, {Kind: yaml.SequenceNode},
	}}
	for _, c := range []struct {
		name string
		m    *yaml.Node
		key  string
		ok   bool
	}{
		{"nil", nil, "a", false},
		{"scalar", &yaml.Node{Kind: yaml.ScalarNode, Value: "a"}, "a", false},
		{"seq", &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "a"}}}, "a", false},
		{"hit", m, "a", true},
		{"unicode", m, "ü", true},
		{"miss", m, "b", false},
		{"value not key", m, "1", false},
		{"empty key", m, "", false},
	} {
		if got := mapGet(c.m, c.key); (got != nil) != c.ok {
			t.Errorf("%s: got %v, want found=%v", c.name, got, c.ok)
		}
	}
	if scalar(m, "ü") != "" {
		t.Error("scalar of a sequence must be empty")
	}
	if scalar(m, "a") != "1" {
		t.Error("scalar a")
	}
}
