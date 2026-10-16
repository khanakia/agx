package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// Fake tokens for the probe tests, one per login.
const (
	tokA = "sk-ant-oat01-FAKE-auto-a-0001"
	tokB = "sk-ant-oat01-FAKE-auto-b-0002"
	tokK = "sk-ant-api03-FAKE-auto-k-0003"
	tokX = "sk-ant-oat01-FAKE-auto-x-0004"
)

// probeConfig: two stored plan logins, an api-key login, an env login, and
// an api-billed profile no login fits. The personal home's usage is 77%
// (newHarness); work is 9%.
const probeConfig = `
logins:
  - {name: a, token: stored, account: a@example.com}
  - {name: b, token: stored}
  - {name: k, token: stored, kind: api-key}
  - {name: x, token_env: X_TOKEN}
profiles:
  - {name: personal, provider: claude, home: HOME/.claude, default: true}
  - {name: work, provider: claude, home: HOME/.claude-work}
  - {name: kimi, provider: claude, home: HOME/.claude-kimi, billing: api, env: {ANTHROPIC_AUTH_TOKEN: x}}
`

// pct is a usage with one session window at p percent.
func pct(p float64) provider.Usage {
	return provider.Usage{Windows: []provider.Window{{Kind: provider.WindowSession, Group: provider.GroupSession, Label: "Current session", Percent: p}}}
}

func probeHarness(t *testing.T) *harness {
	t.Helper()
	h := loginHarness(t, probeConfig)
	h.storeToken("a", tokA)
	h.storeToken("b", tokB)
	h.storeToken("k", tokK)
	h.claude.probeUsage = map[string]provider.Usage{tokA: pct(90), tokB: pct(95), tokX: pct(50)}
	h.claude.probeErrs = map[string]error{}
	return h
}

// TestLoginUsage_NeverProbesUnasked is the D14 guard: a probe spends real
// quota, so nothing but an explicit request may run one. Every everyday
// command runs here with logins configured and resolvable; the probe count
// must stay zero. The last step is the positive control.
func TestLoginUsage_NeverProbesUnasked(t *testing.T) {
	t.Parallel()
	h := probeHarness(t)
	h.env["X_TOKEN"] = tokX
	for _, args := range [][]string{
		{}, {"usage"}, {"usage", "--json"}, {"usage", "work"}, {"usage", "--login", "home"}, {"usage", "--provider", "claude"},
		{"profiles"}, {"profiles", "--json"}, {"login", "ls"}, {"login", "check", "--no-probe"}, {"login", "check", "a", "--no-probe"},
		{"login", "add", "y", "--env", "X_TOKEN"}, {"doctor"}, {"plan"},
		{"run", "--dry-run"}, {"run", "-p", "work", "--login", "a"}, {"run", "-p", "auto"}, {"new", "--dry-run", "t"},
	} {
		h.run(args...)
		if n := h.probeCount(); n != 0 {
			t.Fatalf("agx %v probed %d times; only --logins, --login NAME, --login auto and login check may", args, n)
		}
	}
	if code := h.run("usage", "--logins"); code != ExitOK || h.probeCount() == 0 {
		t.Fatalf("positive control: usage --logins exit %d, %d probes; stderr %s", code, h.probeCount(), h.stderr.String())
	}
}

