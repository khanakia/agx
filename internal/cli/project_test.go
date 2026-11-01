package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// boundDirs are the folders projectHarness binds, relative to <root>/w:
//
//	bp      profile work                 (work's own login is p)
//	bl      login p                      (default profile personal)
//	bl/sub  profile kimi                 (a subfolder binding: no inheritance)
//	bb      profile pinned + login w     (pinned's own login is home)
//	bh      profile work + login home
//	dp      profile gone                 (dangling)
//	dl      login gone                   (dangling)
//	bc      profile cx + login p         (p is a Claude login, cx is Codex)
//	bcx     login c                      (a Codex login)
//	none    unbound
var boundDirs = []string{"bp", "bl", "bl/sub", "bb", "bh", "dp", "dl", "bc", "bcx", "none", "blx"}

// projectHarness is loginHarness(loginConfig) plus the bindings above. The
// config holds canonical paths; h.cwd is set through the temp root's raw
// (possibly symlinked, on macOS) spelling, so every lookup also exercises
// CanonicalPath.
func projectHarness(t *testing.T) (*harness, string) {
	t.Helper()
	h := loginHarness(t, loginConfig)
	root, err := filepath.EvalSymlinks(h.root)
	if err != nil {
		t.Fatal(err)
	}
	w := filepath.Join(root, "w")
	for _, d := range boundDirs {
		if err := os.MkdirAll(filepath.Join(w, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	yaml := fmt.Sprintf(`projects:
  - {path: %[1]s/bp, profile: work}
  - {path: %[1]s/bl, login: p}
  - {path: %[1]s/bl/sub, profile: kimi}
  - {path: %[1]s/bb, profile: pinned, login: w}
  - {path: %[1]s/bh, profile: work, login: home}
  - {path: %[1]s/dp, profile: gone}
  - {path: %[1]s/dl, login: gone}
  - {path: %[1]s/bc, profile: cx, login: p}
  - {path: %[1]s/bcx, login: c}
`, w)
	f, err := os.OpenFile(h.cfgPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return h, w
}

// in points the harness at folder d under the bound tree, spelled through
// the raw temp root.
func (h *harness) in(d string) {
	h.cwd = filepath.Join(h.root, "w", d)
}

// dryPlan runs a dry run and returns its project, profile and login lines.
func (h *harness) dryPlan(cmd string, args ...string) (project, profile, loginLine string, code int) {
	h.t.Helper()
	code = h.run(append([]string{cmd, "--dry-run"}, args...)...)
	for _, line := range strings.Split(h.stdout.String(), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "project":
			project = v
		case "profile":
			profile, _, _ = strings.Cut(v, " ")
		case "login":
			loginLine = v
		}
	}
	return project, profile, loginLine, code
}

// TestProject_Precedence is the precedence matrix with folder bindings:
// --login, AGX_LOGIN, the binding (profile and login resolved
// independently), the profile's login, the provider default, the default
// profile; --provider against a binding to another provider's profile; and
// dangling bindings, which fail only when the dangling half is used. Every
// expectation follows from the documented order, not from running the code.
func TestProject_Precedence(t *testing.T) {
	t.Parallel()
	h, w := projectHarness(t)
	src := func(d string) string { return "(project binding at " + filepath.Join(w, d) + ")" }
	for _, c := range []struct {
		dir     string
		args    []string
		env     string // AGX_LOGIN; "" = unset
		profile string
		login   string // dry-run login line; "" when the run fails
		code    int
		note    string // stderr substring
	}{
		// Binding by profile only: the profile's own login rules follow.
		{dir: "bp", profile: "work", login: "p (profile)"},
		{dir: "bp/deep/er", profile: "work", login: "p (profile)"},
		{dir: "bp", args: []string{"-p", "personal"}, profile: "personal", login: "w (provider)"},
		{dir: "bp", args: []string{"--login", "w"}, profile: "work", login: "w (--login)"},
		{dir: "bp", args: []string{"--login", "home"}, profile: "work", login: "home (--login)"},
		{dir: "bp", env: "w", profile: "work", login: "w (AGX_LOGIN)"},
		{dir: "bp", args: []string{"--provider", "codex"}, profile: "cx", login: "c (provider)"},
		{dir: "bp", args: []string{"--provider", "claude"}, profile: "work", login: "p (profile)"},
		// Binding by login only: the home is the usual choice, the quota the binding's.
		{dir: "bl", profile: "personal", login: "p " + src("bl")},
		{dir: "bl", args: []string{"-p", "pinned"}, profile: "pinned", login: "p " + src("bl")},
		{dir: "bl", args: []string{"--login", "home"}, profile: "personal", login: "home (--login)"},
		{dir: "bl", env: "w", profile: "personal", login: "w (AGX_LOGIN)"},
		{dir: "bl", env: "home", profile: "personal", login: "home (AGX_LOGIN)"},
		{dir: "bl", env: "c", profile: "personal", login: "p " + src("bl"), note: "AGX_LOGIN=c skipped for profile personal"},
		{dir: "bl", args: []string{"-p", "kimi"}, profile: "kimi", login: "home", note: "login p skipped for profile kimi"},
		{dir: "bl", args: []string{"-p", "cx"}, profile: "cx", login: "c (provider)", note: "login p skipped for profile cx"},
		{dir: "bl", args: []string{"--provider", "codex"}, profile: "cx", login: "c (provider)", note: "login p skipped for profile cx"},
		// A subfolder binding replaces its parent's, it does not merge with it.
		{dir: "bl/sub", profile: "kimi", login: "home"},
		// A sibling whose name starts with the bound folder's is unbound.
		{dir: "blx", profile: "personal", login: "w (provider)"},
		// Both halves: the binding's login beats the profile's own.
		{dir: "bb", profile: "pinned", login: "w " + src("bb")},
		{dir: "bb", args: []string{"--login", "p"}, profile: "pinned", login: "p (--login)"},
		{dir: "bb", args: []string{"-p", "work"}, profile: "work", login: "w " + src("bb")},
		{dir: "bh", profile: "work", login: "home " + src("bh")},
		{dir: "bh", env: "w", profile: "work", login: "w (AGX_LOGIN)"},
		// Dangling: fails only when that half is used, and never falls back.
		{dir: "dp", code: ExitUsage, note: `project binding ` + filepath.Join(w, "dp") + `: profile "gone" no longer exists — remove it with`},
		{dir: "dp", args: []string{"-p", "personal"}, profile: "personal", login: "w (provider)"},
		{dir: "dl", code: ExitUsage, note: `login "gone" no longer exists`},
		{dir: "dl", args: []string{"--login", "home"}, profile: "personal", login: "home (--login)"},
		{dir: "dl", env: "w", profile: "personal", login: "w (AGX_LOGIN)"},
		{dir: "dl", env: "c", code: ExitUsage, note: `login "gone" no longer exists`},
		// The binding's own profile cannot take its login: an error, not a skip.
		{dir: "bc", code: ExitUsage, note: "is a claude login, not codex"},
		{dir: "bc", args: []string{"-p", "personal"}, profile: "personal", login: "p " + src("bc")},
		{dir: "bc", args: []string{"--provider", "claude"}, profile: "personal", login: "p " + src("bc")},
		// A Codex login bound to a folder: skipped for Claude, used for Codex.
		{dir: "bcx", profile: "personal", login: "w (provider)", note: "login c skipped for profile personal"},
		{dir: "bcx", args: []string{"--provider", "codex"}, profile: "cx", login: "c " + src("bcx")},
		// Unbound.
		{dir: "none", profile: "personal", login: "w (provider)"},
		{dir: "none", args: []string{"-p", "work"}, profile: "work", login: "p (profile)"},
	} {
		name := fmt.Sprintf("%s %v env=%s", c.dir, c.args, c.env)
		delete(h.env, appmeta.EnvLogin)
		if c.env != "" {
			h.env[appmeta.EnvLogin] = c.env
		}
		h.in(c.dir)
		if err := os.MkdirAll(h.cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		project, profile, got, code := h.dryPlan("run", c.args...)
		if code != c.code || got != c.login || profile != c.profile {
			t.Errorf("%s: profile %q login %q exit %d, want %q %q exit %d; stderr %s", name, profile, got, code, c.profile, c.login, c.code, h.stderr.String())
		}
		if c.note != "" && !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("%s: stderr %q lacks %q", name, h.stderr.String(), c.note)
		}
		if c.note == "" && h.stderr.Len() > 0 {
			t.Errorf("%s: unexpected stderr %q", name, h.stderr.String())
		}
		bound := c.dir != "none" && c.dir != "blx"
		if c.code == ExitOK && (project != "") != bound {
			t.Errorf("%s: project line %q, bound=%v", name, project, bound)
		}
	}
}

// TestProject_ResumeKeepsHome: resume runs in the home that stores the
// conversation whatever the binding's profile says; only the binding's login
// applies, and the binding is the conversation folder's, not the folder
// `resume --all` was typed in.
func TestProject_ResumeKeepsHome(t *testing.T) {
	t.Parallel()
	h, w := projectHarness(t)
	work := filepath.Join(h.home, ".claude-work")
	personal := filepath.Join(h.home, ".claude")
	h.claude.convs[work] = []provider.Conversation{
		{Provider: provider.Claude, Home: work, ID: "in-bb", Title: "bound both", Dir: filepath.Join(w, "bb"), Updated: h.now.Add(-time.Minute)},
	}
	h.claude.convs[personal] = []provider.Conversation{
		{Provider: provider.Claude, Home: personal, ID: "in-bp", Title: "bound profile", Dir: filepath.Join(w, "bp"), Updated: h.now.Add(-time.Hour)},
		{Provider: provider.Claude, Home: personal, ID: "in-bl", Title: "bound login", Dir: filepath.Join(w, "bl"), Updated: h.now.Add(-2 * time.Hour)},
	}
	for _, c := range []struct {
		dir     string
		args    []string
		profile string
		login   string
	}{
		// bb binds profile pinned + login w; the conversation lives in work.
		{"bb", []string{"--last"}, "work", "w (project binding at " + filepath.Join(w, "bb") + ")"},
		{"bb", []string{"--last", "--login", "home"}, "work", "home (--login)"},
		// bp binds profile work only; the conversation lives in personal.
		{"bp", []string{"--last"}, "personal", "w (provider)"},
		// Typed in an unbound folder, the conversation's folder decides.
		{"none", []string{"--all", "--last", "bound login"}, "personal", "p (project binding at " + filepath.Join(w, "bl") + ")"},
	} {
		h.in(c.dir)
		_, profile, got, code := h.dryPlan("resume", c.args...)
		if code != ExitOK || profile != c.profile || got != c.login {
			t.Errorf("%s %v: profile %q login %q exit %d, want %q %q; stderr %s", c.dir, c.args, profile, got, code, c.profile, c.login, h.stderr.String())
		}
	}
}

// TestProject_NewAndPlan: `new` takes the binding of the folder it was typed
// in (its session folder is elsewhere), and the plan the shell layer runs
// later carries the binding's login, since exec runs in another folder.
func TestProject_NewAndPlan(t *testing.T) {
	t.Parallel()
	h, _ := projectHarness(t)
	h.in("bb")
	if _, profile, got, code := h.dryPlan("new", "topic"); code != ExitOK || profile != "pinned" || !strings.HasPrefix(got, "w (project binding") {
		t.Errorf("new in bb: %q %q %d %s", profile, got, code, h.stderr.String())
	}
	planPath := filepath.Join(h.root, "plan.json")
	h.env[appmeta.EnvPlanFile] = planPath
	h.in("bl")
	if code := h.run("new", "topic"); code != ExitOK {
		t.Fatalf("new: %d %s", code, h.stderr.String())
	}
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var pl plan
	if err := json.Unmarshal(raw, &pl); err != nil || pl.Login != "p" || pl.Profile != "personal" {
		t.Errorf("plan = %s (%v)", raw, err)
	}
	// An explicit --login is what the plan carries, not the binding's.
	if code := h.run("new", "--login", "home", "topic2"); code != ExitOK {
		t.Fatalf("new: %d %s", code, h.stderr.String())
	}
	raw, _ = os.ReadFile(planPath)
	pl = plan{}
	if err := json.Unmarshal(raw, &pl); err != nil || pl.Login != "home" {
		t.Errorf("plan with --login = %s", raw)
	}
	// A binding with no login leaves the plan's login empty.
	h.in("bp")
	if code := h.run("new", "topic3"); code != ExitOK {
		t.Fatalf("new: %d %s", code, h.stderr.String())
	}
	raw, _ = os.ReadFile(planPath)
	pl = plan{}
	if err := json.Unmarshal(raw, &pl); err != nil || pl.Login != "" || pl.Profile != "work" {
		t.Errorf("plan in bp = %s", raw)
	}
}

// TestProject_Commands drives use / show / ls / rm the way a user does,
// text and JSON, including every refusal.
func TestProject_Commands(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, loginConfig)
	root, err := filepath.EvalSymlinks(h.root)
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "my app")
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.cwd = filepath.Join(h.root, "my app") // raw spelling: stored canonical

	// Nothing bound yet.
	if code := h.run("project", "ls"); code != ExitOK || !strings.Contains(h.stdout.String(), "no project bindings") {
		t.Errorf("empty ls: %d %q", code, h.stdout.String())
	}
	if code := h.run("project", "ls", "--json"); code != ExitOK {
		t.Fatal(code)
	}
	if e := decode[[]projectRow](t, h.stdout.Bytes()); e.Kind != kindProjectList || e.Count != 0 || e.Data == nil {
		t.Errorf("empty ls json = %+v", e)
	}

	// Refusals change nothing.
	before, _ := os.ReadFile(h.cfgPath)
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"project", "use"}, ExitUsage, "give -p PROFILE, --login LOGIN, or both"},
		{[]string{"project", "use", "--login", "auto"}, ExitUsage, `"auto" is accepted only as --login`},
		{[]string{"project", "use", "-p", "nosuch"}, ExitUsage, "nosuch"},
		{[]string{"project", "use", "--login", "nosuch"}, ExitUsage, `unknown login "nosuch"`},
		{[]string{"project", "use", "-p", "cx", "--login", "p"}, ExitUsage, "is a claude login, not codex"},
		{[]string{"project", "use", "-p", "kimi", "--login", "p"}, ExitUsage, "billed per token"},
		{[]string{"project", "use", "extra"}, ExitUsage, ""},
		{[]string{"project", "rm"}, ExitFailure, "no project binding for " + h.cwd},
		{[]string{"project", "rm", "/nowhere"}, ExitFailure, "no project binding for /nowhere"},
		{[]string{"project", "show", "-p", "auto"}, ExitUsage, "reads the config only"},
		{[]string{"project", "show", "--login", "auto"}, ExitUsage, "reads the config only"},
	} {
		if code := h.run(c.args...); code != c.code || !strings.Contains(h.stderr.String(), c.want) {
			t.Errorf("%v: exit %d stderr %q, want %d %q", c.args, code, h.stderr.String(), c.code, c.want)
		}
	}
	if after, _ := os.ReadFile(h.cfgPath); string(after) != string(before) {
		t.Error("a refused project command changed the config")
	}

	// use: login only, then replace with profile + login.
	if code := h.run("project", "use", "--login", "p"); code != ExitOK || strings.TrimSpace(h.stdout.String()) != "bound "+app+" -> login p" {
		t.Errorf("use: %d %q %s", code, h.stdout.String(), h.stderr.String())
	}
	if code := h.run("project", "use", "-p", "pinned", "--login", "w"); code != ExitOK || strings.TrimSpace(h.stdout.String()) != "bound "+app+" -> profile pinned, login w (was login p)" {
		t.Errorf("replace: %d %q", code, h.stdout.String())
	}

	// show from a subfolder: text and JSON agree with the dry run.
	h.cwd = filepath.Join(app, "src")
	if code := h.run("project", "show"); code != ExitOK {
		t.Fatalf("show: %d %s", code, h.stderr.String())
	}
	for _, want := range []string{"folder:  " + filepath.Join(app, "src"), "project: " + app + " (profile pinned, login w)", "profile: pinned (claude) — project binding at " + app, "home:    ~/.claude-pin — profile pinned", "login:   w — project binding at " + app} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("show lacks %q:\n%s", want, h.stdout.String())
		}
	}
	if code := h.run("project", "show", "--json"); code != ExitOK {
		t.Fatal(code)
	}
	sh := decode[projectShow](t, h.stdout.Bytes())
	if sh.Kind != kindProjectShow || sh.Count != 0 || sh.Data.Project == nil || sh.Data.Project.Path != app || sh.Data.Profile != "pinned" || sh.Data.Login != "w" || sh.Data.Provider != provider.Claude || sh.Data.LoginSource != "project binding at "+app || sh.Data.Home != filepath.Join(h.home, ".claude-pin") {
		t.Errorf("show json = %+v", sh.Data)
	}
	// Flags try a combination, as on run.
	if code := h.run("project", "show", "-p", "work", "--login", "home", "--json"); code != ExitOK {
		t.Fatal(code)
	}
	if sh := decode[projectShow](t, h.stdout.Bytes()).Data; sh.Profile != "work" || sh.ProfileSource != "--profile" || sh.Login != "home" || sh.LoginSource != "--login" {
		t.Errorf("show with flags = %+v", sh)
	}
	// Outside the binding: the defaults, said in words.
	h.cwd = root
	if code := h.run("project", "show", "--json"); code != ExitOK {
		t.Fatal(code)
	}
	if sh := decode[projectShow](t, h.stdout.Bytes()).Data; sh.Project != nil || sh.Profile != "personal" || sh.ProfileSource != "default profile" || sh.Login != "w" || sh.LoginSource != "provider default" {
		t.Errorf("unbound show = %+v", sh)
	}

	// A second binding, then ls in text and JSON.
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	h.cwd = other
	if code := h.run("project", "use", "-p", "work"); code != ExitOK {
		t.Fatal(code)
	}
	if code := h.run("project", "ls"); code != ExitOK {
		t.Fatal(code)
	}
	for _, want := range []string{"PATH", "STATE", app, other, "pinned", "ok"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("ls lacks %q:\n%s", want, h.stdout.String())
		}
	}
	if code := h.run("project", "list", "--json"); code != ExitOK {
		t.Fatal(code)
	}
	ls := decode[[]projectRow](t, h.stdout.Bytes())
	if ls.Count != 2 || ls.Data[0].Path != app || ls.Data[0].Profile != "pinned" || ls.Data[0].Login != "w" || ls.Data[1].Path != other || ls.Data[1].Login != "" || len(ls.Data[0].Problems) != 0 {
		t.Errorf("ls json = %+v", ls)
	}

	// rm by relative path, by ~, and a parent's binding is left alone.
	h.cwd = root
	if code := h.run("project", "rm", "other"); code != ExitOK || !strings.Contains(h.stdout.String(), "removed project binding "+other+" (profile work)") {
		t.Errorf("rm relative: %d %q %s", code, h.stdout.String(), h.stderr.String())
	}
	h.cwd = filepath.Join(app, "src")
	if code := h.run("project", "rm"); code != ExitFailure {
		t.Errorf("rm in a subfolder removed the parent's binding: %d", code)
	}
	if code := h.run("project", "rm", ".."); code != ExitOK {
		t.Errorf("rm ..: %d %s", code, h.stderr.String())
	}
	if cfg := loadCfg(t, h); len(cfg.Projects) != 0 {
		t.Errorf("projects left: %+v", cfg.Projects)
	}
	raw, _ := os.ReadFile(h.cfgPath)
	if strings.Contains(string(raw), "projects:") {
		t.Errorf("empty projects key left:\n%s", raw)
	}
}

