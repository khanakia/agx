package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/khanakia/voltkit/output"
	"github.com/spf13/cobra"

	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
)

// JSON envelope kinds (spec §11).
const (
	kindUsageList        = "usage.list"
	kindProfileList      = "profile.list"
	kindConversationList = "conversation.list"
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
}

func newUsageCmd(a *app) *cobra.Command {
	o := &usageOpts{}
	c := &cobra.Command{
		Use:   "usage [profile...]",
		Short: "Plan usage for every account (5-hour, weekly, per model)",
		Long: "Shows how much of each plan window is used and when it resets, for every plan-billed\n" +
			"login on this machine (one row set per provider home, even when several profiles share it).",
		Example: "  agx                      # same as agx usage\n" +
			"  agx usage work           # one profile\n" +
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
	return c
}

// usageJSON is one entry of the usage.list payload.
type usageJSON struct {
	Profile      string            `json:"profile"`
	Provider     provider.ID       `json:"provider"`
	Home         string            `json:"home"`
	Billing      provider.Billing  `json:"billing"`
	Email        string            `json:"email,omitempty"`
	Organization string            `json:"organization,omitempty"`
	Plan         string            `json:"plan,omitempty"`
	Windows      []provider.Window `json:"windows,omitempty"`
	Extra        json.RawMessage   `json:"extra,omitempty"`
	Error        string            `json:"error,omitempty"`
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
	if len(targets) == 0 {
		return usageErr(errors.New("no profiles to show (run `agx doctor`)"))
	}

	reports := a.fetchUsage(ctx, targets, o.timeout)

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