// TestLoginUsage_Rows: `agx usage --logins` adds one row per login after
// the homes, `--login NAME` shows only that login, and the flags refuse
// combinations that cannot mean anything.
func TestLoginUsage_Rows(t *testing.T) {
	t.Parallel()
	h := probeHarness(t)
	h.env["X_TOKEN"] = tokX

	if code := h.run("usage", "--logins"); code != ExitOK {
		t.Fatalf("usage --logins: %d %s", code, h.stderr.String())
	}
	out := h.stdout.String()
	iHome, iA := strings.Index(out, "me@home"), strings.Index(out, "a@example.com  login a · claude  (probe)")
	if iHome < 0 || iA < 0 || iA < iHome {
		t.Errorf("home rows then login rows expected:\n%s", out)
	}
	for _, want := range []string{"login b  login b · claude  (probe)", "login k · claude", "billed per token", "login x · claude", "90%", "95%", "50%"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage --logins lacks %q:\n%s", want, out)
		}
	}
	if n := h.probeCount(); n != 3 {
		t.Errorf("probes = %d, want 3 (a, b, x; never the api-key login)", n)
	}

	// JSON: kind unchanged, login rows marked source probe, no profile/home.
	if code := h.run("usage", "--logins", "--json"); code != ExitOK {
		t.Fatalf("json: %d", code)
	}
	env := decode[[]usageJSON](t, h.stdout.Bytes())
	if env.Kind != kindUsageList || env.Count != 7 {
		t.Fatalf("kind %q count %d, want usage.list 7 (3 homes + 4 logins)", env.Kind, env.Count)
	}
	for _, r := range env.Data {
		switch {
		case r.Login == "" && (r.Source != "" || r.Profile == "" || r.Home == ""):
			t.Errorf("home row %+v", r)
		case r.Login != "" && (r.Source != usageSourceProbe || r.Profile != "" || r.Home != ""):
			t.Errorf("login row %+v", r)
		case r.Login == "a" && (r.Account != "a@example.com" || len(r.Windows) != 1 || r.Windows[0].Percent != 90):
			t.Errorf("login a %+v", r)
		case r.Login == "k" && (r.Billing != provider.BillingAPI || r.Error == ""):
			t.Errorf("login k %+v", r)
		}
	}

	// --login NAME: that login only.
	if code := h.run("usage", "--login", "b", "--json"); code != ExitOK {
		t.Fatalf("--login b: %d %s", code, h.stderr.String())
	}
	if env := decode[[]usageJSON](t, h.stdout.Bytes()); env.Count != 1 || env.Data[0].Login != "b" || env.Data[0].Windows[0].Percent != 95 {
		t.Errorf("--login b: %+v", env.Data)
	}

	// --provider filters login rows too.
	if code := h.run("usage", "--logins", "--provider", "codex", "--json"); code != ExitUsage {
		t.Errorf("codex has no homes or logins here: %d %s", code, h.stdout.String())
	}

	// A failing probe is an error row and a partial exit, never a 0% row.
	h.claude.probeErrs[tokB] = provider.ErrUnauthorized
	delete(h.env, "X_TOKEN")
	if code := h.run("usage", "--logins"); code != ExitFailure {
		t.Errorf("failed probe: exit %d", code)
	}
	if out := h.stdout.String(); !strings.Contains(out, "error: "+provider.ErrUnauthorized.Error()) || !strings.Contains(out, "X_TOKEN") {
		t.Errorf("failed rows:\n%s", out)
	}

	// Refused combinations.
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"usage", "--login", "auto"}, "use `agx usage --logins`"},
		{[]string{"usage", "--login", "a", "--logins"}, "cannot be combined"},
		{[]string{"usage", "--login", "a", "personal"}, "cannot be combined"},
		{[]string{"usage", "--login", "nosuch"}, `unknown login "nosuch"`},
	} {
		if code := h.run(c.args...); code != ExitUsage || !strings.Contains(h.stderr.String(), c.want) {
			t.Errorf("%v: %d %q", c.args, code, h.stderr.String())
		}
	}
	for _, tok := range []string{tokA, tokB, tokK, tokX} {
		if strings.Contains(h.stdout.String()+h.stderr.String(), tok) {
			t.Errorf("output leaks %s", tok)
		}
	}
}

