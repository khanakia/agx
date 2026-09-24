package usage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func loadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/response.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParse_CurrentShape(t *testing.T) {
	t.Parallel()
	u, err := Parse(loadFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Limits) != 3 {
		t.Fatalf("got %d limits, want 3", len(u.Limits))
	}
	want := []struct {
		label    string
		pct      float64
		sev      Severity
		group    Group
		isActive bool
	}{
		{"Current session", 5, SeverityNormal, GroupSession, false},
		{"All models", 76, SeverityWarning, GroupWeekly, true},
		{"Fable", 68, SeverityNormal, GroupWeekly, false},
	}
	for i, w := range want {
		l := u.Limits[i]
		if l.Label() != w.label || l.Percent != w.pct || l.Severity != w.sev || l.Group != w.group || l.IsActive != w.isActive {
			t.Errorf("limit %d = %+v (label %q), want %+v", i, l, l.Label(), w)
		}
		if l.ResetsAt == nil {
			t.Errorf("limit %d: resets_at not parsed", i)
		}
	}
	if u.ExtraUsage == nil || u.ExtraUsage.IsEnabled || u.ExtraUsage.Utilization != nil {
		t.Errorf("extra usage = %+v, want disabled with nil utilization", u.ExtraUsage)
	}
	if !strings.Contains(string(u.ExtraUsageRaw), `"monthly_limit"`) {
		t.Errorf("raw extra_usage lost fields: %s", u.ExtraUsageRaw)
	}
}

