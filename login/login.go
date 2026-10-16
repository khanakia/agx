// Package login resolves named credentials ("logins") to the token value a
// vendor CLI should run on, so one home's history can spend another
// account's quota (logins spec §4, §10 D12).
//
// A login names exactly one source: a raw token kept in a private token file
// (stored), a command that prints it (cmd), or another env var (env). Values
// are resolved only at launch, for the one login actually used, and never
// appear in an error message.
//
// Invariant: this package imports only the standard library and provider,
// so any tool can reuse it without agx's CLI stack.
package login

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/khanakia/agx/provider"
)

// Source says where a login's token comes from. It is shown in listings
// (`stored`, `cmd`, `env`) in place of the value.
type Source string

const (
	// SourceStored keeps the raw token in the token file (TokenFile).
	SourceStored Source = "stored"
	// SourceCmd runs a shell command that prints the token.
	SourceCmd Source = "cmd"
	// SourceEnv reads the token from another env var.
	SourceEnv Source = "env"
)

// Reserved login names: they select behaviour, never a stored credential.
const (
	// Home uses the login stored in the home itself, overriding defaults.
	Home = "home"
	// Auto picks the login with the most headroom (logins spec §8).
	Auto = "auto"
)

// StoredMarker is the only value config's `token:` field accepts; anything
// else is a raw token pasted into the wrong file.
const StoredMarker = string(SourceStored)

// Login is one named credential, as configured. It holds where the token
// comes from, never the token itself.
type Login struct {
	Name     string
	Provider provider.ID
	Kind     provider.LoginKind
	Source   Source
	// Cmd is the shell command for SourceCmd.
	Cmd string
	// Env is the variable name for SourceEnv.
	Env string
	// Account is a free-form label (an email), for the user's eyes.
	Account string
	// Added is when the login was registered; zero when unknown. Expiry is
	// counted from it.
	Added time.Time
}

// Errors.
var (
	// ErrEmpty means the source produced no token.
	ErrEmpty = errors.New("login: token is empty")
	// ErrMultiline means a command printed more than one non-empty line,
	// which is never a single token (e.g. it printed a whole file).
	ErrMultiline = errors.New("login: token command printed more than one line")
	// ErrMissing means a stored login has no entry in the token file.
	ErrMissing = errors.New("login: no stored token")
)

// cmdTimeout bounds a token command: a store prompting for a passphrase
// nobody answers must not hang a launch forever.
const cmdTimeout = 10 * time.Second

// shell runs token commands, so the user's own syntax (pipes) works.
const shell = "sh"

// Resolver turns a Login into its token value. Every field is optional;
// zero values use the real machine.
type Resolver struct {
	// Tokens is the token file for stored logins.
	Tokens TokenFile
	// LookupEnv overrides os.LookupEnv.
	LookupEnv func(string) (string, bool)
	// RunCmd overrides running a token command; it returns stdout. Tests
	// substitute a fake.
	RunCmd func(ctx context.Context, command string) (string, error)
	// Dir is the working dir for token commands (the user's home).
	Dir string
}

// Resolve returns l's token. Errors name the login and the source, never
// the value.
func (r Resolver) Resolve(ctx context.Context, l Login) (string, error) {
	var (
		val string
		err error
	)
	switch l.Source {
	case SourceStored:
		var all map[string]string
		all, err = r.Tokens.Load()
		if err != nil {
			return "", err
		}
		v, ok := all[l.Name]
		if !ok {
			return "", fmt.Errorf("%w for login %s in %s — run `agx login add %s --stdin --replace`", ErrMissing, l.Name, r.Tokens.Path, l.Name)
		}
		val = v
	case SourceEnv:
		lookup := r.LookupEnv
		if lookup == nil {
			lookup = os.LookupEnv
		}
		v, ok := lookup(l.Env)
		if !ok {
			return "", fmt.Errorf("login %s: env %s is not set", l.Name, l.Env)
		}
		val = v
	case SourceCmd:
		run := r.RunCmd
		if run == nil {
			run = r.runCmd
		}
		val, err = run(ctx, l.Cmd)
		if err != nil {
			return "", fmt.Errorf("login %s: token_cmd: %w", l.Name, err)
		}
	default:
		return "", fmt.Errorf("login %s: unknown source %q", l.Name, l.Source)
	}
	val, err = Clean(val)
	if err != nil {
		return "", fmt.Errorf("login %s (%s): %w", l.Name, l.Source, err)
	}
	return val, nil
}

// Clean trims surrounding whitespace and rejects empty or multi-line values,
// the shapes a real single token never has.
func Clean(v string) (string, error) {
	lines := 0
	for line := range strings.SplitSeq(v, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	switch {
	case lines == 0:
		return "", ErrEmpty
	case lines > 1:
		return "", ErrMultiline
	}
	return strings.TrimSpace(v), nil
}

// runCmd runs command through sh -c with the user's environment, no stdin,
// a bounded time and Dir as the working dir. Only stderr can appear in the
// error; stdout (the token) never does.
func (r Resolver) runCmd(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = r.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("timed out after %s", cmdTimeout)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

// ExpiresAt returns when l stops working given a credential lifetime, and
// false when that is unknown (no lifetime or no added date).
func ExpiresAt(l Login, lifetime time.Duration) (time.Time, bool) {
	if lifetime <= 0 || l.Added.IsZero() {
		return time.Time{}, false
	}
	return l.Added.Add(lifetime), true
}

// Owned reports whether agx holds l's value itself; for cmd and env logins
// `added` is only when the login was registered, so expiry is approximate.
func Owned(l Login) bool { return l.Source == SourceStored }

// ErrIncompatible means a profile already supplies a credential of its own,
// so a login on it would be a second source for one credential.
var ErrIncompatible = errors.New("login: profile already has its own credential")

// Compatible reports whether profile p can run on a login of kind. It
// refuses an api-billed profile and one whose own env/secrets set any
// variable the login sets or removes: the login would silently override or
// delete the profile's credential.
func Compatible(p provider.Profile, ll provider.LoginLauncher, kind provider.LoginKind) error {
	if p.Billing == provider.BillingAPI {
		return fmt.Errorf("%w: profile %s is billed per token (billing: api)", ErrIncompatible, p.Name)
	}
	set, unset := ll.LoginEnv(kind, "")
	owned := append([]string{}, unset...)
	for k := range set {
		owned = append(owned, k)
	}
	for _, k := range owned {
		if _, ok := p.Env[k]; ok {
			return fmt.Errorf("%w: profile %s sets %s in env", ErrIncompatible, p.Name, k)
		}
		if _, ok := p.Secrets[k]; ok {
			return fmt.Errorf("%w: profile %s sets %s in secrets", ErrIncompatible, p.Name, k)
		}
	}
	return nil
}
