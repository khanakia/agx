// Command usage-fake-server reads Claude Code plan usage the way agx does,
// against a local httptest server instead of the real API: once through the
// usage endpoint (a home's stored login), once through the probe (a raw
// token login), and once more to show how a 429 and a rejected token come
// back as provider sentinel errors.
//
// It writes a throwaway Claude Code home with a fake token and never touches
// the network, the keychain or a real account.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// Fake values: the server accepts exactly these, so a wrong token is
// rejected the way the real endpoint rejects one.
const (
	fakeHomeToken  = "fake-home-access-token"
	fakeLoginToken = "fake-setup-token"
	// homeDirName is a non-default home, discovered by agx as profile "work".
	homeDirName = ".claude-work"
	// credentialPerm matches what Claude Code writes for its credentials file.
	credentialPerm os.FileMode = 0o600
	// homePerm keeps the fake home private, like a real one.
	homePerm os.FileMode = 0o700
	// tokenLifetime keeps the fake login unexpired for the whole run.
	tokenLifetime = time.Hour
)

// Paths served by the fake server; the real ones live on api.anthropic.com.
const (
	usagePath   = "/api/oauth/usage"
	probePath   = "/v1/messages"
	limitedPath = "/limited"
	// retryAfterSeconds is what the fake 429 asks callers to wait.
	retryAfterSeconds = "30"
)

// usageBody is a trimmed copy of a real GET /api/oauth/usage response.
const usageBody = `{
  "limits": [
    {"kind": "session", "group": "session", "percent": 5, "severity": "normal",
     "resets_at": "2026-09-24T09:29:59Z", "scope": null, "is_active": false},
    {"kind": "weekly_all", "group": "weekly", "percent": 76, "severity": "warning",
     "resets_at": "2026-09-24T17:59:59Z", "scope": null, "is_active": true},
    {"kind": "weekly_scoped", "group": "weekly", "percent": 68, "severity": "normal",
     "resets_at": "2026-09-24T17:59:59Z",
     "scope": {"model": {"id": null, "display_name": "Fable"}, "surface": null},
     "is_active": false}
  ],
  "extra_usage": {"is_enabled": false, "utilization": null}
}`

// Probe headers: utilization is a 0..1 fraction, reset is unix seconds.
var probeHeaders = map[string]string{
	"anthropic-ratelimit-unified-5h-utilization":       "0.12",
	"anthropic-ratelimit-unified-5h-reset":             "1790242199",
	"anthropic-ratelimit-unified-5h-status":            "allowed",
	"anthropic-ratelimit-unified-7d-utilization":       "0.81",
	"anthropic-ratelimit-unified-7d-reset":             "1790704799",
	"anthropic-ratelimit-unified-7d-status":            "allowed_warning",
	"anthropic-ratelimit-unified-representative-claim": "seven_day",
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	srv := httptest.NewServer(fakeAPI())
	defer srv.Close()

	userHome, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	home := filepath.Join(userHome, homeDirName)
	if err := writeCredential(home, fakeHomeToken, time.Now().Add(tokenLifetime)); err != nil {
		return err
	}

	// Store: nil disables the macOS keychain, so only the credentials file
	// in the fake home is read. Endpoint and Probe point at the fake server.
	p := &claude.Provider{
		UserHome:    userHome,
		Store:       nil,
		UsageClient: &claude.UsageClient{Endpoint: srv.URL + usagePath},
		Probe:       &claude.ProbeClient{Endpoint: srv.URL + probePath},
	}
	ctx := context.Background()

	profile := provider.Profile{Name: "work", Provider: provider.Claude, Home: home, Billing: provider.BillingPlan}
	u, err := p.Usage(ctx, profile)
	if err != nil {
		return fmt.Errorf("usage: %w", err)
	}
	fmt.Printf("home login: logged in=%v plan=%q\n", u.Identity.LoggedIn, u.Identity.Plan)
	printWindows(u)

	probed, _, err := p.LoginUsage(ctx, provider.LoginOAuthToken, fakeLoginToken)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	fmt.Println("token login (probe):")
	printWindows(probed)

	limited := &claude.UsageClient{Endpoint: srv.URL + limitedPath}
	_, err = limited.Fetch(ctx, fakeHomeToken)
	fmt.Printf("429: %v (rate-limited=%v)\n", err, errors.Is(err, provider.ErrRateLimited))

	_, err = p.UsageClient.Fetch(ctx, "wrong-token")
	fmt.Printf("bad token: %v (unauthorized=%v)\n", err, errors.Is(err, provider.ErrUnauthorized))
	return nil
}

// fakeAPI answers the three paths the example calls.
func fakeAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(usagePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeHomeToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(usageBody))
	})
	mux.HandleFunc(probePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeLoginToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		for k, v := range probeHeaders {
			w.Header().Set(k, v)
		}
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc(limitedPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", retryAfterSeconds)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	return mux
}

// writeCredential writes the .credentials.json Claude Code keeps in a home
// when it stores the login on disk.
func writeCredential(home, token string, expires time.Time) error {
	if err := os.MkdirAll(home, homePerm); err != nil {
		return err
	}
	body := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d,"subscriptionType":"max","rateLimitTier":"default_claude_max_20x"}}`,
		token, expires.UnixMilli())
	return os.WriteFile(filepath.Join(home, ".credentials.json"), []byte(body), credentialPerm)
}

// printWindows prints one line per window plus the figure auto-pick uses.
func printWindows(u provider.Usage) {
	for _, w := range u.Windows {
		reset := "no reset"
		if w.ResetsAt != nil {
			reset = "resets " + w.ResetsAt.UTC().Format(time.RFC3339)
		}
		active := ""
		if w.Active {
			active = " (binding)"
		}
		fmt.Printf("  %-7s %-16s %5.1f%% %-8s %s%s\n", w.Group, w.Label, w.Percent, w.Severity, reset, active)
	}
	fmt.Printf("  max: %.0f%%\n", u.MaxPercent())
}
