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
	// table prints token logins as one compact row each (--table).
	table bool
	// pick chooses the token logins interactively, then prints the table.
	pick bool
}

// Flags of `agx usage` beyond the shared ones.
const (
	// flagLogins is `agx usage --logins`.
	flagLogins = "logins"
	// flagTable is `agx usage --table`.
	flagTable = "table"
	// flagPick is `agx usage --pick`.
	flagPick = "pick"
)

// pickPrompt heads the --pick checklist.
const pickPrompt = "token logins to probe"

func newUsageCmd(a *app) *cobra.Command {
	o := &usageOpts{}
	c := &cobra.Command{
		Use:   "usage [profile...]",
		Short: "Plan usage for every account (5-hour, weekly, per model)",
		Long: "Shows how much of each plan window is used and when it resets, for every plan-billed\n" +
			"login on this machine (one row set per provider home, even when several profiles share it).\n" +
			"Token logins are shown only when asked (--logins, --login NAME): their usage is read by a\n" +
			"probe, one minimal request that spends a few tokens of that login's quota.\n\n" +
			"--table prints one row per token login (5H USED, WEEKLY USED, LEFT, RESETS, STATUS) instead\n" +
			"of bars; --login a,b or --login all prints that table for the named logins, and --pick lets\n" +
			"you choose them in a checklist (space: mark, a: all/none, enter: show, esc/q: cancel).\n" +
			"Every login in the table is probed, so each one spends a few tokens of its own quota.",
		Example: "  agx                      # same as agx usage\n" +
			"  agx usage work           # one profile\n" +
			"  agx usage --logins       # homes, then one probed row per token login\n" +
			"  agx usage --logins --table   # one compact row per token login (probes each)\n" +
			"  agx usage --login amank,erman  # table for these logins (probes each)\n" +
			"  agx usage --login all\n" +
			"  agx usage --pick         # choose logins in a checklist, then the table\n" +
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
	f.StringVar(&o.login, flagLogin, "", "show this token login's usage (probe) instead of the homes; a comma list or \"all\" prints the table; \"home\" = the default")
	f.BoolVar(&o.logins, flagLogins, false, "add one probed row per token login (each probe spends a few tokens)")
	f.BoolVar(&o.table, flagTable, false, "print token logins as one row each (with --logins, --login, --pick)")
	f.BoolVar(&o.pick, flagPick, false, "choose token logins in a checklist, then print their table (each probe spends a few tokens)")
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
	// Status, StatusReason, LeftPercent and LeftResetsAt are the verdict
	// the --table view shows (usageVerdictOf), on every row, home or login,
	// with or without --table. LeftPercent is absent when there are no
	// windows (never a fake 100).
	Status       usageStatus `json:"status"`
	StatusReason string      `json:"status_reason,omitempty"`
	LeftPercent  *float64    `json:"left_percent,omitempty"`
	LeftResetsAt *time.Time  `json:"left_resets_at,omitempty"`
}

func (a *app) runUsage(ctx context.Context, o *usageOpts, names []string) error {
	if err := validColor(o.color); err != nil {
		return err
	}
	if err := checkTimeout(o.timeout); err != nil {
		return err
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	if tableMode(o) {
		return a.runLoginTable(ctx, cfg, o, names)
	}
	only, err := usageLogin(cfg, o, names)
	if err != nil {
		return err
	}

	targets, err := profilesByName(cfg, names)
	if err != nil {
		return err
	}
	if len(names) == 0 {
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
	if anyFailed(reports) {
		return errPartial
	}
	return nil
}

// anyFailed reports whether a usage row failed. A per-token (api-billed)
// row carries ErrAPIBilled to say "no windows", which is an answer, not a
// failure, so it never turns the exit code to 1.
func anyFailed(reports []render.UsageReport) bool {
	for _, r := range reports {
		if r.Err != nil && !errors.Is(r.Err, provider.ErrAPIBilled) {
			return true
		}
	}
	return false
}

// profilesByName resolves profile-name arguments in the order given; an
// unknown name is a usage error. No names yields nil, which callers read as
// "use their own default set" (every home, every plan home, ...).
func profilesByName(cfg config.Config, names []string) ([]provider.Profile, error) {
	var out []provider.Profile
	for _, n := range names {
		p, ok := cfg.ByName(n)
		if !ok {
			return nil, errUnknownProfile(n)
		}
		out = append(out, p)
	}
	return out, nil
}

// checkTimeout validates a --timeout value: zero or negative would make
// every fetch fail at once, which reads like every account being down.
func checkTimeout(d time.Duration) error {
	if d <= 0 {
		return usageErr(errors.New("--timeout must be positive"))
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
			reports[i].Usage, reports[i].Err = u, withHint(err, p)
		})
	}
	wg.Wait()
	return reports
}

// withHint appends how to fix login problems: the fix for an expired,
// rejected or missing home login is one interactive run on that profile,
// which lets the vendor CLI refresh or log in (agx itself never refreshes).
func withHint(err error, p provider.Profile) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, provider.ErrInferenceOnly):
		return fmt.Errorf("%w — add it as a login and read its usage with `agx usage --login NAME` (a small probe)", err)
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
	v := usageVerdictOf(r)
	j.Status, j.StatusReason, j.LeftPercent, j.LeftResetsAt = v.Status, v.Reason, v.Left, v.ResetsAt
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
		return nil, errUnknownLogin(o.login)
	}
	return &l, nil
}
