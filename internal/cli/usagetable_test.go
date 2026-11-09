package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
)

// tableNow is the reference time for RESETS in these tests.
var tableNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func tat(d time.Duration) *time.Time { t := tableNow.Add(d); return &t }

func win(kind provider.WindowKind, label string, p float64, reset *time.Time) provider.Window {
	return provider.Window{Kind: kind, Label: label, Percent: p, ResetsAt: reset}
}

// timeoutErr is an http.Client-style timeout (a net.Error, not a context error).
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "Client.Timeout exceeded" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

const orgBlockedMsg = "OAuth authentication is currently not allowed for this organization."

// TestUsageVerdict_Matrix pins STATUS, its reason, LEFT and which window's
// reset is shown, across window shapes and every error class.
func TestUsageVerdict_Matrix(t *testing.T) {
	t.Parallel()
	s5 := func(p float64, d time.Duration) provider.Window {
		return win(provider.WindowSession, "Current session", p, tat(d))
	}
	w7 := func(p float64, d time.Duration) provider.Window {
		return win(provider.WindowWeekly, "All models", p, tat(d))
	}
	for _, c := range []struct {
		name    string
		windows []provider.Window
		err     error
		status  usageStatus
		reason  string
		left    float64 // -1: no LEFT
		resetIn time.Duration
	}{
		{name: "5h missing, weekly only", windows: []provider.Window{w7(2, 160*time.Hour)}, status: usageOK, left: 98, resetIn: 160 * time.Hour},
		{name: "both present, weekly decides", windows: []provider.Window{s5(26, 2*time.Hour), w7(53, 100*time.Hour)}, status: usageOK, left: 47, resetIn: 100 * time.Hour},
		{name: "both present, 5h decides", windows: []provider.Window{s5(80, 2*time.Hour), w7(10, 100*time.Hour)}, status: usageOK, left: 20, resetIn: 2 * time.Hour},
		{name: "limit reached on 5h", windows: []provider.Window{s5(100, time.Hour), w7(53, 100*time.Hour)}, status: usageLimit, left: 0, resetIn: time.Hour},
		{name: "over 100 clamps to 0 left", windows: []provider.Window{s5(104, time.Hour)}, status: usageLimit, left: 0, resetIn: time.Hour},
		{name: "tie at cap: later reset decides", windows: []provider.Window{s5(100, time.Hour), w7(100, 50*time.Hour)}, status: usageLimit, left: 0, resetIn: 50 * time.Hour},
		{name: "exhausted severity below 100", windows: []provider.Window{{Kind: provider.WindowWeekly, Percent: 99.6, Severity: provider.SeverityExhausted, ResetsAt: tat(time.Hour)}}, status: usageLimit, left: 0.4, resetIn: time.Hour},
		{name: "per-model window can decide", windows: []provider.Window{w7(40, 100*time.Hour), win(provider.WindowScoped, "Fable", 90, tat(30*time.Hour))}, status: usageOK, left: 10, resetIn: 30 * time.Hour},
		{name: "rejected 403 blocked by org", err: fmt.Errorf("%w (HTTP 403: %s)", provider.ErrUnauthorized, orgBlockedMsg), status: usageRejected, reason: reasonOrgBlocked, left: -1},
		{name: "rejected 401 other message", err: fmt.Errorf("%w (HTTP 401: invalid bearer token)", provider.ErrUnauthorized), status: usageRejected, reason: "HTTP 401", left: -1},
		{name: "rejected bare", err: provider.ErrUnauthorized, status: usageRejected, left: -1},
		{name: "expired", err: provider.ErrExpired, status: usageExpired, left: -1},
		{name: "timeout via context", err: fmt.Errorf("claude: probe x: %w", context.DeadlineExceeded), status: usageError, reason: reasonTimeout, left: -1},
		{name: "timeout via http client", err: fmt.Errorf("claude: probe x: %w", timeoutErr{}), status: usageError, reason: reasonTimeout, left: -1},
		{name: "rate limited", err: provider.RateLimitError(nil), status: usageError, reason: reasonRateLimited, left: -1},
		{name: "api-key login", err: provider.ErrAPIBilled, status: usagePerToken, left: -1},
		{name: "other error is shortened", err: errors.New(strings.Repeat("x", 100)), status: usageError, reason: strings.Repeat("x", reasonMaxRunes-1) + reasonEllipsis, left: -1},
		{name: "no windows and no error", status: usageOK, left: -1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			v := usageVerdictOf(render.UsageReport{Usage: provider.Usage{Windows: c.windows}, Err: c.err})
			if v.Status != c.status || v.Reason != c.reason {
				t.Errorf("status %q reason %q, want %q %q", v.Status, v.Reason, c.status, c.reason)
			}
			switch {
			case c.left < 0 && v.Left != nil:
				t.Errorf("LEFT = %v, want none", *v.Left)
			case c.left >= 0 && (v.Left == nil || fmt.Sprintf("%.1f", *v.Left) != fmt.Sprintf("%.1f", c.left)):
				t.Errorf("LEFT = %v, want %v", v.Left, c.left)
			case c.left >= 0 && (v.ResetsAt == nil || !v.ResetsAt.Equal(tableNow.Add(c.resetIn))):
				t.Errorf("RESETS = %v, want now+%s", v.ResetsAt, c.resetIn)
			}
		})
	}
}

