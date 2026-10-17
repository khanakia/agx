package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// kindPlanList is the JSON envelope kind of `agx plan --json`.
const kindPlanList = "plan.list"

func newPlanCmd(a *app) *cobra.Command {
	var (
		asJSON    bool
		timeout   time.Duration
		loginName string
	)
	c := &cobra.Command{
		Use:     "plan [profile...]",
		Aliases: []string{"subscription"},
		Short:   "Plan, subscription status and start date for every account",
		Long: "Shows each plan-billed account's plan, subscription status and when the subscription\n" +
			"started. The next renewal date is not shown: the vendors do not give it to the login\n" +
			"agx reads (claude.ai → Settings → Billing has it).",
		Example: "  agx plan\n  agx plan personal\n  agx plan --json | jq '.data[] | {profile, plan: .subscription.plan, status: .subscription.status}'",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkPlanLogin(loginName); err != nil {
				return err
			}
			return a.runPlan(ctxOf(cmd), args, asJSON, timeout)
		},
	}
	c.Flags().BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	c.Flags().DurationVar(&timeout, flagTimeout, defaultUsageTimeout, "per-account fetch timeout")
	bindLoginFlag(c.Flags(), &loginName)
	return c
}

// planRow is one account in `agx plan` (also the plan.list JSON payload).
type planRow struct {
	Profile  string                `json:"profile"`
	Provider provider.ID           `json:"provider"`
	Home     string                `json:"home"`
	Email    string                `json:"email,omitempty"`
	Sub      provider.Subscription `json:"subscription"`
	Error    string                `json:"error,omitempty"`
}

func (a *app) runPlan(ctx context.Context, names []string, asJSON bool, timeout time.Duration) error {
	if timeout <= 0 {
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
			if p.Billing == provider.BillingPlan {
				targets = append(targets, p)
			}
		}
	}
	if len(targets) == 0 {
		return usageErr(errors.New("no plan-billed profiles (run `agx doctor`)"))
	}

	rows := make([]planRow, len(targets))
	var wg sync.WaitGroup
	for i, p := range targets {
		wg.Go(func() { rows[i] = a.planFor(ctx, p, timeout) })
	}
	wg.Wait()

	if asJSON {
		if err := output.JSON(a.Stdout, kindPlanList, rows, len(rows)); err != nil {
			return err
		}
	} else if err := a.printPlan(rows); err != nil {
		return err
	}
	for _, r := range rows {
		if r.Error != "" {
			return errPartial
		}
	}
	return nil
}

// planFor fetches one account's subscription.
func (a *app) planFor(ctx context.Context, p provider.Profile, timeout time.Duration) planRow {
	row := planRow{Profile: p.Name, Provider: p.Provider, Home: p.Home}
	prov, err := a.providerFor(p.Provider)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	if id, err := prov.Identity(p); err == nil {
		row.Email = id.Email
	}
	reader, ok := prov.(provider.SubscriptionReader)
	if !ok {
		row.Error = fmt.Sprintf("%s does not report subscriptions", p.Provider)
		return row
	}
	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sub, err := reader.Subscription(fctx, p)
	if err != nil {
		row.Error = withHint(err, p, a.UserHome).Error()
		return row
	}
	row.Sub = sub
	return row
}

// printPlan renders the table.
func (a *app) printPlan(rows []planRow) error {
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		account := r.Email
		if account == "" {
			account = cellNo
		}
		if r.Error != "" {
			table = append(table, []string{r.Profile, string(r.Provider), account, errorCell, "", ""})
			continue
		}
		since := cellNo
		if r.Sub.StartedAt != nil {
			since = r.Sub.StartedAt.Local().Format(time.DateOnly)
		}
		table = append(table, []string{r.Profile, string(r.Provider), account, orCell(r.Sub.Plan), orCell(r.Sub.Status), since})
	}
	var b strings.Builder
	if err := render.Table(&b, []string{"PROFILE", "PROVIDER", "ACCOUNT", "PLAN", "STATUS", "SINCE"}, table); err != nil {
		return err
	}
	// Errors go under the table: long messages would stretch every column.
	for _, r := range rows {
		if r.Error != "" {
			fmt.Fprintf(&b, "\n%s: %s", r.Profile, r.Error)
		}
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
	_, err := io.WriteString(a.Stdout, b.String())
	return err
}

// errorCell marks a failed row; the message is printed under the table.
const errorCell = "error (see below)"

func orCell(s string) string {
	if s == "" {
		return cellNo
	}
	return s
}

// errPlanLogin refuses a token login on `agx plan`: the plan comes from
// the profile endpoint, which refuses setup-tokens (logins spec §12.1 V2),
// and the usage probe carries no plan.
var errPlanLogin = errors.New("plan cannot show a token login: the profile endpoint refuses setup-tokens; use --login home or no --login (see a login's usage with `agx usage --login NAME`)")

// checkPlanLogin accepts --login only as "home" (today's behaviour).
func checkPlanLogin(name string) error {
	switch name {
	case "", login.Home:
		return nil
	case login.Auto:
		return usageErr(config.ErrAutoLogin)
	}
	return usageErr(errPlanLogin)
}
