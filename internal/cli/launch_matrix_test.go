package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/provider"
)

// plainProv exposes only the base Provider methods of the wrapped provider:
// embedding the interface hides every optional upgrade (conversations,
// history, logins), the shape of a minimal third-party provider.
type plainProv struct{ provider.Provider }

// failingLister is a provider whose conversation listing fails for every
// home, as an unreadable history directory would.
type failingLister struct{ *fakeProv }

func (failingLister) Conversations(context.Context, string, provider.ConversationQuery) ([]provider.Conversation, error) {
	return nil, errors.New("history unreadable")
}

// emptyProv is a provider with no accounts on disk.
func emptyProv(id provider.ID) *fakeProv {
	return &fakeProv{id: id, usage: map[string]provider.Usage{}, errs: map[string]error{}, convs: map[string][]provider.Conversation{}, history: map[string]map[string]bool{}}
}

// TestRun_ResolveProfileMatrix: -p × --provider, including a provider that
// is not registered and one registered with no account. Each wrong request
// is a usage error (exit 2) and launches nothing.
func TestRun_ResolveProfileMatrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, profile, prov string
		code                int
		wantHome            string // base name of the launched home
		note                string
	}{
		{name: "default profile", wantHome: ".claude"},
		{name: "named profile", profile: "work", wantHome: ".claude-work"},
		{name: "explicit provider", prov: "claude", wantHome: ".claude"},
		{name: "auto", profile: "auto", wantHome: ".claude-work"},
		{name: "unknown profile", profile: "ghost", code: ExitUsage, note: `unknown profile "ghost"`},
		{name: "unicode profile", profile: "wörk", code: ExitUsage, note: `unknown profile "wörk"`},
		{name: "case matters", profile: "Personal", code: ExitUsage, note: "unknown profile"},
		{name: "unregistered provider", prov: "gemini", code: ExitUsage, note: `unknown provider "gemini"`},
		{name: "provider without accounts", prov: "codex", code: ExitUsage, note: "no codex profile found"},
	} {
		h := newHarness(t, "")
		h.provs = []provider.Provider{h.claude, plainProv{emptyProv(provider.Codex)}}
		args := []string{"run"}
		if c.profile != "" {
			args = append(args, "-p", c.profile)
		}
		if c.prov != "" {
			args = append(args, "--provider", c.prov)
		}
		code := h.run(args...)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d; stderr %s", c.name, code, c.code, h.stderr.String())
			continue
		}
		if c.note != "" && !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("%s: stderr %q lacks %q", c.name, h.stderr.String(), c.note)
		}
		if c.code != ExitOK {
			if len(h.execs) != 0 {
				t.Errorf("%s: a refused run launched %+v", c.name, h.execs)
			}
			continue
		}
		if !slices.Contains(h.lastExec().Env, "HOME_DIR="+filepath.Join(h.home, c.wantHome)) {
			t.Errorf("%s: launched %v", c.name, h.lastExec().Env)
		}
	}
}

// writePlan writes a raw plan file and returns its path.
func writePlan(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(p, []byte(body), planPerm); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestExec_PlanMatrix: every malformed plan is refused before anything
// runs, and a well-formed plan is consumed (deleted) even when its profile
// no longer exists, so a stale plan can never be replayed.
func TestExec_PlanMatrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		body     string // "" = no file at all
		noFlag   bool
		code     int
		note     string
		consumed bool // the file is gone afterwards
	}{
		{name: "no --plan", noFlag: true, code: ExitUsage, note: "--plan is required"},
		{name: "missing file", code: ExitFailure, note: "read plan"},
		{name: "not json", body: "plan?", code: ExitFailure, note: "decode plan"},
		{name: "json array", body: "[1]", code: ExitFailure, note: "decode plan"},
		{name: "empty object", body: "{}", code: ExitFailure, note: "plan version 0"},
		{name: "future version", body: `{"version":2,"profile":"work"}`, code: ExitFailure, note: "plan version 2"},
		{name: "unknown profile", body: `{"version":1,"profile":"ghost","dir":"/x"}`, code: ExitFailure, note: `plan names unknown profile "ghost"`, consumed: true},
		{name: "unicode profile", body: `{"version":1,"profile":"wörk"}`, code: ExitFailure, note: "unknown profile", consumed: true},
		{name: "ok", body: `{"version":1,"profile":"work","resume_id":"r-1","args":["--x"]}`, consumed: true},
	} {
		h := newHarness(t, "")
		path := filepath.Join(h.root, "plan.json")
		if c.body != "" {
			path = writePlan(t, h.root, c.body)
		}
		args := []string{"exec"}
		if !c.noFlag {
			args = append(args, "--plan", path)
		}
		code := h.run(args...)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d; stderr %s", c.name, code, c.code, h.stderr.String())
		}
		if c.note != "" && !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("%s: stderr %q lacks %q", c.name, h.stderr.String(), c.note)
		}
		_, statErr := os.Stat(path)
		if gone := errors.Is(statErr, os.ErrNotExist); c.body != "" && gone != c.consumed {
			t.Errorf("%s: plan consumed=%v, want %v", c.name, gone, c.consumed)
		}
		if c.code != ExitOK && len(h.execs) != 0 {
			t.Errorf("%s: refused plan launched %+v", c.name, h.execs)
		}
		if c.code == ExitOK {
			if got := h.lastExec().Args; !slices.Equal(got, []string{"--resume", "r-1", "--x"}) {
				t.Errorf("%s: args %v", c.name, got)
			}
		}
	}
}