// TestBuildLoginTable_Mix: a realistic mix in one table. A missing window
// is "—" (never 0%), a per-model column appears only because one probe
// returned it, # is the login ls position, and every row has every column.
func TestBuildLoginTable_Mix(t *testing.T) {
	t.Parallel()
	reports := []render.UsageReport{
		{Login: "khanakia", Profile: provider.Profile{Provider: provider.Claude}, Err: fmt.Errorf("%w (HTTP 403: %s)", provider.ErrUnauthorized, orgBlockedMsg)},
		{Login: "erman", Profile: provider.Profile{Provider: provider.Claude}, Account: "e@example.com", Usage: provider.Usage{Windows: []provider.Window{
			win(provider.WindowSession, "Current session", 0, tat(time.Hour)), win(provider.WindowWeekly, "All models", 98, tat(67*time.Hour))}}},
		{Login: "ismail", Profile: provider.Profile{Provider: provider.Claude}, Usage: provider.Usage{Windows: []provider.Window{
			win(provider.WindowSession, "Current session", 100, tat(101*time.Minute)), win(provider.WindowWeekly, "All models", 53, tat(90*time.Hour)),
			win(provider.WindowScoped, "Fable", 12, tat(90*time.Hour))}}},
		{Login: "amank", Profile: provider.Profile{Provider: provider.Claude}, Usage: provider.Usage{Windows: []provider.Window{win(provider.WindowWeekly, "All models", 2, tat(166*time.Hour))}}},
		{Login: "slow", Profile: provider.Profile{Provider: provider.Codex}, Err: context.DeadlineExceeded},
		{Login: "old", Profile: provider.Profile{Provider: provider.Claude}, Err: provider.ErrExpired},
	}
	tb := buildLoginTable(reports, []int{1, 2, 5, 7, 8, 9}, tableNow, time.UTC)
	wantHeader := []string{colNum, colLogin, colProvider, colAccount, "5H USED", "5H RESETS", "WEEKLY USED", "WEEKLY RESETS", "RESET DAY", "RESETS ON", "FABLE WEEKLY USED", "FABLE WEEKLY RESETS", colLeft, colStatus}
	if strings.Join(tb.Header, "|") != strings.Join(wantHeader, "|") {
		t.Fatalf("header %v, want %v", tb.Header, wantHeader)
	}
	texts := func(r []render.Cell) string {
		var s []string
		for _, c := range r {
			s = append(s, c.Text)
		}
		return strings.Join(s, "|")
	}
	want := []string{
		"1|khanakia|claude|—|—|—|—|—|—|—|—|—|—|rejected: blocked by org",
		"2|erman|claude|e@example.com|0%|1 hr 0 min|98%|2 d 19 hr|Sat|Sat 7 Nov 07:00|—|—|2%|ok",
		"5|ismail|claude|—|100%|1 hr 41 min|53%|3 d 18 hr|Sun|Sun 8 Nov 06:00|12%|3 d 18 hr|0%|limit reached",
		"7|amank|claude|—|—|—|2%|6 d 22 hr|Wed|Wed 11 Nov 10:00|—|—|98%|ok",
		"8|slow|codex|—|—|—|—|—|—|—|—|—|—|error: timeout",
		"9|old|claude|—|—|—|—|—|—|—|—|—|—|expired",
	}
	for i, w := range want {
		if got := texts(tb.Rows[i]); got != w {
			t.Errorf("row %d:\n got %s\nwant %s", i, got, w)
		}
		if len(tb.Rows[i]) != len(tb.Header) {
			t.Errorf("row %d has %d cells, header %d", i, len(tb.Rows[i]), len(tb.Header))
		}
	}
	// LEFT tones: 2% is a warning, 0% is bad, 98% is fine.
	for i, tone := range map[int]render.Tone{1: render.ToneWarn, 2: render.ToneBad, 3: render.ToneGood} {
		if got := tb.Rows[i][12].Tone; got != tone {
			t.Errorf("row %d LEFT tone %v, want %v", i, got, tone)
		}
	}
	// No per-model column when no probe returned one.
	if h := buildLoginTable(reports[:2], []int{1, 2}, tableNow, time.UTC).Header; len(h) != 12 {
		t.Errorf("header without a scoped window: %v", h)
	}
	// A missing weekly reset is a dim "—" in both date cells.
	for _, c := range tb.Rows[0][8:10] {
		if c.Tone != render.ToneDim {
			t.Errorf("missing RESET DAY / RESETS ON not dim: %+v", c)
		}
	}
	// --tz moves ismail's Sun 06:00 UTC reset back past midnight into Saturday.
	la := mustZone(t, "America/Los_Angeles")
	if got := texts(buildLoginTable(reports, []int{1, 2, 5, 7, 8, 9}, tableNow, la).Rows[2][8:10]); got != "Sat|Sat 7 Nov 22:00" {
		t.Errorf("ismail in America/Los_Angeles = %s, want Sat|Sat 7 Nov 22:00", got)
	}
}

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// weeklyReport is a login with a 5-hour and a weekly window.
func weeklyReport(name string, used5h, usedWeekly float64, weeklyIn time.Duration) render.UsageReport {
	return render.UsageReport{Login: name, Profile: provider.Profile{Provider: provider.Claude}, Usage: provider.Usage{Windows: []provider.Window{
		win(provider.WindowSession, "Current session", used5h, tat(time.Hour+time.Duration(used5h)*time.Minute)),
		win(provider.WindowWeekly, "All models", usedWeekly, tat(weeklyIn))}}}
}

