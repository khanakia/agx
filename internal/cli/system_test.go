package cli

import (
	"bytes"
	"os"
	"path/filepath"
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
// location honouring AGX_CONFIG_DIR via voltkit appdir (not parallel: env).
func TestSystemDeps(t *testing.T) {
	dir := t.TempDir()
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
