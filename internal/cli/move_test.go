package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// unsupportedProv is a second provider that cannot move conversations.
type unsupportedProv struct{ *fakeProv }

func (unsupportedProv) MoveUnsupportedReason() string { return "it keeps an index" }

// moveHarness wires the REAL Claude provider over the harness's temp home,
// plus a codex-like provider without a mover, and a config with profiles on
// both providers.
func moveHarness(t *testing.T) (*harness, Deps, string) {
	t.Helper()
	h := newHarness(t, "")
	raw := fmt.Sprintf(`sessions:
  root: %s
profiles:
  - {name: personal, provider: claude, home: %s/.claude, default: true}
  - {name: work, provider: claude, home: %s/.claude-work}
  - {name: kimi, provider: claude, home: %s/.claude, billing: api}
  - {name: codex, provider: codex, home: %s/.codex}
  - {name: codex-work, provider: codex, home: %s/.codex-work}
`, h.sessions, h.home, h.home, h.home, h.home, h.home)
	if err := os.WriteFile(h.cfgPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(h.root, "docker_setup_mac")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	proj := claude.HistoryDir(filepath.Join(h.home, ".claude"), project)
	for path, body := range map[string]string{
		filepath.Join(proj, "sid-main.jsonl"):                                 `{"type":"attachment","cwd":"` + project + `"}` + "\n" + `{"type":"ai-title","aiTitle":"Hatchet services"}`,
		filepath.Join(proj, "sid-main", "tool-results", "r.txt"):              "R",
		filepath.Join(proj, "sid-two.jsonl"):                                  `{"type":"attachment","cwd":"` + project + `"}`,
		filepath.Join(h.home, ".claude", "file-history", "sid-main", "f.txt"): "F",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d := h.deps()
	d.Providers = []provider.Provider{&claude.Provider{UserHome: h.home}, unsupportedProv{&fakeProv{id: provider.Codex}}}
	return h, d, project
}

func runWith(h *harness, d Deps, cwd string, args ...string) int {
	h.stdout.Reset()
	h.stderr.Reset()
	d.Getwd = func() (string, error) { return cwd, nil }
	root := NewRoot(d)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil && !IsSilent(err) {
		fmt.Fprintln(&h.stderr, "agx:", err)
	}
	return ExitCode(err)
}

func TestSessionsMove_EndToEnd(t *testing.T) {
	t.Parallel()
	h, d, project := moveHarness(t)
	personal, work := filepath.Join(h.home, ".claude"), filepath.Join(h.home, ".claude-work")
	src := claude.HistoryDir(personal, project)

	// Dry run: plan printed, nothing moved.
	if code := runWith(h, d, project, "sessions", "move", "--to", "work", "--dry-run"); code != ExitOK {
		t.Fatalf("dry run: %d %s", code, h.stderr.String())
	}
	if out := h.stdout.String(); !strings.Contains(out, "would move 2 claude conversation(s)") || !strings.Contains(out, "Hatchet services") {
		t.Errorf("dry run output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(src, "sid-main.jsonl")); err != nil {
		t.Fatal("dry run moved something")
	}

	// The folder's own name typed from INSIDE it resolves to it (the bug a
	// user hit: it looked for docker_setup_mac/docker_setup_mac).
	if code := runWith(h, d, project, "sessions", "move", "docker_setup_mac", "--to", "work", "--dry-run"); code != ExitOK {
		t.Fatalf("name from inside: %d %s", code, h.stderr.String())
	}

	// Real move, by folder name resolved relative to the cwd.
	if code := runWith(h, d, h.root, "sessions", "move", "docker_setup_mac", "--to", "work"); code != ExitOK {
		t.Fatalf("move: %d\n%s%s", code, h.stdout.String(), h.stderr.String())
	}
	dst := claude.HistoryDir(work, project)
	for _, p := range []string{filepath.Join(dst, "sid-main.jsonl"), filepath.Join(dst, "sid-two.jsonl"), filepath.Join(dst, "sid-main", "tool-results", "r.txt"), filepath.Join(work, "file-history", "sid-main", "f.txt")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("not moved: %s", p)
		}
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("emptied personal history folder was left behind")
	}
	backups, _ := filepath.Glob(filepath.Join(personal, "backups", "agx-move-docker_setup_mac-*", "projects", "*", "sid-main.jsonl"))
	if len(backups) != 1 {
		t.Errorf("backup missing: %v", backups)
	}
	if !strings.Contains(h.stdout.String(), "moved 2 claude conversation(s)") {
		t.Errorf("output:\n%s", h.stdout.String())
	}

	// Resume in the folder now lands on work.
	h.execs = nil
	dd := d
	dd.Exec = func(c provider.Command) error { h.execs = append(h.execs, c); return nil }
	if code := runWith(h, dd, project, "resume", "--last"); code != ExitOK {
		t.Fatalf("resume: %d %s", code, h.stderr.String())
	}
	if c := h.lastExec(); !slices.Contains(c.Env, claude.ConfigDirEnv+"="+work) {
		t.Errorf("resume after move did not use work: %v", c.Env)
	}

	// Moving again finds nothing left in personal.
	if code := runWith(h, d, project, "sessions", "move", "--to", "work"); code != ExitOK || !strings.Contains(h.stderr.String(), "nothing to move") {
		t.Errorf("second move: %d %s", code, h.stderr.String())
	}
	// And it can move back.
	if code := runWith(h, d, project, "sessions", "move", "--to", "personal", "--from", "work", "--no-backup"); code != ExitOK {
		t.Fatalf("move back: %d %s", code, h.stderr.String())
	}
	if _, err := os.Stat(filepath.Join(src, "sid-main.jsonl")); err != nil {
		t.Error("move back failed")
	}
}

func TestSessionsMove_Refusals(t *testing.T) {
	t.Parallel()
	h, d, project := moveHarness(t)
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no --to", []string{"sessions", "move"}, ExitUsage, "--to <profile> is required"},
		{"unknown profile", []string{"sessions", "move", "--to", "nobody"}, ExitUsage, "unknown profile"},
		{"claude to codex", []string{"sessions", "move", "--from", "personal", "--to", "codex"}, ExitUsage, "different formats"},
		{"same home", []string{"sessions", "move", "--from", "personal", "--to", "kimi"}, ExitUsage, "same home"},
		{"provider cannot move", []string{"sessions", "move", "--to", "codex-work"}, ExitFailure, "it keeps an index"},
		{"unsupported before account checks", []string{"sessions", "move", "--to", "codex"}, ExitFailure, "it keeps an index"},
		{"missing folder", []string{"sessions", "move", "/no/such/dir", "--to", "work"}, ExitUsage, "no such folder"},
	} {
		if code := runWith(h, d, project, tc.args...); code != tc.code || !strings.Contains(h.stderr.String(), tc.want) {
			t.Errorf("%s: exit %d (want %d), stderr %q", tc.name, code, tc.code, h.stderr.String())
		}
	}
	// None of the refusals touched the files.
	if _, err := os.Stat(filepath.Join(claude.HistoryDir(filepath.Join(h.home, ".claude"), project), "sid-main.jsonl")); err != nil {
		t.Error("a refused move changed files")
	}
}

func TestSessionsMove_BlockedAndJSON(t *testing.T) {
	t.Parallel()
	h, d, project := moveHarness(t)
	work := filepath.Join(h.home, ".claude-work")
	// Claude "running" in the folder (this test process's pid).
	reg := filepath.Join(work, "sessions", "1.json")
	if err := os.MkdirAll(filepath.Dir(reg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reg, []byte(fmt.Sprintf(`{"pid":%d,"cwd":%q}`, os.Getpid(), project)), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runWith(h, d, project, "sessions", "move", "--to", "work"); code != ExitFailure || !strings.Contains(h.stdout.String(), "Claude is running in this folder") {
		t.Errorf("blocked: %d\n%s", code, h.stdout.String())
	}
	if _, err := os.Stat(filepath.Join(claude.HistoryDir(filepath.Join(h.home, ".claude"), project), "sid-main.jsonl")); err != nil {
		t.Error("blocked move changed files")
	}
	if err := os.Remove(reg); err != nil {
		t.Fatal(err)
	}
	if code := runWith(h, d, project, "sessions", "move", "--to", "work", "--dry-run", "--json"); code != ExitOK {
		t.Fatalf("json: %d %s", code, h.stderr.String())
	}
	e := decode[[]moveResult](t, h.stdout.Bytes())
	if e.Kind != kindSessionMove || len(e.Data) != 1 || len(e.Data[0].Conversations) != 2 || !e.Data[0].DryRun || e.Data[0].Applied {
		t.Errorf("json = %+v", e)
	}
}
