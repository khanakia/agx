package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"time"
)

// Keychain is a SecretStore backed by the macOS `security` command-line
// tool, which ships with macOS. On other platforms (or if `security` is
// missing) every lookup reports ErrSecretNotFound, so credential resolution
// falls back to the credentials file instead of failing.
//
// Account matters: Claude Code saves its login with the macOS username as
// the keychain account, but a keychain can also hold older entries with the
// SAME service name and a different account (seen in the wild: a stale
// "unknown"-account entry from an old Claude Code version with an empty
// token). `security find-generic-password -s <service>` alone returns
// whichever entry comes first, so Get asks for the account-specific entry
// first and only then for any entry with that service.
type Keychain struct {
	// Timeout bounds one lookup. macOS may show an access prompt the first
	// time; a lookup nobody answers must not hang the whole run.
	Timeout time.Duration
	// Bin overrides the `security` executable (tests point it at a fake);
	// empty means securityBin on PATH.
	Bin string
	// Account overrides the keychain account to prefer; empty means the
	// current OS username (what Claude Code writes).
	Account string
}

const (
	securityBin = "security"
	// securityNotFoundExit is the exit status `security find-generic-password`
	// uses for "The specified item could not be found in the keychain."
	securityNotFoundExit = 44
	// defaultKeychainTimeout applies when Keychain.Timeout is zero.
	defaultKeychainTimeout = 10 * time.Second
	// userEnv is the fallback source of the username when os/user fails.
	userEnv = "USER"
)

// Get returns the password stored under service, preferring the entry whose
// account is the current user (see the type doc for why).
func (k Keychain) Get(service string) ([]byte, error) {
	if account := k.account(); account != "" {
		out, err := k.find(service, account)
		if err == nil || !errors.Is(err, ErrSecretNotFound) {
			return out, err
		}
	}
	return k.find(service, "")
}

// account resolves the account to prefer.
func (k Keychain) account() string {
	if k.Account != "" {
		return k.Account
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv(userEnv)
}

// find runs one `security find-generic-password` lookup; an empty account
// matches any entry with the service.
func (k Keychain) find(service, account string) ([]byte, error) {
	timeout := k.Timeout
	if timeout <= 0 {
		timeout = defaultKeychainTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	bin := k.Bin
	if bin == "" {
		bin = securityBin
	}
	args := []string{"find-generic-password", "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	args = append(args, "-w")
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err == nil {
		return bytes.TrimSpace(out), nil
	}

	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, fs.ErrNotExist): // not on PATH, or an absolute path that is missing
		return nil, fmt.Errorf("%w: %s not installed", ErrSecretNotFound, securityBin)
	case errors.As(err, &exitErr) && exitErr.ExitCode() == securityNotFoundExit:
		return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, service)
	case ctx.Err() != nil:
		return nil, fmt.Errorf("%s lookup timed out after %s (unanswered keychain prompt?): %w", securityBin, timeout, ctx.Err())
	default:
		return nil, fmt.Errorf("%s find-generic-password: %w", securityBin, err)
	}
}
