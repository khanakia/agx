package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/pick"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/secret"
	"github.com/khanakia/agx/sessions"
)

// launchOpts are shared by run / new / resume.
type launchOpts struct {
	profile  string
	provider string
	dryRun   bool
	// login is --login: a login name, "home", or "" for the defaults.
	login string
}

func (o *launchOpts) bind(c *cobra.Command) {
	f := c.Flags()
	f.StringVarP(&o.profile, flagProfile, "p", "", "profile to launch, or \"auto\" for the most headroom (default: the provider's default profile)")
	f.StringVar(&o.provider, flagProvider, string(provider.Claude), "provider for the default / auto profile")
	f.BoolVar(&o.dryRun, flagDryRun, false, "print what would run (secret values redacted) and exit")
	bindLoginFlag(f, &o.login)
}

func newRunCmd(a *app) *cobra.Command {
	o := &launchOpts{}
	c := &cobra.Command{
		Use:   "run [-p profile|auto] [-- agent-args...]",
		Short: "Launch an agent here on a profile",
		Example: "  agx run                    # default Claude profile\n" +
			"  agx run -p work\n" +
			"  agx run -p auto            # the Claude account with the most headroom\n" +
			"  agx run -p kimi -- --model x",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := ctxOf(cmd)
			p, err := a.resolveProfile(ctx, o)
			if err != nil {
				return err
			}
			dir, err := a.Getwd()
			if err != nil {
				return err
			}
			return a.launch(ctx, p, provider.LaunchRequest{Dir: dir, Args: args}, o.dryRun, o.login)
		},
	}
	o.bind(c)
	return c
}

func newNewCmd(a *app) *cobra.Command {
	o := &launchOpts{}
	c := &cobra.Command{
		Use:   "new [-p profile|auto] [slug words...]",
		Short: "Create a timestamped session folder and launch an agent in it",
		Long: "Creates sessions.root/[slug_]YYYYMMDD_HHMMSS, enters it and launches. With the shell\n" +
			"layer (`eval \"$(agx shell-init zsh)\"`) your shell also ends up in the new folder.",
		Example: "  agx new\n  agx new -p work pglite golang     # → pglite_golang_20260924_101500\n  agx new -p auto",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := ctxOf(cmd)
			cfg, err := a.config()
			if err != nil {
				return err
			}
			p, err := a.resolveProfile(ctx, o)
			if err != nil {
				return err
			}
			if o.dryRun {
				dir := filepath.Join(cfg.SessionsRoot, sessions.Name(sessions.Slug(args...), a.Now()))
				return a.launch(ctx, p, provider.LaunchRequest{Dir: dir}, true, o.login)
			}
			dir, err := sessions.Create(cfg.SessionsRoot, sessions.Slug(args...), a.Now())
			if err != nil {
				return err
			}
			return a.launchOrPlan(ctx, plan{Dir: dir, Profile: p.Name, Login: o.login}, p, provider.LaunchRequest{Dir: dir})
		},
	}
	o.bind(c)
	return c
}

// resumeOpts configure `agx resume`.
type resumeOpts struct {
	launchOpts
	list   bool
	all    bool
	last   bool
	json   bool
	browse bool
	limit  int
}

// defaultResumeLimit bounds the picker list.
const defaultResumeLimit = 50

func newResumeCmd(a *app) *cobra.Command {
	o := &resumeOpts{}
	c := &cobra.Command{
		Use:   "resume [query...]",
		Short: "Continue a conversation on the account that holds it",
		Long: "Lists conversations started in this folder (or every folder with --all) across\n" +
			"every provider home, lets you pick one, and continues it in its folder on the\n" +
			"profile whose home stores it — so a work conversation resumes on work.",
		Example: "  agx resume                 # pick from this folder's conversations\n" +
			"  agx resume pglite          # filter by title / folder / id\n" +
			"  agx resume --last          # newest, no picker\n" +
			"  agx resume --all --list    # print, don't launch\n" +
			"  agx resume -b              # pick a workspace, then a chat (same as agx ws)",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return a.runResume(ctxOf(cmd), o, args) },
	}
	f := c.Flags()
	f.BoolVarP(&o.browse, flagBrowse, "b", false, "pick a workspace (any folder with conversations), then a chat in it")
	f.BoolVar(&o.list, "list", false, "print matching conversations instead of launching")
	f.BoolVar(&o.all, "all", false, "search every folder, not just this one")
	f.BoolVar(&o.last, "last", false, "take the newest match without a picker")
	f.BoolVar(&o.json, flagJSON, false, "with --list: print the JSON envelope")
	f.IntVar(&o.limit, "limit", defaultResumeLimit, "maximum conversations to consider")
	f.BoolVar(&o.dryRun, flagDryRun, false, "print what would run and exit")
	f.StringVarP(&o.profile, flagProfile, "p", "", "resume on this profile instead of the home's own (advanced)")
	bindLoginFlag(f, &o.login)
	return c
}

