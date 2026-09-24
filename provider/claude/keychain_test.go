package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeSecurity writes a stand-in for macOS `security` that runs body.
func fakeSecurity(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "security")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeychainGet(t *testing.T) {
	t.Parallel()
	// Found: the password is printed with a trailing newline, which Get trims.
	// The service is argument 3 and -w comes last, with or without "-a <account>" between.
	k := Keychain{Bin: fakeSecurity(t, `last=""; for a in "$@"; do last="$a"; done; [ "$1" = find-generic-password ] && [ "$3" = "svc-1" ] && [ "$last" = -w ] && printf 'secret-json\n'`)}
	if got, err := k.Get("svc-1"); err != nil || string(got) != "secret-json" {
		t.Errorf("found = %q, %v", got, err)
	}
	// Exit 44 is `security`'s "item not found".
	k = Keychain{Bin: fakeSecurity(t, "exit 44")}
	if _, err := k.Get("svc"); !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("exit 44 = %v, want ErrSecretNotFound", err)
	}
	// Any other failure is a real error, not "not found".
	k = Keychain{Bin: fakeSecurity(t, "exit 1")}
	if _, err := k.Get("svc"); err == nil || errors.Is(err, ErrSecretNotFound) {
		t.Errorf("exit 1 = %v, want a non-not-found error", err)
	}
	// A missing binary (non-macOS) degrades to not-found so the file path works.
	k = Keychain{Bin: filepath.Join(t.TempDir(), "no-such-security")}
	if _, err := k.Get("svc"); !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("missing binary = %v, want ErrSecretNotFound", err)
	}
	// An unanswered keychain prompt must not hang: it times out.
	k = Keychain{Bin: fakeSecurity(t, "sleep 5"), Timeout: 100 * time.Millisecond}
	start := time.Now()
	if _, err := k.Get("svc"); err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(start) > 3*time.Second {
		t.Errorf("hang = %v after %s", err, time.Since(start))
	}
}

// TestKeychainGet_PrefersAccount reproduces the real bug: two entries share
// the service name — a stale "unknown"-account one listed first, and the live
// one under the user's account. Get must return the account's entry.
func TestKeychainGet_PrefersAccount(t *testing.T) {
	t.Parallel()
	script := `case "$*" in
  *"-a khanakia"*) printf 'fresh-login\n' ;;
  *"-a "*) exit 44 ;;
  *) printf 'stale-first-entry\n' ;;
esac`
	k := Keychain{Bin: fakeSecurity(t, script), Account: "khanakia"}
	if got, err := k.Get("svc"); err != nil || string(got) != "fresh-login" {
		t.Errorf("account entry = %q, %v", got, err)
	}
	// No entry for this account → fall back to any entry with the service.
	k = Keychain{Bin: fakeSecurity(t, script), Account: "someone-else"}
	if got, err := k.Get("svc"); err != nil || string(got) != "stale-first-entry" {
		t.Errorf("fallback = %q, %v", got, err)
	}
	// A real error on the account lookup is reported, not masked by the fallback.
	k = Keychain{Bin: fakeSecurity(t, `case "$*" in *"-a "*) exit 1 ;; *) printf 'x\n' ;; esac`), Account: "khanakia"}
	if _, err := k.Get("svc"); err == nil || errors.Is(err, ErrSecretNotFound) {
		t.Errorf("account lookup error = %v, want a real error", err)
	}
	// With no explicit account the current OS user is used.
	if (Keychain{}).account() == "" {
		t.Error("no account resolved for the current user")
	}
}
