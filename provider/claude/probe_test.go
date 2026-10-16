package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

// probeToken is obviously fake; no error may ever contain it.
const probeToken = "sk-ant-oat01-FAKE-probe-9999"

// realHeaders is the header set the real API sent on 2026-10-16.
func realHeaders() http.Header {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "allowed")
	h.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	h.Set("anthropic-ratelimit-unified-5h-utilization", "0.05")
	h.Set("anthropic-ratelimit-unified-5h-reset", "1792188600")
	h.Set("anthropic-ratelimit-unified-7d-status", "allowed")
	h.Set("anthropic-ratelimit-unified-7d-utilization", "0.05")
	h.Set("anthropic-ratelimit-unified-7d-reset", "1792692000")
	h.Set("anthropic-ratelimit-unified-representative-claim", "five_hour")
	return h
}

func TestParseProbeHeaders_Real(t *testing.T) {
	t.Parallel()
	u, err := ParseProbeHeaders(realHeaders())
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Windows) != 2 {
		t.Fatalf("windows = %d, want 2", len(u.Windows))
	}
	s, w := u.Windows[0], u.Windows[1]
	if s.Kind != provider.WindowSession || s.Group != provider.GroupSession || s.Label != labelSession || s.VendorKind != kindSession {
		t.Errorf("5h window = %+v", s)
	}
	if w.Kind != provider.WindowWeekly || w.Group != provider.GroupWeekly || w.Label != labelWeeklyAll || w.VendorKind != kindWeeklyAll {
		t.Errorf("7d window = %+v", w)
	}
	if s.Percent != 5 || w.Percent != 5 {
		t.Errorf("percent = %v / %v, want 5 / 5 (utilization × 100)", s.Percent, w.Percent)
	}
	if s.ResetsAt == nil || !s.ResetsAt.Equal(time.Unix(1792188600, 0)) || w.ResetsAt == nil || !w.ResetsAt.Equal(time.Unix(1792692000, 0)) {
		t.Errorf("resets = %v / %v", s.ResetsAt, w.ResetsAt)
	}
	if !s.Active || w.Active {
		t.Errorf("active = %v / %v, want the five_hour claim on the 5h window only", s.Active, w.Active)
	}
	if s.Severity != provider.SeverityNormal {
		t.Errorf("severity = %q", s.Severity)
	}
	if u.MaxPercent() != 5 {
		t.Errorf("MaxPercent = %v", u.MaxPercent())
	}
}