func (a *app) runResume(ctx context.Context, o *resumeOpts, query []string) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	cwd, err := a.Getwd()
	if err != nil {
		return err
	}
	// A query must search everything before the limit applies, or older
	// matches would be cut before they are seen.
	q := strings.Join(query, " ")
	if o.browse {
		return a.runBrowse(ctx, cfg, o, q)
	}
	fetchLimit := o.limit
	if q != "" {
		fetchLimit = 0
	}
	convs, err := a.findConversations(ctx, cfg, cwd, o.all, fetchLimit)
	if err != nil {
		return err
	}
	// An empty folder is an answer, not a cue to list every other folder:
	// the global list is reachable by name (--all), never as a fallback.
	// --json still prints its empty envelope so scripts get valid output.
	jsonList := o.list && o.json
	if len(convs) == 0 && !o.all && !jsonList {
		return fmt.Errorf("no conversations in %s\n     (%s resume --all to pick from every folder)", cwd, appmeta.Name)
	}
	convs = filterConversations(convs, q)
	if o.limit > 0 && len(convs) > o.limit {
		convs = convs[:o.limit]
	}
	views := conversationViews(cfg, convs)
	if o.list {
		return a.printConversations(views, o.json)
	}
	if len(convs) == 0 {
		return errors.New("no matching conversations")
	}

	idx := 0
	if !o.last && len(convs) > 1 {
		items := make([]string, len(views))
		for i, v := range views {
			items[i] = a.conversationLine(v)
		}
		idx, err = a.Picker.Pick("resume", items)
		if errors.Is(err, ErrCancelled) {
			return &ExitError{Code: ExitFailure, Err: err}
		}
		if err != nil {
			return err
		}
	}
	return a.resumeConversation(ctx, cfg, o, convs[idx])
}

// resumeConversation continues chosen in its folder on the profile whose
// home holds it (or -p). Shared by the flat picker and --browse so both
// launch the same way.
func (a *app) resumeConversation(ctx context.Context, cfg config.Config, o *resumeOpts, chosen provider.Conversation) error {
	if info, err := os.Stat(chosen.Dir); err != nil || !info.IsDir() {
		return fmt.Errorf("the conversation's folder %s no longer exists — if you moved it (e.g. `agx sessions promote`), cd into the new folder and run `agx resume` there", chosen.Dir)
	}

	p, ok := cfg.ForHome(chosen.Provider, chosen.Home)
	if o.profile != "" {
		p, ok = cfg.ByName(o.profile)
	}
	if !ok {
		return fmt.Errorf("no profile uses %s (%s); add one to the config", chosen.Home, chosen.Provider)
	}
	req := provider.LaunchRequest{Dir: chosen.Dir, ResumeID: chosen.ID}
	if o.dryRun {
		return a.launch(ctx, p, req, true, o.login)
	}
	return a.launchOrPlan(ctx, plan{Dir: chosen.Dir, Profile: p.Name, ResumeID: chosen.ID, Login: o.login}, p, req)
}

// findConversations lists conversations from every distinct provider home,
// limited to cwd unless all is set. It never widens on its own: an empty cwd
// returns an empty list.
func (a *app) findConversations(ctx context.Context, cfg config.Config, cwd string, all bool, limit int) ([]provider.Conversation, error) {
	collect := func(dir string) ([]provider.Conversation, error) {
		var out []provider.Conversation
		for _, p := range cfg.Homes() {
			prov, err := a.providerFor(p.Provider)
			if err != nil {
				return nil, err
			}
			lister, ok := prov.(provider.ConversationLister)
			if !ok {
				continue
			}
			cs, err := lister.Conversations(ctx, p.Home, provider.ConversationQuery{Dir: dir, Limit: limit})
			if err != nil {
				a.warn(fmt.Sprintf("%s: %v", p.Home, err))
				continue
			}
			out = append(out, cs...)
		}
		slices.SortFunc(out, func(x, y provider.Conversation) int { return y.Updated.Compare(x.Updated) })
		if limit > 0 && len(out) > limit {
			out = out[:limit]
		}
		return out, nil
	}
	if all {
		return collect("")
	}
	return collect(cwd)
}

