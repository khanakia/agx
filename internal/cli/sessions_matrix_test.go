package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/sessions"
)

// checkerOnly reports history but cannot move it: the shape of a provider
// that keys history by id (Codex), so promote must leave it behind.
type checkerOnly struct {
	provider.Provider
	has map[string]bool // by dir
}

func (c checkerOnly) HasHistory(_, dir string) bool { return c.has[dir] }

// mkSession makes a session folder (with a file when file is set).
func mkSession(t *testing.T, h *harness, name string, file bool) string {
	t.Helper()
	p := filepath.Join(h.sessions, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if file {
		if err := os.WriteFile(filepath.Join(p, "main.go"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// TestPromote_NameMatrix: the new name is one path element; anything that
// could leave the destination parent is a usage error that moves nothing.
func TestPromote_NameMatrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		ok   bool
	}{
		{"tool", true},
		{"my tool", true},
		{"über-tööl", true},
		{"...", true},
		{".hidden", true},
		{"", false},
		{".", false},
		{"..", false},
		{"../escape", false},
		{"a/b", false},
		{"a/", false},
		{"/abs", false},
		{"./tool", false},
	} {
		h := newHarness(t, "")
		mkSession(t, h, "src_20260101_000000", true)
		code := h.run("sessions", "promote", "src_20260101_000000", c.name, "--json")
		if c.ok != (code == ExitOK) || (!c.ok && code != ExitUsage) {
			t.Errorf("name %q: exit %d, want ok=%v; stderr %s", c.name, code, c.ok, h.stderr.String())
			continue
		}
		if !c.ok {
			if _, err := os.Stat(filepath.Join(h.sessions, "src_20260101_000000")); err != nil {
				t.Errorf("name %q: refused promote moved the folder", c.name)
			}
			continue
		}
		e := decode[promoteResult](t, h.stdout.Bytes())
		if want := filepath.Join(h.root, "projects", c.name); e.Data.To != want || e.Kind != kindSessionPromote {
			t.Errorf("name %q: to %q, want %q", c.name, e.Data.To, want)
		}
		if _, err := os.Stat(filepath.Join(h.root, "projects", c.name, "main.go")); err != nil {
			t.Errorf("name %q: not moved: %v", c.name, err)
		}
	}
}

// TestResolveFolder_Matrix extends TestResolveFolder: a name in both
// sessions.root and cwd, unicode, uncleaned paths and a bare file name.
func TestResolveFolder_Matrix(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	sess := mkSession(t, h, "s_20260101_000000", false)
	here := filepath.Join(h.cwd, "here")
	both := "both"
	for _, d := range []string{here, filepath.Join(h.cwd, both), filepath.Join(h.sessions, both), filepath.Join(h.cwd, "ünï")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(h.cwd, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{Deps: h.deps()}
	cfg, err := a.config()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		arg     string
		want    string
		usage   string // refusal substring; want is ignored
		comment string
	}{
		{arg: ".", want: h.cwd},
		{arg: "s_20260101_000000", want: sess},
		{arg: "here", want: here},
		{arg: both, want: filepath.Join(h.sessions, both), comment: "sessions.root wins over cwd"},
		{arg: filepath.Base(h.cwd), want: h.cwd, comment: "the folder you are in"},
		{arg: "ünï", want: filepath.Join(h.cwd, "ünï")},
		{arg: here + "/", want: here},
		{arg: here + "/../here", want: here},
		{arg: "nosuch", usage: "no such folder"},
		{arg: filepath.Join(h.root, "nosuch"), usage: "no such folder"},
		{arg: file, usage: "not a folder"},
		{arg: "file.txt", usage: "not a folder"},
	} {
		got, err := a.resolveFolder(cfg, c.arg)
		if c.usage != "" {
			if ExitCode(err) != ExitUsage || !strings.Contains(err.Error(), c.usage) {
				t.Errorf("%q: err %v, want usage %q", c.arg, err, c.usage)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q (%s): got %q %v, want %q", c.arg, c.comment, got, err, c.want)
		}
	}
}

// TestPromote_HistoryAndTargets: --to overrides promote_root; a provider
// that only checks history leaves it (warned, history_left); a collision
// with an existing folder moves neither folder nor history.
func TestPromote_HistoryAndTargets(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	src := mkSession(t, h, "chat_20260101_000000", true)
	codex := emptyProv(provider.Codex)
	codexHome := filepath.Join(h.home, ".codex")
	codex.disc = []provider.Profile{{Name: "cx", Provider: provider.Codex, Home: codexHome, Billing: provider.BillingPlan, Source: provider.SourceDiscovered}}
	h.provs = []provider.Provider{h.claude, checkerOnly{Provider: plainProv{codex}, has: map[string]bool{src: true}}}
	personal := filepath.Join(h.home, ".claude")
	h.claude.history[personal] = map[string]bool{src: true}

	// Collision first: nothing moves.
	other := filepath.Join(h.root, "elsewhere")
	if err := os.MkdirAll(filepath.Join(other, "taken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := h.run("sessions", "promote", "chat_20260101_000000", "taken", "--to", other); code != ExitFailure || !strings.Contains(h.stderr.String(), sessions.ErrExists.Error()) {
		t.Errorf("collision: %d %q", code, h.stderr.String())
	}
	if _, err := os.Stat(src); err != nil || len(h.claude.moved) != 0 {
		t.Fatalf("collision moved something: %v %v", err, h.claude.moved)
	}

	// The text view warns about history that stays; --json carries it as
	// history_left instead.
	if code := h.run("sessions", "promote", "chat_20260101_000000", "kept", "--to", other, "--dry-run"); code != ExitOK || !strings.Contains(h.stderr.String(), "keyed by id and stays") {
		t.Errorf("dry run: %d, no history-left warning: %q", code, h.stderr.String())
	}
	if code := h.run("sessions", "promote", "chat_20260101_000000", "kept", "--to", other, "--json"); code != ExitOK {
		t.Fatalf("promote: %d %s", code, h.stderr.String())
	}
	var res promoteResult
	if err := json.Unmarshal(mustData(t, h.stdout.Bytes()), &res); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(other, "kept")
	if res.From != src || res.To != dst || res.DryRun || len(res.History) != 1 || res.History[0] != personal || len(res.Left) != 1 || res.Left[0] != codexHome {
		t.Errorf("result = %+v", res)
	}
	if len(h.claude.moved) != 1 || h.claude.moved[0] != [2]string{src, dst} {
		t.Errorf("moves = %v", h.claude.moved)
	}
}

// mustData extracts the raw data field of an envelope.
func mustData(t *testing.T, b []byte) []byte {
	t.Helper()
	return decode[json.RawMessage](t, b).Data
}

// TestGC_JSONAndSummaries: --json carries every folder with its reason;
// the text summary distinguishes nothing / would / removed.
func TestGC_JSONAndSummaries(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	if code := h.run("sessions", "gc"); code != ExitOK || !strings.Contains(h.stderr.String(), "nothing to remove") || h.stdout.Len() != 0 {
		t.Errorf("empty root: %d %q %q", code, h.stdout.String(), h.stderr.String())
	}
	empty := mkSession(t, h, "20260101_000000", false)
	mkSession(t, h, "files_20260101_000001", true)
	chat := mkSession(t, h, "chat_20260101_000002", false)
	h.claude.history[filepath.Join(h.home, ".claude")] = map[string]bool{chat: true}

	if code := h.run("sessions", "gc", "--json"); code != ExitOK {
		t.Fatalf("gc --json: %d %s", code, h.stderr.String())
	}
	e := decode[[]sessions.GCItem](t, h.stdout.Bytes())
	reasons := map[string]sessions.GCReason{}
	for _, it := range e.Data {
		reasons[it.Folder.Name] = it.Reason
		if it.Remove != (it.Reason == sessions.GCCollect) {
			t.Errorf("%s: remove %v with reason %s", it.Folder.Name, it.Remove, it.Reason)
		}
	}
	if e.Kind != kindSessionGC || e.Count != 3 || reasons["20260101_000000"] != sessions.GCCollect || reasons["files_20260101_000001"] != sessions.GCHasFiles || reasons["chat_20260101_000002"] != sessions.GCHasHistory {
		t.Errorf("gc --json = %+v", e)
	}
	if _, err := os.Stat(empty); err != nil {
		t.Error("gc --json without --yes removed a folder")
	}
	if code := h.run("sessions", "gc"); code != ExitOK || !strings.Contains(h.stderr.String(), "1 empty folder(s) would be removed") {
		t.Errorf("plan summary: %q", h.stderr.String())
	}
	// A huge --min-age keeps everything.
	if code := h.run("sessions", "gc", "--yes", "--min-age", "876000h"); code != ExitOK || !strings.Contains(h.stderr.String(), "nothing to remove") {
		t.Errorf("min-age: %d %q", code, h.stderr.String())
	}
	if code := h.run("sessions", "gc", "--yes"); code != ExitOK || !strings.Contains(h.stderr.String(), "removed 1 empty folder(s)") {
		t.Errorf("removed summary: %q", h.stderr.String())
	}
	if _, err := os.Stat(empty); !errors.Is(err, os.ErrNotExist) {
		t.Error("gc --yes kept the empty folder")
	}
}

// TestGC_RemoveFailureIsPartial: a folder that cannot be removed is
// warned about, exits 1, and is not reported as removed.
func TestGC_RemoveFailureIsPartial(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	h := newHarness(t, "")
	stuck := mkSession(t, h, "20260101_000000", false)
	if err := os.MkdirAll(filepath.Join(stuck, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A read-only folder cannot lose its (empty) child, so RemoveEmpty fails.
	if err := os.Chmod(stuck, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stuck, 0o755) })
	code := h.run("sessions", "gc", "--yes")
	if code != ExitFailure || !strings.Contains(h.stderr.String(), "permission denied") {
		t.Errorf("exit %d stderr %q", code, h.stderr.String())
	}
	if strings.Contains(h.stdout.String(), "20260101_000000") || strings.Contains(h.stderr.String(), "removed 1 empty folder(s)") {
		t.Errorf("a failed removal was reported as removed: stdout %q stderr %q", h.stdout.String(), h.stderr.String())
	}
	if _, err := os.Stat(stuck); err != nil {
		t.Fatal(err)
	}
}
