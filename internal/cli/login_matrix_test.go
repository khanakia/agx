package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// boundaryConfig has a stored and a cmd login added on the same day, so
// both share one expiry instant: added (UTC midnight) + the setup-token
// lifetime.
const boundaryConfig = `
logins:
  - {name: st, token: stored, added: 2026-09-01}
  - {name: cm, token_cmd: "printf fake-token-0001", added: 2026-09-01}
profiles:
  - {name: personal, provider: claude, home: HOME/.claude, default: true}
`

// TestLogin_ExpiryBoundary: the expiry instant itself is expired (the check
// is "now is not before expiry"), one nanosecond earlier still runs. A
// stored login is refused from that instant; a cmd login warns and runs.
func TestLogin_ExpiryBoundary(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(claude.SetupTokenLifetime)
	for _, c := range []struct {
		name  string
		login string
		now   time.Time
		runs  bool
		warn  string
	}{
		{"stored, 1ns before", "st", at.Add(-time.Nanosecond), true, ""},
		{"stored, at expiry", "st", at, false, "login st expired on 2027-09-01"},
		{"stored, a day after", "st", at.Add(24 * time.Hour), false, "login st expired on 2027-09-01"},
		{"cmd, 1ns before", "cm", at.Add(-time.Nanosecond), true, ""},
		{"cmd, at expiry", "cm", at, true, "login cm is past its approximate expiry (2027-09-01"},
		{"stored, other zone same instant", "st", at.In(time.FixedZone("UTC+14", 14*3600)), false, "expired on"},
	} {
		h := loginHarness(t, boundaryConfig)
		h.storeToken("st", tokStored)
		h.now = c.now
		code := h.run("run", "--login", c.login)
		if ran := len(h.execs) > 0; ran != c.runs || (code == ExitOK) != c.runs {
			t.Errorf("%s: ran %v exit %d, want ran %v; stderr %s", c.name, ran, code, c.runs, h.stderr.String())
		}
		if c.warn != "" && !strings.Contains(h.stderr.String(), c.warn) {
			t.Errorf("%s: stderr %q lacks %q", c.name, h.stderr.String(), c.warn)
		}
		if c.warn == "" && strings.Contains(h.stderr.String(), "expir") {
			t.Errorf("%s: unexpected expiry note %q", c.name, h.stderr.String())
		}
		assertNoToken(t, c.name, h)
	}
}

// assertNoToken fails when any fake token reached stdout or stderr.
func assertNoToken(t *testing.T, name string, h *harness) {
	t.Helper()
	for _, tok := range append([]string{"fake-token-0001"}, fakeTokens...) {
		if strings.Contains(h.stdout.String(), tok) || strings.Contains(h.stderr.String(), tok) {
			t.Errorf("%s: output leaked a token", name)
		}
	}
}

// TestLogin_AddInteractiveFailures: when the setup command or the hidden
// prompt fails, or --setup is asked of a provider it does not exist for,
// nothing is written to config.yaml or tokens.yaml.
func TestLogin_AddInteractiveFailures(t *testing.T) {
	t.Parallel()
	errSetup := errors.New("setup-token exited 1")
	errPrompt := errors.New("not a terminal")
	for _, c := range []struct {
		name        string
		args        []string
		interactive error
		prompt      error
		code        int
		note        string
	}{
		{name: "setup fails", args: []string{"--setup"}, interactive: errSetup, code: ExitFailure, note: "setup-token exited 1"},
		{name: "prompt fails", args: nil, prompt: errPrompt, code: ExitFailure, note: "not a terminal"},
		{name: "setup after prompt fails", args: []string{"--setup"}, prompt: errPrompt, code: ExitFailure, note: "not a terminal"},
		{name: "setup on codex", args: []string{"--setup", "--provider", "codex"}, code: ExitUsage, note: "--setup is only known for claude"},
	} {
		h := loginHarness(t, "profiles:\n  - {name: personal, provider: claude, home: HOME/.claude}\n")
		before, err := os.ReadFile(h.cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		d := h.deps()
		d.RunInteractive = func(provider.Command) error { return c.interactive }
		d.PromptSecret = func(string) (string, error) {
			if c.prompt != nil {
				return "", c.prompt
			}
			return tokPrompt, nil
		}
		code := runWith(h, d, h.cwd, append([]string{"login", "add", "x"}, c.args...)...)
		if code != c.code || !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("%s: exit %d, stderr %q; want %d %q", c.name, code, h.stderr.String(), c.code, c.note)
		}
		after, err := os.ReadFile(h.cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("%s: config.yaml changed:\n%s", c.name, after)
		}
		if _, err := os.Stat(h.tokensPath()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: tokens.yaml written (%v)", c.name, err)
		}
		assertNoToken(t, c.name, h)
	}
}