// filterConversations keeps conversations whose title, folder or id contains
// every word of q (case-insensitive).
func filterConversations(cs []provider.Conversation, q string) []provider.Conversation {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return cs
	}
	var out []provider.Conversation
	for _, c := range cs {
		hay := strings.ToLower(c.Title + " " + c.Dir + " " + c.ID)
		match := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				match = false
				break
			}
		}
		if match {
			out = append(out, c)
		}
	}
	return out
}

// Conversation row layout. The widths keep a row near 100 columns so it
// never wraps in a normal terminal; --json carries every value in full.
const (
	// untitled is shown for conversations without a title.
	untitled = "(untitled)"
	// Column widths in picker / list rows.
	agoWidth      = 12
	providerWidth = 6 // "claude", "codex"
	profileWidth  = 10
	titleWidth    = 32
	dirWidth      = 28
	// shortIDLen is how much of the conversation id a row shows: enough to
	// tell rows apart and to type back as a `resume` query (ids match by
	// substring), short enough to keep the row on one line.
	shortIDLen = 8
)

// conversationView is a conversation as `resume` lists it: the listing plus
// the profile it resumes on. The provider fields stay at the top level of the
// JSON (additive to the conversation.list shape).
type conversationView struct {
	provider.Conversation
	// Profile is the profile `resume` launches the conversation on —
	// config.ForHome, the same rule the launch uses, so the list and the
	// launch can never disagree. A home no profile uses shows its basename.
	Profile string `json:"profile"`
	// Profiles lists every configured profile on this home, in config order.
	// More than one means the home is shared (personal + kimi on ~/.claude)
	// and the conversation could have been made on any of them.
	Profiles []string `json:"profiles,omitempty"`
}

// conversationViews attributes each conversation to a profile.
func conversationViews(cfg config.Config, cs []provider.Conversation) []conversationView {
	out := make([]conversationView, len(cs))
	for i, c := range cs {
		v := conversationView{Conversation: c, Profile: filepath.Base(c.Home)}
		if p, ok := cfg.ForHome(c.Provider, c.Home); ok {
			v.Profile = p.Name
		}
		for _, p := range cfg.ProfilesForHome(c.Provider, c.Home) {
			v.Profiles = append(v.Profiles, p.Name)
		}
		out[i] = v
	}
	return out
}

// shortDir fits a folder into dirWidth: `~`-relative when it fits, else the
// last path segment behind an ellipsis (the part that tells folders apart).
func shortDir(dir, userHome string) string {
	d := render.ShortenHome(dir, userHome)
	if utf8.RuneCountInString(d) <= dirWidth {
		return d
	}
	return truncate(ellipsis+"/"+filepath.Base(dir), dirWidth)
}

// shortID is the first shortIDLen runes of id, unmarked: it is a prefix the
// user can type back as a query, so it must not carry an ellipsis.
func shortID(id string) string {
	r := []rune(id)
	return string(r[:min(len(r), shortIDLen)])
}

// conversationLine formats one picker / list row on a single line.
func (a *app) conversationLine(v conversationView) string {
	title := v.Title
	if title == "" {
		title = untitled
	}
	return fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s  %s",
		agoWidth, render.FormatAgo(a.Now(), v.Updated),
		providerWidth, truncate(string(v.Provider), providerWidth),
		profileWidth, truncate(v.Profile, profileWidth),
		titleWidth, truncate(title, titleWidth),
		dirWidth, shortDir(v.Dir, a.UserHome),
		shortID(v.ID))
}

func (a *app) printConversations(vs []conversationView, asJSON bool) error {
	if asJSON {
		return output.JSON(a.Stdout, kindConversationList, vs, len(vs))
	}
	var b strings.Builder
	for _, v := range vs {
		b.WriteString(a.conversationLine(v))
		b.WriteByte('\n')
	}
	_, err := io.WriteString(a.Stdout, b.String())
	return err
}

