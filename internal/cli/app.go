// Package cli is agx's command layer: the cobra command tree and the glue
// between config, providers and terminal output. It is the ONLY package that
// imports cobra or voltkit command modules (enforced by internal/archtest);
// everything it calls is a plain library.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/khanakia/voltkit/appdir"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
	"github.com/khanakia/agx/provider/codex"
	"github.com/khanakia/agx/secret"
)

// Exit codes (spec §4, voltkit convention).
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// ExitError carries a specific process exit code through cobra.
type ExitError struct {
	Code int
	Err  error
}

// Error returns the wrapped error's message.
func (e *ExitError) Error() string { return e.Err.Error() }

// Unwrap exposes the wrapped error to errors.Is / errors.As.
func (e *ExitError) Unwrap() error { return e.Err }

// usageErr marks err as a usage error (exit 2).
func usageErr(err error) error { return &ExitError{Code: ExitUsage, Err: err} }

// ExitCode maps an error returned by Execute to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ExitFailure
}

// errPartial is returned when output was produced but some item failed, so
// the process exits 1 without printing a second error line.
var errPartial = &ExitError{Code: ExitFailure, Err: errors.New("some items failed")}

// IsSilent reports errors whose message has already been shown.
func IsSilent(err error) bool { return errors.Is(err, errPartial) }

// Deps are the process-level dependencies, injected so every command is
// testable without touching the real machine.
type Deps struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	// UserHome anchors ~ expansion and discovery.
	UserHome string
	// Providers are the known providers, in display order.
	Providers []provider.Provider
	// ConfigPath returns the config file path and the appdir rung that
	// decided it.
	ConfigPath func() (path string, rung string, err error)
	Now        func() time.Time
	Getwd      func() (string, error)
	LookupEnv  func(string) (string, bool)
	Environ    func() []string
	// Exec replaces the process with cmd (or runs it, on platforms without
	// exec). Tests capture the command instead.
	Exec    func(cmd provider.Command) error
	Secrets secret.Resolver
	Picker  Picker
	// IsTerminal reports whether w is an interactive terminal.
	IsTerminal func(w io.Writer) bool
	// LookPath finds a binary on PATH (doctor).
	LookPath func(string) (string, error)
}

// SystemDeps wires the real machine.
func SystemDeps() (Deps, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Deps{}, fmt.Errorf("home dir: %w", err)
	}
	return Deps{
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Stdin:     os.Stdin,
		UserHome:  home,
		Providers: []provider.Provider{claude.New(home), codex.New(home)},
		ConfigPath: func() (string, string, error) {
			d, err := appdir.New(appmeta.Name,
				appdir.WithDirName(appmeta.DirName),
				appdir.WithEnvPrefix(appmeta.EnvPrefix),
				appdir.WithLayout(appdir.AllHomeDotfile))
			if err != nil {
				return "", "", err
			}
			p, err := d.ConfigFile(config.FileName)
			if err != nil {
				return "", "", err
			}
			rung, err := d.Rung(appdir.Config)
			return p, string(rung), err
		},
		Now:        time.Now,
		Getwd:      os.Getwd,
		LookupEnv:  os.LookupEnv,
		Environ:    os.Environ,
		Exec:       execReplace,
		Secrets:    secret.System{},
		Picker:     TerminalPicker{},
		IsTerminal: isTerminal,
		LookPath:   lookPath,
	}, nil
}

// app is the per-invocation state shared by commands.
type app struct {
	Deps
	cfg     config.Config
	cfgRung string
	loaded  bool
}

// config loads the config once per invocation.
func (a *app) config() (config.Config, error) {
	if a.loaded {
		return a.cfg, nil
	}
	path, rung, err := a.ConfigPath()
	if err != nil {
		return config.Config{}, fmt.Errorf("locate config: %w", err)
	}
	cfg, err := config.Load(path, a.UserHome, a.Providers)
	if err != nil {
		return config.Config{}, usageErr(err)
	}
	a.cfg, a.cfgRung, a.loaded = cfg, rung, true
	return cfg, nil
}

// providerFor returns the implementation for id.
func (a *app) providerFor(id provider.ID) (provider.Provider, error) {
	for _, p := range a.Providers {
		if p.ID() == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("unknown provider %q", id)
}

// warn writes one diagnostic line to stderr, prefixed with the program name.
func (a *app) warn(msg string) { a.note(appmeta.Name + ": " + msg) }

// note writes one plain line to stderr (status next to stdout data).
func (a *app) note(line string) {
	// stderr is the channel of last resort: if writing to it fails there is
	// nowhere left to report that, so the error is deliberately dropped.
	_, _ = fmt.Fprintln(a.Stderr, line)
}

// ctx returns a context for one command.
func ctxOf(c interface{ Context() context.Context }) context.Context {
	if ctx := c.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
