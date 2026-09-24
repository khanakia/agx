package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanakia/claude-usage/internal/account"
	"github.com/khanakia/claude-usage/internal/usage"
)

// stubFetcher returns a canned result per token. collect calls it from one
// goroutine per account, so the call log is mutex-guarded.
type stubFetcher struct {
	byToken map[string]usage.Usage
	errs    map[string]error
	mu      sync.Mutex
	calls   []string
}

func (s *stubFetcher) Fetch(_ context.Context, tok string) (usage.Usage, error) {
	s.mu.Lock()
	s.calls = append(s.calls, tok)
	s.mu.Unlock()
	if err, ok := s.errs[tok]; ok {
		return usage.Usage{}, err
	}
	return s.byToken[tok], nil
}

// noKeychain is a SecretStore with no entries.
type noKeychain struct{}

func (noKeychain) Get(service string) ([]byte, error) {
	return nil, fmt.Errorf("%w: %s", account.ErrSecretNotFound, service)
}

func writeCred(t *testing.T, dir, token string, expires time.Time) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d,"subscriptionType":"max","rateLimitTier":"default_claude_max_5x"}}`, token, expires.UnixMilli())
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCollect(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	okDir := filepath.Join(home, ".claude")
	expiredDir := filepath.Join(home, ".claude-old")
	rejectedDir := filepath.Join(home, ".claude-work")
	loggedOutDir := filepath.Join(home, ".claude-none")
	writeCred(t, okDir, "ok", now.Add(time.Hour))
	writeCred(t, expiredDir, "old", now.Add(-time.Hour))
	writeCred(t, rejectedDir, "bad", now.Add(time.Hour))
	if err := os.MkdirAll(loggedOutDir, 0o700); err != nil {
		t.Fatal(err)
	}

	f := &stubFetcher{
		byToken: map[string]usage.Usage{"ok": {Limits: []usage.Limit{{Kind: usage.KindSession, Percent: 5}}}},
		errs:    map[string]error{"bad": fmt.Errorf("%w (HTTP 401)", usage.ErrUnauthorized)},
	}
	deps := collectDeps{home: home, store: noKeychain{}, client: f, now: func() time.Time { return now }}
	rs := collect(context.Background(), []string{okDir, expiredDir, rejectedDir, loggedOutDir}, time.Second, deps)

	if len(rs) != 4 {
		t.Fatalf("got %d reports", len(rs))
	}
	if rs[0].Err != nil || rs[0].Plan != "Max (5x)" || len(rs[0].Usage.Limits) != 1 || rs[0].ConfigDir != okDir {
		t.Errorf("ok report = %+v", rs[0])
	}
	if rs[1].Err == nil || !strings.Contains(rs[1].Err.Error(), "token expired") || !strings.Contains(rs[1].Err.Error(), "CLAUDE_CONFIG_DIR="+expiredDir) {
		t.Errorf("expired report err = %v", rs[1].Err)
	}
	if !errors.Is(rs[2].Err, usage.ErrUnauthorized) || !strings.Contains(rs[2].Err.Error(), "refresh") {
		t.Errorf("rejected report err = %v", rs[2].Err)
	}
	if !errors.Is(rs[3].Err, account.ErrNoCredential) {
		t.Errorf("logged-out report err = %v", rs[3].Err)
	}
	for _, tok := range f.calls {
		if tok == "old" {
			t.Error("expired token was sent to the server")
		}
	}
}

func TestRefreshHint(t *testing.T) {
	t.Parallel()
	if got := refreshHint("/h/.claude", true); !strings.Contains(got, "`claude`") {
		t.Errorf("default hint = %q", got)
	}
	if got := refreshHint("/h/.claude-work", false); !strings.Contains(got, "CLAUDE_CONFIG_DIR=/h/.claude-work claude") {
		t.Errorf("custom hint = %q", got)
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()
	reports := []usage.Report{
		{ConfigDir: "/a", Email: "a@x", Plan: "Max (20x)", Usage: usage.Usage{
			Limits:        []usage.Limit{{Kind: usage.KindWeeklyAll, Group: usage.GroupWeekly, Percent: 76, Severity: usage.SeverityWarning}},
			ExtraUsageRaw: json.RawMessage(`{"is_enabled":false}`),
		}},
		{ConfigDir: "/b", Err: errors.New("nope")},
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, reports); err != nil {
		t.Fatal(err)
	}
	// Decode into a struct spelling out the public --json contract, so a
	// renamed field fails here rather than silently in someone's jq script.
	var got []struct {
		ConfigDir  string          `json:"config_dir"`
		Email      string          `json:"email"`
		Plan       string          `json:"plan"`
		ExtraUsage json.RawMessage `json:"extra_usage"`
		Error      string          `json:"error"`
		Limits     []struct {
			Label    string  `json:"label"`
			Kind     string  `json:"kind"`
			Percent  float64 `json:"percent"`
			Severity string  `json:"severity"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries", len(got))
	}
	a := got[0]
	if a.ConfigDir != "/a" || a.Email != "a@x" || a.Plan != "Max (20x)" || len(a.ExtraUsage) == 0 || a.Error != "" {
		t.Errorf("account fields = %+v", a)
	}
	if len(a.Limits) != 1 || a.Limits[0].Label != "All models" || a.Limits[0].Kind != "weekly_all" || a.Limits[0].Percent != 76 || a.Limits[0].Severity != "warning" {
		t.Errorf("limits = %+v", a.Limits)
	}
	if got[1].Error != "nope" || got[1].Limits != nil {
		t.Errorf("error entry = %+v", got[1])
	}
	if strings.Contains(buf.String(), `"limits": null`) {
		t.Error("error entries must omit limits, not emit null")
	}
}

