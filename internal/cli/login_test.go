package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// Fake tokens: obviously fake, distinct per source so a leak names its
// path. No test output may ever contain one.
const (
	tokStored = "sk-ant-oat01-FAKE-stored-1111"
	tokCmd    = "sk-ant-oat01-FAKE-cmd-2222"
	tokEnv    = "sk-ant-api03-FAKE-env-3333"
	tokPrompt = "sk-ant-oat01-FAKE-prompt-4444"
	tokRaw    = "sk-ant-oat01-FAKE-raw-5555"
)

var fakeTokens = []string{tokStored, tokCmd, tokEnv, tokPrompt, tokRaw}

// loginProv is a fake provider that runs on logins with Claude's real
// variable mapping and lifetimes.
type loginProv struct{ *fakeProv }

func (loginProv) LoginKinds() []provider.LoginKind { return (&claude.Provider{}).LoginKinds() }
func (loginProv) LoginEnv(k provider.LoginKind, v string) (map[string]string, []string) {
	return (&claude.Provider{}).LoginEnv(k, v)
}
func (loginProv) LoginLifetime(k provider.LoginKind) time.Duration {
	return (&claude.Provider{}).LoginLifetime(k)
}

// loginConfig exercises every precedence step: a provider default per
// provider, a profile login, a pinned home, an api-billed profile with its
// own credential, and a second provider.
const loginConfig = `
logins:
  - {name: p, token: stored, added: 2026-09-01}
  - {name: w, token_env: W_TOKEN}
  - {name: c, provider: codex, token: stored}
providers:
  claude: {login: w}
  codex: {login: c}
profiles:
  - {name: personal, provider: claude, home: HOME/.claude, default: true}
  - {name: work, provider: claude, home: HOME/.claude-work, login: p}
  - {name: pinned, provider: claude, home: HOME/.claude-pin, login: home}
  - {name: kimi, provider: claude, home: HOME/.claude-kimi, billing: api, env: {ANTHROPIC_AUTH_TOKEN: x}}
  - {name: cx, provider: codex, home: HOME/.codex}
`

