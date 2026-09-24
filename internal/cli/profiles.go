package cli

import (
	"slices"
	"time"

	"github.com/khanakia/voltkit/output"
	"github.com/spf13/cobra"

	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
)

// Login states shown by `agx profiles`.
const (
	loginOK      = "ok"
	loginExpired = "expired"
	loginNone    = "none"
	loginError   = "error"
	loginAPI     = "api key"
	markDefault  = "*"
)

func newProfilesCmd(a *app) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "profiles",
		Aliases: []string{"who", "accounts"},
		Short:   "Every profile: provider, home, account, plan, login state",
		Long: "Lists every profile agx knows — configured in ~/.agx/config.yaml or discovered on disk —\n" +
			"with the account it is logged in as. Reads local files only; no network.",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return a.runProfiles(asJSON) },
	}
	c.Flags().BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

// profileJSON is one entry of the profile.list payload.
type profileJSON struct {
	Name         string           `json:"name"`
	Provider     provider.ID      `json:"provider"`
	Home         string           `json:"home"`
	Billing      provider.Billing `json:"billing"`
	Default      bool             `json:"default"`
	Source       provider.Source  `json:"source"`
	Email        string           `json:"email,omitempty"`
	Organization string           `json:"organization,omitempty"`
	Plan         string           `json:"plan,omitempty"`
	Login        string           `json:"login"`
	ExpiresAt    *time.Time       `json:"expires_at,omitempty"`
	Credential   string           `json:"credential,omitempty"`
	Args         []string         `json:"args,omitempty"`
	// SecretVars lists the env var names filled from secrets (never values).
	SecretVars []string `json:"secret_vars,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func (a *app) runProfiles(asJSON bool) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	now := a.Now()
	rows := make([]profileJSON, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		rows = append(rows, a.describeProfile(p, now))
	}
	if asJSON {
		return output.JSON(a.Stdout, kindProfileList, rows, len(rows))
	}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		def := ""
		if r.Default {
			def = markDefault
		}
		login := r.Login
		if r.Login == loginOK && r.ExpiresAt != nil {
			login += " (" + render.FormatUntil(r.ExpiresAt.Sub(now)) + ")"
		}
		account := r.Email
		if r.Plan != "" {
			account += " · " + r.Plan
		}
		table = append(table, []string{def + r.Name, string(r.Provider), render.ShortenHome(r.Home, a.UserHome), account, login, string(r.Billing), string(r.Source)})
	}
	return render.Table(a.Stdout, []string{"PROFILE", "PROVIDER", "HOME", "ACCOUNT", "LOGIN", "BILLING", "SOURCE"}, table)
}

// describeProfile reads one profile's local login state.
func (a *app) describeProfile(p provider.Profile, now time.Time) profileJSON {
	r := profileJSON{
		Name: p.Name, Provider: p.Provider, Home: p.Home, Billing: p.Billing,
		Default: p.Default, Source: p.Source, Args: p.Args,
	}
	for k := range p.Secrets {
		r.SecretVars = append(r.SecretVars, k)
	}
	slices.Sort(r.SecretVars)
	prov, err := a.providerFor(p.Provider)
	if err != nil {
		r.Login, r.Error = loginError, err.Error()
		return r
	}
	id, err := prov.Identity(p)
	r.Email, r.Organization, r.Plan, r.Credential = id.Email, id.Organization, id.Plan, id.CredentialSource
	switch {
	case err != nil:
		r.Login, r.Error = loginError, err.Error()
	case p.Billing == provider.BillingAPI && !id.LoggedIn:
		r.Login = loginAPI
	case !id.LoggedIn:
		r.Login = loginNone
	case !id.ExpiresAt.IsZero() && !now.Before(id.ExpiresAt):
		r.Login = loginExpired
	default:
		r.Login = loginOK
	}
	if !id.ExpiresAt.IsZero() {
		t := id.ExpiresAt
		r.ExpiresAt = &t
	}
	return r
}