// TestProject_RmTildeAndGoneFolder: rm accepts ~/ paths, and a binding whose
// folder was deleted is still removable by the path ls prints.
func TestProject_RmTildeAndGoneFolder(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, loginConfig)
	gone := filepath.Join(h.home, "gone")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	h.cwd = gone
	if code := h.run("project", "use", "-p", "work"); code != ExitOK {
		t.Fatal(code)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if code := h.run("doctor"); !strings.Contains(h.stdout.String(), "does not exist — remove it with `agx project rm") {
		t.Errorf("doctor on gone folder: %d\n%s", code, h.stdout.String())
	}
	h.cwd = h.root
	if code := h.run("project", "rm", "~/gone"); code != ExitOK {
		t.Errorf("rm ~/gone: %d %s", code, h.stderr.String())
	}
}

// loadCfg reads the harness config the way agx does.
func loadCfg(t *testing.T, h *harness) config.Config {
	t.Helper()
	cfg, err := config.Load(h.cfgPath, h.home, h.providers())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestProject_DoctorAndLoginRm: doctor fails on a dangling or misfitting
// binding and passes a good one; login ls/rm count bindings as users, rm
// refuses without --force and clears them with it.
func TestProject_DoctorAndLoginRm(t *testing.T) {
	t.Parallel()
	h, w := projectHarness(t)
	code := h.run("doctor")
	out := h.stdout.String()
	for _, want := range []string{
		filepath.Join(w, "bp") + " -> profile work",
		`project binding ` + filepath.Join(w, "dp") + `: profile "gone" no longer exists`,
		`project binding ` + filepath.Join(w, "dl") + `: login "gone" no longer exists`,
		`project binding ` + filepath.Join(w, "bc") + `: `,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor lacks %q:\n%s", want, out)
		}
	}
	if code == ExitOK {
		t.Error("doctor passed with dangling bindings")
	}
	if code := h.run("project", "ls", "--json"); code != ExitOK {
		t.Fatal(code)
	}
	problems := map[string][]string{}
	for _, r := range decode[[]projectRow](t, h.stdout.Bytes()).Data {
		problems[filepath.Base(r.Path)] = r.Problems
	}
	for dir, n := range map[string]int{"bp": 0, "bl": 0, "bb": 0, "dp": 1, "dl": 1, "bc": 1, "bcx": 0} {
		if len(problems[dir]) != n {
			t.Errorf("ls problems[%s] = %v, want %d", dir, problems[dir], n)
		}
	}
	if code := h.run("login", "ls"); code != ExitOK || !strings.Contains(h.stdout.String(), "project "+filepath.Join(w, "bl")) {
		t.Errorf("login ls USED-BY lacks the binding:\n%s", h.stdout.String())
	}
	if code := h.run("login", "rm", "c"); code == ExitOK || !strings.Contains(h.stderr.String(), "project "+filepath.Join(w, "bcx")) || !strings.Contains(h.stderr.String(), "agx project rm PATH") {
		t.Errorf("login rm c without --force: %d %s", code, h.stderr.String())
	}
	if code := h.run("login", "rm", "c", "--force"); code != ExitOK {
		t.Fatalf("login rm --force: %d %s", code, h.stderr.String())
	}
	if _, ok := loadCfg(t, h).ProjectFor(filepath.Join(w, "bcx")); ok {
		t.Error("a login-only binding outlived its login")
	}
}