// resolveProfile turns -p / --provider into a concrete profile, running the
// auto-pick policy for "auto".
func (a *app) resolveProfile(ctx context.Context, o *launchOpts) (provider.Profile, error) {
	cfg, err := a.config()
	if err != nil {
		return provider.Profile{}, err
	}
	id := provider.ID(o.provider)
	if _, err := a.providerFor(id); err != nil {
		return provider.Profile{}, usageErr(err)
	}
	switch o.profile {
	case "":
		p, ok := cfg.Default(id)
		if !ok {
			return provider.Profile{}, usageErr(fmt.Errorf("no %s profile found (run `agx doctor`)", id))
		}
		return p, nil
	case config.AutoProfile:
		return a.autoPick(ctx, cfg, id)
	default:
		p, ok := cfg.ByName(o.profile)
		if !ok {
			return provider.Profile{}, usageErr(fmt.Errorf("unknown profile %q (see `agx profiles`)", o.profile))
		}
		return p, nil
	}
}

// autoPick fetches usage for every plan-billed home of provider id and
// applies the pick policy, explaining the decision on stderr.
func (a *app) autoPick(ctx context.Context, cfg config.Config, id provider.ID) (provider.Profile, error) {
	var targets []provider.Profile
	for _, p := range cfg.Homes() {
		if p.Provider == id && p.Billing == provider.BillingPlan {
			targets = append(targets, p)
		}
	}
	reports := a.fetchUsage(ctx, targets, defaultUsageTimeout)
	cands := make([]pick.Candidate, len(reports))
	for i, r := range reports {
		cands[i] = pick.Candidate{Profile: r.Profile, Usage: r.Usage, Err: r.Err}
	}
	res, err := pick.Best(cands)
	if err != nil {
		return provider.Profile{}, err
	}
	a.warn(res.Explain())
	return res.Chosen, nil
}

// childSessionEnv is the variable Claude Code sets on its own internal helper
// processes. Terminals opened from inside a Claude session inherit it, and a
// top-level session started with it present turns transcript saving off, so
// the owner's sessions cannot be resumed. agx only launches sessions the user
// starts, so it must never pass the marker on.
const childSessionEnv = "CLAUDE_CODE_CHILD_SESSION"

// launch builds the provider command (resolving secrets and the login) and
// executes it, or prints it for --dry-run. loginFlag is --login.
func (a *app) launch(ctx context.Context, p provider.Profile, req provider.LaunchRequest, dryRun bool, loginFlag string) error {
	prov, err := a.providerFor(p.Provider)
	if err != nil {
		return err
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	lc, err := a.chooseLogin(ctx, cfg, p, loginFlag)
	if err != nil {
		return err
	}
	req.Env = provider.UnsetEnv(a.Environ(), childSessionEnv)
	secretNames := make([]string, 0, len(p.Secrets))
	for k := range p.Secrets {
		secretNames = append(secretNames, k)
	}
	slices.Sort(secretNames)

	if !dryRun && len(p.Secrets) > 0 {
		vals, err := secret.ResolveAll(ctx, a.Secrets, p.Secrets)
		if err != nil {
			return err
		}
		for _, k := range secretNames {
			req.Env = provider.SetEnv(req.Env, k, vals[k])
		}
	}
	cmd, err := prov.Launch(p, req)
	if err != nil {
		return err
	}
	// The login is applied after the profile's own env/secrets (logins spec
	// §7), so its unset list wins over anything inherited.
	var loginLines []string
	if lc.Login != nil {
		if dryRun {
			cmd.Env, loginLines = a.dryRunLoginEnv(cmd.Env, lc)
		} else if cmd.Env, err = a.applyLogin(ctx, cmd.Env, lc); err != nil {
			return err
		}
	}
	if dryRun {
		return a.printDryRun(p, cmd, secretNames, lc, loginLines)
	}
	if lc.Login != nil {
		a.warn(p.Name + " · login " + lc.Login.Name)
	}
	return a.Exec(cmd)
}

// printDryRun shows the command without secret values: only the env vars
// agx changed are listed, and secrets appear as NAME=<secret:scheme>.
func (a *app) printDryRun(p provider.Profile, cmd provider.Command, secretNames []string, lc loginChoice, loginLines []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "profile: %s (%s, %s)\n", p.Name, p.Provider, p.Home)
	fmt.Fprintf(&b, "login:   %s\n", lc.describe())
	fmt.Fprintf(&b, "dir:     %s\n", cmd.Dir)
	base := map[string]bool{}
	for _, kv := range a.Environ() {
		base[kv] = true
	}
	var changed []string
	for _, kv := range cmd.Env {
		if !base[kv] {
			changed = append(changed, kv)
		}
	}
	for _, kv := range a.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(cmd.Env, func(e string) bool { return strings.HasPrefix(e, name+"=") }) {
			changed = append(changed, "-"+name+" (removed)")
		}
	}
	for _, k := range secretNames {
		scheme, _, _ := strings.Cut(p.Secrets[k], ":")
		changed = append(changed, k+"=<secret:"+scheme+">")
	}
	changed = append(changed, loginLines...)
	slices.Sort(changed)
	for _, kv := range changed {
		fmt.Fprintf(&b, "env:     %s\n", kv)
	}
	fmt.Fprintf(&b, "exec:    %s\n", strings.Join(append([]string{cmd.Path}, cmd.Args...), " "))
	_, err := io.WriteString(a.Stdout, b.String())
	return err
}

