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
	"path/filepath"
	"sync"
	"time"

	"github.com/ubgo/appdir"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
	"github.com/khanakia/agx/provider/codex"
	"github.com/khanakia/agx/secret"
)

// Exit codes (spec §4, voltkit convention). Scripts branch on them, so a
// value never changes meaning.
const (
	// ExitOK: the command did what was asked.
	ExitOK = 0
	// ExitFailure: the request was valid but something failed (an account
	// unreachable, a move blocked, a picker dismissed).
	ExitFailure = 1
	// ExitUsage: the request itself was wrong (unknown profile, bad flag,
	// invalid config); retrying unchanged cannot succeed.
	ExitUsage = 2
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
	// RunInteractive runs cmd attached to the terminal and waits (`claude
	// setup-token` for `login add --setup`).
	RunInteractive func(cmd provider.Command) error
	// PromptSecret shows prompt and reads one line without echo.
	PromptSecret func(prompt string) (string, error)
	// Interactive reports whether stdin and stdout are both terminals,
	// which decides whether a bare `agx` may start an agent. nil is false.
	Interactive func() bool
}

// SystemDeps wires the real machine.
func SystemDeps() (Deps, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Deps{}, fmt.Errorf("home dir: %w", err)
	}
	return Deps{
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Stdin:      os.Stdin,
		UserHome:   home,
		Providers:  []provider.Provider{claude.New(home), codex.New(home)},
		ConfigPath: func() (string, string, error) { return configPath(os.LookupEnv) },
		Now:        time.Now,
		Getwd:      os.Getwd,
		LookupEnv:  os.LookupEnv,
		Environ:    os.Environ,
		Exec:       execReplace,
		Secrets:    secret.System{},
		Picker:     TerminalPicker{},
		IsTerminal: isTerminal,
		LookPath:   lookPath,

		RunInteractive: runInteractive,
		PromptSecret:   promptSecret,
		Interactive:    interactive,
	}, nil
}

// Where agx keeps config.yaml (and tokens.yaml beside it). These rules are
// older than ubgo/appdir and existing users rely on them, so they are kept
// exactly (spec: no user's data moves):
//
//	AGX_CONFIG_DIR set  -> $AGX_CONFIG_DIR/config.yaml      (env-category)
//	AGX_HOME set        -> $AGX_HOME/config/config.yaml     (env-home)
//	otherwise           -> ~/.agx/config.yaml               (home-dotfile)
//
// The rung names are the ones `agx doctor` has always printed.
const (
	rungEnvCategory = "env-category"
	rungEnvHome     = "env-home"
	rungHomeDotfile = "home-dotfile"
)

// configKind is agx's own config kind. appdir's standard config kind is
// config.jsonc and has no <APP>_CONFIG_DIR variable, so agx replaces it with
// a global, overridable config.yaml: AGX_CONFIG_DIR keeps working.
const configKind appdir.Kind = "config"

// configPath resolves the config file through ubgo/appdir with agx's
// historic layout. lookupEnv is the process environment (a seam for tests).
func configPath(lookupEnv func(string) (string, bool)) (string, string, error) {
	envCategory := appmeta.EnvPrefix + "_CONFIG_DIR"
	envHome := appmeta.EnvPrefix + "_HOME"
	set := func(name string) (string, bool) {
		v, ok := lookupEnv(name)
		return v, ok && v != ""
	}
	rung := rungHomeDotfile
	env := lookupEnv
	_, hasCategory := set(envCategory)
	if home, ok := set(envHome); ok && !hasCategory {
		// appdir reads AGX_HOME as the home itself (config at
		// $AGX_HOME/config.yaml); agx always read $AGX_HOME/config/, so the
		// variable is handed to appdir as the config folder instead.
		rung = rungEnvHome
		env = func(name string) (string, bool) {
			switch name {
			case envHome:
				return "", false
			case envCategory:
				return filepath.Join(home, "config"), true
			}
			return lookupEnv(name)
		}
	} else if hasCategory {
		rung = rungEnvCategory
	}
	d, err := appdir.New(appmeta.Name,
		appdir.WithDirName(appmeta.DirName),
		appdir.WithEnvPrefix(appmeta.EnvPrefix),
		appdir.WithMode(appdir.ModeGlobal),
		// One home for every build, as before: a dev profile would move a
		// locally built agx to ~/.agx-dev and hide the user's config.
		appdir.WithRelease(true),
		appdir.WithEnv(env),
		appdir.WithoutKind(appdir.KindConfig),
		appdir.WithKind(configKind, appdir.File(config.FileName), appdir.Scoped(appdir.ScopeGlobal)))
	if err != nil {
		return "", "", err
	}
	p, err := d.PathIn(configKind, appdir.ScopeGlobal)
	if err != nil {
		return "", "", err
	}
	return p, rung, nil
}

// app is the per-invocation state shared by commands.
type app struct {
	Deps
	cfg     config.Config
	cfgRung string
	loaded  bool
	// noteMu serialises stderr lines: probes and usage fetches run
	// concurrently and may each warn (e.g. an approximate expiry).
	noteMu sync.Mutex
	// project is the binding this invocation's launch answers to, zero when
	// none covers it (see useProject). Commands that do not launch never set
	// it, so `agx profiles` and doctor describe a profile, not this folder.
	project config.Project
	// announced means the launch was already explained on stderr, so it
	// does not repeat its own login line.
	announced bool
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
	a.noteMu.Lock()
	defer a.noteMu.Unlock()
	// stderr is the channel of last resort: if writing to it fails there is
	// nowhere left to report that, so the error is deliberately dropped.
	_, _ = fmt.Fprintln(a.Stderr, line)
}

// ctxOf returns the command's context, or Background when cobra was run
// without one (tests call Execute directly). It takes the one method it
// needs so it does not care which cobra type carries the context.
func ctxOf(c interface{ Context() context.Context }) context.Context {
	if ctx := c.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
