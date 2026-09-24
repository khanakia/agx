package render

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

func TestUsage_Plain(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 4, 51, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	reports := []UsageReport{
		{
			Profile: provider.Profile{Name: "personal", Provider: provider.Claude, Home: "/home/me/.claude"},
			Usage: provider.Usage{
				Identity: provider.Identity{Email: "me@example.com", Plan: "Max (20x)"},
				Windows: []provider.Window{
					{Group: provider.GroupSession, Label: "Current session", Percent: 5, Severity: provider.SeverityNormal, ResetsAt: at(4*time.Hour + 38*time.Minute)},
					{Group: provider.GroupWeekly, Label: "All models", Percent: 76, Severity: provider.SeverityWarning, ResetsAt: at(13*time.Hour + 8*time.Minute)},
					{Group: provider.GroupWeekly, Label: "Fable", Percent: 68},
				},
			},
		},
		{Profile: provider.Profile{Name: "kimi", Provider: provider.Claude, Home: "/home/me/.claude"}, Err: provider.ErrAPIBilled},
		{Profile: provider.Profile{Name: "work", Provider: provider.Claude, Home: "/home/me/.claude-work"}, Err: errors.New("boom")},
	}
	var b strings.Builder
	if err := Usage(&b, reports, Options{Now: now, BarWidth: 10, Home: "/home/me"}); err != nil {
		t.Fatal(err)
	}
	want := `me@example.com  Max (20x)  personal · claude  ~/.claude
  Current session    █░░░░░░░░░    5% used  resets in 4 hr 38 min
  Weekly limits
  All models         ████████░░   76% used  resets in 13 hr 8 min
  Fable              ███████░░░   68% used

kimi  kimi · claude  ~/.claude
  billed per token — no usage windows

work  work · claude  ~/.claude-work
  error: boom
`
	if b.String() != want {
		t.Errorf("render mismatch\n--- got ---\n%s\n--- want ---\n%s", b.String(), want)
	}
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("colour codes emitted with Color off")
	}
}

func TestUsage_Color(t *testing.T) {
	t.Parallel()
	r := UsageReport{Profile: provider.Profile{Name: "x"}, Usage: provider.Usage{Windows: []provider.Window{
		{Group: provider.GroupPlan, Label: "5-hour", Percent: 90, Severity: provider.SeverityWarning},
		{Group: provider.GroupPlan, Label: "Weekly", Percent: 100, Severity: provider.SeverityExhausted},
	}}}
	var b strings.Builder
	if err := Usage(&b, []UsageReport{r}, Options{Color: true, BarWidth: 10}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, ansiYellow+"█████████") || !strings.Contains(out, ansiRed+"██████████") {
		t.Errorf("severity colours missing:\n%q", out)
	}
	if strings.Contains(out, "plan") {
		t.Errorf("plan group must render without a heading:\n%s", out)
	}
}

func TestHeading(t *testing.T) {
	t.Parallel()
	if heading(provider.GroupSession) != "" || heading(provider.GroupPlan) != "" || heading(provider.GroupWeekly) != "Weekly limits" || heading("monthly") != "monthly" {
		t.Error("unexpected headings")
	}
}

func TestFormatUntil(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]string{
		-time.Minute:                     "now",
		30 * time.Second:                 "now",
		12*time.Minute + 59*time.Second:  "12 min",
		4*time.Hour + 38*time.Minute:     "4 hr 38 min",
		time.Hour:                        "1 hr 0 min",
		5*24*time.Hour + 3*time.Hour + 9: "5 d 3 hr",
	} {
		if got := FormatUntil(d); got != want {
			t.Errorf("FormatUntil(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestFormatAgo(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		10 * time.Second: "just now",
		5 * time.Minute:  "5 min ago",
		3 * time.Hour:    "3 hr ago",
		49 * time.Hour:   "2 d ago",
	} {
		if got := FormatAgo(now, now.Add(-d)); got != want {
			t.Errorf("FormatAgo(-%s) = %q, want %q", d, got, want)
		}
	}
}

func TestBarCells(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pct         float64
		fill, empty int
	}{{0, 0, 10}, {0.4, 1, 9}, {50, 5, 5}, {100, 10, 0}, {130, 10, 0}, {-5, 0, 10}} {
		f, e := BarCells(tc.pct, 10)
		if f != tc.fill || e != tc.empty {
			t.Errorf("BarCells(%v) = %d,%d want %d,%d", tc.pct, f, e, tc.fill, tc.empty)
		}
	}
}

func TestShortenHome(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, home, want string }{
		{"/h/.claude", "/h", "~/.claude"},
		{"/h", "/h", "~"},
		{"/hx/.claude", "/h", "/hx/.claude"},
		{"/h/.claude", "", "/h/.claude"},
	} {
		if got := ShortenHome(tc.path, tc.home); got != tc.want {
			t.Errorf("ShortenHome(%q,%q) = %q", tc.path, tc.home, got)
		}
	}
}

func TestTable(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	if err := Table(&b, []string{"A", "LONGER"}, [][]string{{"xx", "y"}}); err != nil {
		t.Fatal(err)
	}
	if b.String() != fmt.Sprintf("%-4s%s\n%-4s%s\n", "A", "LONGER", "xx", "y") {
		t.Errorf("table = %q", b.String())
	}
}
