package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// TestParseProbeHeaders_NonFiniteUtilization: strconv.ParseFloat accepts
// "NaN" and "Inf", which are not fractions. A NaN window would compare false
// against every threshold (pick would treat a garbled account as having
// headroom), so they must be refused like any other malformed value.
func TestParseProbeHeaders_NonFiniteUtilization(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "infinity", "1e400"} {
		for _, win := range []string{probeWindow5h, probeWindow7d} {
			h := realHeaders()
			h.Set(hdrPrefix+win+hdrUtilization, v)
			if u, err := ParseProbeHeaders(h); !errors.Is(err, ErrProbeHeaders) {
				t.Errorf("%s=%q: got %+v, %v; want ErrProbeHeaders", win, v, u.Windows, err)
			}
		}
	}
}

// TestParseProbeHeaders_ClaimMatrix: the representative claim marks exactly
// the binding window active; anything else marks none.
func TestParseProbeHeaders_ClaimMatrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		claim            string
		active5, active7 bool
	}{
		{claim5h, true, false},
		{claim7d, false, true},
		{"", false, false},
		{"FIVE_HOUR", false, false},
		{"seven_day_opus", false, false},
		{"five_hour ", false, false},
	} {
		h := realHeaders()
		h.Set(hdrRepresentative, tc.claim)
		u, err := ParseProbeHeaders(h)
		if err != nil {
			t.Fatalf("claim %q: %v", tc.claim, err)
		}
		if len(u.Windows) != len(probeWindows) {
			t.Fatalf("claim %q: %d windows, invariant says %d", tc.claim, len(u.Windows), len(probeWindows))
		}
		if u.Windows[0].Active != tc.active5 || u.Windows[1].Active != tc.active7 {
			t.Errorf("claim %q: active = %v/%v, want %v/%v", tc.claim, u.Windows[0].Active, u.Windows[1].Active, tc.active5, tc.active7)
		}
	}
}

// TestParseProbeHeaders_WindowShape: probed windows must look exactly like
// fetched ones (same kind, group and label), so render and pick treat them
// alike.
func TestParseProbeHeaders_WindowShape(t *testing.T) {
	t.Parallel()
	h := realHeaders()
	h.Set(hdrPrefix+probeWindow5h+hdrUtilization, "0.375")
	h.Set(hdrPrefix+probeWindow7d+hdrUtilization, "1")
	u, err := ParseProbeHeaders(h)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind  provider.WindowKind
		group string
		label string
		pct   float64
		reset int64
	}{
		{provider.WindowSession, groupSession, labelSession, 37.5, 1792188600},
		{provider.WindowWeekly, groupWeekly, labelWeeklyAll, 100, 1792692000},
	}
	for i, w := range want {
		g := u.Windows[i]
		if g.Kind != w.kind || g.Group != w.group || g.Label != w.label || g.Percent != w.pct || g.ResetsAt == nil || g.ResetsAt.Unix() != w.reset {
			t.Errorf("window %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestProbeSeverity_Matrix(t *testing.T) {
	t.Parallel()
	for status, want := range map[string]string{
		"":                   "",
		statusAllowed:        severityNormal,
		statusAllowedWarning: severityWarning,
		"rejected":           severityExhausted,
		"ALLOWED":            severityExhausted, // case matters on the wire
		"allowed ":           severityExhausted,
		"окей":               severityExhausted,
	} {
		if got := probeSeverity(status); got != want {
			t.Errorf("probeSeverity(%q) = %q, want %q", status, got, want)
		}
	}
}

// TestProbe_DefaultsAndTransportErrors covers the request defaults (model)
// and the failures that never reach a status code. The token must never
// appear in any error.
func TestProbe_DefaultsAndTransportErrors(t *testing.T) {
	t.Parallel()
	var model string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body) // test server
		var b probeBody
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Errorf("body: %v", err)
		}
		model = b.Model
		for k, v := range realHeaders() {
			w.Header()[k] = v
		}
	}))
	if _, err := (&ProbeClient{Endpoint: srv.URL, HTTP: srv.Client()}).Probe(context.Background(), probeToken); err != nil {
		t.Fatal(err)
	}
	if model != DefaultProbeModel {
		t.Errorf("model = %q, want the default %q", model, DefaultProbeModel)
	}
	if _, err := (&ProbeClient{Endpoint: srv.URL, Model: "custom-model"}).Probe(context.Background(), probeToken); err != nil || model != "custom-model" {
		t.Errorf("custom model = %q, %v", model, err)
	}
	closedURL := srv.URL
	srv.Close()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name     string
		endpoint string
		ctx      context.Context
		want     string
	}{
		{"bad url", "://not a url", context.Background(), "build probe"},
		{"closed server", closedURL, context.Background(), "claude: probe"},
		{"cancelled context", closedURL, cancelled, "claude: probe"},
	} {
		_, err := (&ProbeClient{Endpoint: tc.endpoint}).Probe(tc.ctx, probeToken)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
		if err != nil && strings.Contains(err.Error(), probeToken) {
			t.Errorf("%s: error leaks the token", tc.name)
		}
	}
}