// TestWeeklyResetSummary: per weekday, the logins resetting then and the
// weekly USED they get back, soonest first, in the chosen zone.
func TestWeeklyResetSummary(t *testing.T) {
	t.Parallel()
	// tableNow is Wed 4 Nov 12:00 UTC.
	reports := []render.UsageReport{
		weeklyReport("sat1", 0, 98, 67*time.Hour), // Sat 7 Nov 07:00 UTC
		weeklyReport("tue", 0, 2, 154*time.Hour),  // Tue 10 Nov 22:00 UTC, Wed 03:30 IST
		weeklyReport("thu", 0, 79, 30*time.Hour),  // Thu 5 Nov 18:00 UTC, 23:30 IST
		weeklyReport("sat2", 0, 66, 70*time.Hour), // Sat 7 Nov 10:00 UTC
		weeklyReport("wed", 0, 40, 156*time.Hour), // Wed 11 Nov 00:00 UTC
		{Login: "dead", Err: provider.ErrExpired}, // no windows: left out
		{Login: "noreset", Usage: provider.Usage{Windows: []provider.Window{win(provider.WindowWeekly, "All models", 50, nil)}}},
	}
	for _, c := range []struct {
		loc  *time.Location
		want string
	}{
		{time.UTC, "Weekly resets: Thu 1 (+79%) · Sat 2 (+164%) · Tue 1 (+2%) · Wed 1 (+40%)"},
		{mustZone(t, "Asia/Kolkata"), "Weekly resets: Thu 1 (+79%) · Sat 2 (+164%) · Wed 2 (+42%)"},
		{mustZone(t, "America/Los_Angeles"), "Weekly resets: Thu 1 (+79%) · Fri 1 (+98%) · Sat 1 (+66%) · Tue 2 (+42%)"},
	} {
		if got := weeklyResetSummary(reports, c.loc); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.loc, got, c.want)
		}
	}
	if got := weeklyResetSummary(reports[5:], time.UTC); got != "" {
		t.Errorf("no weekly reset: %q, want empty", got)
	}
}