// TestParseProbeHeaders_Matrix: every required header missing, malformed
// values, and the optional ones (status, claim) in each state.
func TestParseProbeHeaders_Matrix(t *testing.T) {
	t.Parallel()
	const pre = "anthropic-ratelimit-unified-"
	for _, c := range []struct {
		name  string
		edit  func(h http.Header)
		check func(t *testing.T, u provider.Usage)
		err   string // "" = success
	}{
		{name: "missing 5h utilization", edit: func(h http.Header) { h.Del(pre + "5h-utilization") }, err: pre + "5h-utilization missing"},
		{name: "missing 5h reset", edit: func(h http.Header) { h.Del(pre + "5h-reset") }, err: pre + "5h-reset missing"},
		{name: "missing 7d utilization", edit: func(h http.Header) { h.Del(pre + "7d-utilization") }, err: pre + "7d-utilization missing"},
		{name: "missing 7d reset", edit: func(h http.Header) { h.Del(pre + "7d-reset") }, err: pre + "7d-reset missing"},
		{name: "no headers at all", edit: func(h http.Header) {
			for k := range h {
				delete(h, k)
			}
		}, err: "missing"},
		{name: "utilization not a number", edit: func(h http.Header) { h.Set(pre+"5h-utilization", "five") }, err: `"five" is not a fraction`},
		{name: "utilization negative", edit: func(h http.Header) { h.Set(pre+"7d-utilization", "-0.1") }, err: "is not a fraction"},
		{name: "utilization percent-looking", edit: func(h http.Header) { h.Set(pre+"5h-utilization", "5%") }, err: "is not a fraction"},
		{name: "reset not a number", edit: func(h http.Header) { h.Set(pre+"5h-reset", "tomorrow") }, err: "is not a unix time"},
		{name: "reset float", edit: func(h http.Header) { h.Set(pre+"7d-reset", "1792692000.5") }, err: "is not a unix time"},
		{name: "reset zero", edit: func(h http.Header) { h.Set(pre+"5h-reset", "0") }, err: "is not a unix time"},
		{name: "utilization zero is real data", edit: func(h http.Header) { h.Set(pre+"5h-utilization", "0") }, check: func(t *testing.T, u provider.Usage) {
			if u.Windows[0].Percent != 0 {
				t.Errorf("percent = %v", u.Windows[0].Percent)
			}
		}},
		{name: "over the cap", edit: func(h http.Header) {
			h.Set(pre+"7d-utilization", "1.02")
			h.Set(pre+"7d-status", "rejected")
			h.Set(pre+"representative-claim", "seven_day")
		}, check: func(t *testing.T, u provider.Usage) {
			w := u.Windows[1]
			if w.Percent != 102 || w.Severity != provider.SeverityExhausted || !w.Active || u.Windows[0].Active {
				t.Errorf("7d = %+v", w)
			}
		}},
		{name: "warning status", edit: func(h http.Header) { h.Set(pre+"5h-status", "allowed_warning") }, check: func(t *testing.T, u provider.Usage) {
			if u.Windows[0].Severity != provider.SeverityWarning {
				t.Errorf("severity = %q", u.Windows[0].Severity)
			}
		}},
		{name: "unknown status is exhausted", edit: func(h http.Header) { h.Set(pre+"5h-status", "throttled") }, check: func(t *testing.T, u provider.Usage) {
			if u.Windows[0].Severity != provider.SeverityExhausted {
				t.Errorf("severity = %q", u.Windows[0].Severity)
			}
		}},
		{name: "no status, no claim", edit: func(h http.Header) {
			h.Del(pre + "5h-status")
			h.Del(pre + "7d-status")
			h.Del(pre + "representative-claim")
		}, check: func(t *testing.T, u provider.Usage) {
			for _, w := range u.Windows {
				if w.Severity != "" || w.Active {
					t.Errorf("window = %+v, want no severity, not active", w)
				}
			}
		}},
	} {
		h := realHeaders()
		c.edit(h)
		u, err := ParseProbeHeaders(h)
		switch {
		case c.err != "" && err == nil:
			t.Errorf("%s: accepted: %+v", c.name, u)
		case c.err != "" && (!errors.Is(err, ErrProbeHeaders) || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s: err = %v, want ErrProbeHeaders with %q", c.name, err, c.err)
		case c.err != "" && len(u.Windows) != 0:
			t.Errorf("%s: an error came with windows (silent zero)", c.name)
		case c.err == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.err == "":
			if len(u.Windows) != 2 {
				t.Fatalf("%s: windows = %d", c.name, len(u.Windows))
			}
			c.check(t, u)
		}
	}
}

// TestProbe_Request pins what the probe sends: one POST with the token as a
// bearer, the OAuth beta and API version headers, and the minimal body.
func TestProbe_Request(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		for k, want := range map[string]string{
			"Authorization": "Bearer " + probeToken, "anthropic-beta": "oauth-2025-04-20",
			"anthropic-version": "2023-06-01", "Content-Type": "application/json",
		} {
			if got := r.Header.Get(k); got != want {
				t.Errorf("header %s = %q, want %q", k, got, want)
			}
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			System    string `json:"system"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body %s: %v", raw, err)
		}
		if body.Model != DefaultProbeModel || body.MaxTokens != 1 || body.System != probeSystem ||
			len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != "hi" {
			t.Errorf("body = %s", raw)
		}
		for k, v := range realHeaders() {
			w.Header()[k] = v
		}
		_, _ = io.WriteString(w, `{"type":"message","content":[{"type":"text","text":"Hi"}]}`)
	}))
	defer srv.Close()
	p := &Provider{Probe: &ProbeClient{Endpoint: srv.URL}}
	u, id, err := p.LoginUsage(context.Background(), provider.LoginOAuthToken, probeToken)
	if err != nil {
		t.Fatal(err)
	}
	if !id.LoggedIn || !u.Identity.LoggedIn || id.Email != "" || len(u.Windows) != 2 {
		t.Errorf("usage = %+v, identity = %+v", u, id)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want exactly one", calls.Load())
	}

	// Model override.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"model":"claude-other"`) {
			t.Errorf("model override not sent: %s", raw)
		}
		for k, v := range realHeaders() {
			w.Header()[k] = v
		}
	}))
	defer srv2.Close()
	if _, err := (&ProbeClient{Endpoint: srv2.URL, Model: "claude-other"}).Probe(context.Background(), probeToken); err != nil {
		t.Fatal(err)
	}
}