// TestProbe_429Matrix pins the 429 decision: the full header set wins (an
// account at its limit is shown at 100%), a partial or absent set falls back
// to the rate-limit error with the Retry-After hint.
func TestProbe_429Matrix(t *testing.T) {
	t.Parallel()
	partial := realHeaders()
	partial.Del(hdrPrefix + probeWindow7d + hdrReset)
	for _, tc := range []struct {
		name       string
		headers    http.Header
		retryAfter string
		wantUsage  bool
		wantMsg    string
	}{
		{"full set", realHeaders(), "", true, ""},
		{"full set with retry-after", realHeaders(), "60", true, ""},
		{"partial set", partial, "60", false, "retry after 1m0s"},
		{"none", http.Header{}, "", false, ""},
		{"none with retry-after", http.Header{}, "5", false, "retry after 5s"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range tc.headers {
				w.Header()[k] = v
			}
			if tc.retryAfter != "" {
				w.Header().Set("Retry-After", tc.retryAfter)
			}
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		u, err := (&ProbeClient{Endpoint: srv.URL, HTTP: srv.Client()}).Probe(context.Background(), probeToken)
		srv.Close()
		if tc.wantUsage {
			if err != nil || len(u.Windows) != len(probeWindows) {
				t.Errorf("%s: %+v, %v; want windows", tc.name, u, err)
			}
			continue
		}
		if !errors.Is(err, provider.ErrRateLimited) || !strings.Contains(err.Error(), tc.wantMsg) {
			t.Errorf("%s: err = %v, want ErrRateLimited %q", tc.name, err, tc.wantMsg)
		}
	}
}

func TestAPIMessage_Matrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, want string
	}{
		{"empty", "", ""},
		{"not json", "<html>", ""},
		{"no message", `{"error":{"type":"x"}}`, ""},
		{"message", `{"error":{"message":"bad"}}`, ": bad"},
		{"token redacted", `{"error":{"message":"bad ` + probeToken + `"}}`, ": bad " + redactedToken},
		{"multi-line squashed", `{"error":{"message":"a\n\n  b"}}`, ": a b"},
		{"unicode", `{"error":{"message":"não autorizado"}}`, ": não autorizado"},
	} {
		if got := apiMessage([]byte(tc.body), probeToken); got != tc.want {
			t.Errorf("%s: apiMessage = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestLoginUsage_OAuthProbes: an oauth-token login is probed; success is
// LoggedIn with no account details, failure passes the probe error through.
func TestLoginUsage_OAuthProbes(t *testing.T) {
	t.Parallel()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range realHeaders() {
			w.Header()[k] = v
		}
	}))
	defer ok.Close()
	u, id, err := (&Provider{Probe: &ProbeClient{Endpoint: ok.URL}}).LoginUsage(context.Background(), provider.LoginOAuthToken, probeToken)
	if err != nil || !id.LoggedIn || id.Email != "" || !u.Identity.LoggedIn || len(u.Windows) != len(probeWindows) {
		t.Errorf("ok = %+v, %+v, %v", u, id, err)
	}
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer denied.Close()
	if _, id, err := (&Provider{Probe: &ProbeClient{Endpoint: denied.URL}}).LoginUsage(context.Background(), provider.LoginOAuthToken, probeToken); !errors.Is(err, provider.ErrUnauthorized) || id.LoggedIn {
		t.Errorf("denied = %+v, %v", id, err)
	}
}
