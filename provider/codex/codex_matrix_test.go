package codex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/khanakia/agx/provider"
)

// fakeAccessToken is obviously fake; no error may ever contain it.
const fakeAccessToken = "fake-token-0001"

// authWith returns an auth.json body with the given access token and account.
func authWith(token, account string) string {
	return `{"auth_mode":"chatgpt","tokens":{"access_token":"` + token + `","account_id":"` + account + `"}}`
}

func TestPlanLabel_Matrix(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":         "",
		"plus":     "Plus",
		"pro":      "Pro",
		"Plus":     "Plus",
		"p":        "P",
		"9team":    "9team",
		"édu":      "Édu", // multi-byte first rune: must stay valid UTF-8
		"ñandú":    "Ñandú",
		"日本":       "日本",
		"business": "Business",
	} {
		got := planLabel(in)
		if got != want || !utf8.ValidString(got) {
			t.Errorf("planLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToWindow_Matrix(t *testing.T) {
	t.Parallel()
	reset := int64(1790000000)
	for _, tc := range []struct {
		name  string
		w     wireWindow
		kind  provider.WindowKind
		label string
	}{
		{"five hour", wireWindow{UsedPercent: 1, LimitWindowSeconds: fiveHourSeconds}, provider.WindowSession, labelFiveHour},
		{"week", wireWindow{UsedPercent: 2, LimitWindowSeconds: weekSeconds}, provider.WindowWeekly, labelWeekly},
		{"thirty day", wireWindow{UsedPercent: 3, LimitWindowSeconds: thirtyDaySeconds}, provider.WindowOther, labelThirtyDay},
		{"one day", wireWindow{LimitWindowSeconds: daySeconds}, provider.WindowOther, "1-day"},
		{"ninety day", wireWindow{LimitWindowSeconds: 90 * daySeconds}, provider.WindowOther, "90-day"},
		{"with reset", wireWindow{LimitWindowSeconds: weekSeconds, ResetAt: &reset}, provider.WindowWeekly, labelWeekly},
	} {
		got := toWindow(tc.w, vendorPrimary)
		if got.Kind != tc.kind || got.Label != tc.label || got.Group != provider.GroupPlan || got.Percent != tc.w.UsedPercent || got.VendorKind != vendorPrimary {
			t.Errorf("%s: %+v, want kind %s label %s", tc.name, got, tc.kind, tc.label)
		}
		if (tc.w.ResetAt == nil) != (got.ResetsAt == nil) || (got.ResetsAt != nil && got.ResetsAt.Unix() != reset) {
			t.Errorf("%s: resets = %v", tc.name, got.ResetsAt)
		}
	}
}

// TestParseUsage_LimitReachedMatrix: only a window at or over 100% of an
// account flagged limit_reached is exhausted; the flag alone marks nothing.
func TestParseUsage_LimitReachedMatrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		reached bool
		pct     string
		want    provider.Severity
	}{
		{"reached at 100", true, "100", provider.SeverityExhausted},
		{"reached over 100", true, "100.5", provider.SeverityExhausted},
		{"reached at 99.9", true, "99.9", ""},
		{"not reached at 100", false, "100", ""},
	} {
		body := `{"rate_limit":{"limit_reached":` + map[bool]string{true: "true", false: "false"}[tc.reached] +
			`,"primary_window":{"used_percent":` + tc.pct + `,"limit_window_seconds":18000},"secondary_window":{"used_percent":1,"limit_window_seconds":604800}}}`
		u, err := ParseUsage([]byte(body))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if u.Windows[0].Severity != tc.want || u.Windows[1].Severity != "" {
			t.Errorf("%s: severities = %q/%q, want %q/\"\"", tc.name, u.Windows[0].Severity, u.Windows[1].Severity, tc.want)
		}
	}
}

