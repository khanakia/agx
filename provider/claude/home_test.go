package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// Dummy credentials: the vendor prefixes with an obviously fake body.
const (
	dummyOAT   = "sk-ant-oat01-DUMMY-0000"
	dummyAPI   = "sk-ant-api03-DUMMY-0000"
	dummyORT   = "sk-ant-ort01-DUMMY-0000"
	dummyAdmin = "sk-ant-admin01-DUMMY-0000"
)

// TestDetectLoginKind_Matrix covers every prefix, the shapes a paste takes
// (newline, spaces, tabs) and the near misses (case, prefix only, prefix
// inside the value, other vendors). A recognised but unusable token is an
// ErrUnusableToken error that never quotes the value.
func TestDetectLoginKind_Matrix(t *testing.T) {
	t.Parallel()
	p := New("/home/me")
	for _, tc := range []struct {
		name     string
		value    string
		want     provider.LoginKind
		unusable string // substring of the refusal; "" when accepted
	}{
		{"setup-token", dummyOAT, provider.LoginOAuthToken, ""},
		{"api key", dummyAPI, provider.LoginAPIKey, ""},
		{"refresh token", dummyORT, "", "refresh token"},
		{"admin key", dummyAdmin, "", "Admin API key"},
		{"oat with newline", dummyOAT + "\n", provider.LoginOAuthToken, ""},
		{"api with spaces and tab", " \t" + dummyAPI + " \r\n", provider.LoginAPIKey, ""},
		{"ort with newline", "\n" + dummyORT + "\n", "", "refresh token"},
		{"oat prefix only", "sk-ant-oat", provider.LoginOAuthToken, ""},
		{"api prefix only", "sk-ant-api", provider.LoginAPIKey, ""},
		{"ort prefix only", "sk-ant-ort", "", "refresh token"},
		{"admin prefix only", "sk-ant-admin", "", "Admin API key"},
		{"empty", "", "", ""},
		{"whitespace only", " \n\t", "", ""},
		{"uppercase oat", strings.ToUpper(dummyOAT), "", ""},
		{"uppercase admin", strings.ToUpper(dummyAdmin), "", ""},
		{"bare sk-ant", "sk-ant-", "", ""},
		{"truncated prefix", "sk-ant-oa", "", ""},
		{"prefix in middle", "x" + dummyOAT, "", ""},
		{"openai key", "sk-proj-DUMMY", "", ""},
		{"unknown sk-ant kind", "sk-ant-xyz01-DUMMY", "", ""},
		{"unicode", "sk-ant-oat01-ДАММИ", provider.LoginOAuthToken, ""},
	} {
		got, err := p.DetectLoginKind(tc.value)
		if tc.unusable != "" {
			if !errors.Is(err, ErrUnusableToken) || !strings.Contains(err.Error(), tc.unusable) {
				t.Errorf("%s: err = %v, want ErrUnusableToken about %q", tc.name, err, tc.unusable)
			}
			if err != nil && strings.Contains(err.Error(), "DUMMY") {
				t.Errorf("%s: error quotes the value: %v", tc.name, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: DetectLoginKind = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

// TestDetectLoginKind_AgreesWithLoginKinds: every kind detection can return
// is one the provider can launch, so a detected kind never fails config
// validation.
func TestDetectLoginKind_AgreesWithLoginKinds(t *testing.T) {
	t.Parallel()
	p := New("/home/me")
	for _, v := range []string{dummyOAT, dummyAPI} {
		k, err := p.DetectLoginKind(v)
		if err != nil || !slices.Contains(p.LoginKinds(), k) {
			t.Errorf("%s: kind %q (%v) is not in LoginKinds %v", v, k, err, p.LoginKinds())
		}
	}
}

// TestHomeSetup: the shared list is a copy (a caller cannot change what the
// next profile links), credentials are never shared, and the login command
// selects the new home and drops every variable that would override its
// stored login. The default home runs with CLAUDE_CONFIG_DIR absent.
func TestHomeSetup(t *testing.T) {
	t.Parallel()
	p := New("/home/me")
	files := p.SharedHomeFiles()
	if !slices.Equal(files, []string{"CLAUDE.md", "skills", "settings.json"}) {
		t.Errorf("shared = %v", files)
	}
	files[0] = "changed"
	if p.SharedHomeFiles()[0] != "CLAUDE.md" {
		t.Error("SharedHomeFiles returned its own slice")
	}
	for _, f := range p.SharedHomeFiles() {
		if strings.Contains(f, "credential") || f == ".claude.json" || f == "projects" {
			t.Errorf("%s must stay per account", f)
		}
	}
	base := []string{"PATH=/bin", OAuthTokenEnv + "=x", APIKeyEnv + "=y", AuthTokenEnv + "=z", ConfigDirEnv + "=/stale"}
	for _, tc := range []struct {
		home    string
		wantDir string // "" = absent
	}{
		{"/home/me/.claude-client", "/home/me/.claude-client"},
		{"/home/me/.claude", ""},
		{"/home/me/.claude/", ""},
	} {
		cmd := p.LoginCommand(provider.Profile{Name: "c", Provider: provider.Claude, Home: tc.home}, slices.Clone(base))
		if cmd.Path != Binary || !slices.Equal(cmd.Args, []string{"auth", "login"}) {
			t.Errorf("%s: command = %s %v", tc.home, cmd.Path, cmd.Args)
		}
		var dir string
		for _, kv := range cmd.Env {
			k, v, _ := strings.Cut(kv, "=")
			switch k {
			case OAuthTokenEnv, APIKeyEnv, AuthTokenEnv:
				t.Errorf("%s: %s kept for the login command", tc.home, k)
			case ConfigDirEnv:
				dir = v
			}
		}
		if dir != tc.wantDir {
			t.Errorf("%s: %s = %q, want %q", tc.home, ConfigDirEnv, dir, tc.wantDir)
		}
		if !slices.Contains(cmd.Env, "PATH=/bin") {
			t.Errorf("%s: unrelated env dropped", tc.home)
		}
	}
}

// Real 403 bodies from the usage and profile endpoints (2026-11-01).
const (
	scopeBody = `{"type":"error","error":{"type":"permission_error","message":"OAuth token does not meet scope requirement any_of(user:profile, user:office)","details":{"required_scopes":["user:profile","user:office"],"match":"any","error_visibility":"user_facing","error_code":"oauth_scope_insufficient"}},"request_id":"req_x"}`
	orgBody   = `{"type":"error","error":{"type":"permission_error","message":"OAuth authentication is currently not allowed for this organization."}}`
)

// TestUsageClient_403Matrix: only the scope refusal (by its error_code, not
// its wording) becomes ErrInferenceOnly; every other 403 stays
// ErrUnauthorized and carries the server's message when there is one. The
// same mapping holds for both endpoints, and the token never appears.
func TestUsageClient_403Matrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		inference  bool
		want, deny string
	}{
		{"scope", http.StatusForbidden, scopeBody, true, "can only run Claude", "HTTP 403"},
		{"scope minimal", http.StatusForbidden, `{"error":{"details":{"error_code":"oauth_scope_insufficient"}}}`, true, "no profile scope", ""},
		{"scope code on 401", http.StatusUnauthorized, scopeBody, false, "HTTP 401: OAuth token does not meet scope", ""},
		{"org not allowed", http.StatusForbidden, orgBody, false, "HTTP 403: OAuth authentication is currently not allowed for this organization.", ""},
		{"other error_code", http.StatusForbidden, `{"error":{"message":"nope","details":{"error_code":"something_else"}}}`, false, "HTTP 403: nope", ""},
		{"scope wording without code", http.StatusForbidden, `{"error":{"message":"OAuth token does not meet scope requirement"}}`, false, "HTTP 403: OAuth token does not meet scope", ""},
		{"non-JSON", http.StatusForbidden, "<html>Forbidden</html>", false, "(HTTP 403)", "html"},
		{"empty", http.StatusForbidden, "", false, "(HTTP 403)", ""},
		{"JSON without message", http.StatusForbidden, `{"error":{}}`, false, "(HTTP 403)", ""},
		{"message echoes token", http.StatusForbidden, `{"error":{"message":"bad token ` + fakeAccessToken + `"}}`, false, "bad token", fakeAccessToken},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
			if got := errors.Is(err, provider.ErrInferenceOnly); got != tc.inference {
				t.Errorf("%s/%s: ErrInferenceOnly = %v, want %v (%v)", tc.name, which, got, tc.inference, err)
			}
			if !tc.inference && !errors.Is(err, provider.ErrUnauthorized) {
				t.Errorf("%s/%s: %v is not ErrUnauthorized", tc.name, which, err)
			}
			if tc.inference && errors.Is(err, provider.ErrUnauthorized) {
				t.Errorf("%s/%s: a scope refusal must not read as a rejected login", tc.name, which)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s/%s: %q lacks %q", tc.name, which, err, tc.want)
			}
			if tc.deny != "" && strings.Contains(err.Error(), tc.deny) {
				t.Errorf("%s/%s: %q contains %q", tc.name, which, err, tc.deny)
			}
		}
	}
}