// loginHarness is a harness whose providers run on logins; HOME/ in yaml
// becomes the harness home.
func loginHarness(t *testing.T, yaml string) *harness {
	t.Helper()
	h := newHarness(t, "")
	raw, err := os.ReadFile(h.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw) + strings.ReplaceAll(yaml, "HOME/", h.home+"/")
	if err := os.WriteFile(h.cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	codex := &fakeProv{id: provider.Codex, usage: map[string]provider.Usage{}, errs: map[string]error{}, convs: map[string][]provider.Conversation{}, history: map[string]map[string]bool{}}
	h.provs = []provider.Provider{loginProv{h.claude}, loginProv{codex}}
	return h
}

// tokensPath is tokens.yaml beside the harness config.
func (h *harness) tokensPath() string {
	return filepath.Join(filepath.Dir(h.cfgPath), login.TokenFileName)
}

func (h *harness) storeToken(name, value string) {
	h.t.Helper()
	if err := (login.TokenFile{Path: h.tokensPath()}).Set(name, value); err != nil {
		h.t.Fatal(err)
	}
}

// dryLogin runs a dry run and returns its login line and exit code.
func (h *harness) dryLogin(args ...string) (string, int) {
	h.t.Helper()
	code := h.run(append([]string{"run", "--dry-run"}, args...)...)
	for _, line := range strings.Split(h.stdout.String(), "\n") {
		if v, ok := strings.CutPrefix(line, "login:"); ok {
			return strings.TrimSpace(v), code
		}
	}
	return "", code
}

// TestLogin_Precedence walks spec §5.4 as a matrix: --login × AGX_LOGIN ×
// the profile's login × the provider default × home, with the wrong
// provider, an incompatible profile, unknown names and auto at each step.
func TestLogin_Precedence(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, loginConfig)
	for _, c := range []struct {
		profile, flag, env string
		envSet             bool
		want               string // login line; "" when the run fails
		code               int
		note               string // stderr substring
	}{
		// Steps 3–5 alone.
		{profile: "personal", want: "w (provider)"},
		{profile: "work", want: "p (profile)"},
		{profile: "pinned", want: "home (profile)"},
		{profile: "kimi", want: "home"}, // provider default does not fit an api-billed profile
		{profile: "cx", want: "c (provider)"},
		// Step 1 beats everything.
		{profile: "personal", flag: "p", want: "p (--login)"},
		{profile: "work", flag: "home", want: "home (--login)"},
		{profile: "work", flag: "w", env: "p", envSet: true, want: "w (--login)"},
		{profile: "pinned", flag: "p", want: "p (--login)"},
		{profile: "cx", flag: "home", env: "p", envSet: true, want: "home (--login)"},
		// Step 1 errors: asked for explicitly.
		{profile: "personal", flag: "c", code: ExitUsage, note: "is a codex login, not claude"},
		{profile: "cx", flag: "p", code: ExitUsage, note: "is a claude login, not codex"},
		{profile: "kimi", flag: "p", code: ExitUsage, note: "billed per token"},
		{profile: "personal", flag: "nosuch", code: ExitUsage, note: `unknown login "nosuch"`},
		{profile: "personal", flag: "auto", code: ExitUsage, note: "not available yet"},
		// Step 2 beats the profile and provider defaults.
		{profile: "work", env: "w", envSet: true, want: "w (AGX_LOGIN)"},
		{profile: "pinned", env: "w", envSet: true, want: "w (AGX_LOGIN)"},
		{profile: "personal", env: "home", envSet: true, want: "home (AGX_LOGIN)"},
		{profile: "personal", env: "", envSet: true, want: "w (provider)"}, // empty = unset
		// Step 2 skips with a note when it does not fit.
		{profile: "cx", env: "p", envSet: true, want: "c (provider)", note: "AGX_LOGIN=p skipped for profile cx"},
		{profile: "kimi", env: "p", envSet: true, want: "home", note: "AGX_LOGIN=p skipped for profile kimi"},
		{profile: "personal", env: "c", envSet: true, want: "w (provider)", note: "skipped for profile personal"},
		// Step 2 errors: a typo or auto.
		{profile: "personal", env: "nosuch", envSet: true, code: ExitUsage, note: "AGX_LOGIN=nosuch: unknown login"},
		{profile: "personal", env: "auto", envSet: true, code: ExitUsage, note: "not available yet"},
	} {
		name := c.profile + "/flag=" + c.flag + "/env=" + c.env
		delete(h.env, appmeta.EnvLogin)
		if c.envSet {
			h.env[appmeta.EnvLogin] = c.env
		}
		args := []string{"-p", c.profile}
		if c.flag != "" {
			args = append(args, "--login", c.flag)
		}
		got, code := h.dryLogin(args...)
		if code != c.code || got != c.want {
			t.Errorf("%s: login %q exit %d, want %q exit %d; stderr %s", name, got, code, c.want, c.code, h.stderr.String())
		}
		if c.note != "" && !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("%s: stderr %q lacks %q", name, h.stderr.String(), c.note)
		}
		if c.note == "" && h.stderr.Len() > 0 {
			t.Errorf("%s: unexpected stderr %q", name, h.stderr.String())
		}
		// `agx profiles --json` reports the same choice without a flag.
		if c.flag == "" && c.code == ExitOK {
			if code := h.run("profiles", "--json"); code != ExitOK {
				t.Fatalf("profiles: %d", code)
			}
			for _, r := range decode[[]profileJSON](t, h.stdout.Bytes()).Data {
				if r.Name != c.profile {
					continue
				}
				desc := r.LoginName
				if r.LoginSource != loginSrcHome {
					desc += " (" + r.LoginSource + ")"
				}
				if desc != c.want {
					t.Errorf("%s: profiles says %q", name, desc)
				}
			}
		}
	}
}

