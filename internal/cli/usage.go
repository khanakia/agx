package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// JSON envelope kinds (spec §11).
const (
	kindUsageList        = "usage.list"
	kindProfileList      = "profile.list"
	kindConversationList = "conversation.list"
	kindWorkspaceList    = "workspace.list"
	kindSessionList      = "session.list"
	kindSessionGC        = "session.gc"
	kindSessionPromote   = "session.promote"
	kindDoctorReport     = "doctor.report"
)

// Colour modes for --color.
const (
	colorAuto   = "auto"
	colorAlways = "always"
	colorNever  = "never"
)

// noColorEnv is the https://no-color.org convention.
const noColorEnv = "NO_COLOR"

// defaultUsageTimeout bounds one account's fetch.
const defaultUsageTimeout = 15 * time.Second

type usageOpts struct {
	json     bool
	color    string
	timeout  time.Duration
	provider string
	login    string
	// logins adds one probed row per token login (logins spec §14 D14).
	logins bool
}

// flagLogins is `agx usage --logins`.
const flagLogins = "logins"

func newUsageCmd(a *app) *cobra.Command {
	o := &usageOpts{}
	c := &cobra.Command{
		Use:   "usage [profile...]",
		Short: "Plan usage for every account (5-hour, weekly, per model)",
		Long: "Shows how much of each plan window is used and when it resets, for every plan-billed\n" +
			"login on this machine (one row set per provider home, even when several profiles share it).\n" +
			"Token logins are shown only when asked (--logins, --login NAME): their usage is read by a\n" +
			"probe, one minimal request that spends a few tokens of that login's quota.",
		Example: "  agx                      # same as agx usage\n" +
			"  agx usage work           # one profile\n" +
			"  agx usage --logins       # homes, then one probed row per token login\n" +
			"  agx usage --login personal\n" +
			"  agx usage --provider codex\n" +
			"  agx usage --json | jq '.data[] | {profile, windows: [.windows[] | {label, percent}]}'",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.runUsage(ctxOf(cmd), o, args) },
	}
	f := c.Flags()
	f.BoolVar(&o.json, flagJSON, false, "print the JSON envelope instead of bars")
	f.StringVar(&o.color, flagColor, colorAuto, "colour: auto | always | never")
	f.DurationVar(&o.timeout, flagTimeout, defaultUsageTimeout, "per-account fetch timeout")
	f.StringVar(&o.provider, flagProvider, "", "only this provider (claude, codex)")
	f.StringVar(&o.login, flagLogin, "", "show this token login's usage (probe) instead of the homes; \"home\" = the default")
	f.BoolVar(&o.logins, flagLogins, false, "add one probed row per token login (each probe spends a few tokens)")
	return c
}

// usageJSON is one entry of the usage.list payload.
type usageJSON struct {
	Profile      string            `json:"profile,omitempty"`
	Provider     provider.ID       `json:"provider"`
	Home         string            `json:"home,omitempty"`
	Billing      provider.Billing  `json:"billing"`
	Email        string            `json:"email,omitempty"`
	Organization string            `json:"organization,omitempty"`
	Plan         string            `json:"plan,omitempty"`
	Windows      []provider.Window `json:"windows,omitempty"`
	Extra        json.RawMessage   `json:"extra,omitempty"`
	Error        string            `json:"error,omitempty"`
	// Login, Account and Source are set only on a token-login row:
	// Source is "probe" (it cost a few tokens), Account the unverified
	// label from config. Profile and Home are empty on such a row.
	Login   string `json:"login,omitempty"`
	Account string `json:"account,omitempty"`
	Source  string `json:"source,omitempty"`
}

