package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
)

// TestUsage_ColorAndTimeoutMatrix: --color accepts exactly its three modes
// (case-sensitive), --timeout must be positive; both are usage errors that
// fetch nothing.
func TestUsage_ColorAndTimeoutMatrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		args []string
		code int
	}{
		{[]string{"--color", colorAuto}, ExitOK},
		{[]string{"--color", colorAlways}, ExitOK},
		{[]string{"--color", colorNever}, ExitOK},
		{[]string{"--color", "Always"}, ExitUsage},
		{[]string{"--color", ""}, ExitUsage},
		{[]string{"--color", "jamais"}, ExitUsage},
		{[]string{"--timeout", "0s"}, ExitUsage},
		{[]string{"--timeout", "-1s"}, ExitUsage},
		{[]string{"--timeout", "1ns"}, ExitOK},
	} {
		h := newHarness(t, "")
		if code := h.run(append([]string{"usage"}, c.args...)...); code != c.code {
			t.Errorf("%v: exit %d, want %d; stderr %s", c.args, code, c.code, h.stderr.String())
		}
	}
}

// TestUsage_ColorAlwaysEmitsEscapes: always colours even into a pipe,
// never and auto (not a terminal) do not.
func TestUsage_ColorAlwaysEmitsEscapes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "")
	for mode, want := range map[string]bool{colorAlways: true, colorNever: false, colorAuto: false} {
		h.run("usage", "--color", mode)
		if got := strings.Contains(h.stdout.String(), "\x1b["); got != want {
			t.Errorf("--color %s: escapes %v, want %v", mode, got, want)
		}
	}
}

// TestToUsageJSON: a login row drops the profile and home and is marked as
// a probe; an error row carries only the message; provider extra data
// passes through verbatim.
func TestToUsageJSON(t *testing.T) {
	t.Parallel()
	home := filepath.Join("/home", "me", ".claude")
	prof := provider.Profile{Name: "personal", Provider: provider.Claude, Home: home, Billing: provider.BillingPlan}
	extra := []byte(`{"overage":{"used":3}}`)
	win := []provider.Window{{Label: "Current session", Percent: 12}}

	plain := toUsageJSON(render.UsageReport{Profile: prof, Usage: provider.Usage{Windows: win, Extra: extra, Identity: provider.Identity{Email: "me@home"}}})
	if plain.Profile != "personal" || plain.Home != home || plain.Email != "me@home" || plain.Source != "" || len(plain.Windows) != 1 || string(plain.Extra) != string(extra) {
		t.Errorf("plain = %+v", plain)
	}
	if _, err := json.Marshal(plain); err != nil {
		t.Errorf("marshal: %v", err)
	}

	lr := toUsageJSON(render.UsageReport{Profile: prof, Login: "w", Account: "ops@example.com", Usage: provider.Usage{Windows: win}})
	if lr.Profile != "" || lr.Home != "" || lr.Login != "w" || lr.Account != "ops@example.com" || lr.Source != usageSourceProbe {
		t.Errorf("login row = %+v", lr)
	}

	er := toUsageJSON(render.UsageReport{Profile: prof, Err: errors.New("boom"), Usage: provider.Usage{Windows: win, Extra: extra}})
	if er.Error != "boom" || er.Windows != nil || er.Extra != nil {
		t.Errorf("error row = %+v", er)
	}

	none := toUsageJSON(render.UsageReport{Profile: prof})
	raw, err := json.Marshal(none)
	if err != nil || strings.Contains(string(raw), `"extra"`) || strings.Contains(string(raw), `"windows"`) {
		t.Errorf("empty row = %s %v", raw, err)
	}
}