// TestLoginAuto_Matrix: `--login auto` picks among the home login (usage
// endpoint) and every login that fits the profile (probe) with the pick
// policy unchanged: lowest max percent wins, ties go to the home and then
// config order, a failed probe or resolve makes a login ineligible, and
// nothing eligible is an error that launches nothing.
func TestLoginAuto_Matrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		profile  string
		a, b, x  float64 // probe percent; <0 = probe error
		xSet     bool
		homeErr  error
		want     string // login line; "" = refused
		explain  string
		probes   int
		noteNone bool
	}{
		{name: "home best", profile: "personal", a: 90, b: 95, want: "home (--login auto)", explain: "login auto → home (max 77%) over a (max 90%), b (max 95%)", probes: 2},
		{name: "login best", profile: "personal", a: 10, b: 95, want: "a (--login auto)", explain: "login auto → a (max 10%) over home (max 77%), b (max 95%)", probes: 2},
		{name: "second login best", profile: "personal", a: 60, b: 20, want: "b (--login auto)", explain: "login auto → b (max 20%) over a (max 60%), home (max 77%)", probes: 2},
		{name: "tie home and login goes to home", profile: "personal", a: 77, b: 95, want: "home (--login auto)", probes: 2},
		{name: "tie between logins goes to config order", profile: "personal", a: 10, b: 10, want: "a (--login auto)", probes: 2},
		{name: "probe error makes a login ineligible", profile: "personal", a: -1, b: 30, want: "b (--login auto)", explain: "over home (max 77%)", probes: 2},
		{name: "env login joins when set", profile: "personal", a: 90, b: 95, x: 5, xSet: true, want: "x (--login auto)", probes: 3},
		{name: "env login unset is ineligible", profile: "personal", a: 90, b: 95, want: "home (--login auto)", probes: 2},
		{name: "home error so a login wins", profile: "personal", a: 40, b: 95, homeErr: provider.ErrExpired, want: "a (--login auto)", probes: 2},
		{name: "home 9% beats logins", profile: "work", a: 10, b: 10, want: "home (--login auto)", probes: 2},
		{name: "all ineligible", profile: "personal", a: -1, b: -1, homeErr: provider.ErrUnauthorized, probes: 2},
		{name: "api-billed profile: no candidate", profile: "kimi", a: 1, b: 1, probes: 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			h := probeHarness(t)
			set := func(tok string, p float64) {
				if p < 0 {
					h.claude.probeErrs[tok] = errors.New("fake probe failed")
					return
				}
				h.claude.probeUsage[tok] = pct(p)
			}
			set(tokA, c.a)
			set(tokB, c.b)
			set(tokX, c.x)
			if c.xSet {
				h.env["X_TOKEN"] = tokX
			}
			if c.homeErr != nil {
				for _, p := range h.claude.disc {
					h.claude.errs[p.Home] = c.homeErr
				}
			}
			got, code := h.dryLogin("-p", c.profile, "--login", "auto")
			if got != c.want || (c.want == "") == (code == ExitOK) {
				t.Errorf("login %q exit %d, want %q; stderr %s", got, code, c.want, h.stderr.String())
			}
			if c.want == "" && !strings.Contains(h.stderr.String(), "no eligible profile") {
				t.Errorf("refusal does not explain: %q", h.stderr.String())
			}
			if c.explain != "" && !strings.Contains(h.stderr.String(), c.explain) {
				t.Errorf("stderr %q lacks %q", h.stderr.String(), c.explain)
			}
			if n := h.probeCount(); n != c.probes {
				t.Errorf("probes = %d, want %d", n, c.probes)
			}
			for _, tok := range []string{tokA, tokB, tokK, tokX} {
				if strings.Contains(h.stdout.String()+h.stderr.String(), tok) {
					t.Errorf("output leaks %s", tok)
				}
			}
		})
	}

	// A real launch runs on the chosen login's token.
	h := probeHarness(t)
	h.claude.probeUsage[tokB] = pct(1)
	if code := h.run("run", "-p", "personal", "--login", "auto"); code != ExitOK {
		t.Fatalf("run: %d %s", code, h.stderr.String())
	}
	if v, _ := envOf(h.lastExec(), claude.OAuthTokenEnv); v != tokB {
		t.Errorf("launched on the wrong token (want b's)")
	}
	if !strings.Contains(h.stderr.String(), "personal · login b") {
		t.Errorf("launch line: %q", h.stderr.String())
	}
	// Nothing eligible launches nothing.
	n := len(h.execs)
	h.claude.errs[h.claude.disc[0].Home] = provider.ErrUnauthorized
	h.claude.probeErrs[tokA], h.claude.probeErrs[tokB] = errors.New("x"), errors.New("x")
	if code := h.run("run", "-p", "personal", "--login", "auto"); code == ExitOK || len(h.execs) != n {
		t.Errorf("all ineligible still launched: exit %d", code)
	}
}

