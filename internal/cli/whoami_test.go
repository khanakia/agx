package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// staleRun is what an outer agx session leaves in the environment of a
// shell inside it; a nested launch must report its own values instead.
var staleRun = map[string]string{
	appmeta.EnvRunProfile: "stale-profile",
	appmeta.EnvRunLogin:   "w", // a real login name: it must still not be read
	appmeta.EnvRunHome:    "/stale/home",
}

// assertRunTags checks the three AGX_RUN_* values of a launch and that no
// fake token sits anywhere but in the vendor's own credential variable.
func assertRunTags(t *testing.T, name string, cmd provider.Command, profile, loginName, home string) {
	t.Helper()
	for k, want := range map[string]string{appmeta.EnvRunProfile: profile, appmeta.EnvRunLogin: loginName, appmeta.EnvRunHome: home} {
		got, ok := envOf(cmd, k)
		if !ok || got != want {
			t.Errorf("%s: %s = %q (set %v), want %q", name, k, got, ok, want)
		}
		n := 0
		for _, kv := range cmd.Env {
			if strings.HasPrefix(kv, k+"=") {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s: %s appears %d times", name, k, n)
		}
	}
	for _, kv := range cmd.Env {
		k, v, _ := strings.Cut(kv, "=")
		if k == claude.OAuthTokenEnv || k == claude.APIKeyEnv || k == "W_TOKEN" {
			continue
		}
		for _, tok := range fakeTokens {
			if strings.Contains(v, tok) {
				t.Errorf("%s: token value leaked into %s", name, k)
			}
		}
	}
}

// TestLaunch_RunTagsMatrix: every launching command × profile on the
// default or another home × how the login was chosen (profile/provider
// default, a named --login, --login home) × a parent environment that is
// clean or holds stale AGX_RUN_* plus AGX_LOGIN. The launch reports the
// profile, login and home it actually used; AGX_LOGIN is never added (only
// passed through when the parent had it); no token value appears outside
// the vendor's credential variable.
func TestLaunch_RunTagsMatrix(t *testing.T) {
	t.Parallel()
	for _, cmdName := range []string{"run", "new", "resume", "ws"} {
		for _, prof := range []struct{ name, home, defLogin string }{
			{"personal", ".claude", "w"}, // default home: CLAUDE_CONFIG_DIR stays absent
			{"work", ".claude-work", "p"},
		} {
			for _, flag := range []string{"", "w", login.Home} {
				for _, stale := range []bool{false, true} {
					name := cmdName + "/" + prof.name + "/login=" + flag
					if stale {
						name += "/stale-parent"
					}
					h := loginHarness(t, loginConfig)
					h.storeToken("p", tokStored)
					h.env["W_TOKEN"] = tokEnvOAT
					want := prof.defLogin
					if stale {
						for k, v := range staleRun {
							h.env[k] = v
						}
						// AGX_LOGIN is an input and keeps its precedence.
						h.env[appmeta.EnvLogin] = "p"
						want = "p"
					}
					if flag != "" {
						want = flag
					}
					home := filepath.Join(h.home, prof.home)
					args := []string{cmdName}
					switch cmdName {
					case "run", "new":
						args = append(args, "-p", prof.name)
					case "resume":
						h.claude.convs[home] = []provider.Conversation{{Provider: provider.Claude, Home: home, ID: "c-1", Dir: h.cwd, Updated: h.now}}
						args = append(args, "--last")
					case "ws":
						h.claude.convs[home] = []provider.Conversation{{Provider: provider.Claude, Home: home, ID: "c-1", Dir: h.cwd, Updated: h.now}}
						h.picker.script = []pickStep{{idx: []int{0}}, {idx: []int{0}}}
					}
					if flag != "" {
						args = append(args, "--login", flag)
					}
					if code := h.run(args...); code != ExitOK {
						t.Errorf("%s: exit %d: %s", name, code, h.stderr.String())
						continue
					}
					cmd := h.lastExec()
					assertRunTags(t, name, cmd, prof.name, want, home)
					v, ok := envOf(cmd, appmeta.EnvLogin)
					if stale != ok || (stale && v != "p") {
						t.Errorf("%s: AGX_LOGIN = %q (set %v); the launch must not add or change it", name, v, ok)
					}
					if out := h.stdout.String() + h.stderr.String(); slices.ContainsFunc(fakeTokens, func(tok string) bool { return strings.Contains(out, tok) }) {
						t.Errorf("%s: output leaks a token", name)
					}
				}
			}
		}
	}
}

// TestLaunch_RunTagsOtherPaths: the launch paths outside the matrix — the
// auto-picked profile, --login auto, a bare `agx` in a bound folder, the
// shell layer's exec of a plan, and `profile add --setup` — tag the same way.
func TestLaunch_RunTagsOtherPaths(t *testing.T) {
	t.Parallel()
	// -p auto: the fake usage makes work (9%) beat personal (77%).
	for _, cmd := range []string{"run", "new"} {
		h := newHarness(t, "")
		if code := h.run(cmd, "-p", "auto"); code != ExitOK {
			t.Fatalf("%s -p auto: %d %s", cmd, code, h.stderr.String())
		}
		assertRunTags(t, cmd+" -p auto", h.lastExec(), "work", login.Home, filepath.Join(h.home, ".claude-work"))
	}

	// --login auto: b has the most headroom.
	h := probeHarness(t)
	h.claude.probeUsage[tokB] = pct(1)
	if code := h.run("run", "-p", "personal", "--login", "auto"); code != ExitOK {
		t.Fatalf("--login auto: %d %s", code, h.stderr.String())
	}
	assertRunTags(t, "--login auto", h.lastExec(), "personal", "b", filepath.Join(h.home, ".claude"))

	// A bare agx in a folder bound to pinned with login w.
	h, _ = projectHarness(t)
	h.tty = true
	h.env["W_TOKEN"] = tokEnvOAT
	h.in("bb")
	if code := h.run(); code != ExitOK {
		t.Fatalf("bare agx: %d %s", code, h.stderr.String())
	}
	assertRunTags(t, "bare agx", h.lastExec(), "pinned", "w", filepath.Join(h.home, ".claude-pin"))

	// The shell layer: new writes a plan, exec launches it.
	h = loginHarness(t, loginConfig)
	h.storeToken("p", tokStored)
	planPath := filepath.Join(h.root, "plan.json")
	h.env[appmeta.EnvPlanFile] = planPath
	for k, v := range staleRun {
		h.env[k] = v
	}
	if code := h.run("new", "-p", "work", "--login", "home"); code != ExitOK || len(h.execs) != 0 {
		t.Fatalf("new with plan: %d execs %d", code, len(h.execs))
	}
	if code := h.run("exec", "--plan", planPath); code != ExitOK {
		t.Fatalf("exec: %d %s", code, h.stderr.String())
	}
	assertRunTags(t, "exec plan", h.lastExec(), "work", login.Home, filepath.Join(h.home, ".claude-work"))

	// profile add --setup runs the vendor login for the new home.
	h = profileHarness(t)
	for k, v := range staleRun {
		h.env[k] = v
	}
	if code := h.run("profile", "add", "client", "--setup"); code != ExitOK || len(h.interactive) != 1 {
		t.Fatalf("profile add --setup: %d %v", code, h.interactive)
	}
	assertRunTags(t, "profile add --setup", h.interactive[0], "client", login.Home, filepath.Join(h.home, ".claude-client"))
}

// TestLaunch_RunTagsNeverSteer: AGX_RUN_* are outputs. A parent that holds
// them (even naming a real profile and login) launches exactly what a clean
// parent launches, and --dry-run lists the three values agx sets.
func TestLaunch_RunTagsNeverSteer(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"run"}, {"run", "-p", "work"}, {"run", "-p", "pinned"}} {
		var results [2]string
		for i, withStale := range []bool{false, true} {
			h := loginHarness(t, loginConfig)
			h.storeToken("p", tokStored)
			if withStale {
				h.env[appmeta.EnvRunProfile] = "work"
				h.env[appmeta.EnvRunLogin] = "p"
				h.env[appmeta.EnvRunHome] = filepath.Join(h.home, ".claude-work")
			}
			got, code := h.dryLogin(args[1:]...)
			if code != ExitOK {
				t.Fatalf("%v: %d %s", args, code, h.stderr.String())
			}
			results[i] = got
			// The dry run lists what agx changed; a parent already holding
			// the same value is no change, so only the clean parent counts.
			if withStale {
				continue
			}
			for _, k := range []string{appmeta.EnvRunProfile, appmeta.EnvRunLogin, appmeta.EnvRunHome} {
				if !strings.Contains(h.stdout.String(), "env:     "+k+"=") {
					t.Errorf("%v: dry run does not list %s:\n%s", args, k, h.stdout.String())
				}
			}
		}
		if results[0] != results[1] {
			t.Errorf("%v: stale AGX_RUN_* changed the login: %q vs %q", args, results[0], results[1])
		}
	}
}