func envOf(c provider.Command, key string) (string, bool) {
	for _, kv := range c.Env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// TestLogin_LaunchEnv: a real (non-dry) launch per kind and source sets the
// login's variable to the resolved token, removes every other inherited
// credential, keeps the profile's home, and prints the launch line.
func TestLogin_LaunchEnv(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, strings.Replace(loginConfig, "logins:\n", "logins:\n  - {name: k, provider: claude, kind: api-key, token_cmd: \"printf '%s\\\\n' "+tokCmd+"\"}\n", 1))
	h.storeToken("p", tokStored)
	h.env[claude.OAuthTokenEnv] = "inherited-oauth"
	h.env[claude.APIKeyEnv] = "inherited-key"
	h.env[claude.AuthTokenEnv] = "inherited-auth"
	h.env["W_TOKEN"] = tokEnv
	for _, c := range []struct {
		args        []string
		setVar, val string
		home        string
		line        string
	}{
		{[]string{"-p", "work"}, claude.OAuthTokenEnv, tokStored, ".claude-work", "work · login p"},
		{[]string{"-p", "personal"}, claude.OAuthTokenEnv, tokEnv, ".claude", "personal · login w"},
		{[]string{"-p", "work", "--login", "k"}, claude.APIKeyEnv, tokCmd, ".claude-work", "work · login k"},
	} {
		if code := h.run(append([]string{"run"}, c.args...)...); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", c.args, code, h.stderr.String())
		}
		cmd := h.lastExec()
		if v, _ := envOf(cmd, c.setVar); v != c.val {
			t.Errorf("%v: %s = %q", c.args, c.setVar, v)
		}
		for _, other := range []string{claude.OAuthTokenEnv, claude.APIKeyEnv, claude.AuthTokenEnv} {
			if other == c.setVar {
				continue
			}
			if v, ok := envOf(cmd, other); ok {
				t.Errorf("%v: inherited %s=%q survived", c.args, other, v)
			}
		}
		if v, _ := envOf(cmd, "HOME_DIR"); v != filepath.Join(h.home, c.home) {
			t.Errorf("%v: home %q", c.args, v)
		}
		if !strings.Contains(h.stderr.String(), "agx: "+c.line) {
			t.Errorf("%v: launch line missing: %q", c.args, h.stderr.String())
		}
	}
	// The home login leaves inherited variables alone and prints no line.
	if code := h.run("run", "-p", "pinned"); code != ExitOK {
		t.Fatal(code)
	}
	if v, _ := envOf(h.lastExec(), claude.APIKeyEnv); v != "inherited-key" || h.stderr.Len() != 0 {
		t.Errorf("home login changed the env or printed %q", h.stderr.String())
	}

	// Failures stop the launch; never a silent fall back to the home login.
	n := len(h.execs)
	delete(h.env, "W_TOKEN")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-p", "personal"}, "W_TOKEN is not set"},
		{[]string{"-p", "work", "--login", "c"}, "is a codex login"},
	} {
		if code := h.run(append([]string{"run"}, c.args...)...); code == ExitOK || !strings.Contains(h.stderr.String(), c.want) {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, h.stderr.String())
		}
	}
	h.storeToken("p", "  ")
	if code := h.run("run", "-p", "work"); code == ExitOK || !strings.Contains(h.stderr.String(), "token is empty") {
		t.Errorf("empty stored token: %d %q", code, h.stderr.String())
	}
	if err := os.Remove(h.tokensPath()); err != nil {
		t.Fatal(err)
	}
	if code := h.run("run", "-p", "work"); code == ExitOK || !strings.Contains(h.stderr.String(), "no stored token") {
		t.Errorf("missing stored token: %d %q", code, h.stderr.String())
	}
	if len(h.execs) != n {
		t.Errorf("a failed login still launched")
	}

	// Past the lifetime the run is refused with the renew command.
	h.storeToken("p", tokStored)
	h.now = time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC)
	if code := h.run("run", "-p", "work"); code == ExitOK || !strings.Contains(h.stderr.String(), "expired on 2027-09-01; renew: agx login add p --setup --replace") {
		t.Errorf("expired: %d %q", code, h.stderr.String())
	}
}