func TestParseFlags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr bool
		check   func(config) bool
	}{
		{"defaults", nil, false, func(c config) bool {
			return !c.jsonOut && c.color == colorAuto && c.timeout == usage.DefaultTimeout && len(c.dirs) == 0
		}},
		{"json + dirs", []string{"--json", "a", "b"}, false, func(c config) bool { return c.jsonOut && len(c.dirs) == 2 }},
		{"bad color", []string{"--color=rainbow"}, true, nil},
		{"zero timeout", []string{"--timeout=0s"}, true, nil},
		{"unknown flag", []string{"--nope"}, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			c, err := parseFlags(tc.args, &stderr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.check != nil && !tc.check(c) {
				t.Errorf("config = %+v", c)
			}
		})
	}
}

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run([]string{"--help"}, &out, &errOut); code != exitOK || !strings.Contains(errOut.String(), "Examples:") {
		t.Errorf("--help: code %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"/definitely/not/a/dir"}, &out, &errOut); code != exitUsage {
		t.Errorf("missing dir: code %d", code)
	}
	if code := run([]string{"--color=x"}, &out, &errOut); code != exitUsage {
		t.Errorf("bad flag: code %d", code)
	}
}

func TestResolveDirs_Explicit(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	got, err := resolveDirs([]string{d + "/"}, "/unused")
	if err != nil || len(got) != 1 || got[0] != filepath.Clean(d) {
		t.Errorf("resolveDirs = %v, %v", got, err)
	}
}

func TestWantColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if !wantColor(colorAlways, &buf) || wantColor(colorNever, &buf) || wantColor(colorAuto, &buf) {
		t.Error("wantColor: always/never/auto-on-buffer wrong")
	}
}

func TestResolveVersion(t *testing.T) {
	t.Parallel()
	withMod := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{Main: debug.Module{Version: v}}, true }
	}
	noInfo := func() (*debug.BuildInfo, bool) { return nil, false }
	for _, tc := range []struct {
		name, stamped string
		info          func() (*debug.BuildInfo, bool)
		want          string
	}{
		{"stamped wins", "v1.0.0", withMod("v0.9.0"), "v1.0.0"},
		{"go install version", "", withMod("v0.2.0"), "v0.2.0"},
		{"local build", "", withMod(develVersion), develVersion},
		{"no build info", "", noInfo, develVersion},
		{"empty module version", "", withMod(""), develVersion},
	} {
		if got := resolveVersion(tc.stamped, tc.info); got != tc.want {
			t.Errorf("%s: resolveVersion = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRun_Version(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != exitOK || !strings.HasPrefix(out.String(), "claude-usage ") {
		t.Errorf("--version: code %d, stdout %q", code, out.String())
	}
}