// TestSortLoginRows_Matrix: multi-key sort, "-" for high to low, missing
// values last in both directions, stable, # kept, no keys = unchanged.
func TestSortLoginRows_Matrix(t *testing.T) {
	t.Parallel()
	reports := []render.UsageReport{
		weeklyReport("c", 10, 20, 67*time.Hour),  // Sat UTC, left 80
		weeklyReport("a", 50, 30, 154*time.Hour), // Tue UTC / Wed IST, left 50
		{Login: "x", Err: provider.ErrExpired},   // no windows
		weeklyReport("b", 5, 90, 30*time.Hour),   // Thu, left 10
		weeklyReport("d", 0, 25, 68*time.Hour),   // Sat UTC, left 75
		{Login: "w", Usage: provider.Usage{Windows: []provider.Window{win(provider.WindowWeekly, "All models", 60, tat(156*time.Hour))}}}, // no 5h; Wed 00:00 UTC
	}
	ords := []int{3, 1, 9, 2, 4, 7}
	kolkata := mustZone(t, "Asia/Kolkata")
	for _, c := range []struct {
		spec string
		loc  *time.Location
		want string
	}{
		{"", time.UTC, "c3 a1 x9 b2 d4 w7"},
		{"login", time.UTC, "a1 b2 c3 d4 w7 x9"},
		{"-login", time.UTC, "x9 w7 d4 c3 b2 a1"},
		{"left", time.UTC, "b2 w7 a1 d4 c3 x9"},
		{"-left", time.UTC, "c3 d4 a1 w7 b2 x9"},
		{"5h", time.UTC, "d4 b2 c3 a1 x9 w7"},
		{"-5h", time.UTC, "a1 c3 b2 d4 x9 w7"},
		{"weekly-reset", time.UTC, "b2 c3 d4 a1 w7 x9"},
		{"-weekly-reset", time.UTC, "w7 a1 d4 c3 b2 x9"},
		{"day", time.UTC, "a1 w7 b2 c3 d4 x9"},
		{"day,left", time.UTC, "a1 w7 b2 d4 c3 x9"},
		{"day,-left", time.UTC, "a1 w7 b2 c3 d4 x9"},
		// In Asia/Kolkata a's Tue 22:00 UTC reset is Wednesday, beside w.
		{"day,left", kolkata, "w7 a1 b2 d4 c3 x9"},
		{"day,-left", kolkata, "a1 w7 b2 c3 d4 x9"},
		{"-day", time.UTC, "c3 d4 b2 w7 a1 x9"},
		{"-weekly,login", time.UTC, "b2 w7 a1 d4 c3 x9"},
		{"status,-login", time.UTC, "w7 d4 c3 b2 a1 x9"},
		{"5h-reset", time.UTC, "d4 b2 c3 a1 x9 w7"},
	} {
		keys, err := parseSortKeys(c.spec)
		if err != nil {
			t.Fatalf("%q: %v", c.spec, err)
		}
		rs, so := sortLoginRows(reports, ords, keys, c.loc)
		var got []string
		for i, r := range rs {
			got = append(got, fmt.Sprintf("%s%d", r.Login, so[i]))
		}
		if g := strings.Join(got, " "); g != c.want {
			t.Errorf("--sort %q (%s): %s, want %s", c.spec, c.loc, g, c.want)
		}
	}
	// The inputs are not reordered.
	if reports[0].Login != "c" || ords[0] != 3 {
		t.Errorf("sortLoginRows modified its input")
	}
}

