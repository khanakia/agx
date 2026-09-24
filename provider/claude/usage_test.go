package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

func loadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/response.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseUsage_CurrentShape(t *testing.T) {
	t.Parallel()
	u, err := ParseUsage(loadFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		label  string
		pct    float64
		sev    provider.Severity
		kind   provider.WindowKind
		group  string
		active bool
	}{
		{"Current session", 5, provider.SeverityNormal, provider.WindowSession, provider.GroupSession, false},
		{"All models", 76, provider.SeverityWarning, provider.WindowWeekly, provider.GroupWeekly, true},
		{"Fable", 68, provider.SeverityNormal, provider.WindowScoped, provider.GroupWeekly, false},
	}
	if len(u.Windows) != len(want) {
		t.Fatalf("got %d windows, want %d", len(u.Windows), len(want))
	}
	for i, w := range want {
		g := u.Windows[i]
		if g.Label != w.label || g.Percent != w.pct || g.Severity != w.sev || g.Kind != w.kind || g.Group != w.group || g.Active != w.active {
			t.Errorf("window %d = %+v, want %+v", i, g, w)
		}
		if g.ResetsAt == nil {
			t.Errorf("window %d: resets_at not parsed", i)
		}
	}
	if !strings.Contains(string(u.Extra), `"monthly_limit"`) {
		t.Errorf("raw extra_usage lost fields: %s", u.Extra)
	}
	if u.MaxPercent() != 76 {
		t.Errorf("MaxPercent = %v, want 76", u.MaxPercent())
	}
}

func TestParseUsage_LegacyFallback(t *testing.T) {
	t.Parallel()
	body := `{"five_hour":{"utilization":12,"resets_at":"2026-09-24T09:00:00Z"},"seven_day":{"utilization":40,"resets_at":null},"seven_day_opus":{"utilization":3,"resets_at":null}}`
	u, err := ParseUsage([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, w := range u.Windows {
		labels = append(labels, w.Label)
	}
	if strings.Join(labels, ",") != "Current session,All models,Opus" {
		t.Errorf("labels = %v", labels)
	}
	if u.Windows[0].Percent != 12 || u.Windows[1].ResetsAt != nil || u.Windows[0].Severity != "" {
		t.Errorf("legacy values not carried: %+v", u.Windows)
	}
}

func TestParseUsage_ExtraUsageEnabledAddsRow(t *testing.T) {
	t.Parallel()
	body := `{"limits":[{"kind":"session","group":"session","percent":1}],"extra_usage":{"is_enabled":true,"utilization":42}}`
	u, err := ParseUsage([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	last := u.Windows[len(u.Windows)-1]
	if last.Label != labelExtraUsage || last.Percent != 42 || last.Group != provider.GroupExtra {
		t.Errorf("extra row = %+v", last)
	}
}

func TestParseUsage_Errors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		noLimits   bool
	}{
		{"not json", `<html>`, false},
		{"empty object", `{}`, true},
		{"empty limits no legacy", `{"limits":[]}`, true},
		{"bad extra usage", `{"limits":[{"kind":"session"}],"extra_usage":"oops"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseUsage([]byte(tc.body))
			if err == nil {
				t.Fatal("want error")
			}
			if errors.Is(err, ErrNoLimits) != tc.noLimits {
				t.Errorf("ErrNoLimits = %v, want %v (%v)", !tc.noLimits, tc.noLimits, err)
			}
		})
	}
}

func TestSeverityMapping(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]provider.Severity{
		"":         "",
		"normal":   provider.SeverityNormal,
		"warning":  provider.SeverityWarning,
		"critical": provider.SeverityExhausted, // unknown → treated as worse, not hidden
	} {
		if got := severity(in); got != want {
			t.Errorf("severity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLimitLabel(t *testing.T) {
	t.Parallel()
	empty, name := "", "Sonnet"
	for _, tc := range []struct {
		l    Limit
		want string
	}{
		{Limit{Kind: kindSession}, "Current session"},
		{Limit{Kind: kindWeeklyAll}, "All models"},
		{Limit{Kind: kindWeeklyScoped, Scope: &Scope{Model: &ScopeRef{DisplayName: &name}}}, "Sonnet"},
		{Limit{Kind: kindWeeklyScoped}, "Scoped"},
		{Limit{Kind: kindWeeklyScoped, Scope: &Scope{Model: &ScopeRef{DisplayName: &empty}}}, "Scoped"},
		{Limit{Kind: "monthly_new"}, "monthly_new"},
	} {
		if got := tc.l.Label(); got != tc.want {
			t.Errorf("Label(%+v) = %q, want %q", tc.l, got, tc.want)
		}
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

func TestUsageClientFetch(t *testing.T) {
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
		{"429", http.StatusTooManyRequests, []byte("{\n  \"error\": \"rate\"\n}"), true, false},
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

			c := &UsageClient{Endpoint: srv.URL, HTTP: srv.Client()}
			u, err := c.Fetch(context.Background(), "tok-123")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if errors.Is(err, provider.ErrUnauthorized) != tc.wantUnaut {
				t.Errorf("ErrUnauthorized = %v, want %v (%v)", !tc.wantUnaut, tc.wantUnaut, err)
			}
			if tc.status == http.StatusTooManyRequests && !errors.Is(err, provider.ErrRateLimited) {
				t.Errorf("429 err = %v, want ErrRateLimited", err)
			}
			if !tc.wantErr && len(u.Windows) != 3 {
				t.Errorf("got %d windows", len(u.Windows))
			}
		})
	}
}

func TestUsageClientFetch_ContextCancelled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (&UsageClient{Endpoint: srv.URL, HTTP: srv.Client()}).Fetch(ctx, "t"); err == nil {
		t.Fatal("want timeout error")
	}
}
