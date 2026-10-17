package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

// fakeAccessToken is obviously fake; it stands in for a home's OAuth login.
const fakeAccessToken = "fake-token-0001"

func TestWindowKind_Matrix(t *testing.T) {
	t.Parallel()
	for wire, want := range map[string]provider.WindowKind{
		kindSession:      provider.WindowSession,
		kindWeeklyAll:    provider.WindowWeekly,
		kindWeeklyScoped: provider.WindowScoped,
		"":               provider.WindowOther,
		"monthly":        provider.WindowOther, // added upstream: kept, not dropped
		"SESSION":        provider.WindowOther,
		"session ":       provider.WindowOther,
		"сессия":         provider.WindowOther,
	} {
		if got := windowKind(wire); got != want {
			t.Errorf("windowKind(%q) = %q, want %q", wire, got, want)
		}
	}
}

// TestUsageClient_GetMatrix covers every status branch of the shared GET for
// both endpoints, plus failures before a status exists. The token must never
// appear in an error.
func TestUsageClient_GetMatrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		is      error
		want    string
	}{
		{name: "401", status: http.StatusUnauthorized, is: provider.ErrUnauthorized, want: "HTTP 401"},
		{name: "403", status: http.StatusForbidden, is: provider.ErrUnauthorized, want: "HTTP 403"},
		{name: "429 bare", status: http.StatusTooManyRequests, is: provider.ErrRateLimited},
		{name: "429 retry-after", status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "7"}, is: provider.ErrRateLimited, want: "retry after 7s"},
		{name: "500 snippet", status: http.StatusInternalServerError, body: "{\n  \"oops\": true\n}", want: `unexpected HTTP 500: { "oops": true }`},
		{name: "302", status: http.StatusFound, want: "unexpected HTTP 302"},
		{name: "200 garbage", status: http.StatusOK, body: "<html>", want: "decode"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range tc.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body)) // test server
		}))
		c := &UsageClient{Endpoint: srv.URL, ProfileEndpoint: srv.URL, HTTP: srv.Client()}
		_, uerr := c.Fetch(context.Background(), fakeAccessToken)
		_, perr := c.FetchProfile(context.Background(), fakeAccessToken)
		srv.Close()
		for which, err := range map[string]error{"usage": uerr, "profile": perr} {
			if err == nil {
				t.Errorf("%s/%s: no error", tc.name, which)
				continue
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("%s/%s: %v, want %v", tc.name, which, err, tc.is)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s/%s: %q, want %q", tc.name, which, err, tc.want)
			}
			if strings.Contains(err.Error(), fakeAccessToken) {
				t.Errorf("%s/%s: error leaks the token", tc.name, which)
			}
		}
	}
	for name, endpoint := range map[string]string{"bad url": "://bad", "closed": closedServerURL(t)} {
		c := &UsageClient{Endpoint: endpoint, ProfileEndpoint: endpoint}
		if _, err := c.Fetch(context.Background(), fakeAccessToken); err == nil || strings.Contains(err.Error(), fakeAccessToken) {
			t.Errorf("%s usage: %v", name, err)
		}
		if _, err := c.FetchProfile(context.Background(), fakeAccessToken); err == nil || strings.Contains(err.Error(), fakeAccessToken) {
			t.Errorf("%s profile: %v", name, err)
		}
	}
}

// closedServerURL returns the URL of a server that is no longer listening.
func closedServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

func TestParseProfile_Matrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, plan string
		err              error
	}{
		{"max prefix stripped", `{"organization":{"organization_type":"claude_max","rate_limit_tier":"default_claude_max_5x"}}`, "Max (5x)", nil},
		{"pro", `{"organization":{"organization_type":"claude_pro"}}`, "Pro", nil},
		{"no prefix", `{"organization":{"organization_type":"team"}}`, "Team", nil},
		{"empty org", `{"organization":{}}`, "", nil},
		{"null org", `{"organization":null}`, "", ErrNoOrganization},
		{"empty object", `{}`, "", ErrNoOrganization},
	} {
		sub, err := ParseProfile([]byte(tc.body))
		if tc.err != nil {
			if !errors.Is(err, tc.err) {
				t.Errorf("%s: err = %v, want %v", tc.name, err, tc.err)
			}
			continue
		}
		if err != nil || sub.Plan != tc.plan {
			t.Errorf("%s: plan = %q, %v; want %q", tc.name, sub.Plan, err, tc.plan)
		}
	}
	if _, err := ParseProfile(nil); err == nil {
		t.Error("empty body should fail to decode")
	}
}

