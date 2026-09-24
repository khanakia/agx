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
	k := Keychain{Bin: fakeSecurity(t, `[ "$1" = find-generic-password ] && [ "$3" = "svc-1" ] && [ "$4" = -w ] && printf 'secret-json\n'`)}
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
