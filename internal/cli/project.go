package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
)

// JSON envelope kinds of `agx project`.
const (
	kindProjectList = "project.list"
	kindProjectShow = "project.show"
)

// Where a profile choice came from, shown by `agx project show`.
const (
	profileSrcFlag    = "--profile"
	profileSrcDefault = "default profile"
)

// srcBinding is the source text of a choice made by a project binding; the
// bound path follows it.
const srcBinding = "project binding at "

// projectRowOK is the STATE of a usable binding in `agx project ls`.
const projectRowOK = "ok"

// useProject records the binding covering dir as the one this invocation's
// launch answers to. A launch calls it with the folder that decides the
// binding, which is not always the folder it runs in: `new` passes where it
// was invoked, `resume` the conversation's folder. `exec` never calls it,
// because the plan already carries what the binding chose.
func (a *app) useProject(cfg config.Config, dir string) {
	a.project, _ = cfg.ProjectFor(config.CanonicalPath(dir))
}

// errBrokenBinding is the error for a binding that cannot be used. It never
// falls back: the folder was bound on purpose, so silently running on another
// account is the one outcome that must not happen.
func errBrokenBinding(pr config.Project, problem string) error {
	return usageErr(errors.New(bindingFix(pr, problem)))
}

// bindingFix words a broken binding and how to fix it; doctor shares it.
func bindingFix(pr config.Project, problem string) string {
	return fmt.Sprintf("project binding %s: %s — remove it with `agx project rm %s`, or run `agx project use` in that folder", pr.Path, problem, pr.Path)
}

// profileGone and loginGone word a binding's dangling references.
func profileGone(name string) string {
	return fmt.Sprintf("profile %q no longer exists", name)
}

func loginGone(name string) string {
	return fmt.Sprintf("login %q no longer exists", name)
}

// projectLogin is the binding's step of the login precedence. A login that
// does not fit is an error only when the binding's own profile is running.
// Under another profile (`-p kimi` in a bound folder) it is skipped with a
// note, the rule AGX_LOGIN follows, so a binding never breaks a run the user
// pointed somewhere else by hand.
func (a *app) projectLogin(cfg config.Config, p provider.Profile) (loginChoice, string, error) {
	pr := a.project
	src := srcBinding + pr.Path
	if pr.Login == login.Home {
		return loginChoice{Source: src}, "", nil
	}
	l, ok := cfg.LoginByName(pr.Login)
	if !ok {
		return loginChoice{}, "", errBrokenBinding(pr, loginGone(pr.Login))
	}
	if err := a.loginFits(p, l); err != nil {
		if p.Name == pr.Profile {
			return loginChoice{}, "", errBrokenBinding(pr, err.Error())
		}
		return a.defaultLogin(cfg, p), fmt.Sprintf("project %s: login %s skipped for profile %s: %v", pr.Path, l.Name, p.Name, err), nil
	}
	return loginChoice{Login: &l, Source: src}, "", nil
}

// bindingLogin is the login step after AGX_LOGIN: the binding when one names
// a login, otherwise the profile and provider defaults.
func (a *app) bindingLogin(cfg config.Config, p provider.Profile) (loginChoice, string, error) {
	if a.project.Login != "" {
		return a.projectLogin(cfg, p)
	}
	return a.defaultLogin(cfg, p), "", nil
}

