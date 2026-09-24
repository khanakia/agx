package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// TestPromoteThenResume is the end-to-end promise of `sessions promote`,
// run against the REAL Claude provider on a temp home: after promoting a
// session folder, `agx resume` from the NEW folder finds the conversation
// (whose transcript still records the OLD cwd) and resumes it there, on the
// account that stores it.
func TestPromoteThenResume(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	workHome := filepath.Join(h.home, ".claude-work")
	oldDir := filepath.Join(h.sessions, "pglite_spike_20260924_101500")
	newDir := filepath.Join(h.root, "projects", "pglite-go")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A real Claude transcript, recorded while the folder was still at oldDir.
	transcript := filepath.Join(claude.HistoryDir(workHome, oldDir), "sid-123.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"attachment","cwd":"` + oldDir + `","sessionId":"sid-123"}` + "\n" + `{"type":"ai-title","aiTitle":"Explore PGlite"}` + "\n"
	if err := os.WriteFile(transcript, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	d := h.deps()
	d.Providers = []provider.Provider{&claude.Provider{UserHome: h.home}} // real provider, file-only logins
	run := func(cwd string, args ...string) int {
		h.stdout.Reset()
		h.stderr.Reset()
		d.Getwd = func() (string, error) { return cwd, nil }
		root := NewRoot(d)
		root.SetArgs(args)
		return ExitCode(root.Execute())
	}

	if code := run(h.cwd, "sessions", "promote", "pglite_spike_20260924_101500", "pglite-go"); code != ExitOK {
		t.Fatalf("promote: exit %d\n%s%s", code, h.stdout.String(), h.stderr.String())
	}
	if !strings.Contains(h.stdout.String(), "history in") {
		t.Errorf("promote did not report moving history:\n%s", h.stdout.String())
	}
	if _, err := os.Stat(filepath.Join(newDir, "main.go")); err != nil {
		t.Fatal("folder not moved")
	}
	if _, err := os.Stat(claude.HistoryDir(workHome, newDir)); err != nil {
		t.Fatal("history not re-keyed to the new folder")
	}

	// From the new folder, resume finds it without --all and launches there, on work.
	if code := run(newDir, "resume", "--last"); code != ExitOK {
		t.Fatalf("resume: exit %d\n%s", code, h.stderr.String())
	}
	c := h.lastExec()
	if c.Dir != newDir || !slices.Contains(c.Args, "sid-123") || !slices.Contains(c.Env, claude.ConfigDirEnv+"="+workHome) {
		t.Errorf("resume after promote = %+v", c)
	}
	if strings.Contains(h.stderr.String(), "showing recent ones everywhere") {
		t.Error("resume fell back to searching everywhere instead of finding the folder's conversation")
	}
}