// TestExec_PrintDirKeepsPlan: --print-dir answers from the plan and leaves
// it for the real exec, whose directory the shell cds into first.
func TestExec_PrintDirKeepsPlan(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	dir := filepath.Join(h.root, "dir with spaces", "ünïcode")
	raw, err := json.Marshal(plan{Version: planVersion, Dir: dir, Profile: "work"})
	if err != nil {
		t.Fatal(err)
	}
	path := writePlan(t, h.root, string(raw))
	for range 2 {
		if code := h.run("exec", "--plan", path, "--print-dir"); code != ExitOK || h.stdout.String() != dir+"\n" {
			t.Errorf("print-dir: %d %q", code, h.stdout.String())
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("print-dir consumed the plan: %v", err)
	}
	if len(h.execs) != 0 {
		t.Error("print-dir launched")
	}
}

// resumeHarness has conversations in cwd on both homes plus one in a home
// no profile uses.
func resumeHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, "")
	personal := filepath.Join(h.home, ".claude")
	work := filepath.Join(h.home, ".claude-work")
	stray := filepath.Join(h.home, ".claude-stray")
	h.claude.convs[personal] = []provider.Conversation{
		{Provider: provider.Claude, Home: personal, ID: "p-1", Title: "Fix the parser", Dir: h.cwd, Updated: h.now.Add(-3 * time.Hour)},
		{Provider: provider.Claude, Home: personal, ID: "p-2", Title: "Über refactor ✨", Dir: h.cwd, Updated: h.now.Add(-2 * time.Hour)},
	}
	h.claude.convs[work] = []provider.Conversation{
		{Provider: provider.Claude, Home: work, ID: "w-1", Title: "Deploy notes", Dir: h.cwd, Updated: h.now.Add(-time.Hour)},
		// Listed under work by the fake, but stored in a home no profile names.
		{Provider: provider.Claude, Home: stray, ID: "s-1", Title: "Stray chat", Dir: h.cwd, Updated: h.now.Add(-4 * time.Hour)},
	}
	return h
}

// TestResume_QueryAndProfileMatrix: query × -p × --last, including a
// conversation whose home has no profile.
func TestResume_QueryAndProfileMatrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		args     []string
		code     int
		wantID   string // resumed conversation; "" when nothing runs
		wantHome string
		note     string
	}{
		{name: "newest", args: []string{"--last"}, wantID: "w-1", wantHome: ".claude-work"},
		{name: "query", args: []string{"--last", "parser"}, wantID: "p-1", wantHome: ".claude"},
		{name: "query case-insensitive", args: []string{"--last", "PARSER"}, wantID: "p-1", wantHome: ".claude"},
		{name: "query by id", args: []string{"--last", "w-1"}, wantID: "w-1", wantHome: ".claude-work"},
		{name: "every word must match", args: []string{"--last", "fix", "deploy"}, code: ExitFailure, note: "no matching conversations"},
		{name: "unicode query", args: []string{"--last", "über"}, wantID: "p-2", wantHome: ".claude"},
		{name: "emoji query", args: []string{"--last", "✨"}, wantID: "p-2", wantHome: ".claude"},
		{name: "no match", args: []string{"--last", "nothing-like-this"}, code: ExitFailure, note: "no matching conversations"},
		{name: "-p overrides home", args: []string{"--last", "-p", "personal"}, wantID: "w-1", wantHome: ".claude"},
		{name: "-p unknown", args: []string{"--last", "-p", "ghost"}, code: ExitUsage, note: `unknown profile "ghost"`},
		{name: "home without profile", args: []string{"--last", "stray"}, code: ExitFailure, note: ".claude-stray (claude); add one to the config"},
		{name: "home without profile, -p rescues", args: []string{"--last", "stray", "-p", "work"}, wantID: "s-1", wantHome: ".claude-work"},
	} {
		h := resumeHarness(t)
		code := h.run(append([]string{"resume"}, c.args...)...)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d; stderr %s", c.name, code, c.code, h.stderr.String())
			continue
		}
		if c.note != "" && !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("%s: stderr %q lacks %q", c.name, h.stderr.String(), c.note)
		}
		if c.wantID == "" {
			if len(h.execs) != 0 {
				t.Errorf("%s: launched %+v", c.name, h.execs)
			}
			continue
		}
		e := h.lastExec()
		if !slices.Contains(e.Args, c.wantID) || !slices.Contains(e.Env, "HOME_DIR="+filepath.Join(h.home, c.wantHome)) {
			t.Errorf("%s: exec %v env %v", c.name, e.Args, e.Env)
		}
	}
}