// bindingProblems lists why pr cannot be used as written.
func (a *app) bindingProblems(cfg config.Config, pr config.Project) []string {
	var out []string
	var p provider.Profile
	hasProfile := false
	if pr.Profile != "" {
		if p, hasProfile = cfg.ByName(pr.Profile); !hasProfile {
			out = append(out, profileGone(pr.Profile))
		}
	}
	if pr.Login == "" || pr.Login == login.Home {
		return out
	}
	l, ok := cfg.LoginByName(pr.Login)
	if !ok {
		return append(out, loginGone(pr.Login))
	}
	if hasProfile {
		if err := a.loginFits(p, l); err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}

// bindingSummary words what a binding sets.
func bindingSummary(pr config.Project) string {
	var parts []string
	if pr.Profile != "" {
		parts = append(parts, "profile "+pr.Profile)
	}
	if pr.Login != "" {
		parts = append(parts, "login "+pr.Login)
	}
	return strings.Join(parts, ", ")
}

func newProjectCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "project",
		Short: "Bind a folder to a profile and/or login",
		Long: "A binding makes every run in a folder, and in the folders under it, use a profile, a login\n" +
			"or both, without flags. It is stored in config.yaml, keyed by path, never inside the repo.\n" +
			"An explicit -p or --login still wins, and a bare `agx` in a bound folder starts the agent.",
		Example: "  agx project use -p client --login work\n" +
			"  agx project use --login work     # keep the home, change whose quota it spends\n" +
			"  agx project show\n" +
			"  agx project rm",
		Args: usageArgs(cobra.NoArgs),
	}
	c.AddCommand(newProjectUseCmd(a), newProjectShowCmd(a), newProjectLsCmd(a), newProjectRmCmd(a))
	return c
}

func newProjectUseCmd(a *app) *cobra.Command {
	var profile, loginName string
	c := &cobra.Command{
		Use:   "use [-p PROFILE] [--login LOGIN]",
		Short: "Bind the current folder (replaces its binding)",
		Example: "  agx project use -p client\n" +
			"  agx project use --login work\n" +
			"  agx project use -p client --login home",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return a.runProjectUse(profile, loginName) },
	}
	c.Flags().StringVarP(&profile, flagProfile, "p", "", "profile for this folder")
	c.Flags().StringVar(&loginName, flagLogin, "", "login for this folder (a name from 'agx login ls', or \"home\" for the home's own login)")
	return c
}

