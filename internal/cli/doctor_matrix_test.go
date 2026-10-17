package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

// identityProv answers Identity per home, so every local login state can
// be shown without a real credential store.
type identityProv struct {
	*fakeProv
	ids  map[string]provider.Identity
	errs map[string]error
}

func (p identityProv) Identity(pr provider.Profile) (provider.Identity, error) {
	return p.ids[pr.Home], p.errs[pr.Home]
}

// TestDescribeProfile_StateMatrix: identity × billing × expiry decides the
// STATE column and the doctor row. The expiry instant itself is expired; an
// api-billed profile without a home login is fine (its secrets carry it).
func TestDescribeProfile_StateMatrix(t *testing.T) {
	t.Parallel()
	errBroken := errors.New("credentials file is not JSON")
	for _, c := range []struct {
		name    string
		billing provider.Billing
		id      provider.Identity
		err     error
		state   string
		doctor  string // substring of the doctor row
		mark    string
	}{
		{name: "ok", id: provider.Identity{LoggedIn: true, Email: "me@home", Plan: "Max"}, state: loginOK, doctor: "me@home Max", mark: "✓"},
		{name: "ok, no email", id: provider.Identity{LoggedIn: true}, state: loginOK, doctor: loggedInNoEmail, mark: "✓"},
		{name: "not logged in", state: loginNone, doctor: "not logged in — run `agx run -p p1` and log in", mark: "!"},
		{name: "api, no home login", billing: provider.BillingAPI, state: loginAPI, doctor: "api-billed", mark: "✓"},
		{name: "api, logged in too", billing: provider.BillingAPI, id: provider.Identity{LoggedIn: true}, state: loginOK, mark: "✓"},
		{name: "expires 1ns later", id: provider.Identity{LoggedIn: true, ExpiresAt: testNow.Add(time.Nanosecond)}, state: loginOK, mark: "✓"},
		{name: "expires now", id: provider.Identity{LoggedIn: true, ExpiresAt: testNow}, state: loginExpired, doctor: "login expired — run `agx run -p p1` once to refresh", mark: "!"},
		{name: "expired yesterday", id: provider.Identity{LoggedIn: true, ExpiresAt: testNow.Add(-24 * time.Hour)}, state: loginExpired, mark: "!"},
		{name: "identity error", err: errBroken, state: loginError, doctor: errBroken.Error(), mark: "✗"},
		{name: "identity error beats api", billing: provider.BillingAPI, err: errBroken, state: loginError, mark: "✗"},
	} {
		h := newHarness(t, "")
		home := filepath.Join(h.home, ".claude-p1")
		if err := os.MkdirAll(home, 0o755); err != nil {
			t.Fatal(err)
		}
		billing := c.billing
		if billing == "" {
			billing = provider.BillingPlan
		}
		h.now = testNow
		h.claude.disc = []provider.Profile{{Name: "p1", Provider: provider.Claude, Home: home, Billing: billing, Default: true, Source: provider.SourceDiscovered}}
		h.provs = []provider.Provider{identityProv{fakeProv: h.claude, ids: map[string]provider.Identity{home: c.id}, errs: map[string]error{home: c.err}}}
		if code := h.run("profiles", "--json"); code != ExitOK {
			t.Fatalf("%s: profiles %d %s", c.name, code, h.stderr.String())
		}
		rows := decode[[]profileJSON](t, h.stdout.Bytes()).Data
		if len(rows) != 1 || rows[0].Login != c.state {
			t.Errorf("%s: rows %+v, want state %q", c.name, rows, c.state)
			continue
		}
		if (c.err != nil) != (rows[0].Error != "") {
			t.Errorf("%s: error %q", c.name, rows[0].Error)
		}
		h.run("doctor")
		var line string
		for l := range strings.SplitSeq(h.stdout.String(), "\n") {
			if strings.Contains(l, "profile p1") {
				line = l
			}
		}
		if !strings.HasPrefix(line, c.mark) || !strings.Contains(line, c.doctor) {
			t.Errorf("%s: doctor row %q, want %s …%s", c.name, line, c.mark, c.doctor)
		}
	}
}

// testNow is a fixed instant for expiry boundaries.
var testNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

// doctorLine returns the first doctor output line containing sub.
func doctorLine(h *harness, sub string) string {
	for l := range strings.SplitSeq(h.stdout.String(), "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

// TestDoctor_TokenFileRows: tokens.yaml findings — inside a git work tree
// (warn), a group-readable directory (warn), an unparsable file (fail, and
// every stored login reports the refusal), a stored login without a token
// (fail). The doctor never prints a token.
func TestDoctor_TokenFileRows(t *testing.T) {
	t.Parallel()
	h := loginHarness(t, boundaryConfig)
	h.storeToken("other", tokStored)
	if code := h.run("doctor"); code != ExitFailure {
		t.Errorf("missing stored token: exit %d", code)
	}
	if l := doctorLine(h, "login st"); !strings.HasPrefix(l, "✗") || !strings.Contains(l, "no token in") {
		t.Errorf("missing token row %q", l)
	}

	cfgDir := filepath.Dir(h.tokensPath())
	if err := os.Chmod(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(cfgDir), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.run("doctor")
	if l := doctorLine(h, "chmod 700"); !strings.HasPrefix(l, "!") {
		t.Errorf("dir perm row %q", l)
	}
	if l := doctorLine(h, "inside git work tree"); !strings.HasPrefix(l, "!") || !strings.Contains(l, "must never be committed") {
		t.Errorf("git row %q", l)
	}

	if err := os.WriteFile(h.tokensPath(), []byte("- "+tokRaw+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := h.run("doctor"); code != ExitFailure {
		t.Errorf("malformed: exit %d", code)
	}
	if l := doctorLine(h, "is not a name: token map"); !strings.HasPrefix(l, "✗") {
		t.Errorf("malformed row %q", l)
	}
	if l := doctorLine(h, "login st"); !strings.Contains(l, "was refused (see above)") {
		t.Errorf("refused login row %q", l)
	}
	assertNoToken(t, "doctor", h)
}

// TestDoctor_SessionsRootIsFile: a sessions root that is a file fails.
func TestDoctor_SessionsRootIsFile(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	if err := os.WriteFile(h.sessions, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := h.run("doctor"); code != ExitFailure {
		t.Errorf("exit %d", code)
	}
	if l := doctorLine(h, "is not a directory"); !strings.HasPrefix(l, "✗") {
		t.Errorf("row %q\n%s", l, h.stdout.String())
	}
}