// TestUsage_AuthStatesMatrix: every auth.json shape that cannot be used is
// decided locally, before any request.
func TestUsage_AuthStatesMatrix(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	for _, tc := range []struct {
		name, auth string
		is         error
		want       string
	}{
		{"garbage json", "{not json", nil, "decode auth.json"},
		{"tokens null, no key", `{"tokens":null}`, provider.ErrNotLoggedIn, ""},
		{"empty access token", authWith("", "acc"), provider.ErrNotLoggedIn, ""},
		{"api key only", `{"OPENAI_API_KEY":"fake-key-0001","tokens":null}`, provider.ErrAPIBilled, ""},
		{"empty api key, no tokens", `{"OPENAI_API_KEY":"","tokens":null}`, provider.ErrNotLoggedIn, ""},
		{"jwt expired exactly now", authWith(fakeJWT(t, now.Unix()), "acc"), provider.ErrExpired, ""},
	} {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, authFile), tc.auth)
		p := &Provider{Endpoint: srv.URL, HTTP: srv.Client(), Now: func() time.Time { return now }}
		_, err := p.Usage(context.Background(), provider.Profile{Home: home})
		if err == nil || (tc.is != nil && !errors.Is(err, tc.is)) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %v %q", tc.name, err, tc.is, tc.want)
		}
		if _, serr := p.Subscription(context.Background(), provider.Profile{Home: home}); serr == nil {
			t.Errorf("%s: Subscription succeeded on unusable auth", tc.name)
		}
	}
	if hits != 0 {
		t.Errorf("local auth states reached the network %d times", hits)
	}
}

// TestUsage_RequestMatrix: the account header is sent only when known, and
// every failure after the request names no token.
func TestUsage_RequestMatrix(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var sawAccount []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, has := r.Header[http.CanonicalHeaderKey(accountIDHeader)]
		sawAccount = append(sawAccount, map[bool]string{true: r.Header.Get(accountIDHeader), false: "<absent>"}[has])
		switch r.URL.Path {
		case "/403":
			w.WriteHeader(http.StatusForbidden)
		case "/429":
			w.Header().Set("Retry-After", "9")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/502":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("upstream\n  down " + strings.Repeat("é", 200))) // test server
		case "/empty":
			_, _ = w.Write([]byte(`{}`)) // test server
		default:
			_, _ = w.Write(body) // test server
		}
	}))
	defer srv.Close()

	withAcct, noAcct := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(withAcct, authFile), authWith(fakeAccessToken, "acc-ü"))
	writeFile(t, filepath.Join(noAcct, authFile), authWith(fakeAccessToken, ""))

	p := &Provider{Endpoint: srv.URL, HTTP: srv.Client()}
	if _, err := p.Usage(context.Background(), provider.Profile{Home: withAcct}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Usage(context.Background(), provider.Profile{Home: noAcct}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sawAccount, []string{"acc-ü", "<absent>"}) {
		t.Errorf("account header = %q", sawAccount)
	}

	for _, tc := range []struct {
		path string
		is   error
		want string
	}{
		{"/403", provider.ErrUnauthorized, "HTTP 403"},
		{"/429", provider.ErrRateLimited, "retry after 9s"},
		{"/502", nil, "unexpected HTTP 502: upstream down"},
		{"/empty", ErrNoWindows, ""},
	} {
		p := &Provider{Endpoint: srv.URL + tc.path, HTTP: srv.Client()}
		u, err := p.Usage(context.Background(), provider.Profile{Home: withAcct})
		if err == nil || (tc.is != nil && !errors.Is(err, tc.is)) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %v %q", tc.path, err, tc.is, tc.want)
		}
		if err != nil && (strings.Contains(err.Error(), fakeAccessToken) || !utf8.ValidString(err.Error())) {
			t.Errorf("%s: error leaks the token or is invalid UTF-8: %q", tc.path, err)
		}
		if tc.path == "/empty" && !strings.HasSuffix(u.Identity.CredentialSource, authFile) {
			t.Errorf("credential source lost on parse error: %+v", u.Identity)
		}
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	for name, endpoint := range map[string]string{"bad url": "://bad", "closed": closed.URL} {
		_, err := (&Provider{Endpoint: endpoint}).Usage(context.Background(), provider.Profile{Home: withAcct})
		if err == nil || strings.Contains(err.Error(), fakeAccessToken) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestDiscoverAndIdentity_Matrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, auth string
		billing    provider.Billing
		loggedIn   bool
		idErr      bool
	}{
		{"chatgpt", chatgptAuth, provider.BillingPlan, true, false},
		{"api key", `{"OPENAI_API_KEY":"fake-key-0001","tokens":null}`, provider.BillingAPI, true, false},
		{"both: tokens win", `{"OPENAI_API_KEY":"fake-key-0001","tokens":{"access_token":"` + fakeAccessToken + `"}}`, provider.BillingPlan, true, false},
		{"empty object", `{}`, provider.BillingPlan, false, false},
		{"garbage", `{nope`, provider.BillingPlan, false, true},
	} {
		userHome := t.TempDir()
		writeFile(t, filepath.Join(userHome, defaultDirName, authFile), tc.auth)
		p := &Provider{}
		ps, err := p.Discover(userHome)
		if err != nil || len(ps) != 1 || ps[0].Billing != tc.billing {
			t.Errorf("%s: Discover = %+v, %v", tc.name, ps, err)
			continue
		}
		id, err := p.Identity(ps[0])
		if (err != nil) != tc.idErr || id.LoggedIn != tc.loggedIn {
			t.Errorf("%s: Identity = %+v, %v", tc.name, id, err)
		}
	}
	// No auth.json: logged out, not an error.
	if id, err := (&Provider{}).Identity(provider.Profile{Home: t.TempDir()}); err != nil || id.LoggedIn {
		t.Errorf("no auth = %+v, %v", id, err)
	}
}