// TestParseSortKeysAndTZ_Errors: bad keys and zones are usage errors that
// say what is valid.
func TestParseSortKeysAndTZ_Errors(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{"bogus", "day,nope", "day,", "--left", "Day"} {
		_, err := parseSortKeys(spec)
		if err == nil || ExitCode(err) != ExitUsage || !strings.Contains(err.Error(), "valid keys: login, left, 5h, 5h-reset, weekly, weekly-reset, day, status") {
			t.Errorf("--sort %q: %v", spec, err)
		}
	}
	if keys, err := parseSortKeys(" day , -left "); err != nil || fmt.Sprint(keys) != "[{day false} {left true}]" {
		t.Errorf("spaced keys: %v %v", keys, err)
	}
	for _, name := range []string{"Nowhere/Land", "asia/kolkata_x"} {
		if _, err := parseTZ(name); err == nil || ExitCode(err) != ExitUsage || !strings.Contains(err.Error(), "unknown time zone") {
			t.Errorf("--tz %q: %v", name, err)
		}
	}
	if loc, err := parseTZ(""); err != nil || loc != time.Local {
		t.Errorf("empty --tz = %v %v, want local", loc, err)
	}
}

// TestUsageTable_SortTZCommand runs the real command with --sort and --tz:
// rows reorder with # kept, the date columns and summary use the zone, the
// JSON gains weekly_resets_at / weekly_reset_day, and bad values are
// refused before any probe.
func TestUsageTable_SortTZCommand(t *testing.T) {
	t.Parallel()
	h := probeHarness(t)
	h.now = tableNow
	h.env["X_TOKEN"] = tokX
	weekly := func(p float64, d time.Duration) provider.Usage {
		return provider.Usage{Windows: []provider.Window{win(provider.WindowWeekly, "All models", p, tat(d))}}
	}
	h.claude.probeUsage = map[string]provider.Usage{
		tokA: weekly(30, 154*time.Hour), // Tue 10 Nov 22:00 UTC = Wed 03:30 IST
		tokB: weekly(70, 30*time.Hour),  // Thu 5 Nov 18:00 UTC = 23:30 IST
		tokX: weekly(10, 160*time.Hour), // Wed 11 Nov 04:00 UTC = 09:30 IST
	}
	if code := h.run("usage", "--login", "a,b,x", "--sort", "day,-left", "--tz", "Asia/Kolkata"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	lines := strings.Split(strings.TrimRight(h.stdout.String(), "\n"), "\n")
	if len(lines) != 5 || !strings.Contains(lines[0], "WEEKLY RESETS  RESET DAY  RESETS ON") {
		t.Fatalf("table:\n%s", h.stdout.String())
	}
	for i, want := range []string{"4  x", "1  a", "2  b"} {
		if !strings.HasPrefix(lines[1+i], want) {
			t.Errorf("row %d = %q, want prefix %q", i, lines[1+i], want)
		}
	}
	if !strings.Contains(lines[1], "Wed 11 Nov 09:30") || !strings.Contains(lines[3], "Thu 5 Nov 23:30") {
		t.Errorf("dates not in Asia/Kolkata:\n%s", h.stdout.String())
	}
	if lines[4] != "Weekly resets: Thu 1 (+70%) · Wed 2 (+40%)" {
		t.Errorf("summary = %q", lines[4])
	}

	if code := h.run("usage", "--login", "a,b", "--json", "--tz", "Asia/Kolkata", "--sort", "-weekly"); code != ExitOK {
		t.Fatalf("json: exit %d %s", code, h.stderr.String())
	}
	env := decode[[]usageJSON](t, h.stdout.Bytes())
	if len(env.Data) != 2 || env.Data[0].Login != "b" || env.Data[0].WeeklyResetsAt != "2026-11-05T23:30:00+05:30" || env.Data[0].WeeklyResetDay != "Thu" ||
		env.Data[1].WeeklyResetsAt != "2026-11-11T03:30:00+05:30" || env.Data[1].WeeklyResetDay != "Wed" {
		t.Errorf("json: %+v", env.Data)
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"usage", "--login", "all", "--sort", "bogus"}, "valid keys: login, left"},
		{[]string{"usage", "--login", "all", "--tz", "Nowhere/Land"}, "unknown time zone"},
		{[]string{"usage", "--sort", "day"}, "--sort orders the login table"},
		{[]string{"usage", "--login", "a", "--sort", "day"}, "--sort orders the login table"},
	} {
		before := h.probeCount()
		if code := h.run(c.args...); code != ExitUsage || !strings.Contains(h.stderr.String(), c.want) {
			t.Errorf("%v: exit %d %q", c.args, code, h.stderr.String())
		}
		if h.probeCount() != before {
			t.Errorf("%v probed although refused", c.args)
		}
	}
}

