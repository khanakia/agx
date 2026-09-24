package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"time"
)

// Keychain is a SecretStore backed by the macOS `security` command-line
// tool, which ships with macOS. On other platforms (or if `security` is
// missing) every lookup reports ErrSecretNotFound, so credential resolution
// falls back to the credentials file instead of failing.
type Keychain struct {
	// Timeout bounds one lookup. macOS may show an access prompt the first
	// time; a lookup nobody answers must not hang the whole run.
	Timeout time.Duration
	// Bin overrides the `security` executable (tests point it at a fake);
	// empty means securityBin on PATH.
	Bin string
}

const (
	securityBin = "security"
	// securityNotFoundExit is the exit status `security find-generic-password`
	// uses for "The specified item could not be found in the keychain."
	securityNotFoundExit = 44
	// defaultKeychainTimeout applies when Keychain.Timeout is zero.
	defaultKeychainTimeout = 10 * time.Second
)

// Get returns the password stored under service.
func (k Keychain) Get(service string) ([]byte, error) {
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
	out, err := exec.CommandContext(ctx, bin, "find-generic-password", "-s", service, "-w").Output()
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