// TestLogin_PlanCarriesLogin: new/resume through the shell layer write
// --login into the plan, and exec launches on it.
func TestLogin_PlanCarriesLogin(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, loginConfig)
	h.storeToken("p", tokStored)
	planPath := filepath.Join(h.root, "plan.json")
	h.env[appmeta.EnvPlanFile] = planPath
	if code := h.run("new", "-p", "personal", "--login", "p", "topic"); code != ExitOK {
		t.Fatalf("new: %d %s", code, h.stderr.String())
	}
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var pl plan
	if err := json.Unmarshal(raw, &pl); err != nil || pl.Login != "p" || strings.Contains(string(raw), tokStored) {
		t.Fatalf("plan = %s (%v)", raw, err)
	}
	delete(h.env, appmeta.EnvPlanFile)
	if code := h.run("exec", "--plan", planPath); code != ExitOK {
		t.Fatalf("exec: %d %s", code, h.stderr.String())
	}
	if v, _ := envOf(h.lastExec(), claude.OAuthTokenEnv); v != tokStored {
		t.Errorf("exec did not run on login p")
	}
}

// TestLogin_Commands drives add / ls / check / use / rm / exists the way a
// user does and checks what they see.
func TestLogin_Commands(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, "profiles:\n  - {name: personal, provider: claude, home: HOME/.claude}\n  - {name: work, provider: claude, home: HOME/.claude-work}\n")
	h.stdin = tokStored + "\n"
	if code := h.run("login", "add", "p", "--stdin", "--account", "me@example.com"); code != ExitOK {
		t.Fatalf("add: %d %s", code, h.stderr.String())
	}
	if out := h.stdout.String(); !strings.Contains(out, "added login p (claude, oauth-token, stored)") || !strings.Contains(out, "login p: "+checkPending) {
		t.Errorf("add output: %q", out)
	}
	if !strings.Contains(h.stderr.String(), syncWarning) {
		t.Errorf("no sync warning: %q", h.stderr.String())
	}
	if info, err := os.Stat(h.tokensPath()); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("tokens.yaml: %v %v", info, err)
	}
	if code := h.run("login", "add", "w", "--cmd", "printf "+tokCmd); code != ExitOK {
		t.Fatalf("add cmd: %d %s", code, h.stderr.String())
	}
	// An unset env source: the login is kept, but the check fails.
	if code := h.run("login", "add", "k", "--env", "K_TOKEN", "--kind", "api-key"); code != ExitFailure || !strings.Contains(h.stdout.String(), "K_TOKEN is not set") {
		t.Errorf("add env (unset): exit %d %q", code, h.stdout.String())
	}
	h.secretInput = tokPrompt
	if code := h.run("login", "add", "s", "--setup"); code != ExitOK {
		t.Fatalf("add --setup: %d %s", code, h.stderr.String())
	}
	if len(h.interactive) != 1 || h.interactive[0].Path != claude.Binary || !slices.Equal(h.interactive[0].Args, []string{claude.SetupTokenSubcommand}) {
		t.Errorf("setup ran %+v", h.interactive)
	}
	if code := h.run("login", "add", "q"); code != ExitOK {
		t.Fatalf("add (prompt): %d %s", code, h.stderr.String())
	}
	stored, _ := (login.TokenFile{Path: h.tokensPath()}).Load()
	if stored["p"] != tokStored || stored["s"] != tokPrompt || stored["q"] != tokPrompt || len(stored) != 3 {
		t.Errorf("stored = %d entries", len(stored))
	}

	// Refusals at add.
	for _, c := range []struct {
		args  []string
		stdin string
		want  string
	}{
		{[]string{"p", "--stdin"}, tokStored, "already exists (use --replace"},
		{[]string{"home", "--stdin"}, tokStored, "reserved"},
		{[]string{"Bad", "--stdin"}, tokStored, "must match"},
		{[]string{"x", "--stdin"}, tokStored + "\n" + tokStored, "more than one line"},
		{[]string{"x", "--stdin"}, " \n", "token is empty"},
		{[]string{"x", "--cmd", "a", "--env", "B"}, "", "give one token source"},
		{[]string{"x", "--stdin", "--setup"}, "", "cannot be combined"},
		{[]string{"x", "--kind", "password", "--env", "B"}, "", `"password" is not supported`},
		{[]string{"x", "--provider", "gemini", "--env", "B"}, "", "gemini"},
	} {
		h.stdin = c.stdin
		if code := h.run(append([]string{"login", "add"}, c.args...)...); code != ExitUsage || !strings.Contains(h.stderr.String(), c.want) {
			t.Errorf("add %v: exit %d %q", c.args, code, h.stderr.String())
		}
	}
	if all, _ := (login.TokenFile{Path: h.tokensPath()}).Load(); all["x"] != "" {
		t.Error("a refused add stored a token")
	}
	// Rotation replaces the token and keeps the kind and account when they
	// are not given again.
	h.stdin = tokRaw
	if code := h.run("login", "add", "p", "--stdin", "--replace"); code != ExitOK || !strings.Contains(h.stdout.String(), "replaced login p") {
		t.Errorf("replace: %d %q", code, h.stdout.String())
	}
	h.env["K2"] = tokEnv
	if code := h.run("login", "add", "k", "--env", "K2", "--replace"); code != ExitOK || !strings.Contains(h.stdout.String(), "replaced login k (claude, api-key, env)") {
		t.Errorf("replace api-key login: %d %q", code, h.stdout.String())
	}
	if stored, _ := (login.TokenFile{Path: h.tokensPath()}).Load(); stored["p"] != tokRaw {
		t.Error("replace did not rotate the stored token")
	}

	// ls (text + JSON), use, USED-BY, exists, rm.
	if code := h.run("login", "use", "p", "-p", "work"); code != ExitOK || !strings.Contains(h.stdout.String(), "profiles[work].login\n- login: (unset)\n+ login: p") {
		t.Errorf("use: %d %q %q", code, h.stdout.String(), h.stderr.String())
	}
	if code := h.run("login", "ls"); code != ExitOK {
		t.Fatal(code)
	}
	for _, want := range []string{"NAME", "USED-BY", "me@example.com", "2026-09-24", "2027-09-24", "~2027-09-24", "work"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("ls lacks %q:\n%s", want, h.stdout.String())
		}
	}
	if code := h.run("login", "ls", "--json"); code != ExitOK {
		t.Fatal(code)
	}
	rows := decode[[]loginRow](t, h.stdout.Bytes()).Data
	byName := map[string]loginRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	if r := byName["p"]; r.Source != login.SourceStored || r.State != stateOK || !slices.Equal(r.UsedBy, []string{"work"}) || r.ExpiresApprox {
		t.Errorf("p row = %+v", r)
	}
	if r := byName["w"]; r.Source != login.SourceCmd || !r.ExpiresApprox {
		t.Errorf("w row = %+v", r)
	}
	if r := byName["k"]; r.Kind != provider.LoginAPIKey || r.Expires != nil {
		t.Errorf("k row = %+v", r)
	}
	for _, c := range []struct {
		name string
		code int
	}{{"p", ExitOK}, {"home", ExitOK}, {"nosuch", ExitFailure}, {"auto", ExitUsage}} {
		if code := h.run("login", "exists", c.name); code != c.code {
			t.Errorf("exists %s: %d", c.name, code)
		}
	}
	if code := h.run("login", "rm", "p"); code != ExitFailure || !strings.Contains(h.stderr.String(), "used by work") {
		t.Errorf("rm in use: %d %q", code, h.stderr.String())
	}
	if code := h.run("login", "rm", "p", "--force"); code != ExitOK || !strings.Contains(h.stdout.String(), "cleared profiles[work].login") || !strings.Contains(h.stderr.String(), "does not revoke") {
		t.Errorf("rm --force: %d %q %q", code, h.stdout.String(), h.stderr.String())
	}
	stored, _ = (login.TokenFile{Path: h.tokensPath()}).Load()
	if _, ok := stored["p"]; ok {
		t.Error("rm left the token in tokens.yaml")
	}
	if code := h.run("login", "rm", "nosuch"); code != ExitUsage {
		t.Errorf("rm nosuch: %d", code)
	}
	// use: argument checks.
	for _, args := range [][]string{{"w"}, {"w", "-p", "work", "--provider", "claude"}, {"auto", "-p", "work"}, {"nosuch", "-p", "work"}, {"w", "-p", "ghost"}, {"w", "--provider", "codex"}} {
		if code := h.run(append([]string{"login", "use"}, args...)...); code != ExitUsage {
			t.Errorf("use %v: %d", args, code)
		}
	}
	// usage / plan refuse token logins, accept home.
	for _, cmd := range []string{"usage", "plan"} {
		if code := h.run(cmd, "--login", "w"); code != ExitUsage || !strings.Contains(h.stderr.String(), "not available yet") {
			t.Errorf("%s --login w: %d %q", cmd, code, h.stderr.String())
		}
		if code := h.run(cmd, "--login", "auto"); code != ExitUsage {
			t.Errorf("%s --login auto: %d", cmd, code)
		}
	}
	if code := h.run("usage", "--login", "home"); code != ExitOK {
		t.Errorf("usage --login home: %d %s", code, h.stderr.String())
	}
}

