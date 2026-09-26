package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/secret"
)

// fakeProv is a provider whose accounts, usage and conversations are fixed.
type fakeProv struct {
	id    provider.ID
	disc  []provider.Profile
	usage map[string]provider.Usage // by home
	errs  map[string]error          // by home
	convs map[string][]provider.Conversation
	// moved records MoveHistory calls.
	mu    sync.Mutex
	moved [][2]string
	// history is the set of dirs with history, by home.
	history map[string]map[string]bool
	// subs are subscriptions by home (nil map = zero subscription).
	subs map[string]provider.Subscription
}

func (f *fakeProv) ID() provider.ID                             { return f.id }
func (f *fakeProv) Binary() string                              { return string(f.id) + "-bin" }
func (f *fakeProv) DefaultHome(home string) string              { return filepath.Join(home, "."+string(f.id)) }
func (f *fakeProv) Discover(string) ([]provider.Profile, error) { return f.disc, nil }
func (f *fakeProv) Identity(p provider.Profile) (provider.Identity, error) {
	u := f.usage[p.Home]
	return provider.Identity{Email: u.Identity.Email, Plan: u.Identity.Plan, LoggedIn: true}, nil
}
func (f *fakeProv) Usage(_ context.Context, p provider.Profile) (provider.Usage, error) {
	if p.Billing == provider.BillingAPI {
		return provider.Usage{}, provider.ErrAPIBilled
	}
	if err := f.errs[p.Home]; err != nil {
		return provider.Usage{}, err
	}
	return f.usage[p.Home], nil
}
func (f *fakeProv) Launch(p provider.Profile, req provider.LaunchRequest) (provider.Command, error) {
	args := append([]string{}, p.Args...)
	if req.ResumeID != "" {
		args = append(args, "--resume", req.ResumeID)
	}
	args = append(args, req.Args...)
	env := provider.SetEnv(req.Env, "HOME_DIR", p.Home)
	for k, v := range p.Env {
		env = provider.SetEnv(env, k, v)
	}
	return provider.Command{Path: f.Binary(), Args: args, Env: env, Dir: req.Dir}, nil
}
func (f *fakeProv) Conversations(_ context.Context, home string, q provider.ConversationQuery) ([]provider.Conversation, error) {
	var out []provider.Conversation
	for _, c := range f.convs[home] {
		if q.Dir == "" || c.Dir == q.Dir {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *fakeProv) HasHistory(home, dir string) bool { return f.history[home][dir] }
func (f *fakeProv) Subscription(_ context.Context, p provider.Profile) (provider.Subscription, error) {
	if err := f.errs[p.Home]; err != nil {
		return provider.Subscription{}, err
	}
	return f.subs[p.Home], nil
}
func (f *fakeProv) MoveHistory(home, from, to string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.moved = append(f.moved, [2]string{from, to})
	return true, nil
}

// fakePicker returns a fixed choice.
type fakePicker struct {
	choice int
	err    error
	items  []string
}

func (p *fakePicker) Pick(_ string, items []string) (int, error) {
	p.items = items
	return p.choice, p.err
}

// mapSecrets resolves from a map.
type mapSecrets map[string]string

func (m mapSecrets) Resolve(_ context.Context, ref secret.Ref) (string, error) {
	v, ok := m[string(ref.Scheme)+":"+ref.Path]
	if !ok {
		return "", errors.New("no such secret")
	}
	return v, nil
}

// harness is one test's machine: temp dirs, fakes and captured output.
type harness struct {
	t        *testing.T
	root     string
	home     string
	cfgPath  string
	sessions string
	cwd      string
	env      map[string]string
	claude   *fakeProv
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	execs    []provider.Command
	picker   *fakePicker
	now      time.Time
}

func newHarness(t *testing.T, configYAML string) *harness {
	t.Helper()
	root := t.TempDir()
	h := &harness{
		t: t, root: root,
		home:     filepath.Join(root, "home"),
		cfgPath:  filepath.Join(root, "cfg", "config.yaml"),
		sessions: filepath.Join(root, "sessions"),
		cwd:      filepath.Join(root, "cwd"),
		env:      map[string]string{"PATH": "/bin"},
		picker:   &fakePicker{},
		now:      time.Date(2026, 9, 24, 12, 0, 0, 0, time.Local),
	}
	for _, d := range []string{h.home, filepath.Dir(h.cfgPath), h.cwd} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	personal := filepath.Join(h.home, ".claude")
	work := filepath.Join(h.home, ".claude-work")
	h.claude = &fakeProv{
		id: provider.Claude,
		disc: []provider.Profile{
			{Name: "personal", Provider: provider.Claude, Home: personal, Billing: provider.BillingPlan, Default: true, Source: provider.SourceDiscovered},
			{Name: "work", Provider: provider.Claude, Home: work, Billing: provider.BillingPlan, Source: provider.SourceDiscovered},
		},
		usage: map[string]provider.Usage{
			personal: {Identity: provider.Identity{Email: "me@home"}, Windows: []provider.Window{{Group: provider.GroupSession, Label: "Current session", Percent: 5}, {Group: provider.GroupWeekly, Label: "All models", Percent: 77}}},
			work:     {Identity: provider.Identity{Email: "me@work"}, Windows: []provider.Window{{Group: provider.GroupSession, Label: "Current session", Percent: 9}}},
		},
		errs:    map[string]error{},
		convs:   map[string][]provider.Conversation{},
		history: map[string]map[string]bool{},
	}
	body := fmt.Sprintf("sessions:\n  root: %s\n  promote_root: %s\n%s", h.sessions, filepath.Join(root, "projects"), configYAML)
	if err := os.WriteFile(h.cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) deps() Deps {
	return Deps{
		Stdout: &h.stdout, Stderr: &h.stderr, Stdin: strings.NewReader(""),
		UserHome:   h.home,
		Providers:  []provider.Provider{h.claude},
		ConfigPath: func() (string, string, error) { return h.cfgPath, "test", nil },
		Now:        func() time.Time { return h.now },
		Getwd:      func() (string, error) { return h.cwd, nil },
		LookupEnv: func(k string) (string, bool) {
			v, ok := h.env[k]
			return v, ok
		},
		Environ: func() []string {
			var out []string
			for k, v := range h.env {
				out = append(out, k+"="+v)
			}
			slices.Sort(out)
			return out
		},
		Exec: func(c provider.Command) error {
			h.execs = append(h.execs, c)
			return nil
		},
		Secrets:    mapSecrets{"gopass:ai/kimi": "sk-secret-value"},
		Picker:     h.picker,
		IsTerminal: func(io.Writer) bool { return false },
		LookPath: func(name string) (string, error) {
			if name == "missing-bin" {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
	}
}

// run executes agx with args and returns the exit code.
func (h *harness) run(args ...string) int {
	h.t.Helper()
	h.stdout.Reset()
	h.stderr.Reset()
	root := NewRoot(h.deps())
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	if err != nil && !IsSilent(err) {
		fmt.Fprintln(&h.stderr, "agx:", err)
	}
	return ExitCode(err)
}

func (h *harness) lastExec() provider.Command {
	h.t.Helper()
	if len(h.execs) == 0 {
		h.t.Fatalf("nothing was executed; stderr: %s", h.stderr.String())
	}
	return h.execs[len(h.execs)-1]
}

// envelope is the voltkit output envelope, decoded with a typed payload.
type envelope[T any] struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Count         int    `json:"count"`
	Data          T      `json:"data"`
}

func decode[T any](t *testing.T, b []byte) envelope[T] {
	t.Helper()
	var e envelope[T]
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, b)
	}
	return e
}

func TestUsage_TextAndBareCommand(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	if code := h.run(); code != ExitOK {
		t.Fatalf("exit %d, stderr %s", code, h.stderr.String())
	}
	out := h.stdout.String()
	for _, want := range []string{"me@home", "personal · claude", "77% used", "me@work", "Weekly limits"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	bare := out
	if h.run("usage"); h.stdout.String() != bare {
		t.Error("`agx` and `agx usage` differ")
	}
}

func TestUsage_JSONAndPartialFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.claude.errs[filepath.Join(h.home, ".claude-work")] = provider.ErrExpired
	code := h.run("usage", "--json")
	if code != ExitFailure {
		t.Errorf("exit %d, want %d when one account fails", code, ExitFailure)
	}
	e := decode[[]usageJSON](t, h.stdout.Bytes())
	if e.Kind != kindUsageList || e.Count != 2 || e.SchemaVersion != 1 {
		t.Fatalf("envelope = %+v", e)
	}
	if e.Data[0].Profile != "personal" || len(e.Data[0].Windows) != 2 || e.Data[1].Error == "" || !strings.Contains(e.Data[1].Error, "agx run -p work") {
		t.Errorf("data = %+v", e.Data)
	}
}

func TestUsage_APIBilledIsNotAFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "profiles:\n  - {name: kimi, provider: claude, billing: api}\n")
	if code := h.run("usage", "kimi"); code != ExitOK || !strings.Contains(h.stdout.String(), "billed per token") {
		t.Errorf("exit %d, out %s", code, h.stdout.String())
	}
}