func TestParse_LegacyFallback(t *testing.T) {
	t.Parallel()
	body := `{"five_hour":{"utilization":12,"resets_at":"2026-09-24T09:00:00Z"},"seven_day":{"utilization":40,"resets_at":null},"seven_day_opus":{"utilization":3,"resets_at":null}}`
	u, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(u.Limits))
	for _, l := range u.Limits {
		got = append(got, l.Label())
	}
	if strings.Join(got, ",") != "Current session,All models,Opus" {
		t.Errorf("labels = %v", got)
	}
	if u.Limits[0].Percent != 12 || u.Limits[1].ResetsAt != nil {
		t.Errorf("legacy values not carried: %+v", u.Limits)
	}
	if u.ExtraUsage != nil {
		t.Errorf("extra usage should be nil when absent")
	}
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		wantNoLim  bool
	}{
		{"not json", `<html>`, false},
		{"empty object", `{}`, true},
		{"empty limits no legacy", `{"limits":[]}`, true},
		{"bad extra usage", `{"limits":[{"kind":"session"}],"extra_usage":"oops"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tc.body))
			if err == nil {
				t.Fatal("want error")
			}
			if errors.Is(err, ErrNoLimits) != tc.wantNoLim {
				t.Errorf("errors.Is(ErrNoLimits) = %v, want %v (%v)", !tc.wantNoLim, tc.wantNoLim, err)
			}
		})
	}
}

func TestLabel(t *testing.T) {
	t.Parallel()
	empty := ""
	name := "Sonnet"
	for _, tc := range []struct {
		name string
		l    Limit
		want string
	}{
		{"session", Limit{Kind: KindSession}, "Current session"},
		{"weekly all", Limit{Kind: KindWeeklyAll}, "All models"},
		{"scoped with model", Limit{Kind: KindWeeklyScoped, Scope: &Scope{Model: &ScopeRef{DisplayName: &name}}}, "Sonnet"},
		{"scoped nil scope", Limit{Kind: KindWeeklyScoped}, "Scoped"},
		{"scoped empty name", Limit{Kind: KindWeeklyScoped, Scope: &Scope{Model: &ScopeRef{DisplayName: &empty}}}, "Scoped"},
		{"unknown kind kept", Limit{Kind: "monthly_new"}, "monthly_new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.l.Label(); got != tc.want {
				t.Errorf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGroupHeading(t *testing.T) {
	t.Parallel()
	if GroupHeading(GroupSession) != "" || GroupHeading(GroupWeekly) != "Weekly limits" || GroupHeading("monthly") != "monthly" {
		t.Error("unexpected group headings")
	}
}

func TestPlanLabel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ sub, tier, want string }{
		{"max", "default_claude_max_20x", "Max (20x)"},
		{"max", "default_claude_max_5x", "Max (5x)"},
		{"pro", "default_claude_pro", "Pro"},
		{"team", "", "Team"},
		{"", "default_claude_max_20x", ""},
	} {
		if got := PlanLabel(tc.sub, tc.tier); got != tc.want {
			t.Errorf("PlanLabel(%q,%q) = %q, want %q", tc.sub, tc.tier, got, tc.want)
		}
	}
}

func TestFormatUntil(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Minute, "now"},
		{30 * time.Second, "now"},
		{12*time.Minute + 59*time.Second, "12 min"},
		{4*time.Hour + 38*time.Minute, "4 hr 38 min"},
		{time.Hour, "1 hr 0 min"},
		{5*24*time.Hour + 3*time.Hour + 10*time.Minute, "5 d 3 hr"},
	} {
		if got := FormatUntil(tc.d); got != tc.want {
			t.Errorf("FormatUntil(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestBarCells(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pct               float64
		width, fill, rest int
	}{
		{0, 10, 0, 10},
		{0.4, 10, 1, 9}, // non-zero never renders as empty
		{50, 10, 5, 5},
		{100, 10, 10, 0},
		{130, 10, 10, 0}, // clamped
		{-5, 10, 0, 10},  // clamped
	} {
		f, r := barCells(tc.pct, tc.width)
		if f != tc.fill || r != tc.rest {
			t.Errorf("barCells(%v,%d) = %d,%d want %d,%d", tc.pct, tc.width, f, r, tc.fill, tc.rest)
		}
	}
}

func TestSeverityColor(t *testing.T) {
	t.Parallel()
	if severityColor(SeverityNormal) != ansiBlue || severityColor("") != ansiBlue ||
		severityColor(SeverityWarning) != ansiYellow || severityColor("critical") != ansiRed {
		t.Error("unexpected severity colours")
	}
}

func TestRenderText_Plain(t *testing.T) {
	t.Parallel()
	u, err := Parse(loadFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 24, 4, 51, 0, 0, time.UTC)
	reports := []Report{
		{ConfigDir: "/home/me/.claude", Email: "me@example.com", Plan: "Max (20x)", Usage: u},
		{ConfigDir: "/home/me/.claude-work", Err: errors.New("boom")},
	}
	var b strings.Builder
	if err := RenderText(&b, reports, RenderOptions{Now: now, BarWidth: 10, Home: "/home/me"}); err != nil {
		t.Fatal(err)
	}
	want := `me@example.com  Max (20x)  ~/.claude
  Current session    █░░░░░░░░░    5% used  resets in 4 hr 38 min
  Weekly limits
  All models         ████████░░   76% used  resets in 13 hr 8 min
  Fable              ███████░░░   68% used  resets in 13 hr 8 min

~/.claude-work
  error: boom
`
	if b.String() != want {
		t.Errorf("render mismatch\n--- got ---\n%s\n--- want ---\n%s", b.String(), want)
	}
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("colour codes emitted with Color off")
	}
}

func TestRenderText_ColorAndExtraUsage(t *testing.T) {
	t.Parallel()
	util := 42.0
	u := Usage{
		Limits:     []Limit{{Kind: KindWeeklyAll, Group: GroupWeekly, Percent: 90, Severity: SeverityWarning}},
		ExtraUsage: &ExtraUsage{IsEnabled: true, Utilization: &util},
	}
	var b strings.Builder
	if err := RenderText(&b, []Report{{ConfigDir: "/x", Usage: u}}, RenderOptions{Color: true, BarWidth: 10}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, ansiYellow+"█████████") {
		t.Errorf("warning bar not yellow:\n%q", out)
	}
	if !strings.Contains(out, "Extra usage") || !strings.Contains(out, "42% used") {
		t.Errorf("extra usage row missing:\n%s", out)
	}
	if strings.Contains(out, "resets in") {
		t.Errorf("rows without resets_at must not print a reset:\n%s", out)
	}
}

func TestShortenHome(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, home, want string }{
		{"/h/.claude", "/h", "~/.claude"},
		{"/h", "/h", "~"},
		{"/hx/.claude", "/h", "/hx/.claude"}, // prefix but not a path boundary
		{"/h/.claude", "", "/h/.claude"},
	} {
		if got := shortenHome(tc.path, tc.home); got != tc.want {
			t.Errorf("shortenHome(%q,%q) = %q, want %q", tc.path, tc.home, got, tc.want)
		}
	}
}

func TestFetch(t *testing.T) {
	t.Parallel()
	fixture := loadFixture(t)
	for _, tc := range []struct {
		name      string
		status    int
		body      []byte
		wantErr   bool
		wantUnaut bool
	}{
		{"ok", http.StatusOK, fixture, false, false},
		{"401", http.StatusUnauthorized, []byte(`{"error":"expired"}`), true, true},
		{"403", http.StatusForbidden, nil, true, true},
		{"500", http.StatusInternalServerError, []byte("oops"), true, false},
		{"200 garbage", http.StatusOK, []byte("<html>"), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", r.Method)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
					t.Errorf("Authorization = %q", got)
				}
				if got := r.Header.Get(oauthBetaHeader); got != oauthBetaValue {
					t.Errorf("%s = %q", oauthBetaHeader, got)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write(tc.body) // test server; a failed write surfaces as a client error
			}))
			defer srv.Close()

			c := &Client{Endpoint: srv.URL, HTTP: srv.Client()}
			u, err := c.Fetch(context.Background(), "tok-123")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if errors.Is(err, ErrUnauthorized) != tc.wantUnaut {
				t.Errorf("ErrUnauthorized = %v, want %v (%v)", !tc.wantUnaut, tc.wantUnaut, err)
			}
			if !tc.wantErr && len(u.Limits) != 3 {
				t.Errorf("got %d limits", len(u.Limits))
			}
		})
	}
}

func TestFetch_ContextCancelled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (&Client{Endpoint: srv.URL, HTTP: srv.Client()}).Fetch(ctx, "t"); err == nil {
		t.Fatal("want timeout error")
	}
}