// TestLogin_TokenFilePermissions: a tokens.yaml others can read is refused
// by every command that would read it, with the chmod that fixes it.
func TestLogin_TokenFilePermissions(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, loginConfig)
	h.storeToken("p", tokStored)
	if err := os.Chmod(h.tokensPath(), 0o644); err != nil {
		t.Fatal(err)
	}
	fix := "chmod 600 " + h.tokensPath()
	if code := h.run("run", "-p", "work"); code == ExitOK || !strings.Contains(h.stderr.String(), fix) || len(h.execs) != 0 {
		t.Errorf("run: %d %q", code, h.stderr.String())
	}
	if code := h.run("login", "check", "p"); code != ExitFailure || !strings.Contains(h.stdout.String(), fix) {
		t.Errorf("check: %d %q", code, h.stdout.String())
	}
	if code := h.run("login", "ls"); code != ExitOK || !strings.Contains(h.stdout.String(), "stored (unreadable)") || !strings.Contains(h.stderr.String(), fix) {
		t.Errorf("ls: %d %q %q", code, h.stdout.String(), h.stderr.String())
	}
	if code := h.run("doctor"); code != ExitFailure || !strings.Contains(h.stdout.String(), fix) || !strings.Contains(h.stdout.String(), "token unreadable") {
		t.Errorf("doctor: %d %q", code, h.stdout.String())
	}
	h.stdin = tokRaw
	if code := h.run("login", "add", "x", "--stdin"); code == ExitOK || !strings.Contains(h.stderr.String(), fix) {
		t.Errorf("add: %d %q", code, h.stderr.String())
	}
	if info, _ := os.Stat(h.tokensPath()); info.Mode().Perm() != 0o644 {
		t.Error("agx changed the user's file mode")
	}
}