// TestLoginCheck_Probe: `agx login check` probes and prints the windows;
// --no-probe only resolves; failures are partial exits.
func TestLoginCheck_Probe(t *testing.T) {
	t.Parallel()
	h := probeHarness(t)
	if code := h.run("login", "check", "a"); code != ExitOK {
		t.Fatalf("check a: %d %s", code, h.stderr.String())
	}
	if out := h.stdout.String(); !strings.Contains(out, "login a: "+checkResolved) || !strings.Contains(out, "Current session") || !strings.Contains(out, "90%") {
		t.Errorf("check a:\n%s", out)
	}
	if h.probeCount() != 1 {
		t.Errorf("probes = %d", h.probeCount())
	}
	if code := h.run("login", "check", "a", "--no-probe"); code != ExitOK || !strings.Contains(h.stdout.String(), "login a: "+checkNoProbe) || h.probeCount() != 1 {
		t.Errorf("--no-probe: %d %q probes %d", code, h.stdout.String(), h.probeCount())
	}
	if code := h.run("login", "check", "k"); code != ExitOK || !strings.Contains(h.stdout.String(), "billed per token") {
		t.Errorf("api-key: %d %q", code, h.stdout.String())
	}
	h.claude.probeErrs[tokB] = provider.ErrRateLimited
	if code := h.run("login", "check", "b"); code != ExitFailure || !strings.Contains(h.stdout.String(), "rate-limited") {
		t.Errorf("probe error: %d %q", code, h.stdout.String())
	}
	// All logins: x has no env → resolve failure → partial.
	if code := h.run("login", "check"); code != ExitFailure || !strings.Contains(h.stdout.String(), "login x: failed") {
		t.Errorf("check all: %d %q", code, h.stdout.String())
	}
}

// TestLogin_ExpiryD13: past expiry a stored login is refused; a cmd/env
// login warns and runs, because agx does not know when the token behind it
// was minted (logins spec §14 D13). Doctor: fail vs warn.
func TestLogin_ExpiryD13(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, `
logins:
  - {name: st-old, token: stored, added: 2025-01-01}
  - {name: cm-old, token_cmd: "printf '%s' `+tokCmd+`", added: 2025-01-01}
  - {name: en-old, token_env: E_TOK, added: 2025-01-01}
  - {name: st-new, token: stored, added: 2026-09-01}
  - {name: cm-new, token_cmd: "printf '%s' `+tokCmd+`", added: 2026-09-01}
  - {name: en-new, token_env: E_TOK, added: 2026-09-01}
profiles:
  - {name: personal, provider: claude, home: HOME/.claude, default: true}
`)
	h.storeToken("st-old", tokStored)
	h.storeToken("st-new", tokStored)
	h.env["E_TOK"] = tokEnv
	for _, c := range []struct {
		login     string
		runs      bool
		warn      string
		doctor    string // line prefix mark
		doctorMsg string
	}{
		{"st-old", false, "expired on 2026-01-01; renew: agx login add st-old --setup --replace", "✗", "expired 2026-01-01"},
		{"cm-old", true, "login cm-old is past its approximate expiry (2026-01-01", "!", "past its approximate expiry 2026-01-01"},
		{"en-old", true, "login en-old is past its approximate expiry (2026-01-01", "!", "past its approximate expiry 2026-01-01"},
		{"st-new", true, "", "✓", "expires 2027-09-01"},
		{"cm-new", true, "", "✓", "expires approximately 2027-09-01"},
		{"en-new", true, "", "✓", "expires approximately 2027-09-01"},
	} {
		n := len(h.execs)
		code := h.run("run", "--login", c.login)
		ran := len(h.execs) > n
		if ran != c.runs || (code == ExitOK) != c.runs {
			t.Errorf("%s: ran %v exit %d, want ran %v; stderr %s", c.login, ran, code, c.runs, h.stderr.String())
		}
		if c.warn != "" && !strings.Contains(h.stderr.String(), c.warn) {
			t.Errorf("%s: stderr %q lacks %q", c.login, h.stderr.String(), c.warn)
		}
		if c.warn == "" && strings.Contains(h.stderr.String(), "expir") {
			t.Errorf("%s: unexpected expiry note %q", c.login, h.stderr.String())
		}
		h.run("doctor")
		found := false
		for _, line := range strings.Split(h.stdout.String(), "\n") {
			if f := strings.Fields(line); len(f) > 2 && f[1] == "login" && f[2] == c.login {
				found = true
				if !strings.HasPrefix(line, c.doctor) || !strings.Contains(line, c.doctorMsg) {
					t.Errorf("%s: doctor line %q, want %s …%s", c.login, line, c.doctor, c.doctorMsg)
				}
			}
		}
		if !found {
			t.Errorf("%s: no doctor line", c.login)
		}
	}
}