// plan is the launch plan written for the shell layer (spec §10). It never
// contains secrets: exec re-resolves the profile and its secrets.
type plan struct {
	Version  int      `json:"version"`
	Dir      string   `json:"dir"`
	Profile  string   `json:"profile"`
	ResumeID string   `json:"resume_id,omitempty"`
	Args     []string `json:"args,omitempty"`
	// Login is the --login value the user gave (a name or "home"), so exec
	// launches on the same login; never a token.
	Login string `json:"login,omitempty"`
}

// planVersion is bumped when the plan shape changes incompatibly; exec
// refuses other versions (a stale shell layer after an upgrade).
const planVersion = 1

// planPerm keeps the plan private to the user.
const planPerm os.FileMode = 0o600

// launchOrPlan writes a plan when the shell layer asked for one (so the
// shell can cd first), otherwise launches directly.
func (a *app) launchOrPlan(ctx context.Context, pl plan, p provider.Profile, req provider.LaunchRequest) error {
	path, ok := a.LookupEnv(appmeta.EnvPlanFile)
	if !ok || path == "" {
		return a.launch(ctx, p, req, false, pl.Login)
	}
	pl.Version = planVersion
	raw, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, planPerm); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}

func newExecCmd(a *app) *cobra.Command {
	var planPath string
	var printDir bool
	c := &cobra.Command{
		Use:    "exec --plan FILE",
		Short:  "Run a launch plan written by `new` / `resume` (used by the shell layer)",
		Hidden: true,
		Args:   usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if planPath == "" {
				return usageErr(errors.New("--plan is required"))
			}
			raw, err := os.ReadFile(planPath)
			if err != nil {
				return fmt.Errorf("read plan: %w", err)
			}
			var pl plan
			if err := json.Unmarshal(raw, &pl); err != nil {
				return fmt.Errorf("decode plan: %w", err)
			}
			if pl.Version != planVersion {
				return fmt.Errorf("plan version %d, want %d — re-run `eval \"$(agx shell-init zsh)\"`", pl.Version, planVersion)
			}
			if printDir {
				_, err := fmt.Fprintln(a.Stdout, pl.Dir)
				return err
			}
			// The plan is single-use: remove it before exec replaces us.
			if err := os.Remove(planPath); err != nil {
				return fmt.Errorf("remove plan: %w", err)
			}
			cfg, err := a.config()
			if err != nil {
				return err
			}
			p, ok := cfg.ByName(pl.Profile)
			if !ok {
				return fmt.Errorf("plan names unknown profile %q", pl.Profile)
			}
			req := provider.LaunchRequest{Dir: pl.Dir, ResumeID: pl.ResumeID, Args: pl.Args}
			return a.launch(ctxOf(cmd), p, req, false, pl.Login)
		},
	}
	c.Flags().StringVar(&planPath, "plan", "", "plan file")
	c.Flags().BoolVar(&printDir, "print-dir", false, "print the plan's directory and keep the file")
	return c
}