func (a *app) runUsage(ctx context.Context, o *usageOpts, names []string) error {
	if err := validColor(o.color); err != nil {
		return err
	}
	if o.timeout <= 0 {
		return usageErr(errors.New("--timeout must be positive"))
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	only, err := usageLogin(cfg, o, names)
	if err != nil {
		return err
	}

	var targets []provider.Profile
	if len(names) > 0 {
		for _, n := range names {
			p, ok := cfg.ByName(n)
			if !ok {
				return usageErr(fmt.Errorf("unknown profile %q (see `agx profiles`)", n))
			}
			targets = append(targets, p)
		}
	} else {
		for _, p := range cfg.Homes() {
			if o.provider == "" || string(p.Provider) == o.provider {
				targets = append(targets, p)
			}
		}
	}
	var probes []login.Login
	switch {
	case only != nil:
		targets, probes = nil, []login.Login{*only}
	case o.logins:
		for _, l := range cfg.Logins {
			if o.provider == "" || string(l.Provider) == o.provider {
				probes = append(probes, l)
			}
		}
	}
	if len(targets) == 0 && len(probes) == 0 {
		return usageErr(errors.New("no profiles to show (run `agx doctor`)"))
	}

	reports := a.fetchUsage(ctx, targets, o.timeout)
	reports = append(reports, a.probeLogins(ctx, probes, o.timeout)...)

	if o.json {
		out := make([]usageJSON, 0, len(reports))
		for _, r := range reports {
			out = append(out, toUsageJSON(r))
		}
		if err := output.JSON(a.Stdout, kindUsageList, out, len(out)); err != nil {
			return err
		}
	} else {
		opt := render.Options{Now: a.Now(), Color: a.wantColor(o.color), Home: a.UserHome}
		if err := render.Usage(a.Stdout, reports, opt); err != nil {
			return err
		}
	}
	for _, r := range reports {
		if r.Err != nil && !errors.Is(r.Err, provider.ErrAPIBilled) {
			return errPartial
		}
	}
	return nil
}

// fetchUsage fetches every target concurrently; report order matches
// targets. One failing account never aborts the others.
func (a *app) fetchUsage(ctx context.Context, targets []provider.Profile, timeout time.Duration) []render.UsageReport {
	reports := make([]render.UsageReport, len(targets))
	var wg sync.WaitGroup
	for i, p := range targets {
		wg.Go(func() {
			reports[i] = render.UsageReport{Profile: p}
			prov, err := a.providerFor(p.Provider)
			if err != nil {
				reports[i].Err = err
				return
			}
			fctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			u, err := prov.Usage(fctx, p)
			reports[i].Usage, reports[i].Err = u, withHint(err, p, a.UserHome)
		})
	}
	wg.Wait()
	return reports
}

// withHint appends how to fix login problems.
func withHint(err error, p provider.Profile, userHome string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, provider.ErrExpired), errors.Is(err, provider.ErrUnauthorized), errors.Is(err, provider.ErrNotLoggedIn):
		return fmt.Errorf("%w — run `agx run -p %s` once to log in / refresh", err, p.Name)
	default:
		return err
	}
}

func toUsageJSON(r render.UsageReport) usageJSON {
	j := usageJSON{
		Profile: r.Profile.Name, Provider: r.Profile.Provider, Home: r.Profile.Home, Billing: r.Profile.Billing,
		Email: r.Usage.Identity.Email, Organization: r.Usage.Identity.Organization, Plan: r.Usage.Identity.Plan,
	}
	if r.Login != "" {
		j.Profile, j.Home = "", ""
		j.Login, j.Account, j.Source = r.Login, r.Account, usageSourceProbe
	}
	if r.Err != nil {
		j.Error = r.Err.Error()
		return j
	}
	j.Windows = r.Usage.Windows
	if len(r.Usage.Extra) > 0 {
		j.Extra = json.RawMessage(r.Usage.Extra)
	}
	return j
}

func validColor(mode string) error {
	switch mode {
	case colorAuto, colorAlways, colorNever:
		return nil
	}
	return usageErr(fmt.Errorf("--color must be %s, %s or %s (got %q)", colorAuto, colorAlways, colorNever, mode))
}

// wantColor resolves --color against the output stream and NO_COLOR.
func (a *app) wantColor(mode string) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	}
	if _, set := a.LookupEnv(noColorEnv); set {
		return false
	}
	return a.IsTerminal(a.Stdout)
}

// usageLogin validates --login / --logins and returns the one login to show
// for `--login NAME` (nil otherwise: no flag, or "home", which is the plain
// home view).
func usageLogin(cfg config.Config, o *usageOpts, names []string) (*login.Login, error) {
	switch o.login {
	case "", login.Home:
		return nil, nil
	case login.Auto:
		return nil, usageErr(errors.New("--login auto chooses a login for a launch; to compare every login use `agx usage --logins`"))
	}
	if o.logins || len(names) > 0 {
		return nil, usageErr(fmt.Errorf("--login %s shows one login; it cannot be combined with --logins or profile names", o.login))
	}
	l, ok := cfg.LoginByName(o.login)
	if !ok {
		return nil, usageErr(fmt.Errorf("unknown login %q (see `agx login ls`)", o.login))
	}
	return &l, nil
}
