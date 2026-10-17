package cli

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/pick"
	"github.com/khanakia/agx/provider"
)

// Token-login usage (logins spec §8, §14 D14). A token login's usage is read
// by a probe that spends a few tokens of its plan quota, so every function
// here runs only on an explicit request: `agx usage --logins`, `agx usage
// --login NAME`, `--login auto` and `agx login check`. A plain `agx` /
// `agx usage` never reaches this file (TestLoginUsage_NeverProbesUnasked).

// loginSrcAuto marks a login chosen by `--login auto`.
const loginSrcAuto = "--login auto"

// usageSourceProbe marks a usage.list row read by a probe, so --json callers
// can tell it from a home row (which costs nothing).
const usageSourceProbe = "probe"

// errNotReadable is a token login whose provider cannot read its usage.
var errNotReadable = errors.New("usage not readable for this login's provider")

// probeLogin resolves l's token and probes its usage. The token never
// leaves this function. Billing follows the kind: an api-key login is
// reported billed per token without a probe.
func (a *app) probeLogin(ctx context.Context, l login.Login, timeout time.Duration) render.UsageReport {
	r := render.UsageReport{
		Profile: provider.Profile{Name: l.Name, Provider: l.Provider, Billing: provider.BillingPlan},
		Login:   l.Name, Account: l.Account,
	}
	if l.Kind == provider.LoginAPIKey {
		r.Profile.Billing = provider.BillingAPI
	}
	prov, err := a.providerFor(l.Provider)
	if err != nil {
		r.Err = err
		return r
	}
	reader, ok := prov.(provider.LoginUsageReader)
	if !ok {
		r.Err = errNotReadable
		return r
	}
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	val, err := a.resolveLoginValue(pctx, l)
	if err != nil {
		r.Err = err
		return r
	}
	u, id, err := reader.LoginUsage(pctx, l.Kind, val)
	u.Identity = id
	r.Usage, r.Err = u, err
	return r
}

// probeLogins probes every login concurrently; order matches ls. One
// failing login never aborts the others.
func (a *app) probeLogins(ctx context.Context, ls []login.Login, timeout time.Duration) []render.UsageReport {
	reports := make([]render.UsageReport, len(ls))
	var wg sync.WaitGroup
	for i, l := range ls {
		wg.Go(func() { reports[i] = a.probeLogin(ctx, l, timeout) })
	}
	wg.Wait()
	return reports
}

// autoLogin is `--login auto` for profile p: the candidates are the home's
// own login (the free usage endpoint) and every login of p's provider that
// p can run on (one probe each); the pick policy chooses, unchanged, and the
// decision is explained on stderr. Ties go to the home, then config order.
func (a *app) autoLogin(ctx context.Context, cfg config.Config, p provider.Profile) (loginChoice, error) {
	var fits []login.Login
	for _, l := range cfg.Logins {
		if l.Provider == p.Provider && a.loginFits(p, l) == nil {
			fits = append(fits, l)
		}
	}
	var (
		wg      sync.WaitGroup
		home    []render.UsageReport
		probed  []render.UsageReport
		homeRow = provider.Profile{Name: login.Home, Provider: p.Provider, Home: p.Home, Billing: p.Billing}
	)
	wg.Go(func() { home = a.fetchUsage(ctx, []provider.Profile{p}, defaultUsageTimeout) })
	wg.Go(func() { probed = a.probeLogins(ctx, fits, defaultUsageTimeout) })
	wg.Wait()

	cands := make([]pick.Candidate, 0, 1+len(probed))
	cands = append(cands, pick.Candidate{Profile: homeRow, Usage: home[0].Usage, Err: home[0].Err})
	for _, r := range probed {
		cands = append(cands, pick.Candidate{Profile: r.Profile, Usage: r.Usage, Err: r.Err})
	}
	res, err := pick.Best(cands)
	if err != nil {
		return loginChoice{}, fmt.Errorf("--login auto for profile %s: %w", p.Name, err)
	}
	a.warn("login " + res.Explain())
	if res.Chosen.Name == login.Home {
		return loginChoice{Source: loginSrcAuto}, nil
	}
	for i := range fits {
		if fits[i].Name == res.Chosen.Name {
			return loginChoice{Login: &fits[i], Source: loginSrcAuto}, nil
		}
	}
	// Unreachable: Best only chooses among the candidates built above.
	return loginChoice{}, fmt.Errorf("--login auto chose unknown candidate %q", res.Chosen.Name)
}
