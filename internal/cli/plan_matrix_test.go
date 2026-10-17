package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// TestPlan_LoginFlagMatrix: plan takes no token login (the profile
// endpoint refuses setup-tokens); only the home login and no flag pass.
func TestPlan_LoginFlagMatrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		flag string
		code int
		note string
	}{
		{flag: "", code: ExitOK},
		{flag: "home", code: ExitOK},
		{flag: "auto", code: ExitUsage, note: `"auto" is accepted only as --login`},
		{flag: "personal", code: ExitUsage, note: "profile endpoint refuses setup-tokens"},
		{flag: "HOME", code: ExitUsage, note: "profile endpoint refuses setup-tokens"},
		{flag: "hôme", code: ExitUsage, note: "profile endpoint refuses setup-tokens"},
	} {
		h := newHarness(t, "")
		args := []string{"plan", "--json"}
		if c.flag != "" {
			args = append(args, "--login", c.flag)
		}
		if code := h.run(args...); code != c.code || !strings.Contains(h.stderr.String(), c.note) {
			t.Errorf("--login %q: exit %d stderr %q, want %d %q", c.flag, code, h.stderr.String(), c.code, c.note)
		}
	}
}

// TestPlan_ProviderShapes: a provider without the subscription upgrade is
// a failed row (exit 1) with the reason, not a crash or a silent blank;
// with no plan-billed profile at all the request itself is wrong.
func TestPlan_ProviderShapes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	h.provs = []provider.Provider{plainProv{h.claude}}
	if code := h.run("plan", "--json"); code != ExitFailure {
		t.Fatalf("exit %d, want %d", code, ExitFailure)
	}
	for _, r := range decode[[]planRow](t, h.stdout.Bytes()).Data {
		if r.Error != "claude does not report subscriptions" || r.Email == "" {
			t.Errorf("row %+v", r)
		}
	}

	h = newHarness(t, "profiles:\n  - {name: kimi, provider: claude, home: "+h.home+"/.claude-kimi, billing: api}\n")
	h.claude.disc = nil
	if code := h.run("plan"); code != ExitUsage || !strings.Contains(h.stderr.String(), "no plan-billed profiles") {
		t.Errorf("api only: %d %q", code, h.stderr.String())
	}
}

// TestWithHint: login-shaped failures (also when wrapped) get the command
// that fixes them; everything else passes through unchanged.
func TestWithHint(t *testing.T) {
	t.Parallel()
	p := provider.Profile{Name: "wörk"}
	other := errors.New("boom")
	for _, c := range []struct {
		err  error
		hint bool
	}{
		{provider.ErrExpired, true},
		{provider.ErrUnauthorized, true},
		{provider.ErrNotLoggedIn, true},
		{fmt.Errorf("fetch: %w", provider.ErrExpired), true},
		{provider.ErrRateLimited, false},
		{provider.ErrAPIBilled, false},
		{other, false},
	} {
		got := withHint(c.err, p)
		if !errors.Is(got, c.err) {
			t.Errorf("%v: hint lost the cause", c.err)
		}
		if has := strings.Contains(got.Error(), "run `agx run -p wörk` once"); has != c.hint {
			t.Errorf("%v: %q, want hint %v", c.err, got, c.hint)
		}
	}
	if withHint(nil, p) != nil {
		t.Error("nil gained a hint")
	}
}
