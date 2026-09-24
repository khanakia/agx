package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

func TestProfiles_Text(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	if code := h.run("profiles"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	out := h.stdout.String()
	for _, want := range []string{"PROFILE", "*personal", "work", "me@home", "discovered"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, alias := range []string{"who", "accounts"} {
		if code := h.run(alias); code != ExitOK || !strings.Contains(h.stdout.String(), "PROFILE") {
			t.Errorf("alias %s failed", alias)
		}
	}
}

func TestResume_ListText(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	home := filepath.Join(h.home, ".claude")
	h.claude.convs[home] = []provider.Conversation{
		{Provider: provider.Claude, Home: home, ID: "id-1", Title: "", Dir: h.cwd, Updated: h.now.Add(-2 * time.Hour)},
	}
	if code := h.run("resume", "--list"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if out := h.stdout.String(); !strings.Contains(out, untitled) || !strings.Contains(out, "2 hr ago") || !strings.Contains(out, "id-1") {
		t.Errorf("list = %q", out)
	}
	// No conversations anywhere: a clear error, nothing launched.
	h2 := newHarness(t, "")
	if code := h2.run("resume"); code == ExitOK || !strings.Contains(h2.stderr.String(), "no matching conversations") {
		t.Errorf("empty resume: %d %s", code, h2.stderr.String())
	}
}

func TestSessionsLs_Text(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	dir := filepath.Join(h.sessions, "x_20260101_000000")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	h.claude.history[filepath.Join(h.home, ".claude")] = map[string]bool{dir: true}
	if code := h.run("sessions", "ls"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if out := h.stdout.String(); !strings.Contains(out, "x_20260101_000000") || !strings.Contains(out, cellYes) {
		t.Errorf("ls = %q", out)
	}
	if !strings.Contains(h.stderr.String(), "(1 folders)") {
		t.Errorf("summary = %q", h.stderr.String())
	}
}

func TestWantColor(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	a := &app{Deps: h.deps()}
	if !a.wantColor(colorAlways) || a.wantColor(colorNever) {
		t.Error("always/never ignored")
	}
	a.IsTerminal = func(io.Writer) bool { return true }
	if !a.wantColor(colorAuto) {
		t.Error("auto on a terminal should colour")
	}
	h.env[noColorEnv] = ""
	if a.wantColor(colorAuto) {
		t.Error("NO_COLOR (even empty) must disable auto colour")
	}
}

func TestResolveFolder(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	a := &app{Deps: h.deps()}
	cfg, err := a.config()
	if err != nil {
		t.Fatal(err)
	}
	named := filepath.Join(h.sessions, "named")
	if err := os.MkdirAll(named, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(h.root, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	here := filepath.Join(h.cwd, "project-here")
	if err := os.MkdirAll(here, 0o755); err != nil {
		t.Fatal(err)
	}
	// h.cwd's own base name is "cwd": typed from inside, it is the cwd.
	for arg, want := range map[string]string{".": h.cwd, "named": named, named + "/": named, "project-here": here, filepath.Base(h.cwd): h.cwd} {
		if got, err := a.resolveFolder(cfg, arg); err != nil || got != want {
			t.Errorf("resolveFolder(%q) = %q, %v; want %q", arg, got, err, want)
		}
	}
	for _, bad := range []string{"missing", file, filepath.Join(h.root, "nope", "x")} {
		if _, err := a.resolveFolder(cfg, bad); ExitCode(err) != ExitUsage {
			t.Errorf("resolveFolder(%q) err = %v, want usage error", bad, err)
		}
	}
}

func TestDoctor_Branches(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "profiles:\n  - {name: personal, provider: claude, home: HOMEDIR/.claude}\n  - {name: gone, provider: claude, home: HOMEDIR/.claude-gone}\n")
	raw, err := os.ReadFile(h.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.cfgPath, []byte(strings.ReplaceAll(string(raw), "HOMEDIR", h.home)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(h.home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Sessions root inside a git repo → warning.
	if err := os.MkdirAll(filepath.Join(h.root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(h.sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	h.claude.usage[filepath.Join(h.home, ".claude")] = provider.Usage{Identity: provider.Identity{Email: "me@home"}}
	h.env["AGX_SHELL_INIT"] = "zsh"
	h.run("doctor")
	out := h.stdout.String()
	for _, want := range []string{"inside git repo", "home " + filepath.Join(h.home, ".claude-gone") + " does not exist", "✓ profile personal", "shell layer active (zsh)"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor missing %q:\n%s", want, out)
		}
	}

	// Missing agent binary is a failure.
	h2 := newHarness(t, "")
	d := h2.deps()
	d.LookPath = func(string) (string, error) { return "", errors.New("missing") }
	root := NewRoot(d)
	root.SetArgs([]string{"doctor"})
	if err := root.Execute(); ExitCode(err) != ExitFailure || !strings.Contains(h2.stdout.String(), "is not on PATH") {
		t.Errorf("missing binary: %v\n%s", err, h2.stdout.String())
	}
}

func TestEnclosingRepo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := enclosingRepo(deep); got != "" && !strings.HasPrefix(root, got) {
		t.Errorf("no repo: %q", got) // a repo above the temp dir is allowed, not one inside it
	}
	if err := os.MkdirAll(filepath.Join(root, "a", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := enclosingRepo(deep); got != filepath.Join(root, "a") {
		t.Errorf("repo = %q", got)
	}
}

func TestYesNo(t *testing.T) {
	t.Parallel()
	if yesNo(true) != cellYes || yesNo(false) != cellNo {
		t.Error("yesNo")
	}
}