// TestReadMeta_Matrix: only a first line that is a complete session_meta
// record (type, id and cwd) makes a rollout listable.
func TestReadMeta_Matrix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, tc := range []struct {
		name, content string
		ok            bool
	}{
		{"valid", `{"type":"session_meta","payload":{"id":"i1","cwd":"/w"}}` + "\n{}", true},
		{"unicode cwd", `{"type":"session_meta","payload":{"id":"i2","cwd":"/wörk/日本"}}`, true},
		{"empty file", "", false},
		{"blank first line", "\n" + `{"type":"session_meta","payload":{"id":"i3","cwd":"/w"}}`, false},
		{"wrong type", `{"type":"response_item","payload":{"id":"i4","cwd":"/w"}}`, false},
		{"no id", `{"type":"session_meta","payload":{"cwd":"/w"}}`, false},
		{"no cwd", `{"type":"session_meta","payload":{"id":"i5"}}`, false},
		{"not json", `session_meta i6 /w`, false},
		{"first line too long", `{"type":"session_meta","payload":{"id":"i7","cwd":"/w","pad":"` + strings.Repeat("x", headScanBytes) + `"}}`, false},
	} {
		path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+transcriptExt)
		writeFile(t, path, tc.content)
		if _, ok := readMeta(path); ok != tc.ok {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.ok)
		}
	}
	if _, ok := readMeta(filepath.Join(dir, "missing.jsonl")); ok {
		t.Error("missing file read as a rollout")
	}
}