// TestParseLoginList_Matrix: comma lists and "all" resolve in login ls
// order with positions; home/auto/unknown/empty are usage errors.
func TestParseLoginList_Matrix(t *testing.T) {
	t.Parallel()
	h := probeHarness(t)
	cfg, err := (&app{Deps: h.deps()}).config()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		spec, want string // want: "name@pos,..." or "ERR:<substring>"
	}{
		{"b,a", "a@1,b@2"},
		{" a , x ", "a@1,x@4"},
		{"a,a", "a@1"},
		{"all", "a@1,b@2,k@3,x@4"},
		{"a,all", "a@1,b@2,k@3,x@4"},
		{"a,", "a@1"},
		{",", "ERR:lists no logins"},
		{"a,home", "ERR:cannot be part of a list"},
		{"auto,b", "ERR:cannot be part of a list"},
		{"a,nosuch", `ERR:unknown login "nosuch"`},
		{"x,b,x,b", "b@2,x@4"},
		{" all , k ", "a@1,b@2,k@3,x@4"},
		{"  ", "ERR:lists no logins"},
		{"A", `ERR:unknown login "A"`},
		{"home", "ERR:cannot be part of a list"},
	} {
		ls, ord, err := parseLoginList(cfg, c.spec)
		var got string
		if err != nil {
			got = "ERR:" + err.Error()
			if !strings.HasPrefix(c.want, "ERR:") || !strings.Contains(got, strings.TrimPrefix(c.want, "ERR:")) || ExitCode(err) != ExitUsage {
				t.Errorf("%q: %v (exit %d), want %s", c.spec, err, ExitCode(err), c.want)
			}
			continue
		}
		var parts []string
		for i, l := range ls {
			parts = append(parts, fmt.Sprintf("%s@%d", l.Name, ord[i]))
		}
		if got = strings.Join(parts, ","); got != c.want {
			t.Errorf("%q: %s, want %s", c.spec, got, c.want)
		}
	}
}

