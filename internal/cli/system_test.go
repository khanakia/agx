package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

func TestIsTerminalAndLookPath(t *testing.T) {
	t.Parallel()
	if isTerminal(&bytes.Buffer{}) {
		t.Error("a buffer is not a terminal")
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }() // read-only test handle
	if !isTerminal(devNull) {
		t.Error("/dev/null is a character device and should count as a terminal")
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "file"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // test file
	if isTerminal(f) {
		t.Error("a regular file is not a terminal")
	}
	if _, err := lookPath("sh"); err != nil {
		t.Errorf("lookPath(sh) = %v", err)
	}
}

// TestSystemDeps checks the real wiring: providers registered, and the config
// location honouring AGX_CONFIG_DIR via ubgo/appdir (not parallel: env).
func TestSystemDeps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGX_HOME", "")
	t.Setenv("AGX_CONFIG_DIR", dir)
	d, err := SystemDeps()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Providers) != 2 || d.Providers[0].ID() != "claude" || d.Providers[1].ID() != "codex" {
		t.Errorf("providers = %v", d.Providers)
	}
	path, rung, err := d.ConfigPath()
	if err != nil || path != filepath.Join(dir, "config.yaml") || rung == "" {
		t.Errorf("ConfigPath = %s, %s, %v", path, rung, err)
	}
	for name, fn := range map[string]bool{"Exec": d.Exec != nil, "Secrets": d.Secrets != nil, "Picker": d.Picker != nil, "Now": d.Now != nil, "LookPath": d.LookPath != nil} {
		if !fn {
			t.Errorf("%s not wired", name)
		}
	}
}

// TestConfigPathKeepsLegacyLayout pins agx's folder locations across the
// move from voltkit/appdir to ubgo/appdir: HOME is a temp folder holding the
// old layout (~/.agx/config.yaml + tokens.yaml), and every path agx resolves
// must be the old one, for every override, with nothing created or moved.
// Never touches the real ~/.agx (not parallel: env).
func TestConfigPathKeepsLegacyLayout(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".agx")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"config.yaml", "tokens.yaml"} {
		if err := os.WriteFile(filepath.Join(old, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tree := func() []string {
		var out []string
		_ = filepath.WalkDir(home, func(p string, _ fs.DirEntry, err error) error {
			if err == nil {
				out = append(out, p)
			}
			return err
		})
		sort.Strings(out)
		return out
	}
	before := tree()

	alt := t.TempDir()
	cases := []struct {
		name, agxHome, agxConfigDir, profile string
		wantDir, wantRung                    string
	}{
		{name: "default", wantDir: old, wantRung: "home-dotfile"},
		{name: "empty vars count as unset", agxHome: "", agxConfigDir: "", wantDir: old, wantRung: "home-dotfile"},
		{name: "AGX_PROFILE=dev keeps ~/.agx", profile: "dev", wantDir: old, wantRung: "home-dotfile"},
		{name: "AGX_HOME reads $AGX_HOME/config", agxHome: alt, wantDir: filepath.Join(alt, "config"), wantRung: "env-home"},
		{name: "AGX_CONFIG_DIR", agxConfigDir: alt, wantDir: alt, wantRung: "env-category"},
		{name: "AGX_CONFIG_DIR wins over AGX_HOME", agxHome: filepath.Join(alt, "h"), agxConfigDir: alt, wantDir: alt, wantRung: "env-category"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("AGX_HOME", tc.agxHome)
			t.Setenv("AGX_CONFIG_DIR", tc.agxConfigDir)
			t.Setenv("AGX_PROFILE", tc.profile)
			d, err := SystemDeps()
			if err != nil {
				t.Fatal(err)
			}
			if d.UserHome != home {
				t.Fatalf("UserHome = %s, want %s", d.UserHome, home)
			}
			path, rung, err := d.ConfigPath()
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(tc.wantDir, "config.yaml"); path != want || rung != tc.wantRung {
				t.Errorf("ConfigPath = %s (%s), want %s (%s)", path, rung, want, tc.wantRung)
			}
			a := &app{Deps: d}
			tf, err := a.tokenFile()
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(tc.wantDir, "tokens.yaml"); tf.Path != want {
				t.Errorf("tokens file = %s, want %s", tf.Path, want)
			}
			cfg, err := a.config()
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantDir == old && !cfg.Found {
				t.Errorf("old config.yaml at %s not found", path)
			}
			if want := filepath.Join(home, "agx-sessions"); cfg.SessionsRoot != want {
				t.Errorf("sessions root = %s, want %s", cfg.SessionsRoot, want)
			}
		})
	}
	if after := tree(); !slices.Equal(before, after) {
		t.Errorf("the home changed:\nbefore %v\nafter  %v", before, after)
	}
}