func TestUsage_UsageErrors(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	for _, args := range [][]string{
		{"usage", "nobody"},
		{"usage", "--color=rainbow"},
		{"usage", "--timeout=0s"},
		{"--no-such-flag"},
		{"not-a-command"},
		{"sessions", "promote", "only-one-arg"},
	} {
		if code := h.run(args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d (stderr %s)", args, code, ExitUsage, h.stderr.String())
		}
	}
}

func TestProfiles_JSON(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "profiles:\n  - {name: kimi, provider: claude, billing: api, secrets: {TOKEN: 'gopass:ai/kimi'}}\n")
	if code := h.run("profiles", "--json"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	e := decode[[]profileJSON](t, h.stdout.Bytes())
	if e.Kind != kindProfileList || len(e.Data) != 1 || e.Data[0].Name != "kimi" || !slices.Equal(e.Data[0].SecretVars, []string{"TOKEN"}) {
		t.Errorf("profiles = %+v", e.Data)
	}
	if strings.Contains(h.stdout.String(), "sk-secret-value") {
		t.Error("secret value leaked into profiles output")
	}
}

func TestRun_ProfilesAndSecrets(t *testing.T) {
	t.Parallel()
	h := newHarness(t, `
providers:
  claude: {args: [--flag]}
profiles:
  - {name: personal, provider: claude, home: HOME/.claude}
  - name: kimi
    provider: claude
    home: HOME/.claude
    billing: api
    env: {BASE: moon}
    secrets: {TOKEN: 'gopass:ai/kimi'}
`)
	// Rewrite HOME placeholders now that the harness knows its home.
	raw, _ := os.ReadFile(h.cfgPath)
	if err := os.WriteFile(h.cfgPath, bytes.ReplaceAll(raw, []byte("HOME/"), []byte(h.home+"/")), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := h.run("run", "--", "--extra"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	c := h.lastExec()
	if c.Path != "claude-bin" || strings.Join(c.Args, " ") != "--flag --extra" || c.Dir != h.cwd {
		t.Errorf("default run = %+v", c)
	}

	if code := h.run("run", "-p", "kimi"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	c = h.lastExec()
	if !slices.Contains(c.Env, "TOKEN=sk-secret-value") || !slices.Contains(c.Env, "BASE=moon") {
		t.Errorf("kimi env = %v", c.Env)
	}

	// Dry run shows the secret's scheme, never its value, and execs nothing.
	n := len(h.execs)
	if code := h.run("run", "-p", "kimi", "--dry-run"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(h.execs) != n || strings.Contains(h.stdout.String(), "sk-secret-value") || !strings.Contains(h.stdout.String(), "TOKEN=<secret:gopass>") {
		t.Errorf("dry run:\n%s", h.stdout.String())
	}
}

func TestRun_Auto(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	if code := h.run("run", "-p", "auto"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	if !slices.Contains(h.lastExec().Env, "HOME_DIR="+filepath.Join(h.home, ".claude-work")) {
		t.Errorf("auto did not pick work: %v", h.lastExec().Env)
	}
	if !strings.Contains(h.stderr.String(), "auto → work (max 9%) over personal (max 77%)") {
		t.Errorf("no explanation: %s", h.stderr.String())
	}
	// Every account failing is a clear error, not a random pick.
	h.claude.errs[filepath.Join(h.home, ".claude")] = provider.ErrExpired
	h.claude.errs[filepath.Join(h.home, ".claude-work")] = provider.ErrExpired
	if code := h.run("run", "-p", "auto"); code == ExitOK {
		t.Error("auto with no eligible account should fail")
	}
}

func TestNew_DirectAndPlan(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	if code := h.run("new", "-p", "work", "My", "Topic"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	want := filepath.Join(h.sessions, "my_topic_20260924_120000")
	if c := h.lastExec(); c.Dir != want {
		t.Errorf("new ran in %s, want %s", c.Dir, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Error("session folder not created")
	}

	// Plan mode: write the plan, launch nothing.
	planPath := filepath.Join(h.root, "plan.json")
	h.env[appmeta.EnvPlanFile] = planPath
	n := len(h.execs)
	if code := h.run("new", "-p", "work", "planned"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	if len(h.execs) != n {
		t.Error("plan mode launched")
	}
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var pl plan
	if err := json.Unmarshal(raw, &pl); err != nil || pl.Version != planVersion || pl.Profile != "work" || filepath.Base(pl.Dir) != "planned_20260924_120000" {
		t.Errorf("plan = %+v, %v", pl, err)
	}
	if info, _ := os.Stat(planPath); info.Mode().Perm() != planPerm {
		t.Errorf("plan perm = %v", info.Mode().Perm())
	}
	delete(h.env, appmeta.EnvPlanFile)

	// exec --print-dir keeps the file; exec runs it and deletes it.
	if code := h.run("exec", "--plan", planPath, "--print-dir"); code != ExitOK || strings.TrimSpace(h.stdout.String()) != pl.Dir {
		t.Errorf("print-dir: %d %q", code, h.stdout.String())
	}
	if code := h.run("exec", "--plan", planPath); code != ExitOK {
		t.Fatalf("exec: %d %s", code, h.stderr.String())
	}
	if c := h.lastExec(); c.Dir != pl.Dir || !slices.Contains(c.Env, "HOME_DIR="+filepath.Join(h.home, ".claude-work")) {
		t.Errorf("exec = %+v", c)
	}
	if _, err := os.Stat(planPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("plan file not deleted after exec")
	}
}

func TestExec_RejectsStalePlan(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	planPath := filepath.Join(h.root, "old.json")
	if err := os.WriteFile(planPath, []byte(`{"version":99,"dir":"/x","profile":"work"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := h.run("exec", "--plan", planPath); code == ExitOK || !strings.Contains(h.stderr.String(), "shell-init") {
		t.Errorf("stale plan accepted: %d %s", code, h.stderr.String())
	}
}

func TestResume(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	personal := filepath.Join(h.home, ".claude")
	work := filepath.Join(h.home, ".claude-work")
	other := filepath.Join(h.root, "other")
	for _, d := range []string{other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	h.claude.convs[personal] = []provider.Conversation{
		{Provider: provider.Claude, Home: personal, ID: "p-old", Title: "Old personal", Dir: h.cwd, Updated: h.now.Add(-3 * time.Hour)},
		{Provider: provider.Claude, Home: personal, ID: "p-else", Title: "Elsewhere pglite", Dir: other, Updated: h.now.Add(-time.Hour)},
	}
	h.claude.convs[work] = []provider.Conversation{
		{Provider: provider.Claude, Home: work, ID: "w-new", Title: "Work thing", Dir: h.cwd, Updated: h.now.Add(-time.Minute)},
	}

	// --last in cwd picks the newest across homes and routes it to work.
	if code := h.run("resume", "--last"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	c := h.lastExec()
	if !slices.Contains(c.Args, "w-new") || !slices.Contains(c.Env, "HOME_DIR="+work) || c.Dir != h.cwd {
		t.Errorf("resume --last = %+v", c)
	}

	// The picker sees only this folder's two conversations; choice 1 = older.
	h.picker.choice = 1
	if code := h.run("resume"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(h.picker.items) != 2 || !slices.Contains(h.lastExec().Args, "p-old") {
		t.Errorf("picker items %v, exec %+v", h.picker.items, h.lastExec())
	}

	// A query searches everywhere when --all; it finds the other folder.
	if code := h.run("resume", "--all", "--last", "pglite"); code != ExitOK || h.lastExec().Dir != other {
		t.Errorf("query resume: %d %+v", code, h.lastExec())
	}

	// Cancelling the picker exits non-zero and launches nothing.
	h.picker.err = ErrCancelled
	n := len(h.execs)
	if code := h.run("resume"); code == ExitOK || len(h.execs) != n {
		t.Errorf("cancel: exit %d, execs %d→%d", code, n, len(h.execs))
	}

	// --list --json prints and never launches.
	h.picker.err = nil
	if code := h.run("resume", "--all", "--list", "--json"); code != ExitOK || len(h.execs) != n {
		t.Fatalf("list: %d", code)
	}
	e := decode[[]provider.Conversation](t, h.stdout.Bytes())
	if e.Kind != kindConversationList || len(e.Data) != 3 || e.Data[0].ID != "w-new" {
		t.Errorf("list = %+v", e)
	}
}

func TestResume_FolderGone(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	home := filepath.Join(h.home, ".claude")
	h.claude.convs[home] = []provider.Conversation{{Provider: provider.Claude, Home: home, ID: "x", Dir: filepath.Join(h.root, "deleted"), Updated: h.now}}
	if code := h.run("resume", "--all", "--last"); code == ExitOK || !strings.Contains(h.stderr.String(), "no longer exists") {
		t.Errorf("exit %d: %s", code, h.stderr.String())
	}
}

func TestSessions_LsGCPromote(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	mk := func(name string, file bool) string {
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
	empty := mk("20260101_000000", false)
	chat := mk("chat_20260101_000001", false)
	work := mk("tool_20260101_000002", true)
	personal := filepath.Join(h.home, ".claude")
	h.claude.history[personal] = map[string]bool{chat: true, work: true}

	if code := h.run("sessions", "ls", "--json"); code != ExitOK {
		t.Fatalf("ls: %d %s", code, h.stderr.String())
	}
	if e := decode[[]json.RawMessage](t, h.stdout.Bytes()); e.Kind != kindSessionList || e.Count != 3 {
		t.Errorf("ls = %+v", e)
	}

	// Plan only: nothing removed.
	if code := h.run("sessions", "gc"); code != ExitOK || strings.TrimSpace(h.stdout.String()) != "20260101_000000" {
		t.Errorf("gc plan: %d %q", code, h.stdout.String())
	}
	if _, err := os.Stat(empty); err != nil {
		t.Fatal("gc without --yes removed a folder")
	}
	if code := h.run("sessions", "gc", "--yes"); code != ExitOK {
		t.Fatalf("gc --yes: %d", code)
	}
	if _, err := os.Stat(empty); !errors.Is(err, os.ErrNotExist) {
		t.Error("empty folder survived gc --yes")
	}
	if _, err := os.Stat(chat); err != nil {
		t.Error("folder with history was removed")
	}

	// Promote dry run changes nothing; the real run moves folder + history.
	if code := h.run("sessions", "promote", "tool_20260101_000002", "tool", "--dry-run"); code != ExitOK || !strings.Contains(h.stdout.String(), "would move") {
		t.Fatalf("promote dry: %d %s", code, h.stdout.String())
	}
	if _, err := os.Stat(work); err != nil || len(h.claude.moved) != 0 {
		t.Fatal("dry run changed something")
	}
	if code := h.run("sessions", "promote", "tool_20260101_000002", "tool"); code != ExitOK {
		t.Fatalf("promote: %d %s", code, h.stderr.String())
	}
	dst := filepath.Join(h.root, "projects", "tool")
	if _, err := os.Stat(filepath.Join(dst, "main.go")); err != nil {
		t.Error("folder not moved")
	}
	if len(h.claude.moved) != 1 || h.claude.moved[0] != [2]string{work, dst} {
		t.Errorf("history moves = %v", h.claude.moved)
	}
	// Never overwrite; never escape the destination parent.
	mk("again_20260101_000003", true)
	if code := h.run("sessions", "promote", "again_20260101_000003", "tool"); code == ExitOK {
		t.Error("promote overwrote an existing project")
	}
	if code := h.run("sessions", "promote", "again_20260101_000003", "../escape"); code != ExitUsage {
		t.Errorf("path-escaping name: exit %d", code)
	}
}

func TestDoctor(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	if code := h.run("doctor", "--json"); code != ExitOK {
		t.Fatalf("doctor: %d %s", code, h.stdout.String())
	}
	e := decode[[]finding](t, h.stdout.Bytes())
	if e.Kind != kindDoctorReport || len(e.Data) < 4 {
		t.Errorf("findings = %+v", e.Data)
	}

	// A missing gopass for a gopass secret is a failure → exit 1.
	h2 := newHarness(t, "profiles:\n  - {name: kimi, provider: claude, billing: api, secrets: {T: 'gopass:x'}}\n")
	d := h2.deps()
	d.LookPath = func(name string) (string, error) {
		if name == secret.GopassBin {
			return "", errors.New("missing")
		}
		return "/bin/" + name, nil
	}
	root := NewRoot(d)
	root.SetArgs([]string{"doctor"})
	if err := root.Execute(); ExitCode(err) != ExitFailure || !strings.Contains(h2.stdout.String(), "needs gopass") {
		t.Errorf("doctor without gopass: %v\n%s", err, h2.stdout.String())
	}
}

func TestShellInit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "shell:\n  aliases:\n    cl: run -p personal\n")
	if code := h.run("shell-init"); code != ExitOK || !strings.Contains(h.stdout.String(), `cl() { agx run -p personal "$@"; }`) {
		t.Errorf("shell-init: %d\n%s", code, h.stdout.String())
	}
	if code := h.run("shell-init", "fish"); code != ExitUsage {
		t.Errorf("fish: exit %d", code)
	}
}

func TestConfigErrorIsUsageError(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "profiles: [{name: BAD, provider: claude}]\n")
	if code := h.run("profiles"); code != ExitUsage || !strings.Contains(h.stderr.String(), h.cfgPath) {
		t.Errorf("exit %d: %s", code, h.stderr.String())
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if truncate("héllo", 10) != "héllo" || truncate("héllo world", 5) != "héll…" {
		t.Errorf("truncate = %q / %q", truncate("héllo", 10), truncate("héllo world", 5))
	}
}
