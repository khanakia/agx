package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

func TestParseProfile(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/profile.json")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := ParseProfile(body)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Plan != "Max (20x)" || sub.Status != "active" || sub.Billing != "stripe_subscription" {
		t.Errorf("sub = %+v", sub)
	}
	if sub.StartedAt == nil || sub.StartedAt.Format(time.DateOnly) != "2025-11-25" {
		t.Errorf("started = %v", sub.StartedAt)
	}
	if _, err := ParseProfile([]byte(`{"account":{}}`)); !errors.Is(err, ErrNoOrganization) {
		t.Errorf("no org: %v", err)
	}
	if _, err := ParseProfile([]byte("<html>")); err == nil {
		t.Error("garbage should fail")
	}
}

func TestProviderSubscription(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/profile.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{"ok", http.StatusOK, nil},
		{"401", http.StatusUnauthorized, provider.ErrUnauthorized},
		{"429", http.StatusTooManyRequests, provider.ErrRateLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(oauthBetaHeader) != oauthBetaValue || r.Header.Get("Authorization") != "Bearer tok" {
					t.Errorf("headers = %v", r.Header)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write(body) // test server
			}))
			defer srv.Close()
			now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
			home := t.TempDir()
			writeFile(t, filepath.Join(home, ".claude-work", credentialsFile), credJSON("tok", now.Add(time.Hour).UnixMilli()))
			p := &Provider{UserHome: home, UsageClient: &UsageClient{ProfileEndpoint: srv.URL, HTTP: srv.Client()}, Now: func() time.Time { return now }}
			sub, err := p.Subscription(context.Background(), provider.Profile{Home: filepath.Join(home, ".claude-work")})
			if tc.want == nil && (err != nil || sub.Status != "active") {
				t.Errorf("sub = %+v, %v", sub, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
	p := &Provider{UserHome: t.TempDir()}
	if _, err := p.Subscription(context.Background(), provider.Profile{Home: t.TempDir()}); !errors.Is(err, provider.ErrNotLoggedIn) {
		t.Errorf("logged out: %v", err)
	}
	if _, err := p.Subscription(context.Background(), provider.Profile{Billing: provider.BillingAPI}); !errors.Is(err, provider.ErrAPIBilled) {
		t.Errorf("api billed: %v", err)
	}
}
