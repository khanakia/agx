package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// TestGroupWorkspaces pins the level-1 grouping: newest activity first,
// a count per folder, and each profile once in order of its newest chat.
func TestGroupWorkspaces(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	v := func(dir, profile string, ago time.Duration) conversationView {
		return conversationView{Conversation: provider.Conversation{Dir: dir, Updated: now.Add(-ago)}, Profile: profile}
	}
	// Input is newest first, as findConversations returns it.
	ws := groupWorkspaces([]conversationView{
		v("/a", "work", time.Minute),
		v("/b", "personal", 2*time.Minute),
		v("/a", "personal", 3*time.Minute),
		v("/a", "work", 4*time.Minute),
	})
	if len(ws) != 2 || ws[0].Dir != "/a" || ws[1].Dir != "/b" {
		t.Fatalf("order = %+v", ws)
	}
	if ws[0].Count != 3 || !slices.Equal(ws[0].Profiles, []string{"work", "personal"}) || !ws[0].Updated.Equal(now.Add(-time.Minute)) || len(ws[0].convs) != 3 {
		t.Errorf("a = %+v", ws[0])
	}
	if ws[1].Count != 1 || !slices.Equal(ws[1].Profiles, []string{"personal"}) {
		t.Errorf("b = %+v", ws[1])
	}
	if groupWorkspaces(nil) != nil {
		t.Error("no conversations must give no workspaces")
	}
}

