// Package secret resolves "scheme:ref" secret references to their values at
// the moment they are needed, so secrets never live in config files, plan
// files or agx's own memory longer than one launch.
//
// Supported schemes:
//
//	gopass:<path>   runs `gopass show -o <path>` (the gopass CLI must be on PATH)
//	env:<NAME>      reads another environment variable
//
// Resolver is an interface so tests (and other tools importing this package)
// can substitute their own backend.
package secret

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Scheme is the part of a reference before the first ':'.
type Scheme string

const (
	SchemeGopass Scheme = "gopass"
	SchemeEnv    Scheme = "env"
)

// Schemes is the canonical list, for validation and help text.
var Schemes = []Scheme{SchemeGopass, SchemeEnv}

// Errors.
var (
	ErrBadRef        = errors.New("secret: reference must be scheme:ref")
	ErrUnknownScheme = errors.New("secret: unknown scheme")
	ErrEmpty         = errors.New("secret: resolved to an empty value")
)

// Ref is a parsed reference.
type Ref struct {
	Scheme Scheme
	Path   string
}

// Parse splits and validates a "scheme:ref" string.
func Parse(s string) (Ref, error) {
	scheme, path, ok := strings.Cut(s, ":")
	if !ok || scheme == "" || path == "" {
		return Ref{}, fmt.Errorf("%w: %q", ErrBadRef, s)
	}
	r := Ref{Scheme: Scheme(scheme), Path: path}
	for _, known := range Schemes {
		if r.Scheme == known {
			return r, nil
		}
	}
	return Ref{}, fmt.Errorf("%w %q (want one of %v)", ErrUnknownScheme, scheme, Schemes)
}

// Resolver turns a reference into its value.
type Resolver interface {
	Resolve(ctx context.Context, ref Ref) (string, error)
}

// GopassBin is the external program the gopass scheme runs; exported so a
// doctor check can look for it on PATH without resolving anything.
const GopassBin = "gopass"

// defaultTimeout bounds one gopass call; gopass may prompt for a GPG
// passphrase, and a prompt nobody answers must not hang a launch forever.
const defaultTimeout = 60 * time.Second

// System resolves references using the real environment and gopass.
type System struct {
	// LookupEnv overrides os.LookupEnv (tests).
	LookupEnv func(string) (string, bool)
	// Timeout bounds a gopass call; zero means defaultTimeout.
	Timeout time.Duration
}

// Resolve implements Resolver. An empty result is an error (ErrEmpty): an
// empty API token would otherwise surface much later as a confusing auth
// failure inside the launched tool.
func (s System) Resolve(ctx context.Context, ref Ref) (string, error) {
	var (
		val string
		err error
	)
	switch ref.Scheme {
	case SchemeEnv:
		lookup := s.LookupEnv
		if lookup == nil {
			lookup = os.LookupEnv
		}
		v, ok := lookup(ref.Path)
		if !ok {
			return "", fmt.Errorf("secret: env %s is not set", ref.Path)
		}
		val = v
	case SchemeGopass:
		val, err = s.gopass(ctx, ref.Path)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("%w %q", ErrUnknownScheme, ref.Scheme)
	}
	if val == "" {
		return "", fmt.Errorf("%w: %s:%s", ErrEmpty, ref.Scheme, ref.Path)
	}
	return val, nil
}

// gopass runs `gopass show -o <path>`. Stdin/stderr stay attached to the
// terminal so a GPG pinentry prompt works; only stdout is captured.
func (s System) gopass(ctx context.Context, path string) (string, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, GopassBin, "show", "-o", path)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("secret: %s is not installed", GopassBin)
		}
		// Never include stdout in the error: it may hold a partial secret.
		return "", fmt.Errorf("secret: gopass show %s: %w", path, err)
	}
	return strings.TrimRight(out.String(), "\r\n"), nil
}

// ResolveAll resolves every reference in refs (env var name → "scheme:ref")
// and returns env var name → value. It stops at the first failure and names
// the variable, never the value.
func ResolveAll(ctx context.Context, r Resolver, refs map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(refs))
	for name, raw := range refs {
		ref, err := Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("secret for %s: %w", name, err)
		}
		v, err := r.Resolve(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("secret for %s: %w", name, err)
		}
		out[name] = v
	}
	return out, nil
}