func (a *app) runProjectUse(profile, loginName string) error {
	if profile == "" && loginName == "" {
		return usageErr(errors.New("give -p PROFILE, --login LOGIN, or both"))
	}
	if loginName == login.Auto {
		return usageErr(config.ErrAutoLogin)
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	var p provider.Profile
	if profile != "" {
		var ok bool
		if p, ok = cfg.ByName(profile); !ok {
			return errUnknownProfile(profile)
		}
	}
	if loginName != "" && loginName != login.Home {
		l, ok := cfg.LoginByName(loginName)
		if !ok {
			return errUnknownLogin(loginName)
		}
		if profile != "" {
			if err := a.loginFits(p, l); err != nil {
				return usageErr(err)
			}
		}
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	pr := config.Project{Path: config.CanonicalPath(dir), Profile: profile, Login: loginName}
	before, replaced := exactProject(cfg, pr.Path)
	if err := a.editor(cfg).SetProject(pr); err != nil {
		return err
	}
	a.loaded = false
	line := fmt.Sprintf("bound %s -> %s", pr.Path, bindingSummary(pr))
	if replaced {
		line += " (was " + bindingSummary(before) + ")"
	}
	_, err = fmt.Fprintln(a.Stdout, line)
	return err
}

// exactProject returns the binding stored for exactly path.
func exactProject(cfg config.Config, path string) (config.Project, bool) {
	for _, pr := range cfg.Projects {
		if pr.Path == path {
			return pr, true
		}
	}
	return config.Project{}, false
}

// projectShow is the project.show payload. It holds names and paths only; a
// token never reaches it.
type projectShow struct {
	Folder        string          `json:"folder"`
	Project       *config.Project `json:"project"`
	Profile       string          `json:"profile"`
	ProfileSource string          `json:"profile_source"`
	Provider      provider.ID     `json:"provider"`
	Home          string          `json:"home"`
	HomeSource    string          `json:"home_source"`
	Login         string          `json:"login"`
	LoginSource   string          `json:"login_source"`
}

func newProjectShowCmd(a *app) *cobra.Command {
	o := &launchOpts{}
	var asJSON bool
	c := &cobra.Command{
		Use:   "show [-p PROFILE] [--login LOGIN]",
		Short: "Show what a run here would use, and why",
		Long: "Prints the profile, home and login a run in this folder resolves to, and which rule chose\n" +
			"each. Flags work as they do on `agx run`, to try a combination. Reads the config only.",
		Example: "  agx project show\n  agx project show -p other\n  agx project show --json | jq .data.login",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.providerSet = cmd.Flags().Changed(flagProvider)
			return a.runProjectShow(ctxOf(cmd), o, asJSON)
		},
	}
	f := c.Flags()
	f.StringVarP(&o.profile, flagProfile, "p", "", "try this profile")
	f.StringVar(&o.provider, flagProvider, string(provider.Claude), "provider for the default profile")
	f.StringVar(&o.login, flagLogin, "", "try this login")
	f.BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

func (a *app) runProjectShow(ctx context.Context, o *launchOpts, asJSON bool) error {
	// Both "auto" values would spend network or quota; show only reads.
	if o.profile == config.AutoProfile || o.login == login.Auto {
		return usageErr(errors.New(`project show reads the config only; "auto" is decided at launch from live usage`))
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	a.useProject(cfg, dir)
	p, err := a.resolveProfile(ctx, o)
	if err != nil {
		return err
	}
	lc, note, err := a.pickLogin(cfg, p, o.login)
	if err != nil {
		return err
	}
	if note != "" {
		a.warn(note)
	}
	v := projectShow{
		Folder: config.CanonicalPath(dir), Profile: p.Name, ProfileSource: a.profileSource(o, p),
		Provider: p.Provider, Home: p.Home, HomeSource: "profile " + p.Name,
		Login: lc.name(), LoginSource: loginSourceText(lc.Source),
	}
	if a.project.Path != "" {
		pr := a.project
		v.Project = &pr
	}
	if asJSON {
		// One object, not a list: count 0 leaves it out of the envelope,
		// as for session.promote and version.show.
		return output.JSON(a.Stdout, kindProjectShow, v, 0)
	}
	project := "none"
	if v.Project != nil {
		project = v.Project.Path + " (" + bindingSummary(*v.Project) + ")"
	}
	_, err = fmt.Fprintf(a.Stdout, "folder:  %s\nproject: %s\nprofile: %s (%s) — %s\nhome:    %s — %s\nlogin:   %s — %s\n",
		v.Folder, project, v.Profile, v.Provider, v.ProfileSource, render.ShortenHome(v.Home, a.UserHome), v.HomeSource, v.Login, v.LoginSource)
	return err
}

// profileSource says which rule chose p for options o.
func (a *app) profileSource(o *launchOpts, p provider.Profile) string {
	switch {
	case o.profile != "":
		return profileSrcFlag
	case a.project.Profile == p.Name:
		return srcBinding + a.project.Path
	}
	return profileSrcDefault
}

// loginSourceText spells a login source for a reader; the short forms are
// what --dry-run and `agx profiles` print.
func loginSourceText(src string) string {
	switch src {
	case loginSrcProfile:
		return "profile login"
	case loginSrcProvider:
		return "provider default"
	case loginSrcHome:
		return "the home's own login"
	}
	return src
}

// projectRow is one entry of `agx project ls`.
type projectRow struct {
	config.Project
	Problems []string `json:"problems,omitempty"`
}

func newProjectLsCmd(a *app) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List every project binding",
		Example: "  agx project ls\n  agx project ls --json | jq '.data[].path'",
		Args:    usageArgs(cobra.NoArgs),
		RunE:    func(cmd *cobra.Command, _ []string) error { return a.runProjectLs(asJSON) },
	}
	c.Flags().BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

func (a *app) runProjectLs(asJSON bool) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	rows := make([]projectRow, 0, len(cfg.Projects))
	for _, pr := range cfg.Projects {
		rows = append(rows, projectRow{Project: pr, Problems: a.bindingProblems(cfg, pr)})
	}
	if asJSON {
		return output.JSON(a.Stdout, kindProjectList, rows, len(rows))
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(a.Stdout, "no project bindings — bind this folder with `agx project use -p PROFILE`")
		return err
	}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		state := projectRowOK
		if len(r.Problems) > 0 {
			state = strings.Join(r.Problems, "; ")
		}
		table = append(table, []string{r.Path, orCell(r.Profile), orCell(r.Login), state})
	}
	return render.Table(a.Stdout, []string{"PATH", "PROFILE", "LOGIN", "STATE"}, table)
}

func newProjectRmCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "rm [PATH]",
		Aliases: []string{"remove"},
		Short:   "Remove the binding of PATH (default: the current folder)",
		Long:    "Removes the binding stored for exactly that folder; a parent's binding is left alone.\nExits 1 when there is none.",
		Example: "  agx project rm\n  agx project rm ~/work/client",
		Args:    usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runProjectRm(args)
		},
	}
}

func (a *app) runProjectRm(args []string) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	target := dir
	if len(args) == 1 {
		target = args[0]
		if !strings.HasPrefix(target, "~") && !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		target = config.ExpandPath(target, a.UserHome)
	}
	// The stored path is canonical, but a folder since replaced by a symlink
	// must still be removable by the spelling `project ls` prints.
	pr, ok := exactProject(cfg, config.CanonicalPath(target))
	if !ok {
		if pr, ok = exactProject(cfg, filepath.Clean(target)); !ok {
			return fmt.Errorf("no project binding for %s (see `agx project ls`)", target)
		}
	}
	if err := a.editor(cfg).RemoveProject(pr.Path); err != nil {
		return err
	}
	a.loaded = false
	_, err = fmt.Fprintf(a.Stdout, "removed project binding %s (%s)\n", pr.Path, bindingSummary(pr))
	return err
}

// doctorProjects checks every binding: what it names must exist and fit, and
// a folder that is gone is worth a warning, since the binding can never match.
func (a *app) doctorProjects(cfg config.Config) []finding {
	var out []finding
	for _, pr := range cfg.Projects {
		problems := a.bindingProblems(cfg, pr)
		switch _, err := os.Stat(pr.Path); {
		case len(problems) > 0:
			out = append(out, finding{areaProject, levelFail, bindingFix(pr, strings.Join(problems, "; "))})
		case err != nil:
			out = append(out, finding{areaProject, levelWarn, pr.Path + " does not exist — remove it with `agx project rm " + pr.Path + "`"})
		default:
			out = append(out, finding{areaProject, levelOK, pr.Path + " -> " + bindingSummary(pr)})
		}
	}
	return out
}

// autoStartable reports whether a bare `agx` should start the agent instead
// of showing usage: a binding covers this folder, no flag was given, and a
// person is at the terminal on both ends. Anything else keeps the usage view,
// so a script or a pipe always gets the same output as before.
func (a *app) autoStartable(cmd *cobra.Command) bool {
	if cmd.Flags().NFlag() != 0 || a.Interactive == nil || !a.Interactive() {
		return false
	}
	cfg, err := a.config()
	if err != nil {
		return false // the usage path reports a bad config
	}
	dir, err := a.Getwd()
	if err != nil {
		return false
	}
	a.useProject(cfg, dir)
	return a.project.Path != ""
}

// autoStart launches as `agx run` would, after saying why on stderr.
func (a *app) autoStart(ctx context.Context) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	dir, err := a.Getwd()
	if err != nil {
		return err
	}
	p, err := a.resolveProfile(ctx, &launchOpts{provider: string(provider.Claude)})
	if err != nil {
		return err
	}
	lc, _, err := a.pickLogin(cfg, p, "")
	if err != nil {
		return err
	}
	a.warn(fmt.Sprintf("project %s -> profile %s, login %s", a.project.Path, p.Name, lc.name()))
	a.announced = true
	return a.launch(ctx, p, provider.LaunchRequest{Dir: dir}, false, "")
}
