package cli

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
)

// kindWhoamiReport is the JSON envelope kind of `agx whoami --json`.
const kindWhoamiReport = "whoami.report"

// Session states `agx whoami` reports, decided by how many AGX_RUN_*
// variables (appmeta) are set: all, some or none.
const (
	sessionAgx     = "agx"
	sessionPartial = "partial"
	sessionNone    = "none"
)

// Where a provider's home comes from, and what is known about its token
// login variables, in `agx whoami`.
const (
	homeFromDefault     = "default"
	tokenPresent        = "present"
	tokenAbsent         = "absent"
	tokenUnsupported    = "unsupported"
	noValue             = "-"
	notInsideAgxSession = "not inside a session started by agx"
)

// whoamiJSON is the whoami.report payload. It names variables and paths
// only: a token variable is reported as present or absent, never its value.
type whoamiJSON struct {
	// Session is "agx" when every AGX_RUN_* variable is set, "partial" when
	// only some are, "none" outside a session agx started.
	Session string `json:"session"`
	Profile string `json:"profile,omitempty"`
	Login   string `json:"login,omitempty"`
	Home    string `json:"home,omitempty"`
	// Missing names the AGX_RUN_* variables that are unset (partial only).
	Missing []string `json:"missing,omitempty"`
	// Providers is what the environment itself implies, per provider.
	Providers []whoamiProvider `json:"providers"`
}

// whoamiProvider is one provider as the current environment selects it.
type whoamiProvider struct {
	Provider provider.ID `json:"provider"`
	// Home is the home the vendor CLI would use here; HomeFrom is the
	// variable that chose it, or "default" when none is set.
	Home     string `json:"home"`
	HomeFrom string `json:"home_from"`
	// TokenLogin is "present" when a variable a login sets is in the
	// environment, "absent" when none is, "unsupported" for a provider that
	// cannot run on logins. TokenVars names the present variables.
	TokenLogin string   `json:"token_login"`
	TokenVars  []string `json:"token_vars,omitempty"`
}

func newWhoamiCmd(a *app) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "whoami",
		Short: "Which profile, login and home this session runs on",
		Long: "Reads the AGX_RUN_PROFILE / AGX_RUN_LOGIN / AGX_RUN_HOME variables agx sets on every\n" +
			"agent it launches, so a shell or status line inside a session can tell what launched it.\n" +
			"Outside such a session it says so and shows what the environment implies. Token\n" +
			"variables are reported as present or absent, never their values. No config, no network.",
		Example: "  agx whoami\n  agx whoami --json",
		Args:    usageArgs(cobra.NoArgs),
		RunE:    func(*cobra.Command, []string) error { return a.runWhoami(asJSON) },
	}
	c.Flags().BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

// runWhoami reports the current environment; it never loads the config, so
// it works in any shell, with or without one.
func (a *app) runWhoami(asJSON bool) error {
	r := a.whoami()
	if asJSON {
		return output.JSON(a.Stdout, kindWhoamiReport, r, len(r.Providers))
	}
	var b strings.Builder
	switch r.Session {
	case sessionAgx:
		b.WriteString("session:  started by " + appmeta.Name + "\n")
	case sessionPartial:
		fmt.Fprintf(&b, "session:  partial: %s not set (not a complete %s launch)\n", strings.Join(r.Missing, ", "), appmeta.Name)
	default:
		b.WriteString("session:  " + notInsideAgxSession + "\n")
	}
	if r.Session != sessionNone {
		fmt.Fprintf(&b, "profile:  %s\nlogin:    %s\nhome:     %s\n", orDash(r.Profile), orDash(r.Login), orDash(render.ShortenHome(r.Home, a.UserHome)))
	}
	for _, p := range r.Providers {
		token := "token login " + p.TokenLogin
		switch p.TokenLogin {
		case tokenPresent:
			token += " (" + strings.Join(p.TokenVars, ", ") + ")"
		case tokenUnsupported:
			token = "no token logins"
		}
		fmt.Fprintf(&b, "%-*s %s (%s) · %s\n", whoamiLabelWidth, string(p.Provider)+":", render.ShortenHome(p.Home, a.UserHome), p.HomeFrom, token)
	}
	_, err := io.WriteString(a.Stdout, b.String())
	return err
}

// whoamiLabelWidth aligns the provider rows with the "profile:  " labels.
const whoamiLabelWidth = 9

func orDash(s string) string {
	if s == "" {
		return noValue
	}
	return s
}

// whoami collects the report from the environment alone.
func (a *app) whoami() whoamiJSON {
	var r whoamiJSON
	set := 0
	for _, v := range []struct {
		name string
		dst  *string
	}{
		{appmeta.EnvRunProfile, &r.Profile},
		{appmeta.EnvRunLogin, &r.Login},
		{appmeta.EnvRunHome, &r.Home},
	} {
		if val, ok := a.LookupEnv(v.name); ok && val != "" {
			*v.dst = val
			set++
		} else {
			r.Missing = append(r.Missing, v.name)
		}
	}
	switch {
	case len(r.Missing) == 0:
		r.Session = sessionAgx
	case set > 0:
		r.Session = sessionPartial
	default:
		r.Session, r.Missing = sessionNone, nil
	}
	r.Providers = make([]whoamiProvider, 0, len(a.Providers))
	for _, p := range a.Providers {
		r.Providers = append(r.Providers, a.whoamiProvider(p))
	}
	return r
}

// whoamiProvider reads p's home variable and the variables its logins set.
func (a *app) whoamiProvider(p provider.Provider) whoamiProvider {
	w := whoamiProvider{Provider: p.ID(), Home: p.DefaultHome(a.UserHome), HomeFrom: homeFromDefault}
	if n, ok := p.(provider.HomeEnvNamer); ok {
		if v, ok := a.LookupEnv(n.HomeEnvName()); ok && v != "" {
			w.Home, w.HomeFrom = filepath.Clean(v), n.HomeEnvName()
		}
	}
	ll, ok := p.(provider.LoginLauncher)
	if !ok {
		w.TokenLogin = tokenUnsupported
		return w
	}
	for _, name := range loginVarNames(ll) {
		if v, ok := a.LookupEnv(name); ok && v != "" {
			w.TokenVars = append(w.TokenVars, name)
		}
	}
	w.TokenLogin = tokenAbsent
	if len(w.TokenVars) > 0 {
		w.TokenLogin = tokenPresent
	}
	return w
}

// loginVarNames is every variable a login of any kind sets or clears, sorted:
// the credential variables the vendor CLI reads. Asked with an empty value,
// so no token is ever resolved.
func loginVarNames(ll provider.LoginLauncher) []string {
	names := map[string]bool{}
	for _, k := range ll.LoginKinds() {
		set, unset := ll.LoginEnv(k, "")
		for n := range set {
			names[n] = true
		}
		for _, n := range unset {
			names[n] = true
		}
	}
	return slices.Sorted(maps.Keys(names))
}
