package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

func TestPlan_TextAndJSON(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	personal, work := filepath.Join(h.home, ".claude"), filepath.Join(h.home, ".claude-work")
	started := time.Date(2025, 11, 25, 14, 37, 0, 0, time.Local)
	h.claude.subs = map[string]provider.Subscription{
		personal: {Plan: "Max (20x)", Status: "active", Billing: "stripe_subscription", StartedAt: &started},
	}
	h.claude.errs[work] = provider.ErrExpired
	if code := h.run("plan"); code != ExitFailure {
		t.Fatalf("exit %d (one account failed → 1)", code)
	}
	out := h.stdout.String()
	for _, want := range []string{"personal", "Max (20x)", "active", "2025-11-25", "error (see below)", "work: login expired"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "RENEWS") {
		t.Error("renewal column must not be shown")
	}

	h.claude.errs = map[string]error{}
	h.claude.subs[work] = provider.Subscription{Plan: "Pro"}
	if code := h.run("plan", "--json"); code != ExitOK {
		t.Fatalf("json exit %d: %s", code, h.stderr.String())
	}
	e := decode[[]planRow](t, h.stdout.Bytes())
	if e.Kind != kindPlanList || len(e.Data) != 2 || e.Data[0].Sub.Plan != "Max (20x)" || e.Data[0].Sub.StartedAt == nil || e.Data[1].Sub.Plan != "Pro" {
		t.Errorf("json = %+v", e)
	}
	if strings.Contains(h.stdout.String(), "renew") {
		t.Error("renewal fields must not be in the JSON")
	}
}

func TestPlan_UsageErrors(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	for _, args := range [][]string{{"plan", "nobody"}, {"plan", "--timeout=0s"}} {
		if code := h.run(args...); code != ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if code := h.run("subscription"); code != ExitOK {
		t.Errorf("alias subscription: exit %d %s", code, h.stderr.String())
	}
}

func TestOrCell(t *testing.T) {
	t.Parallel()
	if orCell("") != cellNo || orCell("x") != "x" {
		t.Error("orCell")
	}
}
