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

	"github.com/khanakia/agx/provider"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const chatgptAuth = `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"access_token":"tok-1","account_id":"acc-1","refresh_token":"r","id_token":"i"}}`

func TestParseUsage(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/usage.json")
	if err != nil {
		t.Fatal(err)
	}
	u, err := ParseUsage(body)
	if err != nil {
		t.Fatal(err)
	}
	if u.Identity.Email != "me@example.com" || u.Identity.Plan != "Plus" {
		t.Errorf("identity = %+v", u.Identity)
	}
	if len(u.Windows) != 2 {
		t.Fatalf("windows = %+v", u.Windows)
	}
	w0, w1 := u.Windows[0], u.Windows[1]
	if w0.Label != labelFiveHour || w0.Kind != provider.WindowSession || w0.Percent != 42 || w0.ResetsAt == nil || w0.ResetsAt.Unix() != 1789000000 {
		t.Errorf("primary = %+v", w0)
	}
	if w1.Label != labelWeekly || w1.Kind != provider.WindowWeekly || w1.Group != provider.GroupPlan {
		t.Errorf("secondary = %+v", w1)
	}
}

func TestParseUsage_Shapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		wantErr    error
		check      func(provider.Usage) bool
	}{
		{"thirty day free plan", `{"plan_type":"free","rate_limit":{"primary_window":{"used_percent":9,"limit_window_seconds":2592000}}}`, nil,
			func(u provider.Usage) bool {
				return u.Windows[0].Label == labelThirtyDay && u.Windows[0].ResetsAt == nil
			}},
		{"odd length labelled in days", `{"rate_limit":{"primary_window":{"used_percent":1,"limit_window_seconds":1209600}}}`, nil,
			func(u provider.Usage) bool { return u.Windows[0].Label == "14-day" }},
		{"limit reached marks exhausted", `{"rate_limit":{"limit_reached":true,"primary_window":{"used_percent":100,"limit_window_seconds":18000}}}`, nil,
			func(u provider.Usage) bool { return u.Windows[0].Severity == provider.SeverityExhausted }},
		{"no windows", `{"rate_limit":{"primary_window":null,"secondary_window":null}}`, ErrNoWindows, nil},
		{"no rate_limit", `{}`, ErrNoWindows, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, err := ParseUsage([]byte(tc.body))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || !tc.check(u) {
				t.Errorf("usage = %+v, err %v", u, err)
			}
		})
	}
	if _, err := ParseUsage([]byte("<html>")); err == nil {
		t.Error("garbage should fail")
	}
}

func TestUsage_HTTP(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/usage.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		status    int
		wantUnaut bool
		wantErr   bool
	}{
		{"ok", http.StatusOK, false, false},
		{"401", http.StatusUnauthorized, true, true},
		{"500", http.StatusInternalServerError, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer tok-1" || r.Header.Get(accountIDHeader) != "acc-1" {
					t.Errorf("headers = %v", r.Header)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write(body) // test server
			}))
			defer srv.Close()
			home := t.TempDir()
			writeFile(t, filepath.Join(home, authFile), chatgptAuth)
			p := &Provider{Endpoint: srv.URL, HTTP: srv.Client()}
			u, err := p.Usage(context.Background(), provider.Profile{Home: home})
			if (err != nil) != tc.wantErr || errors.Is(err, provider.ErrUnauthorized) != tc.wantUnaut {
				t.Fatalf("err = %v", err)
			}
			if !tc.wantErr && (len(u.Windows) != 2 || !strings.HasSuffix(u.Identity.CredentialSource, authFile)) {
				t.Errorf("usage = %+v", u)
			}
		})
	}
}

func TestUsage_LoginStates(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	if _, err := p.Usage(context.Background(), provider.Profile{Home: t.TempDir()}); !errors.Is(err, provider.ErrNotLoggedIn) {
		t.Errorf("no auth.json: %v", err)
	}
	apiHome := t.TempDir()
	writeFile(t, filepath.Join(apiHome, authFile), `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-x","tokens":null}`)
	if _, err := p.Usage(context.Background(), provider.Profile{Home: apiHome}); !errors.Is(err, provider.ErrAPIBilled) {
		t.Errorf("api key login: %v", err)
	}
	if _, err := p.Usage(context.Background(), provider.Profile{Home: apiHome, Billing: provider.BillingAPI}); !errors.Is(err, provider.ErrAPIBilled) {
		t.Errorf("api billing: %v", err)
	}
}

func TestDiscoverAndIdentity(t *testing.T) {
	t.Parallel()
	userHome := t.TempDir()
	p := &Provider{}
	if ps, err := p.Discover(userHome); err != nil || len(ps) != 0 {
		t.Errorf("no ~/.codex: %v %v", ps, err)
	}
	writeFile(t, filepath.Join(userHome, defaultDirName, authFile), chatgptAuth)
	ps, err := p.Discover(userHome)
	if err != nil || len(ps) != 1 || ps[0].Name != defaultProfileName || ps[0].Billing != provider.BillingPlan || !ps[0].Default {
		t.Fatalf("Discover = %+v, %v", ps, err)
	}
	id, err := p.Identity(ps[0])
	if err != nil || !id.LoggedIn {
		t.Errorf("Identity = %+v, %v", id, err)
	}
}

func TestLaunch(t *testing.T) {
	t.Parallel()
	p := &Provider{UserHome: "/u"}
	cmd, err := p.Launch(provider.Profile{Home: "/u/.codex-work", Args: []string{"--full-auto"}},
		provider.LaunchRequest{ResumeID: "id-9", Env: []string{"PATH=/bin"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cmd.Args, " ") != "resume --full-auto id-9" || !slices.Contains(cmd.Env, HomeEnv+"=/u/.codex-work") {
		t.Errorf("cmd = %+v", cmd)
	}
	cmd, _ = p.Launch(provider.Profile{Home: "/u/.codex"}, provider.LaunchRequest{Env: []string{HomeEnv + "=/x"}})
	if len(cmd.Args) != 0 || slices.Contains(cmd.Env, HomeEnv+"=/x") {
		t.Errorf("default home cmd = %+v", cmd)
	}
}

func TestConversationsAndHistory(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	meta := func(id, cwd string) string {
		return `{"timestamp":"2026-09-13T01:10:18Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `"}}` + "\n{}\n"
	}
	writeFile(t, filepath.Join(home, sessionsDir, "2026", "09", "13", "rollout-a.jsonl"), meta("id-a", "/w/a"))
	writeFile(t, filepath.Join(home, sessionsDir, "2026", "09", "14", "rollout-b.jsonl"), meta("id-b", "/w/b"))
	writeFile(t, filepath.Join(home, sessionsDir, "2026", "09", "14", "rollout-bad.jsonl"), `{"type":"other"}`)
	writeFile(t, filepath.Join(home, sessionsDir, "notes.txt"), "ignored")

	p := &Provider{}
	all, err := p.Conversations(context.Background(), home, provider.ConversationQuery{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %+v, %v", all, err)
	}
	scoped, _ := p.Conversations(context.Background(), home, provider.ConversationQuery{Dir: "/w/b"})
	if len(scoped) != 1 || scoped[0].ID != "id-b" || scoped[0].Provider != provider.Codex {
		t.Errorf("scoped = %+v", scoped)
	}
	if !p.HasHistory(home, "/w/a") || p.HasHistory(home, "/w/zzz") {
		t.Error("HasHistory wrong")
	}
	if none, err := p.Conversations(context.Background(), t.TempDir(), provider.ConversationQuery{}); err != nil || len(none) != 0 {
		t.Errorf("empty home = %v, %v", none, err)
	}
}