// TestProbe_Errors: every failing status, with and without an error body,
// including bodies that repeat the token; none may leak it.
func TestProbe_Errors(t *testing.T) {
	t.Parallel()
	apiErr := func(msg string) string {
		return `{"type":"error","error":{"type":"permission_error","message":"` + msg + `"}}`
	}
	for _, c := range []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		is      error
		want    string
	}{
		{name: "401 no body", status: 401, is: provider.ErrUnauthorized, want: "(HTTP 401)"},
		{name: "401 api error", status: 401, body: apiErr("invalid bearer token"), is: provider.ErrUnauthorized, want: "(HTTP 401: invalid bearer token)"},
		{name: "403 no body", status: 403, is: provider.ErrUnauthorized, want: "(HTTP 403)"},
		{name: "403 api error", status: 403, body: apiErr("This credential is only authorized for use with Claude Code"), is: provider.ErrUnauthorized, want: "only authorized for use with Claude Code"},
		{name: "403 echoes token", status: 403, body: apiErr("token " + probeToken + " revoked"), is: provider.ErrUnauthorized, want: "token <redacted> revoked"},
		{name: "403 html body", status: 403, body: "<html>forbidden</html>", is: provider.ErrUnauthorized, want: "(HTTP 403)"},
		{name: "429", status: 429, is: provider.ErrRateLimited},
		{name: "429 retry-after", status: 429, headers: map[string]string{"Retry-After": "42"}, is: provider.ErrRateLimited, want: "retry after 42s"},
		{name: "500 echoes token", status: 500, body: "boom " + probeToken, want: "unexpected HTTP 500: boom <redacted>"},
		{name: "500 token at the snippet edge", status: 500, body: strings.Repeat("x", 190) + " " + probeToken, want: "unexpected HTTP 500"},
		{name: "529 overloaded", status: 529, body: apiErr("Overloaded"), want: "unexpected HTTP 529"},
		{name: "200 without headers", status: 200, body: `{"type":"message"}`, is: ErrProbeHeaders},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range c.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(c.status)
			_, _ = io.WriteString(w, c.body)
		}))
		_, err := (&ProbeClient{Endpoint: srv.URL}).Probe(context.Background(), probeToken)
		srv.Close()
		switch {
		case err == nil:
			t.Errorf("%s: no error", c.name)
			continue
		case c.is != nil && !errors.Is(err, c.is):
			t.Errorf("%s: %v is not %v", c.name, err, c.is)
		case !strings.Contains(err.Error(), c.want):
			t.Errorf("%s: %v lacks %q", c.name, err, c.want)
		}
		if strings.Contains(err.Error(), probeToken) || strings.Contains(err.Error(), "FAKE-probe") {
			t.Errorf("%s: error leaks (part of) the token: %v", c.name, err)
		}
	}

	// Transport failure and a cancelled context: no token either.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	if _, err := (&ProbeClient{Endpoint: url}).Probe(context.Background(), probeToken); err == nil || strings.Contains(err.Error(), probeToken) {
		t.Errorf("closed server: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&ProbeClient{Endpoint: url}).Probe(ctx, probeToken); !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), probeToken) {
		t.Errorf("cancelled: %v", err)
	}
}

// TestLoginUsage_APIKeyNeverProbes: an api-key login has no plan windows;
// it is reported billed per token without a request.
func TestLoginUsage_APIKeyNeverProbes(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer srv.Close()
	p := &Provider{Probe: &ProbeClient{Endpoint: srv.URL}}
	for _, k := range []provider.LoginKind{provider.LoginAPIKey, "unknown"} {
		if _, _, err := p.LoginUsage(context.Background(), k, probeToken); !errors.Is(err, provider.ErrAPIBilled) {
			t.Errorf("%s: %v", k, err)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("api-key login probed %d times", calls.Load())
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()
	if got := redact("a "+probeToken+" b "+probeToken, probeToken); got != "a <redacted> b <redacted>" {
		t.Errorf("redact = %q", got)
	}
	if got := redact("abc", ""); got != "abc" {
		t.Errorf("empty token = %q", got)
	}
}
