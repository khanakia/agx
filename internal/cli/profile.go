package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// homePerm keeps a new home private: it will hold the account's login.
const homePerm os.FileMode = 0o700

// homeSuffixSep joins the default home's name and a profile name, the
// ~/.claude-NAME shape discovery already recognises.
const homeSuffixSep = "-"

func newProfileCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "profile",
		Short: "Create a profile (a home on disk plus its config entry)",
		Long:  "Creates profiles. List them with `agx profiles`.",
		Args:  usageArgs(cobra.NoArgs),
	}
	c.AddCommand(newProfileAddCmd(a))
	return c
}

// profileAddOpts configure `agx profile add`.
type profileAddOpts struct {
	provider, home, login string
	setup, bare, dryRun   bool
}

func newProfileAddCmd(a *app) *cobra.Command {
	o := &profileAddOpts{}
	c := &cobra.Command{
		Use:   "add NAME",
		Short: "Create a home and add it to the config as a profile",
		Long: "Creates the home (default ~/.claude-NAME, or ~/.codex-NAME) and adds the profile to\n" +
			"config.yaml, keeping comments. A new Claude home links CLAUDE.md, skills and settings.json\n" +
			"from ~/.claude when they exist there; nothing is ever overwritten, and a home that already\n" +
			"exists is adopted exactly as it is.",
		Example: "  agx profile add client\n" +
			"  agx profile add client --login work       # spends the work login's quota\n" +
			"  agx profile add client --setup            # then log the new home in\n" +
			"  agx profile add scratch --bare --dry-run",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return a.runProfileAdd(o, args[0]) },
	}
	f := c.Flags()
	f.StringVar(&o.provider, flagProvider, string(provider.Claude), "provider of the profile")
	f.StringVar(&o.home, "home", "", "home folder (default: the provider's default home + -NAME)")
	f.StringVar(&o.login, flagLogin, "", "the profile's login (a name from 'agx login ls', or \"home\")")
	f.BoolVar(&o.setup, "setup", false, "run the vendor's interactive login for the new home")
	f.BoolVar(&o.bare, "bare", false, "do not link shared files from the default home")
	f.BoolVar(&o.dryRun, flagDryRun, false, "print what would happen and change nothing")
	return c
}