// TestProject_BareAgxRouting: a bare `agx` starts the agent only in a bound
// folder with a person at the terminal and no flags; every other shape keeps
// the usage view, and `agx usage` always shows usage.
func TestProject_BareAgxRouting(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		dir    string
		tty    bool
		args   []string
		launch bool
	}{
		{"bound tty", "bb", true, nil, true},
		{"bound subfolder tty", "bp/x", true, nil, true},
		{"bound piped", "bb", false, nil, false},
		{"bound tty json", "bb", true, []string{"--json"}, false},
		{"bound tty usage", "bb", true, []string{"usage"}, false},
		{"unbound tty", "none", true, nil, false},
		{"sibling prefix tty", "blx", true, nil, false},
	} {
		h, w := projectHarness(t)
		h.storeToken("p", tokStored)
		h.env["W_TOKEN"] = tokEnvOAT
		h.tty = c.tty
		h.in(c.dir)
		if err := os.MkdirAll(h.cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		code := h.run(c.args...)
		launched := len(h.execs) == 1
		if launched != c.launch {
			t.Errorf("%s: launched=%v, want %v (exit %d, stderr %s)", c.name, launched, c.launch, code, h.stderr.String())
		}
		if !c.launch {
			if !strings.Contains(h.stdout.String(), "me@home") {
				t.Errorf("%s: no usage view:\n%s", c.name, h.stdout.String())
			}
			continue
		}
		if code != ExitOK {
			t.Errorf("%s: exit %d", c.name, code)
		}
		bound := filepath.Join(w, strings.Split(c.dir, "/")[0])
		lines := strings.Split(strings.TrimSpace(h.stderr.String()), "\n")
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "agx: project "+bound+" -> profile ") {
			t.Errorf("%s: stderr %q, want one announce line", c.name, h.stderr.String())
		}
		if ex := h.lastExec(); ex.Dir != h.cwd {
			t.Errorf("%s: ran in %s", c.name, ex.Dir)
		}
	}
	// What the announcement says matches what runs.
	h, w := projectHarness(t)
	h.tty = true
	h.env["W_TOKEN"] = tokEnvOAT
	h.in("bb")
	if code := h.run(); code != ExitOK {
		t.Fatalf("exit %d %s", code, h.stderr.String())
	}
	if want := "agx: project " + filepath.Join(w, "bb") + " -> profile pinned, login w\n"; h.stderr.String() != want {
		t.Errorf("stderr %q, want %q", h.stderr.String(), want)
	}
	ex := h.lastExec()
	if v, _ := envOf(ex, claude.OAuthTokenEnv); v != tokEnvOAT || !slices.Contains(ex.Env, "HOME_DIR="+filepath.Join(h.home, ".claude-pin")) {
		t.Errorf("auto-start did not run pinned on login w")
	}
	// A broken binding fails loudly instead of falling back.
	h.in("dp")
	if code := h.run(); code != ExitUsage || !strings.Contains(h.stderr.String(), "no longer exists") {
		t.Errorf("dangling auto-start: %d %s", code, h.stderr.String())
	}
}

