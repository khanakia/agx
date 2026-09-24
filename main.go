// Command claude-usage shows how much of each Claude subscription plan limit
// is used — current 5-hour session, weekly (all models), weekly per model —
// for every Claude Code account on this machine at once (~/.claude,
// ~/.claude-work, …). It is the terminal equivalent of claude.ai
// Settings → Usage, across accounts.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/khanakia/claude-usage/internal/account"
	"github.com/khanakia/claude-usage/internal/usage"
)

// Exit codes: 0 every account reported; 1 at least one account failed;
// 2 bad usage (flags, no accounts found).
const (
	exitOK      = 0
	exitPartial = 1
	exitUsage   = 2
)

// Colour modes for --color.
const (
	colorAuto   = "auto"
	colorAlways = "always"
	colorNever  = "never"
)

// version is stamped at build time (-ldflags "-X main.version=v1.2.3"). When
// empty, resolveVersion falls back to the module version `go install …@vX`
// records in the binary, so both install paths report something real.
var version string

// develVersion is what the Go toolchain records for a local (non-module) build.
const develVersion = "(devel)"

// noColorEnv is the https://no-color.org convention.
const noColorEnv = "NO_COLOR"

const usageText = `claude-usage — plan usage limits for every Claude Code account on this machine

Usage:
  claude-usage [flags] [config-dir ...]

With no config dirs, reports ~/.claude and every ~/.claude-* account dir.

Examples:
  claude-usage                        all accounts
  claude-usage ~/.claude-work         one account
  claude-usage --json | jq '.[] | {email, limits: [.limits[] | {label, percent}]}'

Numbers are percent of each plan window used (the same data as claude.ai
Settings → Usage). Tokens are read from Claude Code's credentials file or the
macOS keychain and are never refreshed; an expired token is reported, and
using that account in Claude Code once refreshes it.

Exit status: 0 ok · 1 some account failed · 2 usage error.

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// config is the parsed command line.
type config struct {
	jsonOut     bool
	showVersion bool
	color       string
	timeout     time.Duration
	dirs        []string
}

// run is main without process-global side effects, so it can be tested.
func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if cfg.showVersion {
		fmt.Fprintln(stdout, "claude-usage", resolveVersion(version, debug.ReadBuildInfo))
		return exitOK
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "claude-usage: home dir: %v\n", err)
		return exitUsage
	}
	dirs, err := resolveDirs(cfg.dirs, home)
	if err != nil {
		fmt.Fprintf(stderr, "claude-usage: %v\n", err)
		return exitUsage
	}
	if len(dirs) == 0 {
		fmt.Fprintf(stderr, "claude-usage: no Claude Code config dirs found under %s (looked for ~/.claude and ~/.claude-*)\n", home)
		return exitUsage
	}

	deps := collectDeps{
		home:   home,
		store:  account.Keychain{},
		client: usage.NewClient(),
		now:    time.Now,
	}
	reports := collect(context.Background(), dirs, cfg.timeout, deps)

	if cfg.jsonOut {
		if err := writeJSON(stdout, reports); err != nil {
			fmt.Fprintf(stderr, "claude-usage: %v\n", err)
			return exitPartial
		}
	} else {
		opt := usage.RenderOptions{Color: wantColor(cfg.color, stdout), Home: home}
		if err := usage.RenderText(stdout, reports, opt); err != nil {
			fmt.Fprintf(stderr, "claude-usage: %v\n", err)
			return exitPartial
		}
	}

	for _, r := range reports {
		if r.Err != nil {
			return exitPartial
		}
	}
	return exitOK
}

// parseFlags parses args; errors have already been printed to stderr.
func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("claude-usage", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usageText)
		fs.PrintDefaults()
	}
	var cfg config
	fs.BoolVar(&cfg.jsonOut, "json", false, "print machine-readable JSON instead of bars")
	fs.BoolVar(&cfg.showVersion, "version", false, "print the version and exit")
	fs.StringVar(&cfg.color, "color", colorAuto, "colour output: auto | always | never")
	fs.DurationVar(&cfg.timeout, "timeout", usage.DefaultTimeout, "per-account fetch timeout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	switch cfg.color {
	case colorAuto, colorAlways, colorNever:
	default:
		fmt.Fprintf(stderr, "claude-usage: --color must be %s, %s or %s (got %q)\n", colorAuto, colorAlways, colorNever, cfg.color)
		return config{}, fmt.Errorf("bad --color %q", cfg.color)
	}
	if cfg.timeout <= 0 {
		fmt.Fprintln(stderr, "claude-usage: --timeout must be positive")
		return config{}, fmt.Errorf("bad --timeout %s", cfg.timeout)
	}
	cfg.dirs = fs.Args()
	return cfg, nil
}

// resolveVersion picks the stamped version, else the module version from the
// build info, else develVersion. readBuildInfo is injected for tests.
func resolveVersion(stamped string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if stamped != "" {
		return stamped
	}
	if bi, ok := readBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return develVersion
}

// resolveDirs returns explicit dirs as absolute, cleaned paths (the form the
// keychain hash is computed over), or discovers them when none were given.
func resolveDirs(explicit []string, home string) ([]string, error) {
	if len(explicit) == 0 {
		return account.Discover(home)
	}
	out := make([]string, 0, len(explicit))
	for _, d := range explicit {
		abs, err := filepath.Abs(d)
		if err != nil {
			return nil, fmt.Errorf("config dir %q: %w", d, err)
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("config dir %q is not a directory", d)
		}
		out = append(out, abs)
	}
	return out, nil
}

// fetcher is the slice of usage.Client that collect needs; tests swap it.
// Implementations must be safe for concurrent use: collect calls Fetch from
// one goroutine per account.
type fetcher interface {
	Fetch(ctx context.Context, accessToken string) (usage.Usage, error)
}

// collectDeps are collect's injected dependencies.
type collectDeps struct {
	home   string
	store  account.SecretStore
	client fetcher
	now    func() time.Time
}

// collect builds one Report per dir, fetching concurrently. Report order
// matches dirs. Failures are per account and never abort the others.
func collect(ctx context.Context, dirs []string, timeout time.Duration, d collectDeps) []usage.Report {
	reports := make([]usage.Report, len(dirs))
	var wg sync.WaitGroup
	for i, dir := range dirs {
		wg.Go(func() { reports[i] = collectOne(ctx, dir, timeout, d) })
	}
	wg.Wait()
	return reports
}

// collectOne resolves one account's identity and token, then fetches usage.
func collectOne(ctx context.Context, dir string, timeout time.Duration, d collectDeps) usage.Report {
	isDefault := account.IsDefaultDir(dir, d.home)
	prof := account.LoadProfile(dir, d.home, isDefault)
	r := usage.Report{ConfigDir: dir, Email: prof.Email, Organization: prof.Organization}

	cred, err := account.ResolveCredential(dir, isDefault, d.store)
	if err != nil {
		r.Err = err
		return r
	}
	r.Plan = usage.PlanLabel(cred.SubscriptionType, cred.RateLimitTier)

	if cred.Expired(d.now()) {
		r.Err = fmt.Errorf("token expired %s — %s", cred.ExpiresAt.Local().Format(time.DateTime), refreshHint(dir, isDefault))
		return r
	}

	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	u, err := d.client.Fetch(fctx, cred.AccessToken)
	if errors.Is(err, usage.ErrUnauthorized) {
		err = fmt.Errorf("%w — %s", err, refreshHint(dir, isDefault))
	}
	r.Usage, r.Err = u, err
	return r
}

// refreshHint tells the user how to get Claude Code to refresh a token.
func refreshHint(dir string, isDefault bool) string {
	if isDefault {
		return "run `claude` once to refresh it"
	}
	return fmt.Sprintf("run `CLAUDE_CONFIG_DIR=%s claude` once to refresh it", dir)
}

// wantColor resolves --color against the output stream and NO_COLOR.
func wantColor(mode string, out io.Writer) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	}
	if _, set := os.LookupEnv(noColorEnv); set {
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// jsonReport is the --json shape of one account. Field names are this tool's
// public output contract; limits keep the upstream field names plus a label.
type jsonReport struct {
	ConfigDir    string          `json:"config_dir"`
	Email        string          `json:"email,omitempty"`
	Organization string          `json:"organization,omitempty"`
	Plan         string          `json:"plan,omitempty"`
	Limits       []jsonLimit     `json:"limits,omitempty"`
	ExtraUsage   json.RawMessage `json:"extra_usage,omitempty"`
	Error        string          `json:"error,omitempty"`
}

// jsonLimit is a usage.Limit plus its rendered label.
type jsonLimit struct {
	Label string `json:"label"`
	usage.Limit
}

// writeJSON prints reports as an indented JSON array.
func writeJSON(w io.Writer, reports []usage.Report) error {
	out := make([]jsonReport, 0, len(reports))
	for _, r := range reports {
		jr := jsonReport{ConfigDir: r.ConfigDir, Email: r.Email, Organization: r.Organization, Plan: r.Plan}
		if r.Err != nil {
			jr.Error = r.Err.Error()
		} else {
			for _, l := range r.Usage.Limits {
				jr.Limits = append(jr.Limits, jsonLimit{Label: l.Label(), Limit: l})
			}
			jr.ExtraUsage = r.Usage.ExtraUsageRaw
		}
		out = append(out, jr)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