// TestProvider_StateGuardsMatrix: Usage and Subscription share the same
// gates (api billing, logged out, expired at or before now, store failure)
// and none of them may reach the network.
func TestProvider_StateGuardsMatrix(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	userHome := t.TempDir()
	mk := func(name string, expires time.Time) string {
		home := filepath.Join(userHome, ".claude-"+name)
		writeFile(t, filepath.Join(home, credentialsFile), credJSON(fakeAccessToken, expires.UnixMilli()))
		return home
	}
	loggedOut := filepath.Join(userHome, ".claude-none")
	if err := os.MkdirAll(loggedOut, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		pr    provider.Profile
		store SecretStore
		is    error
		want  string
	}{
		{"api billed", provider.Profile{Home: mk("api", now.Add(time.Hour)), Billing: provider.BillingAPI}, nil, provider.ErrAPIBilled, ""},
		{"logged out", provider.Profile{Home: loggedOut}, nil, provider.ErrNotLoggedIn, ""},
		{"expires exactly now", provider.Profile{Home: mk("edge", now)}, nil, provider.ErrExpired, ""},
		{"expired", provider.Profile{Home: mk("old", now.Add(-time.Minute))}, nil, provider.ErrExpired, ""},
	} {
		p := &Provider{UserHome: userHome, Store: tc.store, Now: func() time.Time { return now },
			UsageClient: &UsageClient{Endpoint: srv.URL, ProfileEndpoint: srv.URL, HTTP: srv.Client()}}
		_, uerr := p.Usage(context.Background(), tc.pr)
		_, serr := p.Subscription(context.Background(), tc.pr)
		for which, err := range map[string]error{"usage": uerr, "subscription": serr} {
			if err == nil || (tc.is != nil && !errors.Is(err, tc.is)) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s/%s: err = %v, want %v %q", tc.name, which, err, tc.is, tc.want)
			}
		}
	}
	if hits != 0 {
		t.Errorf("guards reached the network %d times", hits)
	}
}

func TestProvider_Basics(t *testing.T) {
	t.Parallel()
	p := New("/u")
	if p.ID() != provider.Claude || p.Binary() != Binary || p.DefaultHome("/u") != filepath.Join("/u", defaultDirName) {
		t.Errorf("basics = %s %s %s", p.ID(), p.Binary(), p.DefaultHome("/u"))
	}
	if _, ok := p.Store.(Keychain); !ok || p.UsageClient == nil {
		t.Errorf("New = %+v, want the keychain store and a usage client", p)
	}
	if got := (&Provider{}).now(); time.Since(got) > time.Minute {
		t.Errorf("default clock = %v", got)
	}
}

// TestLaunch_HomeEnvMatrix: CLAUDE_CONFIG_DIR is absent for every spelling
// of the default home and set for every other home, whatever the shell had.
func TestLaunch_HomeEnvMatrix(t *testing.T) {
	t.Parallel()
	p := &Provider{UserHome: "/u"}
	stale := []string{"PATH=/bin", ConfigDirEnv + "=/u/.claude"}
	for _, tc := range []struct {
		home string
		want string // "" = must be absent
	}{
		{"/u/.claude", ""},
		{"/u/.claude/", ""},
		{"/u/./.claude", ""},
		{"/u/.claude-work", "/u/.claude-work"},
		{"/u/.claude-wörk", "/u/.claude-wörk"},
		{"/other/.claude", "/other/.claude"},
	} {
		cmd, err := p.Launch(provider.Profile{Home: tc.home}, provider.LaunchRequest{Env: stale})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, kv := range cmd.Env {
			if v, ok := strings.CutPrefix(kv, ConfigDirEnv+"="); ok {
				got = append(got, v)
			}
		}
		if tc.want == "" && len(got) != 0 {
			t.Errorf("home %q: %s must be absent, got %v", tc.home, ConfigDirEnv, got)
		}
		if tc.want != "" && (len(got) != 1 || got[0] != tc.want) {
			t.Errorf("home %q: %s = %v, want [%s]", tc.home, ConfigDirEnv, got, tc.want)
		}
	}
}
