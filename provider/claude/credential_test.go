package claude

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeStore is an in-memory SecretStore.
type fakeStore struct {
	secrets map[string]string
	errs    map[string]error
}

func (f fakeStore) Get(service string) ([]byte, error) {
	if err, ok := f.errs[service]; ok {
		return nil, err
	}
	if s, ok := f.secrets[service]; ok {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, service)
}

func credJSON(token string, expiresAtMs int64) string {
	return fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"refreshToken":"r","expiresAt":%d,"subscriptionType":"max","rateLimitTier":"default_claude_max_20x"},"mcpOAuth":{}}`, token, expiresAtMs)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestKeychainServices(t *testing.T) {
	t.Parallel()
	// The naming scheme (first 8 hex of sha256(dir)) was verified against a
	// real Claude Code 2.1.281 keychain. The expected value here was computed
	// independently: printf %s /Users/you/.claude-work | shasum -a 256
	got := KeychainServices("/Users/you/.claude-work", false)
	if len(got) != 1 || got[0] != "Claude Code-credentials-f7aef36f" {
		t.Errorf("KeychainServices = %v", got)
	}
	// Trailing slash is cleaned before hashing.
	if KeychainServices("/Users/you/.claude-work/", false)[0] != got[0] {
		t.Error("trailing slash changed the hash")
	}
	def := KeychainServices("/h/.claude", true)
	if len(def) != 2 || def[1] != "Claude Code-credentials" {
		t.Errorf("default dir services = %v", def)
	}
}

func TestResolveCredential(t *testing.T) {
	t.Parallel()
	later := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	earlier := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	for _, tc := range []struct {
		name      string
		file      string // "" = no credentials file
		isDefault bool
		secrets   map[string]string // keyed by index into KeychainServices
		errs      map[int]error
		wantToken string
		wantErr   bool
	}{
		{name: "file only", file: credJSON("file-tok", later), wantToken: "file-tok"},
		{name: "keychain only", secrets: map[string]string{"0": credJSON("kc-tok", later)}, wantToken: "kc-tok"},
		{name: "keychain fresher than file", file: credJSON("file-tok", earlier), secrets: map[string]string{"0": credJSON("kc-tok", later)}, wantToken: "kc-tok"},
		{name: "file fresher than keychain", file: credJSON("file-tok", later), secrets: map[string]string{"0": credJSON("kc-tok", earlier)}, wantToken: "file-tok"},
		{name: "tie keeps file", file: credJSON("file-tok", later), secrets: map[string]string{"0": credJSON("kc-tok", later)}, wantToken: "file-tok"},
		{name: "stale legacy entry loses to file", isDefault: true, file: credJSON("file-tok", later), secrets: map[string]string{"1": credJSON("legacy-tok", 0)}, wantToken: "file-tok"},
		{name: "legacy entry used when alone", isDefault: true, secrets: map[string]string{"1": credJSON("legacy-tok", 0)}, wantToken: "legacy-tok"},
		{name: "legacy name ignored for non-default dir", secrets: map[string]string{"legacy": credJSON("legacy-tok", later)}, wantErr: true},
		{name: "nothing", wantErr: true},
		{name: "malformed file, good keychain", file: `{nope`, secrets: map[string]string{"0": credJSON("kc-tok", later)}, wantToken: "kc-tok"},
		{name: "api-key style file without oauth", file: `{"primaryApiKey":"x"}`, wantErr: true},
		{name: "keychain failure surfaced", errs: map[int]error{0: errors.New("denied")}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), ".claude-x")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if tc.file != "" {
				writeFile(t, filepath.Join(dir, credentialsFile), tc.file)
			}
			services := KeychainServices(dir, tc.isDefault)
			store := fakeStore{secrets: map[string]string{}, errs: map[string]error{}}
			for k, v := range tc.secrets {
				switch k {
				case "0":
					store.secrets[services[0]] = v
				case "1":
					store.secrets[services[1]] = v
				case "legacy":
					store.secrets[keychainService] = v
				}
			}
			for i, e := range tc.errs {
				store.errs[services[i]] = e
			}

			c, err := ResolveCredential(dir, tc.isDefault, store)
			if tc.wantErr {
				if !errors.Is(err, ErrNoCredential) {
					t.Fatalf("err = %v, want ErrNoCredential", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.AccessToken != tc.wantToken {
				t.Errorf("token = %q, want %q (source %s)", c.AccessToken, tc.wantToken, c.Source)
			}
			if c.SubscriptionType != "max" || c.RateLimitTier != "default_claude_max_20x" {
				t.Errorf("plan fields not carried: %+v", c)
			}
		})
	}
}

func TestResolveCredential_KeychainErrorMessageKept(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := KeychainServices(dir, false)[0]
	_, err := ResolveCredential(dir, false, fakeStore{errs: map[string]error{svc: errors.New("user denied access")}})
	if err == nil || !errors.Is(err, ErrNoCredential) {
		t.Fatalf("err = %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "user denied access") {
		t.Errorf("underlying reason dropped: %s", got)
	}
}

func TestCredentialExpired(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		exp  time.Time
		want bool
	}{
		{"unknown expiry", time.Time{}, false},
		{"future", now.Add(time.Hour), false},
		{"exactly now", now, true},
		{"past", now.Add(-time.Hour), true},
	} {
		if got := (Credential{ExpiresAt: tc.exp}).Expired(now); got != tc.want {
			t.Errorf("%s: Expired = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDiscoverHomes(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	writeFile(t, filepath.Join(home, ".claude-work", "projects", ".keep"), "")
	writeFile(t, filepath.Join(home, ".claude-alt", credentialsFile), "{}")
	writeFile(t, filepath.Join(home, ".claude-worktrees", "repo", "README"), "") // no marker
	writeFile(t, filepath.Join(home, ".claude-notes"), "a file, not a dir")

	got, err := DiscoverHomes(home)
	if err != nil {
		t.Fatal(err)
	}
	want := []DiscoveredHome{
		{Name: "personal", Home: filepath.Join(home, ".claude")},
		{Name: "alt", Home: filepath.Join(home, ".claude-alt")},
		{Name: "work", Home: filepath.Join(home, ".claude-work")},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("DiscoverHomes = %v, want %v", got, want)
	}
}

func TestDiscoverHomes_Empty(t *testing.T) {
	t.Parallel()
	got, err := DiscoverHomes(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Errorf("DiscoverHomes(empty) = %v, %v", got, err)
	}
}

func TestIsDefaultHome(t *testing.T) {
	t.Parallel()
	if !IsDefaultHome("/h/.claude/", "/h") || IsDefaultHome("/h/.claude-work", "/h") {
		t.Error("IsDefaultHome wrong")
	}
}

func TestLoadAccount(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	def := filepath.Join(home, ".claude")
	work := filepath.Join(home, ".claude-work")
	writeFile(t, filepath.Join(home, profileFile), `{"oauthAccount":{"emailAddress":"me@example.com","organizationName":"Mine"}}`)
	writeFile(t, filepath.Join(work, profileFile), `{"oauthAccount":{"emailAddress":"me@work.example","organizationName":"Work"}}`)
	if err := os.MkdirAll(def, 0o700); err != nil {
		t.Fatal(err)
	}
	if a := LoadAccount(def, home, true); a.Email != "me@example.com" || a.Organization != "Mine" {
		t.Errorf("default account = %+v", a)
	}
	if a := LoadAccount(work, home, false); a.Email != "me@work.example" {
		t.Errorf("work account = %+v", a)
	}
	// A non-default dir must not borrow ~/.claude.json's identity.
	if a := LoadAccount(filepath.Join(home, ".claude-empty"), home, false); a != (Account{}) {
		t.Errorf("missing account = %+v, want empty", a)
	}
}