// TestUsageTable_Command runs the real command: the table for --logins
// --table and a --login list, the JSON verdict fields, --pick through the
// picker and its refusals.
func TestUsageTable_Command(t *testing.T) {
	t.Parallel()
	h := probeHarness(t)
	h.claude.probeErrs[tokB] = fmt.Errorf("%w (HTTP 403: %s)", provider.ErrUnauthorized, orgBlockedMsg)

	if code := h.run("usage", "--logins", "--table"); code != ExitFailure {
		t.Fatalf("one rejected login must exit 1, got %d: %s", code, h.stderr.String())
	}
	out := h.stdout.String()
	for _, want := range []string{"#  LOGIN  PROVIDER  ACCOUNT", "1  a      claude    a@example.com", "rejected: blocked by org", "per token", "x_token", "b: login rejected"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("table lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "me@home") {
		t.Errorf("the login table shows no homes:\n%s", out)
	}

	if code := h.run("usage", "--login", "a,x", "--json"); code != ExitFailure {
		t.Fatalf("x is unset: exit %d", code)
	}
	env := decode[[]usageJSON](t, h.stdout.Bytes())
	if env.Kind != kindUsageList || env.Count != 2 || env.Data[0].Login != "a" || env.Data[0].Status != usageOK ||
		env.Data[0].LeftPercent == nil || *env.Data[0].LeftPercent != 10 || env.Data[1].Status != usageError {
		t.Errorf("list json: %+v", env.Data)
	}

	// --table combines with a --login list and with a single name (one row).
	h.claude.probeErrs = map[string]error{}
	for _, c := range []struct {
		spec string
		rows []string
	}{
		{"b,a", []string{"1  a", "2  b"}},
		{"b", []string{"2  b"}},
	} {
		if code := h.run("usage", "--login", c.spec, "--table"); code != ExitOK {
			t.Fatalf("--login %s --table: exit %d %s", c.spec, code, h.stderr.String())
		}
		out := h.stdout.String()
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if !strings.HasPrefix(lines[0], "#  LOGIN") || len(lines) != 1+len(c.rows) {
			t.Errorf("--login %s --table:\n%s", c.spec, out)
		}
		for i, r := range c.rows {
			if !strings.HasPrefix(lines[1+i], r) {
				t.Errorf("--login %s --table row %d = %q, want %q", c.spec, i, lines[1+i], r)
			}
		}
	}
	h.claude.probeErrs[tokB] = fmt.Errorf("%w (HTTP 403: %s)", provider.ErrUnauthorized, orgBlockedMsg)

	// The home view keeps its shape and gains the same verdict fields.
	if code := h.run("usage", "--json"); code != ExitOK {
		t.Fatalf("usage --json: %d", code)
	}
	for _, r := range decode[[]usageJSON](t, h.stdout.Bytes()).Data {
		if r.Status == "" {
			t.Errorf("home row without status: %+v", r)
		}
	}

	// --pick: marks come back in any order, the table keeps login ls order.
	h.tty = true
	h.picker.script = []pickStep{{idx: []int{3, 0}}}
	if code := h.run("usage", "--pick"); code != ExitFailure {
		t.Fatalf("--pick: exit %d %s", code, h.stderr.String())
	}
	if out := h.stdout.String(); !strings.Contains(out, "1  a") || strings.Index(out, "4  x") < strings.Index(out, "1  a") || strings.Contains(out, "2  b") {
		t.Errorf("--pick table:\n%s", out)
	}
	h.picker.script = []pickStep{{err: ErrCancelled}}
	if code := h.run("usage", "--pick"); code != ExitFailure || h.stdout.Len() != 0 {
		t.Errorf("cancelled pick: exit %d, stdout %q", code, h.stdout.String())
	}

	for _, c := range []struct {
		args []string
		tty  bool
		want string
	}{
		{[]string{"usage", "--pick"}, false, "needs an interactive terminal"},
		{[]string{"usage", "--table"}, true, "--table shows token logins"},
		{[]string{"usage", "--pick", "--logins"}, true, "one way"},
		{[]string{"usage", "--pick", "--login", "a"}, true, "one way"},
		{[]string{"usage", "--login", "a,b", "personal"}, true, "profile names"},
	} {
		h.tty = c.tty
		before := h.probeCount()
		if code := h.run(c.args...); code != ExitUsage || !strings.Contains(h.stderr.String(), c.want) {
			t.Errorf("%v: exit %d %q", c.args, code, h.stderr.String())
		}
		if h.probeCount() != before {
			t.Errorf("%v probed although refused", c.args)
		}
	}
}

// keyReader returns one scripted key per Read, as a terminal in raw mode does.
type keyReader struct {
	keys []string
	out  strings.Builder
}

func (k *keyReader) Read(p []byte) (int, error) {
	if len(k.keys) == 0 {
		return 0, io.EOF
	}
	n := copy(p, k.keys[0])
	k.keys = k.keys[1:]
	return n, nil
}

func (k *keyReader) Write(p []byte) (int, error) { return k.out.Write(p) }

// TestChecklist_Keys drives the checklist with real key bytes: space,
// down, a (all / none), Enter, Esc, q, arrows in both cursor modes.
func TestChecklist_Keys(t *testing.T) {
	t.Parallel()
	items := []string{"a", "b", "c"}
	for _, c := range []struct {
		name string
		keys []string
		want string // picked indexes, or "cancel"
	}{
		{"space down a enter: a selects all", []string{" ", "\x1b[B", "a", "\r"}, "[0 1 2]"},
		{"a twice clears, enter takes cursor", []string{"a", "a", "j", "\r"}, "[1]"},
		{"mark two out of order", []string{"\x1b[B", "\x1b[B", " ", "\x1bOA", "\x1bOA", " ", "\n"}, "[0 2]"},
		{"up wraps to last", []string{"k", " ", "\r"}, "[2]"},
		{"esc cancels", []string{" ", "\x1b"}, "cancel"},
		{"q cancels", []string{"q"}, "cancel"},
		{"ctrl-c cancels", []string{"\x03"}, "cancel"},
		{"eof cancels", nil, "cancel"},
		{"unknown keys ignored", []string{"z", "\x1b[C", " ", "\r"}, "[0]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			kr := &keyReader{keys: c.keys}
			got, err := runChecklist(kr, "pick", items)
			switch {
			case c.want == "cancel" && !errors.Is(err, ErrCancelled):
				t.Errorf("got %v %v, want cancel", got, err)
			case c.want != "cancel" && (err != nil || fmt.Sprint(got) != c.want):
				t.Errorf("got %v %v, want %s", got, err, c.want)
			}
		})
	}
}

// TestResetsCell_Format: a window's RESETS cell reads as time left,
// coarsening with distance; a reset in the past reads "now" and an unknown
// one is a dim "—" (never "0 min").
func TestResetsCell_Format(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		at   *time.Time
		want string
		tone render.Tone
	}{
		{nil, cellMissingWindow, render.ToneDim},
		{tat(-time.Hour), "now", render.TonePlain},
		{tat(30 * time.Second), "now", render.TonePlain},
		{tat(59 * time.Minute), "59 min", render.TonePlain},
		{tat(time.Hour), "1 hr 0 min", render.TonePlain},
		{tat(23*time.Hour + 59*time.Minute), "23 hr 59 min", render.TonePlain},
		{tat(24 * time.Hour), "1 d 0 hr", render.TonePlain},
		{tat(166*time.Hour + 30*time.Minute), "6 d 22 hr", render.TonePlain},
	} {
		got := resetsCell(c.at, tableNow)
		if got.Text != c.want || got.Tone != c.tone {
			t.Errorf("resetsCell(%v) = %q/%v, want %q/%v", c.at, got.Text, got.Tone, c.want, c.tone)
		}
	}
}