func (a *app) runProfileAdd(o *profileAddOpts, name string) error {
	if err := config.ValidProfileName(name); err != nil {
		return usageErr(err)
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	id := provider.ID(o.provider)
	prov, err := a.providerFor(id)
	if err != nil {
		return usageErr(err)
	}
	defaultHome := prov.DefaultHome(a.UserHome)
	home := config.ExpandPath(o.home, a.UserHome)
	if home == "" {
		home = filepath.Join(filepath.Dir(defaultHome), filepath.Base(defaultHome)+homeSuffixSep+name)
	}
	if err := checkNewProfile(cfg, name, id, home); err != nil {
		return err
	}
	spec := provider.Profile{Name: name, Provider: id, Home: home, Billing: provider.BillingPlan}
	if err := a.checkProfileAddLogin(cfg, spec, o.login); err != nil {
		return err
	}
	setup, hasSetup := prov.(provider.HomeSetup)
	if o.setup && !hasSetup {
		return usageErr(fmt.Errorf("--setup is not known for %s", id))
	}
	adopted, err := existingHome(home)
	if err != nil {
		return err
	}
	// Adopted means untouched: even a link that would only add a file is a
	// change to a home the user already arranged.
	var links []provider.HomeLink
	if hasSetup && !o.bare && !adopted && filepath.Clean(home) != filepath.Clean(defaultHome) {
		links = provider.PlanHomeLinks(defaultHome, home, setup.SharedHomeFiles())
	}
	shown := render.ShortenHome(home, a.UserHome)
	step := func(done, planned string) error {
		line := done
		if o.dryRun {
			line = "would " + planned
		}
		_, err := fmt.Fprintln(a.Stdout, line)
		return err
	}

	if adopted {
		if err := step("adopted existing home "+shown+" (contents left as they are)", "adopt existing home "+shown+" (contents left as they are)"); err != nil {
			return err
		}
	} else {
		if !o.dryRun {
			if err := os.MkdirAll(home, homePerm); err != nil {
				return fmt.Errorf("create home: %w", err)
			}
		}
		if err := step("created home "+shown, "create home "+shown); err != nil {
			return err
		}
	}
	if !o.dryRun {
		notes, err := a.editor(cfg).AddProfile(cfg, config.ProfileSpec{Name: name, Provider: string(id), Home: home, Login: o.login})
		for _, n := range notes {
			a.warn(n)
		}
		if err != nil {
			if !adopted {
				// A failed add must not leave an empty home behind; Remove
				// refuses anything that is not empty.
				_ = os.Remove(home)
			}
			return err
		}
		a.loaded = false
	}
	if err := step("added profile "+profileSummary(name, id, shown, o.login), "add profile "+profileSummary(name, id, shown, o.login)); err != nil {
		return err
	}
	if !o.dryRun {
		if err := provider.ApplyHomeLinks(links); err != nil {
			return err
		}
	}
	for _, l := range links {
		var err error
		if l.Skip != "" {
			_, err = fmt.Fprintf(a.Stdout, "skipped %s (%s)\n", l.Name, l.Skip)
		} else {
			err = step("linked "+l.Name+" -> "+render.ShortenHome(l.Source, a.UserHome), "link "+l.Name+" -> "+render.ShortenHome(l.Source, a.UserHome))
		}
		if err != nil {
			return err
		}
	}
	if !o.setup {
		return nil
	}
	cmd := setup.LoginCommand(spec, provider.UnsetEnv(a.Environ(), childSessionEnv))
	line := strings.Join(append([]string{cmd.Path}, cmd.Args...), " ")
	if err := step("running `"+line+"` for "+shown, "run `"+line+"` for "+shown); err != nil {
		return err
	}
	if o.dryRun {
		return nil
	}
	if err := a.RunInteractive(cmd); err != nil {
		return fmt.Errorf("%s: %w — the profile was added; log it in later with `agx run -p %s`", line, err, name)
	}
	return nil
}

// checkNewProfile refuses a name already in use. The exception is a profile
// only discovered on disk at the same home: naming it is how the user adopts
// it into the config, and the editor writes it once.
func checkNewProfile(cfg config.Config, name string, id provider.ID, home string) error {
	p, exists := cfg.ByName(name)
	if !exists {
		return nil
	}
	if p.Source == provider.SourceDiscovered && p.Provider == id && filepath.Clean(p.Home) == filepath.Clean(home) {
		return nil
	}
	return usageErr(fmt.Errorf("profile %q already exists (home %s; see `agx profiles`)", name, p.Home))
}

// checkProfileAddLogin validates --login against the new profile.
func (a *app) checkProfileAddLogin(cfg config.Config, p provider.Profile, name string) error {
	switch name {
	case "", login.Home:
		return nil
	case login.Auto:
		return usageErr(config.ErrAutoLogin)
	}
	l, ok := cfg.LoginByName(name)
	if !ok {
		return errUnknownLogin(name)
	}
	if err := a.loginFits(p, l); err != nil {
		return usageErr(err)
	}
	return nil
}

// existingHome reports whether home is already a folder, refusing anything
// else at that path.
func existingHome(home string) (bool, error) {
	info, err := os.Stat(home)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("home: %w", err)
	case !info.IsDir():
		return false, usageErr(fmt.Errorf("%s exists and is not a folder", home))
	}
	return true, nil
}

// profileSummary is the one-line description of a new profile.
func profileSummary(name string, id provider.ID, home, loginName string) string {
	s := fmt.Sprintf("%s (%s, %s", name, id, home)
	if loginName != "" {
		s += ", login " + loginName
	}
	return s + ")"
}