// TestLogin_DoctorExpiry: ok, warn inside the window, fail after; cmd/env
// logins say "approximately".
func TestLogin_DoctorExpiry(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, loginConfig+"")
	h.storeToken("p", tokStored)
	h.storeToken("c", tokStored)
	h.env["W_TOKEN"] = tokEnv
	h.env["C_TOKEN"] = tokEnv
	loginLine := func(name string) string {
		for _, line := range strings.Split(h.stdout.String(), "\n") {
			if f := strings.Fields(line); len(f) > 2 && f[1] == "login" && f[2] == name {
				return line
			}
		}
		return ""
	}
	for _, c := range []struct {
		now          time.Time
		mark, detail string
		code         int
	}{
		{time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "✓", "oauth-token (stored), expires 2027-09-01", ExitOK},
		{time.Date(2027, 8, 20, 0, 0, 0, 0, time.UTC), "!", "expires 2027-09-01 (in 12 d", ExitOK},
		{time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC), "✗", "expired 2027-09-01; renew: agx login add p --setup --replace", ExitFailure},
		{time.Date(2027, 9, 2, 0, 0, 0, 0, time.UTC), "✗", "expired 2027-09-01; renew", ExitFailure},
	} {
		h.now = c.now
		code := h.run("doctor")
		line := loginLine("p")
		if !strings.HasPrefix(line, c.mark) || !strings.Contains(line, c.detail) || code != c.code {
			t.Errorf("%s: exit %d, login p line %q, want %s …%s", c.now.Format(time.DateOnly), code, line, c.mark, c.detail)
		}
	}
	// A cmd/env login's expiry is approximate: agx does not own the value.
	h.stdin = ""
	h.now = time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	if code := h.run("login", "add", "e", "--env", "C_TOKEN"); code != ExitOK {
		t.Fatalf("add: %s", h.stderr.String())
	}
	h.run("doctor")
	if line := loginLine("e"); !strings.Contains(line, "expires approximately 2027-09-24") {
		t.Errorf("approximate expiry: %q", line)
	}
	h.now = time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	delete(h.env, "W_TOKEN")
	h.run("doctor")
	if !strings.Contains(h.stdout.String(), "W_TOKEN is not set in this shell") {
		t.Errorf("doctor env source:\n%s", h.stdout.String())
	}
}