// TestTailDir keeps the end of a long folder (the project name).
func TestTailDir(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ dir, want string }{
		{"/home/me/p", "~/p"},
		{"/home/me", "~"},
		{"/srv/x", "/srv/x"},
		{"/srv/very/long/path/project", "…ath/project"},
		{"/srv/ééééé/project", "…ééé/project"},
	} {
		if got := tailDir(tc.dir, "/home/me", 12); got != tc.want {
			t.Errorf("tailDir(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

// browseHarness has conversations in two existing folders on two homes and
// one in a folder that no longer exists.
func browseHarness(t *testing.T) (h *harness, other, gone string) {
	t.Helper()
	h = newHarness(t, "")
	personal := filepath.Join(h.home, ".claude")
	work := filepath.Join(h.home, ".claude-work")
	other = filepath.Join(h.root, "other")
	gone = filepath.Join(h.root, "deleted")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	h.claude.convs[personal] = []provider.Conversation{
		{Provider: provider.Claude, Home: personal, ID: "p-here", Title: "Here personal", Dir: h.cwd, Updated: h.now.Add(-3 * time.Hour)},
		{Provider: provider.Claude, Home: personal, ID: "p-other", Title: "Other pglite", Dir: other, Updated: h.now.Add(-time.Hour)},
		{Provider: provider.Claude, Home: personal, ID: "p-gone", Title: "Gone", Dir: gone, Updated: h.now},
	}
	h.claude.convs[work] = []provider.Conversation{
		{Provider: provider.Claude, Home: work, ID: "w-here", Title: "Here work", Dir: h.cwd, Updated: h.now.Add(-time.Minute)},
	}
	return h, other, gone
}

func TestWs_List(t *testing.T) {
	t.Parallel()
	h, other, gone := browseHarness(t)

	// Text: one row per existing folder, newest first; the deleted folder
	// (newest of all) is hidden.
	if code := h.run("ws", "--list"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(h.stdout.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "2 chats") || !strings.Contains(lines[0], "work,personal") || !strings.Contains(lines[1], "1 chat ") {
		t.Errorf("rows:\n%s", h.stdout.String())
	}
	if strings.Contains(h.stdout.String(), "deleted") {
		t.Errorf("missing folder listed:\n%s", h.stdout.String())
	}

	// JSON: workspace.list with full folders, counts and profiles.
	if code := h.run("resume", "--browse", "--list", "--json"); code != ExitOK {
		t.Fatalf("json exit %d", code)
	}
	e := decode[[]workspace](t, h.stdout.Bytes())
	if e.Kind != kindWorkspaceList || e.Count != 2 || e.Data[0].Dir != h.cwd || e.Data[0].Count != 2 || e.Data[1].Dir != other {
		t.Errorf("json = %+v", e)
	}
	for _, w := range e.Data {
		if w.Dir == gone {
			t.Errorf("missing folder in JSON: %+v", w)
		}
	}

	// A query keeps only workspaces with a matching chat.
	if code := h.run("ws", "--list", "pglite"); code != ExitOK || strings.Count(h.stdout.String(), "\n") != 1 || !strings.Contains(h.stdout.String(), "other") {
		t.Errorf("query: %d\n%s", code, h.stdout.String())
	}
}

func TestWs_Browse(t *testing.T) {
	t.Parallel()
	h, other, _ := browseHarness(t)
	work := filepath.Join(h.home, ".claude-work")

	// Workspace 0 (cwd) → Esc on chats → back to workspaces → workspace 0 →
	// chat 0 (newest there: w-here) → resumes on work in cwd.
	h.picker.script = []pickStep{{idx: []int{0}}, {err: ErrCancelled}, {idx: []int{0}}, {idx: []int{0}}}
	if code := h.run("ws"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	if want := []string{"workspace", "chat (esc: back)", "workspace", "chat (esc: back)"}; !slices.Equal(h.picker.prompts, want) {
		t.Errorf("prompts = %v, want %v", h.picker.prompts, want)
	}
	if len(h.picker.shown[1]) != 2 || !strings.Contains(h.picker.shown[1][0], "work") {
		t.Errorf("chat rows = %v", h.picker.shown[1])
	}
	c := h.lastExec()
	if !slices.Contains(c.Args, "w-here") || !slices.Contains(c.Env, "HOME_DIR="+work) || c.Dir != h.cwd {
		t.Errorf("exec = %+v", c)
	}

	// resume -b reaches the other folder.
	h.picker.script = []pickStep{{idx: []int{1}}, {idx: []int{0}}}
	if code := h.run("resume", "-b"); code != ExitOK || h.lastExec().Dir != other {
		t.Errorf("resume -b: %d %+v", code, h.lastExec())
	}

	// Esc on the workspace list exits non-zero and launches nothing.
	n := len(h.execs)
	h.picker.script = []pickStep{{err: ErrCancelled}}
	if code := h.run("ws"); code != ExitFailure || len(h.execs) != n {
		t.Errorf("esc: exit %d, execs %d→%d", code, n, len(h.execs))
	}
}

// pickMoveHarness is moveHarness with the two chats given distinct ages
// (sid-main newer) so picker rows have a fixed order, and a scripted picker.
func pickMoveHarness(t *testing.T) (*harness, Deps, string) {
	t.Helper()
	h, d, project := moveHarness(t)
	proj := claude.HistoryDir(filepath.Join(h.home, ".claude"), project)
	for name, age := range map[string]time.Duration{"sid-main.jsonl": time.Minute, "sid-two.jsonl": time.Hour} {
		at := time.Now().Add(-age)
		if err := os.Chtimes(filepath.Join(proj, name), at, at); err != nil {
			t.Fatal(err)
		}
	}
	d.Picker = h.picker
	return h, d, project
}

func TestSessionsMove_Pick(t *testing.T) {
	t.Parallel()

	t.Run("this folder one chat with --to", func(t *testing.T) {
		t.Parallel()
		h, d, project := pickMoveHarness(t)
		h.picker.script = []pickStep{{idx: []int{1}}} // sid-two
		if code := runWith(h, d, project, "sessions", "move", "-i", "--to", "work", "--dry-run"); code != ExitOK {
			t.Fatalf("exit %d: %s", code, h.stderr.String())
		}
		// Only this folder's chats are offered, no workspace list first.
		if !slices.Equal(h.picker.prompts, []string{"move (tab: mark)"}) || len(h.picker.shown[0]) != 2 {
			t.Errorf("prompts %v, rows %v", h.picker.prompts, h.picker.shown)
		}
		out := h.stdout.String()
		if !strings.Contains(out, "would move 1 claude conversation(s)") || !strings.Contains(out, "sid-two") || !strings.Contains(out, "left in place") {
			t.Errorf("plan:\n%s", out)
		}
	})

	t.Run("several chats and a picked target", func(t *testing.T) {
		t.Parallel()
		h, d, project := pickMoveHarness(t)
		h.picker.script = []pickStep{{idx: []int{0, 1}}, {idx: []int{1}}} // both chats → work
		if code := runWith(h, d, project, "sessions", "move", "--pick", "--dry-run"); code != ExitOK {
			t.Fatalf("exit %d: %s", code, h.stderr.String())
		}
		targets := h.picker.shown[1]
		// Same-provider profiles only (no codex), in config order; the homes
		// holding the chats are marked.
		if len(targets) != 3 || !strings.HasPrefix(targets[0], "personal") || !strings.HasPrefix(targets[1], "work") || !strings.HasPrefix(targets[2], "kimi") {
			t.Fatalf("targets = %q", targets)
		}
		if !strings.Contains(targets[0], "← 2 chat(s) here now") || strings.Contains(targets[1], "here now") || !strings.Contains(targets[2], "← 2 chat(s) here now") {
			t.Errorf("marks = %q", targets)
		}
		if out := h.stdout.String(); !strings.Contains(out, "would move 2 claude conversation(s)") || !strings.Contains(out, "(work)") {
			t.Errorf("plan:\n%s", out)
		}
	})

	t.Run("no chats here is an error not a global list", func(t *testing.T) {
		t.Parallel()
		h, d, _ := pickMoveHarness(t)
		if code := runWith(h, d, h.root, "sessions", "move", "-i", "--to", "work", "--dry-run"); code != ExitFailure || !strings.Contains(h.stderr.String(), "use --all") {
			t.Errorf("exit %d: %s", code, h.stderr.String())
		}
		if len(h.picker.prompts) != 0 {
			t.Errorf("picker opened: %v", h.picker.prompts)
		}
	})

	t.Run("--all picks the folder first", func(t *testing.T) {
		t.Parallel()
		h, d, _ := pickMoveHarness(t)
		h.picker.script = []pickStep{{idx: []int{0}}, {idx: []int{0}}}
		if code := runWith(h, d, h.root, "sessions", "move", "-i", "--all", "--to", "work", "--dry-run"); code != ExitOK {
			t.Fatalf("exit %d: %s", code, h.stderr.String())
		}
		if !slices.Equal(h.picker.prompts, []string{"workspace", "move (tab: mark)"}) {
			t.Errorf("prompts = %v", h.picker.prompts)
		}
		if out := h.stdout.String(); !strings.Contains(out, "would move 1 claude conversation(s)") || !strings.Contains(out, "sid-main") {
			t.Errorf("plan:\n%s", out)
		}
	})

	t.Run("refused combinations and esc", func(t *testing.T) {
		t.Parallel()
		h, d, project := pickMoveHarness(t)
		for _, args := range [][]string{
			{"sessions", "move", "docker_setup_mac", "-i", "--to", "work"},
			{"sessions", "move", "-i", "--only", "sid-main", "--to", "work"},
		} {
			if code := runWith(h, d, project, args...); code != ExitUsage {
				t.Errorf("%v: exit %d, want usage", args, code)
			}
		}
		h.picker.script = []pickStep{{err: ErrCancelled}}
		if code := runWith(h, d, project, "sessions", "move", "-i", "--to", "work"); code != ExitFailure || strings.Contains(h.stdout.String(), "moved") {
			t.Errorf("esc: exit %d\n%s", code, h.stdout.String())
		}
		if _, err := os.Stat(filepath.Join(claude.HistoryDir(filepath.Join(h.home, ".claude"), project), "sid-main.jsonl")); err != nil {
			t.Error("esc moved something")
		}
	})
}

func TestPickNumberedMany(t *testing.T) {
	t.Parallel()
	items := []string{"a", "b", "c"}
	for _, tc := range []struct {
		input  string
		want   []int
		cancel bool
		bad    bool
	}{
		{"1 3\n", []int{0, 2}, false, false},
		{"1,3\n", []int{0, 2}, false, false},
		{" 2 , 1 \n", []int{1, 0}, false, false},
		{"2\n", []int{1}, false, false},
		{"\n", nil, true, false},
		{"", nil, true, false}, // EOF
		{"1 9\n", nil, false, true},
		{"x\n", nil, false, true},
	} {
		tty := &ttyStub{in: strings.NewReader(tc.input)}
		got, err := pickNumberedMany(tty, "move", items)
		switch {
		case tc.cancel:
			if err != ErrCancelled {
				t.Errorf("%q: err = %v, want cancel", tc.input, err)
			}
		case tc.bad:
			if err == nil || err == ErrCancelled {
				t.Errorf("%q: err = %v, want an error", tc.input, err)
			}
		default:
			if err != nil || !slices.Equal(got, tc.want) {
				t.Errorf("%q: = %v, %v; want %v", tc.input, got, err, tc.want)
			}
		}
	}
}

// TestPickMany_Fzf: --multi is passed and every selected line comes back.
func TestPickMany_Fzf(t *testing.T) {
	t.Parallel()
	bin := fakeFzf(t, `case "$*" in *--multi*) sed -n '1p;3p' ;; *) exit 2 ;; esac`)
	got, err := TerminalPicker{FzfBin: bin}.PickMany("move", []string{"a", "b", "c"})
	if err != nil || !slices.Equal(got, []int{0, 2}) {
		t.Errorf("PickMany = %v, %v", got, err)
	}
	if _, err := (TerminalPicker{FzfBin: fakeFzf(t, "exit 130")}).PickMany("move", []string{"a"}); err != ErrCancelled {
		t.Errorf("esc = %v", err)
	}
	if _, err := (TerminalPicker{}).PickMany("move", nil); err != ErrCancelled {
		t.Errorf("empty = %v", err)
	}
}