// TestLogin_LsStates: an empty list says how to add one; a stored login
// without its token, and one behind an unreadable token file, are marked in
// both the text and JSON views without failing the listing.
func TestLogin_LsStates(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, "")
	if code := h.run("login", "ls"); code != ExitOK || !strings.Contains(h.stdout.String(), "no logins — add one with `agx login add NAME`") {
		t.Errorf("empty ls: %d %q", code, h.stdout.String())
	}
	if code := h.run("login", "ls", "--json"); code != ExitOK || decode[[]loginRow](t, h.stdout.Bytes()).Count != 0 {
		t.Errorf("empty ls --json: %d %q", code, h.stdout.String())
	}

	h = loginHarness(t, boundaryConfig)
	stateOf := func() map[string]string {
		if code := h.run("login", "ls", "--json"); code != ExitOK {
			t.Fatalf("ls --json: %d %s", code, h.stderr.String())
		}
		out := map[string]string{}
		for _, r := range decode[[]loginRow](t, h.stdout.Bytes()).Data {
			out[r.Name] = r.State
		}
		return out
	}
	if s := stateOf(); s["st"] != stateMissing || s["cm"] != stateOK {
		t.Errorf("no token file: %v", s)
	}
	if code := h.run("login", "ls"); code != ExitOK || !strings.Contains(h.stdout.String(), "stored"+cellMissing) {
		t.Errorf("text missing: %q", h.stdout.String())
	}
	h.storeToken("st", tokStored)
	if s := stateOf(); s["st"] != stateOK {
		t.Errorf("stored: %v", s)
	}
	if err := os.Chmod(h.tokensPath(), 0o640); err != nil {
		t.Fatal(err)
	}
	if s := stateOf(); s["st"] != stateUnreadable || s["cm"] != stateOK || !strings.Contains(h.stderr.String(), "chmod 600") {
		t.Errorf("unreadable: %v, stderr %q", s, h.stderr.String())
	}
	if code := h.run("login", "ls"); code != ExitOK || !strings.Contains(h.stdout.String(), "stored"+cellRefused) {
		t.Errorf("text unreadable: %q", h.stdout.String())
	}
	assertNoToken(t, "ls", h)
}

// TestLogin_CheckRefusals: an unknown name is a usage error; an expired
// stored login fails without a probe (a probe spends quota).
func TestLogin_CheckRefusals(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, boundaryConfig)
	h.storeToken("st", tokStored)
	h.now = time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		code int
		out  string // stdout substring
		err  string // stderr substring
	}{
		{name: "nosuch", code: ExitUsage, err: `unknown login "nosuch"`},
		{name: "Ünïcode", code: ExitUsage, err: "unknown login"},
		{name: "st", code: ExitFailure, out: "login st: failed: login st expired on 2027-09-01"},
	} {
		code := h.run("login", "check", c.name)
		if code != c.code || !strings.Contains(h.stdout.String(), c.out) || !strings.Contains(h.stderr.String(), c.err) {
			t.Errorf("check %s: exit %d stdout %q stderr %q", c.name, code, h.stdout.String(), h.stderr.String())
		}
	}
	if n := h.probeCount(); n != 0 {
		t.Errorf("refused checks probed %d times", n)
	}
	assertNoToken(t, "check", h)
}

// TestLoginUse_RefusalMatrix: `login use` refuses what a launch would
// refuse — an incompatible profile, a provider without logins, a login of
// the wrong provider — and leaves config.yaml byte-for-byte unchanged.
func TestLoginUse_RefusalMatrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		args []string
		code int
		note string
	}{
		{name: "api-billed profile", args: []string{"p", "-p", "kimi"}, code: ExitUsage, note: "billed per token"},
		{name: "wrong provider for profile", args: []string{"c", "-p", "personal"}, code: ExitUsage, note: "is a codex login, not claude"},
		{name: "unregistered provider", args: []string{"p", "--provider", "gemini"}, code: ExitUsage, note: `unknown provider "gemini"`},
		{name: "unknown login", args: []string{"nope", "--provider", "claude"}, code: ExitUsage, note: `unknown login "nope"`},
		{name: "auto", args: []string{"auto", "--provider", "claude"}, code: ExitUsage, note: `"auto" is accepted only as --login`},
		{name: "neither target", args: []string{"p"}, code: ExitUsage, note: "exactly one of -p PROFILE or --provider"},
		{name: "home on api profile is fine", args: []string{"home", "-p", "kimi"}},
		{name: "profile login", args: []string{"p", "-p", "personal"}},
		{name: "provider default", args: []string{"p", "--provider", "claude"}},
	} {
		h := loginHarness(t, loginConfig)
		before, err := os.ReadFile(h.cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		code := h.run(append([]string{"login", "use"}, c.args...)...)
		if code != c.code || !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("%s: exit %d stderr %q, want %d %q", c.name, code, h.stderr.String(), c.code, c.note)
		}
		after, err := os.ReadFile(h.cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if changed := !bytes.Equal(before, after); changed != (c.code == ExitOK) {
			t.Errorf("%s: config changed=%v", c.name, changed)
		}
	}
}

// TestLogin_ProviderWithoutLogins: a provider lacking the login upgrade
// cannot take a login from any direction, with the provider's reason.
func TestLogin_ProviderWithoutLogins(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, "profiles:\n  - {name: personal, provider: claude, home: HOME/.claude, default: true}\n")
	codex := emptyProv(provider.Codex)
	codex.disc = []provider.Profile{{Name: "cx", Provider: provider.Codex, Home: filepath.Join(h.home, ".codex"), Billing: provider.BillingPlan, Source: provider.SourceDiscovered}}
	h.provs = []provider.Provider{loginProv{h.claude}, plainProv{codex}}
	h.storeToken("p", tokStored)
	for _, c := range []struct {
		name string
		args []string
	}{
		{"add", []string{"login", "add", "x", "--provider", "codex", "--env", "X_TOKEN"}},
		{"use provider", []string{"login", "use", login.Home, "--provider", "codex"}},
	} {
		if code := h.run(c.args...); code != ExitUsage || !strings.Contains(h.stderr.String(), "codex has no token logins yet") {
			t.Errorf("%s: exit %d stderr %q", c.name, code, h.stderr.String())
		}
	}
	// home always works: it is the home's own login, not a token.
	if code := h.run("run", "-p", "cx", "--login", login.Home); code != ExitOK {
		t.Errorf("run cx on home: %d %s", code, h.stderr.String())
	}
	if _, set := envOf(h.lastExec(), "CLAUDE_CODE_OAUTH_TOKEN"); set {
		t.Error("home login set a token variable")
	}
}