// TestWhoami_Matrix: inside a full agx session, with only some variables
// set, and outside one; × the vendor home variable set or not × a token
// variable present or not; text and --json. No config is read (a broken
// config must not matter) and no token value is ever printed.
func TestWhoami_Matrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		run     map[string]string
		session string
		missing []string
		text    []string
	}{
		{name: "inside", run: map[string]string{appmeta.EnvRunProfile: "work", appmeta.EnvRunLogin: "p", appmeta.EnvRunHome: "/h/.claude-work"},
			session: sessionAgx, text: []string{"session:  started by agx", "profile:  work", "login:    p", "home:     /h/.claude-work"}},
		{name: "partial", run: map[string]string{appmeta.EnvRunProfile: "work"},
			session: sessionPartial, missing: []string{appmeta.EnvRunLogin, appmeta.EnvRunHome}, text: []string{"session:  partial: AGX_RUN_LOGIN, AGX_RUN_HOME not set", "login:    -"}},
		{name: "empty values count as unset", run: map[string]string{appmeta.EnvRunProfile: "", appmeta.EnvRunLogin: "", appmeta.EnvRunHome: ""},
			session: sessionNone, text: []string{"session:  " + notInsideAgxSession}},
		{name: "outside", session: sessionNone, text: []string{"session:  " + notInsideAgxSession}},
	} {
		for _, homeSet := range []bool{false, true} {
			for _, tokenSet := range []bool{false, true} {
				name := c.name
				h := newHarness(t, "profiles: [not a list item]\n  broken: yaml: here\n")
				h.provs = []provider.Provider{claude.New(h.home), plainProv{emptyProv(provider.Codex)}}
				for k, v := range c.run {
					h.env[k] = v
				}
				claudeHome, from := filepath.Join(h.home, ".claude"), homeFromDefault
				if homeSet {
					claudeHome, from = filepath.Join(h.root, "alt"), claude.ConfigDirEnv
					h.env[claude.ConfigDirEnv] = claudeHome
					name += "/home-var"
				}
				if tokenSet {
					h.env[claude.OAuthTokenEnv] = tokStored
					h.env[claude.AuthTokenEnv] = tokEnv
					name += "/token"
				}
				if code := h.run("whoami"); code != ExitOK {
					t.Fatalf("%s: exit %d %s", name, code, h.stderr.String())
				}
				out := h.stdout.String()
				for _, want := range c.text {
					if !strings.Contains(out, want) {
						t.Errorf("%s: output lacks %q:\n%s", name, want, out)
					}
				}
				if c.session == sessionNone && strings.Contains(out, "profile:") {
					t.Errorf("%s: outside a session but shows a profile:\n%s", name, out)
				}
				wantToken := "token login absent"
				if tokenSet {
					wantToken = "token login present (" + claude.AuthTokenEnv + ", " + claude.OAuthTokenEnv + ")"
				}
				if !strings.Contains(out, "claude:   ") || !strings.Contains(out, "("+from+") · "+wantToken) {
					t.Errorf("%s: claude row wrong (want %s, %s):\n%s", name, from, wantToken, out)
				}
				if !strings.Contains(out, "codex:    ") || !strings.Contains(out, "no token logins") {
					t.Errorf("%s: codex row wrong:\n%s", name, out)
				}

				if code := h.run("whoami", "--json"); code != ExitOK {
					t.Fatalf("%s json: exit %d", name, code)
				}
				e := decode[whoamiJSON](t, h.stdout.Bytes())
				r := e.Data
				if e.Kind != kindWhoamiReport || r.Session != c.session || !slices.Equal(r.Missing, c.missing) {
					t.Errorf("%s json: kind %q session %q missing %v", name, e.Kind, r.Session, r.Missing)
				}
				if c.session == sessionAgx && (r.Profile != "work" || r.Login != "p" || r.Home != "/h/.claude-work") {
					t.Errorf("%s json: %+v", name, r)
				}
				if len(r.Providers) != 2 {
					t.Fatalf("%s json: providers %+v", name, r.Providers)
				}
				cl, cx := r.Providers[0], r.Providers[1]
				if cl.Home != claudeHome || cl.HomeFrom != from {
					t.Errorf("%s json: claude home %q from %q", name, cl.Home, cl.HomeFrom)
				}
				if tokenSet != (cl.TokenLogin == tokenPresent) || (!tokenSet && cl.TokenLogin != tokenAbsent) {
					t.Errorf("%s json: claude token %q %v", name, cl.TokenLogin, cl.TokenVars)
				}
				if cx.TokenLogin != tokenUnsupported || cx.HomeFrom != homeFromDefault {
					t.Errorf("%s json: codex %+v", name, cx)
				}
				all := out + h.stdout.String() + h.stderr.String()
				for _, tok := range fakeTokens {
					if strings.Contains(all, tok) {
						t.Errorf("%s: whoami printed a token value", name)
					}
				}
			}
		}
	}
}

// TestWhoami_CodexHome: a provider's own home variable is honoured, and
// the provider list comes from Deps, not from a hardcoded vendor.
func TestWhoami_CodexHome(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.provs = []provider.Provider{codexNamed{emptyProv(provider.Codex)}}
	h.env["FAKE_CODEX_HOME"] = filepath.Join(h.root, "cx")
	if code := h.run("whoami"); code != ExitOK {
		t.Fatal(code)
	}
	if want := "codex:    " + filepath.Join(h.root, "cx") + " (FAKE_CODEX_HOME) · no token logins\n"; !strings.HasSuffix(h.stdout.String(), want) {
		t.Errorf("got %q, want suffix %q", h.stdout.String(), want)
	}
}

// codexNamed is a provider with a home variable and no logins.
type codexNamed struct{ *fakeProv }

func (codexNamed) HomeEnvName() string { return "FAKE_CODEX_HOME" }