func TestConversations_LimitAndFilter(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, cwd := range []string{"/a", "/b", "/a/", "/a"} {
		path := filepath.Join(home, sessionsDir, "2026", "10", "01", rolloutPrefix+string(rune('w'+i))+transcriptExt)
		writeFile(t, path, `{"type":"session_meta","payload":{"id":"id-`+string(rune('w'+i))+`","cwd":"`+cwd+`"}}`)
		mod := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	// Not rollouts: ignored.
	writeFile(t, filepath.Join(home, sessionsDir, "notes.jsonl"), `{"type":"session_meta","payload":{"id":"x","cwd":"/a"}}`)
	writeFile(t, filepath.Join(home, sessionsDir, rolloutPrefix+"x.txt"), `{"type":"session_meta","payload":{"id":"y","cwd":"/a"}}`)

	p := &Provider{}
	ids := func(cs []provider.Conversation) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	all, err := p.Conversations(context.Background(), home, provider.ConversationQuery{})
	if err != nil || !slices.Equal(ids(all), []string{"id-z", "id-y", "id-x", "id-w"}) {
		t.Errorf("all = %v, %v (want newest first)", ids(all), err)
	}
	a, _ := p.Conversations(context.Background(), home, provider.ConversationQuery{Dir: "/a"})
	if !slices.Equal(ids(a), []string{"id-z", "id-y", "id-w"}) {
		t.Errorf("dir /a = %v (trailing slash must match)", ids(a))
	}
	two, _ := p.Conversations(context.Background(), home, provider.ConversationQuery{Limit: 2})
	if !slices.Equal(ids(two), []string{"id-z", "id-y"}) {
		t.Errorf("limit 2 = %v", ids(two))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Conversations(ctx, home, provider.ConversationQuery{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled = %v", err)
	}
}

func TestMoveUnsupportedReason(t *testing.T) {
	t.Parallel()
	var mu provider.MoveUnsupported = &Provider{}
	r := mu.MoveUnsupportedReason()
	// The reason is shown to the user instead of "not supported": it must
	// say why (the SQLite index) and what to do instead.
	for _, want := range []string{"SQLite", "state_5.sqlite", "original account"} {
		if !strings.Contains(r, want) {
			t.Errorf("reason %q lacks %q", r, want)
		}
	}
	if _, isMover := mu.(provider.ConversationMover); isMover {
		t.Error("codex must not offer PlanMove while it reports moves unsupported")
	}
}

func TestProvider_Basics(t *testing.T) {
	t.Parallel()
	p := New("/u")
	if p.ID() != provider.Codex || p.Binary() != Binary || p.DefaultHome("/u") != filepath.Join("/u", defaultDirName) || p.HTTP == nil {
		t.Errorf("basics = %+v", p)
	}
	if got := (&Provider{}).now(); time.Since(got) > time.Minute {
		t.Errorf("default clock = %v", got)
	}
}

// TestLaunch_HomeEnvMatrix: CODEX_HOME is absent for every spelling of the
// default home and set for every other home.
func TestLaunch_HomeEnvMatrix(t *testing.T) {
	t.Parallel()
	p := &Provider{UserHome: "/u"}
	for _, tc := range []struct {
		home, want string // want "" = must be absent
	}{
		{"/u/.codex", ""},
		{"/u/.codex/", ""},
		{"/u//.codex", ""},
		{"/u/.codex-work", "/u/.codex-work"},
		{"/u/.codex-ñ", "/u/.codex-ñ"},
	} {
		cmd, err := p.Launch(provider.Profile{Home: tc.home, Env: map[string]string{"X": "1"}}, provider.LaunchRequest{Env: []string{HomeEnv + "=/stale"}, Args: []string{"exec"}})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, kv := range cmd.Env {
			if v, ok := strings.CutPrefix(kv, HomeEnv+"="); ok {
				got = append(got, v)
			}
		}
		if (tc.want == "" && len(got) != 0) || (tc.want != "" && !slices.Equal(got, []string{tc.want})) {
			t.Errorf("home %q: %s = %v, want %q", tc.home, HomeEnv, got, tc.want)
		}
		if !slices.Contains(cmd.Env, "X=1") || !slices.Equal(cmd.Args, []string{"exec"}) {
			t.Errorf("home %q: cmd = %+v", tc.home, cmd)
		}
	}
}