// TestLogin_NoLeak runs every login-aware command, including the error
// paths, with known fake tokens and asserts no token ever reaches stdout or
// stderr. A real launch is the positive control: the tokens were resolved,
// so their absence from output means something.
func TestLogin_NoLeak(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, "profiles:\n  - {name: personal, provider: claude, home: HOME/.claude}\n  - {name: work, provider: claude, home: HOME/.claude-work}\n  - {name: cx, provider: codex, home: HOME/.codex}\n")
	h.env["E_TOK"] = tokEnv
	h.secretInput = tokPrompt
	var transcript strings.Builder
	step := func(stdin string, args ...string) {
		t.Helper()
		h.stdin = stdin
		code := h.run(args...)
		out := h.stdout.String() + h.stderr.String()
		transcript.WriteString("$ agx " + strings.Join(args, " ") + "\n" + out)
		for _, tok := range fakeTokens {
			if strings.Contains(out, tok) {
				t.Errorf("agx %v (exit %d) leaked %s:\n%s", args, code, tok, out)
			}
		}
	}
	step(tokStored+"\n", "login", "add", "p", "--stdin", "--account", "me@example.com")
	step("", "login", "add", "w", "--cmd", "printf '%s\\n' "+tokCmd)
	step("", "login", "add", "e", "--env", "E_TOK", "--kind", "api-key")
	step("", "login", "add", "s", "--setup")
	step("", "login", "add", "q")
	step("", "login", "add", "cxl", "--provider", "codex", "--cmd", "printf "+tokCmd)
	step(tokStored, "login", "add", "p", "--stdin")
	step(tokStored+"\n"+tokCmd, "login", "add", "multi", "--stdin")
	// A failing command that echoes its token to stderr too (set -x style).
	// stderr is shown by design, so only what it repeats from stdout can be
	// hidden; that is what this pins.
	step("", "login", "add", "bad", "--cmd", "echo "+tokCmd+"; echo got "+tokCmd+" >&2; exit 1")
	step("", "login", "add", "two", "--cmd", "printf '%s\\n%s\\n' "+tokCmd+" "+tokCmd)
	for _, args := range [][]string{
		{"login", "ls"}, {"login", "ls", "--json"}, {"login", "check"}, {"login", "check", "p"}, {"login", "check", "bad"}, {"login", "check", "two"},
		{"login", "use", "p", "-p", "work"}, {"login", "use", "w", "--provider", "claude"}, {"login", "use", "e", "-p", "personal"},
		{"login", "exists", "p"}, {"login", "exists", "nosuch"},
		{"profiles"}, {"profiles", "--json"}, {"doctor"}, {"doctor", "--json"},
		{"run", "-p", "work", "--dry-run"}, {"run", "-p", "personal", "--dry-run"}, {"run", "-p", "work", "--login", "w", "--dry-run"},
		{"run", "-p", "work"}, {"run", "-p", "personal"}, {"run", "-p", "work", "--login", "w"}, {"run", "-p", "work", "--login", "s"},
		{"run", "-p", "work", "--login", "bad"}, {"run", "-p", "work", "--login", "two"}, {"run", "-p", "cx", "--login", "p"},
		{"new", "-p", "work", "--login", "q", "--dry-run", "t"}, {"usage", "--login", "p"}, {"plan", "--login", "p"},
		{"shell-init", "zsh"},
	} {
		step("", args...)
	}
	h.env[appmeta.EnvLogin] = "w"
	step("", "run", "-p", "cx")
	step("", "profiles")
	delete(h.env, appmeta.EnvLogin)

	// Positive control: the real launches carried the tokens.
	var seen []string
	for _, c := range h.execs {
		for _, tok := range fakeTokens {
			if slices.Contains(c.Env, claude.OAuthTokenEnv+"="+tok) || slices.Contains(c.Env, claude.APIKeyEnv+"="+tok) {
				seen = append(seen, tok)
			}
		}
	}
	for _, tok := range []string{tokStored, tokCmd, tokEnv, tokPrompt} {
		if !slices.Contains(seen, tok) {
			t.Errorf("no launch carried %s: the sweep did not exercise that source", tok)
		}
	}

	// Refused token file and expiry paths.
	if err := os.Chmod(h.tokensPath(), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"login", "ls"}, {"login", "check"}, {"run", "-p", "work"}, {"doctor"}} {
		step("", args...)
	}
	if err := os.Chmod(h.tokensPath(), 0o600); err != nil {
		t.Fatal(err)
	}
	h.now = h.now.AddDate(2, 0, 0)
	for _, args := range [][]string{{"login", "ls"}, {"login", "check"}, {"run", "-p", "work"}, {"doctor"}} {
		step("", args...)
	}
	step("", "login", "rm", "p", "--force")
	step("", "login", "rm", "e", "--force")

	// A raw token pasted into config.yaml: every command refuses to load it
	// and none echoes it.
	raw, _ := os.ReadFile(h.cfgPath)
	if err := os.WriteFile(h.cfgPath, []byte(strings.Replace(string(raw), "logins:\n", "logins:\n  - name: pasted\n    token: "+tokRaw+"\n", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"login", "ls"}, {"profiles"}, {"run", "-p", "work"}, {"doctor"}, {"login", "add", "z", "--env", "Z"}} {
		step("", args...)
		if !strings.Contains(h.stderr.String()+h.stdout.String(), "a raw token does not belong in config.yaml") {
			t.Errorf("%v did not report the raw token:\n%s%s", args, h.stdout.String(), h.stderr.String())
		}
	}
	if t.Failed() {
		t.Logf("transcript:\n%s", transcript.String())
	}
}