// TestResume_LimitAndList: --limit cuts after the query filter, so an old
// match is still found; --limit 0 means no limit.
func TestResume_LimitAndList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		args  []string
		count int
		first string
	}{
		{name: "limit 1", args: []string{"--limit", "1"}, count: 1, first: "w-1"},
		{name: "limit 2", args: []string{"--limit", "2"}, count: 2, first: "w-1"},
		{name: "limit 0 is unlimited", args: []string{"--limit", "0"}, count: 4, first: "w-1"},
		{name: "limit larger than list", args: []string{"--limit", "99"}, count: 4, first: "w-1"},
		{name: "query beats limit", args: []string{"--limit", "1", "stray"}, count: 1, first: "s-1"},
	} {
		h := resumeHarness(t)
		if code := h.run(append([]string{"resume", "--list", "--json"}, c.args...)...); code != ExitOK {
			t.Errorf("%s: exit %d %s", c.name, code, h.stderr.String())
			continue
		}
		e := decode[[]conversationView](t, h.stdout.Bytes())
		if len(e.Data) != c.count || e.Data[0].ID != c.first {
			t.Errorf("%s: %d rows, first %+v", c.name, len(e.Data), e.Data)
		}
	}
}

// TestResume_DryRunAndPlan: a dry run prints and launches nothing; plan
// mode writes the resume id, agent args and login into the plan.
func TestResume_DryRunAndPlan(t *testing.T) {
	t.Parallel()
	h := resumeHarness(t)
	if code := h.run("resume", "--last", "--dry-run"); code != ExitOK || len(h.execs) != 0 || !strings.Contains(h.stdout.String(), "w-1") {
		t.Errorf("dry run: %d %d %q", code, len(h.execs), h.stdout.String())
	}
	planPath := filepath.Join(h.root, "resume-plan.json")
	h.env[appmeta.EnvPlanFile] = planPath
	if code := h.run("resume", "--last", "--login", "home", "parser", "--", "--verbose"); code != ExitOK || len(h.execs) != 0 {
		t.Fatalf("plan: %d %d %s", code, len(h.execs), h.stderr.String())
	}
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var pl plan
	if err := json.Unmarshal(raw, &pl); err != nil {
		t.Fatal(err)
	}
	want := plan{Version: planVersion, Dir: h.cwd, Profile: "personal", ResumeID: "p-1", Args: []string{"--verbose"}, Login: "home"}
	if pl.Version != want.Version || pl.Dir != want.Dir || pl.Profile != want.Profile || pl.ResumeID != want.ResumeID || !slices.Equal(pl.Args, want.Args) || pl.Login != want.Login {
		t.Errorf("plan = %+v, want %+v", pl, want)
	}
	// An empty plan variable means no shell layer: launch directly.
	h.env[appmeta.EnvPlanFile] = ""
	if code := h.run("resume", "--last"); code != ExitOK || len(h.execs) != 1 {
		t.Errorf("empty plan var: %d %d", code, len(h.execs))
	}
}

// TestResume_ListerUpgradeOptional: a provider without the conversation
// upgrade is skipped, and one whose listing fails warns without hiding the
// conversations of every other provider.
func TestResume_ListerUpgradeOptional(t *testing.T) {
	t.Parallel()
	h := resumeHarness(t)
	codex := emptyProv(provider.Codex)
	codex.disc = []provider.Profile{{Name: "cx", Provider: provider.Codex, Home: filepath.Join(h.home, ".codex"), Billing: provider.BillingPlan, Default: true, Source: provider.SourceDiscovered}}
	h.provs = []provider.Provider{h.claude, plainProv{codex}}
	if code := h.run("resume", "--list", "--json"); code != ExitOK {
		t.Fatalf("plain codex: %d %s", code, h.stderr.String())
	}
	if n := len(decode[[]conversationView](t, h.stdout.Bytes()).Data); n != 4 || h.stderr.Len() != 0 {
		t.Errorf("plain codex: %d rows, stderr %q", n, h.stderr.String())
	}
	h.provs = []provider.Provider{h.claude, failingLister{codex}}
	if code := h.run("resume", "--list", "--json"); code != ExitOK {
		t.Fatalf("failing codex: %d %s", code, h.stderr.String())
	}
	if n := len(decode[[]conversationView](t, h.stdout.Bytes()).Data); n != 4 || !strings.Contains(h.stderr.String(), "history unreadable") {
		t.Errorf("failing codex: %d rows, stderr %q", n, h.stderr.String())
	}
}