// TestDetectedKind_Matrix: detected prefix × default kind × explicit --kind
// (an inherited kind on --replace arrives as kind with no flag). The prefix
// wins over a default or inherited kind, contradicts an explicit one, and an
// unrecognised value or a provider without detection keeps kind.
func TestDetectedKind_Matrix(t *testing.T) {
	t.Parallel()
	det := &claude.Provider{}
	plain := loginProvNoDetect{}
	const (
		oat = provider.LoginOAuthToken
		api = provider.LoginAPIKey
	)
	for _, c := range []struct {
		name     string
		ll       provider.LoginLauncher
		kind     provider.LoginKind
		flagKind string
		val      string
		want     provider.LoginKind
		err      string
	}{
		{"oat default", det, oat, "", tokStored, oat, ""},
		{"api default", det, oat, "", tokEnv, api, ""},
		{"api inherited oat", det, oat, "", tokEnv, api, ""},
		{"oat inherited api", det, api, "", tokStored, oat, ""},
		{"api flag api", det, api, "api-key", tokEnv, api, ""},
		{"oat flag oat", det, oat, "oauth-token", tokStored, oat, ""},
		{"api flag oat", det, oat, "oauth-token", tokEnv, "", "--kind oauth-token does not match the token, which is an api-key"},
		{"oat flag api", det, api, "api-key", tokStored, "", "--kind api-key does not match the token, which is an oauth-token"},
		{"unknown default", det, oat, "", "opaque-value", oat, ""},
		{"unknown flag api", det, api, "api-key", "opaque-value", api, ""},
		{"refresh", det, oat, "", "sk-ant-ort01-DUMMY", "", "refresh token"},
		{"refresh flag", det, oat, "oauth-token", "sk-ant-ort01-DUMMY", "", "refresh token"},
		{"admin", det, api, "api-key", "sk-ant-admin01-DUMMY", "", "Admin API key"},
		{"no detector api value", plain, oat, "", tokEnv, oat, ""},
		{"no detector refresh value", plain, oat, "", "sk-ant-ort01-DUMMY", oat, ""},
	} {
		got, err := detectedKind(c.ll, c.kind, c.flagKind, c.val)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: err %v, want %q", c.name, err, c.err)
			}
			if err != nil && strings.Contains(err.Error(), "DUMMY") {
				t.Errorf("%s: error quotes the value", c.name)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

// loginProvNoDetect launches on logins but cannot recognise a token.
type loginProvNoDetect struct{}

func (loginProvNoDetect) LoginKinds() []provider.LoginKind { return (&claude.Provider{}).LoginKinds() }
func (loginProvNoDetect) LoginEnv(k provider.LoginKind, v string) (map[string]string, []string) {
	return (&claude.Provider{}).LoginEnv(k, v)
}

// TestLoginKind_Commands drives kind detection through `login add` (paste,
// --cmd, --env, --replace) and the launch-time check, which is the only
// place a --cmd/--env token is seen.
func TestLoginKind_Commands(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, "profiles:\n  - {name: personal, provider: claude, home: HOME/.claude}\n")
	kindOf := func(name string) provider.LoginKind {
		l, ok := loadCfg(t, h).LoginByName(name)
		if !ok {
			t.Fatalf("login %s missing", name)
		}
		return l.Kind
	}
	for _, c := range []struct {
		stdin string
		args  []string
		code  int
		kind  provider.LoginKind // checked when code is OK
		err   string
	}{
		{tokStored + "\n", []string{"login", "add", "a1", "--stdin"}, ExitOK, provider.LoginOAuthToken, ""},
		{tokEnv + "\n", []string{"login", "add", "a2", "--stdin"}, ExitOK, provider.LoginAPIKey, ""},
		{"  " + tokEnv + "  \n", []string{"login", "add", "a3", "--stdin", "--kind", "api-key"}, ExitOK, provider.LoginAPIKey, ""},
		{tokEnv + "\n", []string{"login", "add", "a4", "--stdin", "--kind", "oauth-token"}, ExitUsage, "", "--kind oauth-token does not match"},
		{"sk-ant-ort01-DUMMY\n", []string{"login", "add", "a5", "--stdin"}, ExitUsage, "", "OAuth refresh token"},
		{"sk-ant-admin01-DUMMY\n", []string{"login", "add", "a6", "--stdin", "--kind", "api-key"}, ExitUsage, "", "Admin API key"},
		// --replace: the old kind is inherited, and the new token's prefix wins over it.
		{tokStored + "\n", []string{"login", "add", "a2", "--stdin", "--replace"}, ExitOK, provider.LoginOAuthToken, ""},
	} {
		h.stdin = c.stdin
		code := h.run(c.args...)
		if code != c.code || !strings.Contains(h.stderr.String(), c.err) {
			t.Errorf("%v: exit %d stderr %q, want %d %q", c.args, code, h.stderr.String(), c.code, c.err)
			continue
		}
		if code == ExitOK && kindOf(c.args[2]) != c.kind {
			t.Errorf("%v: kind %q, want %q", c.args, kindOf(c.args[2]), c.kind)
		}
		if strings.Contains(h.stdout.String()+h.stderr.String(), "DUMMY") || strings.Contains(h.stdout.String()+h.stderr.String(), tokEnv) {
			t.Errorf("%v: output quotes a token", c.args)
		}
	}
	// Refused adds stored nothing.
	stored, _ := os.ReadFile(h.tokensPath())
	for _, n := range []string{"a4:", "a5:", "a6:"} {
		if strings.Contains(string(stored), n) {
			t.Errorf("refused login %s left a token", n)
		}
	}
	// --cmd / --env sources are checked when they resolve: login check and launch.
	h.env["REFRESH"] = "sk-ant-ort01-DUMMY"
	h.env["APIVAL"] = tokEnv
	for _, c := range []struct {
		add  []string
		want string
	}{
		{[]string{"login", "add", "r1", "--env", "REFRESH"}, "login r1: claude: this token cannot run Claude: it is an OAuth refresh token"},
		{[]string{"login", "add", "r2", "--env", "APIVAL"}, "login r2 is kind oauth-token, but its token is an api-key by its prefix — re-add it with `agx login add r2 --replace --kind api-key`"},
		{[]string{"login", "add", "r3", "--cmd", "printf sk-ant-admin01-DUMMY"}, "Admin API key"},
	} {
		// The add keeps the login (the env or command may hand back another
		// value at launch) but its check fails and says why.
		if code := h.run(c.add...); code != ExitFailure || !strings.Contains(h.stdout.String(), c.want) {
			t.Errorf("%v: exit %d, check output %q lacks %q", c.add, code, h.stdout.String(), c.want)
		}
		n := len(h.execs)
		if code := h.run("run", "-p", "personal", "--login", c.add[2]); code == ExitOK || len(h.execs) != n || !strings.Contains(h.stderr.String(), c.want) {
			t.Errorf("launch on %s: exit %d execs %d→%d stderr %q, want %q", c.add[2], code, n, len(h.execs), h.stderr.String(), c.want)
		}
		if strings.Contains(h.stderr.String(), "DUMMY") || strings.Contains(h.stderr.String(), tokEnv) {
			t.Errorf("launch on %s: error quotes the token", c.add[2])
		}
	}
}

// homeProv is a loginProv that can set up a home, with Claude's shared list
// (or none, for the Codex stand-in).
type homeProv struct {
	loginProv
	shared []string
}

func (p homeProv) SharedHomeFiles() []string { return slices.Clone(p.shared) }
func (p homeProv) LoginCommand(pr provider.Profile, env []string) provider.Command {
	return provider.Command{Path: string(p.id) + "-bin", Args: []string{"auth", "login"}, Env: provider.SetEnv(env, "HOME_DIR", pr.Home)}
}

// profileHarness has no profiles in its config (personal and work exist
// only by discovery), logins p (claude) and c (codex), providers that can
// set up homes, and a default home holding CLAUDE.md and skills but no
// settings.json.
func profileHarness(t *testing.T) *harness {
	t.Helper()
	h := loginHarness(t, "logins:\n  - {name: p, token: stored}\n  - {name: c, provider: codex, token: stored}\n")
	def := filepath.Join(h.home, ".claude")
	if err := os.MkdirAll(filepath.Join(def, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(def, "CLAUDE.md"), []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	cl := h.provs[0].(loginProv)
	cx := h.provs[1].(loginProv)
	h.provs = []provider.Provider{homeProv{cl, (&claude.Provider{}).SharedHomeFiles()}, homeProv{cx, nil}}
	return h
}

// TestProfileAdd_Matrix covers the home choice (default per provider,
// --home, adopting an existing folder), linking (only when the source exists
// and the target is absent, never with --bare or into an adopted home),
// --dry-run (prints, changes nothing), --setup, --login, and every refusal.
func TestProfileAdd_Matrix(t *testing.T) {
	t.Parallel()
	type result struct {
		code   int
		stdout string
		stderr string
	}
	for _, c := range []struct {
		name     string
		prep     func(h *harness)
		args     []string
		code     int
		home     string   // relative to the harness home; "" = no profile added
		out      []string // stdout lines expected, in order
		links    []string // symlinks that must exist in the new home
		noLinks  bool
		setup    bool // a vendor login command ran
		login    string
		errMatch string
	}{
		{name: "claude default", args: []string{"client"}, home: ".claude-client",
			out:   []string{"created home ~/.claude-client", "added profile client (claude, ~/.claude-client)", "linked CLAUDE.md -> ~/.claude/CLAUDE.md", "linked skills -> ~/.claude/skills", "skipped settings.json (not in the default home)"},
			links: []string{"CLAUDE.md", "skills"}},
		{name: "codex default", args: []string{"cx2", "--provider", "codex"}, home: ".codex-cx2",
			out: []string{"created home ~/.codex-cx2", "added profile cx2 (codex, ~/.codex-cx2)"}, noLinks: true},
		{name: "custom home", args: []string{"client", "--home", "~/homes/client"}, home: "homes/client",
			out: []string{"created home ~/homes/client", "added profile client (claude, ~/homes/client)", "linked CLAUDE.md -> ~/.claude/CLAUDE.md"}, links: []string{"CLAUDE.md", "skills"}},
		{name: "bare", args: []string{"client", "--bare"}, home: ".claude-client",
			out: []string{"created home ~/.claude-client", "added profile client (claude, ~/.claude-client)"}, noLinks: true},
		{name: "with login", args: []string{"client", "--login", "p"}, home: ".claude-client", login: "p",
			out: []string{"created home ~/.claude-client", "added profile client (claude, ~/.claude-client, login p)"}},
		{name: "login home", args: []string{"client", "--login", "home"}, home: ".claude-client", login: "home"},
		{name: "adopt existing home", prep: func(h *harness) {
			mustWrite(h.t, filepath.Join(h.home, ".claude-client", "mine.txt"), "keep")
		}, args: []string{"client"}, home: ".claude-client", noLinks: true,
			out: []string{"adopted existing home ~/.claude-client (contents left as they are)", "added profile client (claude, ~/.claude-client)"}},
		{name: "adopt discovered profile", args: []string{"work", "--login", "p"}, home: ".claude-work", login: "p"},
		{name: "setup", args: []string{"client", "--setup"}, home: ".claude-client", setup: true,
			out: []string{"created home ~/.claude-client"}},
		{name: "dry run", args: []string{"client", "--dry-run", "--setup"},
			out: []string{"would create home ~/.claude-client", "would add profile client (claude, ~/.claude-client)", "would link CLAUDE.md -> ~/.claude/CLAUDE.md", "would link skills -> ~/.claude/skills", "skipped settings.json (not in the default home)", "would run `claude-bin auth login` for ~/.claude-client"}},
		{name: "dry run adopt", prep: func(h *harness) {
			mustWrite(h.t, filepath.Join(h.home, ".claude-client", "mine.txt"), "keep")
		}, args: []string{"client", "--dry-run"},
			out: []string{"would adopt existing home ~/.claude-client (contents left as they are)"}},
		{name: "duplicate config name", prep: func(h *harness) {
			if code := h.run("profile", "add", "client"); code != ExitOK {
				h.t.Fatalf("prep: %d %s", code, h.stderr.String())
			}
		}, args: []string{"client", "--home", "~/elsewhere"}, code: ExitUsage, errMatch: `profile "client" already exists`},
		{name: "discovered name, other home", args: []string{"work", "--home", "~/elsewhere"}, code: ExitUsage, errMatch: `profile "work" already exists`},
		{name: "reserved name", args: []string{"auto"}, code: ExitUsage, errMatch: "reserved for auto-pick"},
		{name: "bad name", args: []string{"Client"}, code: ExitUsage, errMatch: "profile name must match"},
		{name: "unknown provider", args: []string{"x", "--provider", "nosuch"}, code: ExitUsage, errMatch: "nosuch"},
		{name: "auto login", args: []string{"client", "--login", "auto"}, code: ExitUsage, errMatch: `"auto" is accepted only as --login`},
		{name: "unknown login", args: []string{"client", "--login", "nosuch"}, code: ExitUsage, errMatch: `unknown login "nosuch"`},
		{name: "wrong provider login", args: []string{"client", "--login", "c"}, code: ExitUsage, errMatch: "is a codex login, not claude"},
		{name: "home is a file", prep: func(h *harness) {
			mustWrite(h.t, filepath.Join(h.home, ".claude-client"), "x")
		}, args: []string{"client"}, code: ExitUsage, errMatch: "exists and is not a folder"},
	} {
		h := profileHarness(t)
		if c.prep != nil {
			c.prep(h)
		}
		cfgBefore, _ := os.ReadFile(h.cfgPath)
		code := h.run(append([]string{"profile", "add"}, c.args...)...)
		r := result{code, h.stdout.String(), h.stderr.String()}
		if r.code != c.code || !strings.Contains(r.stderr, c.errMatch) {
			t.Errorf("%s: exit %d stderr %q, want %d %q", c.name, r.code, r.stderr, c.code, c.errMatch)
			continue
		}
		lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
		for i, want := range c.out {
			if i >= len(lines) || lines[i] != want {
				t.Errorf("%s: stdout line %d = %q, want %q\n%s", c.name, i, at(lines, i), want, r.stdout)
				break
			}
		}
		if (c.setup) != (len(h.interactive) == 1) {
			t.Errorf("%s: interactive commands %v", c.name, h.interactive)
		}
		if c.setup {
			if ic := h.interactive[0]; !slices.Contains(ic.Env, "HOME_DIR="+filepath.Join(h.home, c.home)) {
				t.Errorf("%s: setup ran for the wrong home: %v", c.name, ic.Env)
			}
		}
		if c.home == "" {
			cfgAfter, _ := os.ReadFile(h.cfgPath)
			if c.name != "duplicate config name" && string(cfgAfter) != string(cfgBefore) {
				t.Errorf("%s: config changed:\n%s", c.name, cfgAfter)
			}
			if c.code == ExitOK || c.name == "home is a file" {
				// dry run / refusal: the default new home was not created as a folder
				if info, err := os.Stat(filepath.Join(h.home, ".claude-client")); err == nil && info.IsDir() && !strings.Contains(c.name, "adopt") {
					t.Errorf("%s: created a home", c.name)
				}
			}
			continue
		}
		home := filepath.Join(h.home, c.home)
		info, err := os.Stat(home)
		if err != nil || !info.IsDir() {
			t.Errorf("%s: home %s: %v", c.name, home, err)
			continue
		}
		if !strings.Contains(c.name, "adopt") && info.Mode().Perm() != homePerm {
			t.Errorf("%s: home perm %v", c.name, info.Mode().Perm())
		}
		name := c.args[0]
		p, ok := loadCfg(t, h).ByName(name)
		if !ok || p.Home != home || p.Login != c.login || p.Source == provider.SourceDiscovered {
			t.Errorf("%s: profile = %+v (found %v)", c.name, p, ok)
		}
		if raw, _ := os.ReadFile(h.cfgPath); strings.Count(string(raw), "name: "+name+"\n") != 1 {
			t.Errorf("%s: %q written %d times:\n%s", c.name, name, strings.Count(string(raw), "name: "+name+"\n"), raw)
		}
		for _, l := range c.links {
			if dest, err := os.Readlink(filepath.Join(home, l)); err != nil || dest != filepath.Join(h.home, ".claude", l) {
				t.Errorf("%s: link %s -> %q %v", c.name, l, dest, err)
			}
		}
		if c.noLinks {
			for _, l := range []string{"CLAUDE.md", "skills", "settings.json"} {
				if _, err := os.Lstat(filepath.Join(home, l)); err == nil {
					t.Errorf("%s: %s linked", c.name, l)
				}
			}
		}
		if strings.Contains(c.name, "adopt existing") {
			if b, _ := os.ReadFile(filepath.Join(home, "mine.txt")); string(b) != "keep" {
				t.Errorf("%s: adopted home changed", c.name)
			}
		}
	}
}

// TestProfileAdd_SetupUnsupported: --setup is refused before anything is
// created for a provider that cannot set up a home.
func TestProfileAdd_SetupUnsupported(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, "")
	if code := h.run("profile", "add", "client", "--setup"); code != ExitUsage || !strings.Contains(h.stderr.String(), "--setup is not known for claude") {
		t.Errorf("exit %d %s", code, h.stderr.String())
	}
	if _, err := os.Stat(filepath.Join(h.home, ".claude-client")); err == nil {
		t.Error("home created before the refusal")
	}
	// Without --setup the same provider still gets a profile, just no links.
	if code := h.run("profile", "add", "client"); code != ExitOK {
		t.Errorf("plain add: %d %s", code, h.stderr.String())
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// TestProject_NoLeak runs every new command (profile, project, bare agx,
// kind refusals) with known fake tokens wired into every source and asserts
// none reaches stdout or stderr. Real launches are the positive control.
func TestProject_NoLeak(t *testing.T) {
	t.Parallel()
	h, _ := projectHarness(t)
	h.storeToken("p", tokStored)
	h.env["W_TOKEN"] = tokEnvOAT
	h.env["BAD_ENV"] = tokEnv // an api key under an oauth-token login
	h.tty = true
	leaks := append(slices.Clone(fakeTokens), "sk-ant-ort01-DUMMY-LEAK", "sk-ant-admin01-DUMMY-LEAK")
	step := func(stdin string, args ...string) {
		t.Helper()
		h.stdin = stdin
		code := h.run(args...)
		out := h.stdout.String() + h.stderr.String()
		for _, tok := range leaks {
			if strings.Contains(out, tok) {
				t.Errorf("agx %v (exit %d) leaked %s:\n%s", args, code, tok, out)
			}
		}
	}
	step("sk-ant-ort01-DUMMY-LEAK\n", "login", "add", "r", "--stdin")
	step("sk-ant-admin01-DUMMY-LEAK\n", "login", "add", "a", "--stdin", "--kind", "api-key")
	step(tokEnv+"\n", "login", "add", "k", "--stdin", "--kind", "oauth-token")
	step("", "login", "add", "bad", "--env", "BAD_ENV")
	step("", "login", "add", "badcmd", "--cmd", "printf sk-ant-ort01-DUMMY-LEAK")
	for _, d := range boundDirs {
		h.in(d)
		for _, args := range [][]string{
			{}, {"project", "show"}, {"project", "show", "--json"}, {"project", "ls"}, {"project", "ls", "--json"},
			{"run", "--dry-run"}, {"run"}, {"run", "--login", "bad"}, {"run", "--login", "badcmd"}, {"new", "--dry-run", "t"},
		} {
			step("", args...)
		}
	}
	h.in("none")
	for _, args := range [][]string{
		{"project", "use", "--login", "bad"}, {"project", "use", "-p", "work", "--login", "p"}, {"project", "show"},
		{"run", "--dry-run"}, {"doctor"}, {"doctor", "--json"}, {"login", "ls"}, {"login", "check", "--no-probe"},
		{"profile", "add", "leak", "--login", "p", "--dry-run"}, {"profile", "add", "leak", "--login", "p"},
		{"project", "rm"}, {"login", "rm", "p", "--force"},
	} {
		step("", args...)
	}
	var seen []string
	for _, c := range h.execs {
		for _, tok := range []string{tokStored, tokEnvOAT} {
			if slices.Contains(c.Env, claude.OAuthTokenEnv+"="+tok) {
				seen = append(seen, tok)
			}
		}
	}
	for _, tok := range []string{tokStored, tokEnvOAT} {
		if !slices.Contains(seen, tok) {
			t.Errorf("no launch carried %s: the sweep did not exercise it", tok)
		}
	}
}
