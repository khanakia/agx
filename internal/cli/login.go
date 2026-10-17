package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// flagLogin is --login on run / new / resume / ws / usage / plan.
const flagLogin = "login"

// kindLoginList is the JSON envelope kind of `agx login ls --json`.
const kindLoginList = "login.list"

// Where a login choice came from (logins spec §5.4), shown by `agx
// profiles`, --dry-run and in notes.
const (
	loginSrcFlag     = "--login"
	loginSrcEnv      = appmeta.EnvLogin
	loginSrcProfile  = "profile"
	loginSrcProvider = "provider"
	loginSrcHome     = "home"
)

// maxTokenInput bounds what `login add --stdin` reads: a token is a few
// hundred bytes, so anything near this is a mistake (a piped file).
const maxTokenInput = 64 << 10

// expiryWarnWindow is how early doctor warns that a login will expire.
const expiryWarnWindow = 30 * 24 * time.Hour

// Outcomes of `agx login check`.
const (
	checkResolved = "resolved ok"
	checkNoProbe  = "resolved ok (not probed)"
)

// flagNoProbe skips the usage probe in `agx login check`.
const flagNoProbe = "no-probe"

// revokeHint is printed by `login rm`: removing a login does not revoke it.
const revokeHint = "removing a login from agx does not revoke the token — revoke it in the vendor's token list (claude.ai settings; spec §12 V3)"

// syncWarning is printed whenever a raw token is stored.
const syncWarning = "tokens.yaml holds raw tokens: never sync, share or commit it"

// Display placeholders.
const (
	cellNever   = "never"
	cellMissing = " (missing)"
	cellRefused = " (unreadable)"
	approxMark  = "~"
)

// bindLoginFlag registers --login on f. The usage text must not contain
// back quotes: pflag takes the first back-quoted word as the value name, so
// help would read "--login agx login ls" instead of "--login string".
func bindLoginFlag(f *pflag.FlagSet, dst *string) {
	f.StringVar(dst, flagLogin, "", "run on this login's quota (a name from 'agx login ls', \"home\" for the home's own login, or \"auto\" for the most headroom — probes each login)")
}

// loginChoice is the login a launch runs on; Login nil means the home login.
type loginChoice struct {
	Login  *login.Login
	Source string
}

// describe renders the choice for --dry-run and `agx profiles`.
func (c loginChoice) describe() string {
	if c.Login == nil {
		if c.Source == loginSrcHome {
			return login.Home
		}
		return login.Home + " (" + c.Source + ")"
	}
	return c.Login.Name + " (" + c.Source + ")"
}

// chooseLogin applies the precedence of logins spec §5.4. A wrong-provider
// or incompatible login from AGX_LOGIN is skipped with a note (an exported
// Claude login must not break Codex or kimi runs); from --login or the
// profile it is an error, because it was asked for explicitly.
func (a *app) chooseLogin(ctx context.Context, cfg config.Config, p provider.Profile, flag string) (loginChoice, error) {
	if flag == login.Auto {
		return a.autoLogin(ctx, cfg, p)
	}
	lc, note, err := a.pickLogin(cfg, p, flag)
	if note != "" {
		a.warn(note)
	}
	return lc, err
}

// checkLogin validates the login p would run on (--login, AGX_LOGIN, the
// profile's own) without side effects, for callers that must refuse before
// they create anything (`agx new`). "auto" passes: choosing it probes
// quota, which the launch itself does once.
func (a *app) checkLogin(cfg config.Config, p provider.Profile, flag string) error {
	if flag == login.Auto {
		return nil
	}
	_, _, err := a.pickLogin(cfg, p, flag)
	return err
}

// pickLogin is chooseLogin without side effects: the note is returned.
func (a *app) pickLogin(cfg config.Config, p provider.Profile, flag string) (loginChoice, string, error) {
	explicit := func(name, src string) (loginChoice, error) {
		switch name {
		case login.Home:
			return loginChoice{Source: src}, nil
		case login.Auto:
			return loginChoice{}, usageErr(config.ErrAutoLogin)
		}
		l, ok := cfg.LoginByName(name)
		if !ok {
			return loginChoice{}, errUnknownLogin(name)
		}
		if err := a.loginFits(p, l); err != nil {
			return loginChoice{}, usageErr(err)
		}
		return loginChoice{Login: &l, Source: src}, nil
	}
	if flag != "" {
		lc, err := explicit(flag, loginSrcFlag)
		return lc, "", err
	}
	if name, ok := a.LookupEnv(appmeta.EnvLogin); ok && name != "" {
		switch name {
		case login.Home:
			return loginChoice{Source: loginSrcEnv}, "", nil
		case login.Auto:
			return loginChoice{}, "", usageErr(fmt.Errorf("%s: %w", appmeta.EnvLogin, config.ErrAutoLogin))
		}
		l, ok := cfg.LoginByName(name)
		if !ok {
			return loginChoice{}, "", usageErr(fmt.Errorf("%s=%s: unknown login (see `agx login ls`; `aglogin` with no argument unsets it)", appmeta.EnvLogin, name))
		}
		if err := a.loginFits(p, l); err != nil {
			return a.defaultLogin(cfg, p), fmt.Sprintf("%s=%s skipped for profile %s: %v", appmeta.EnvLogin, name, p.Name, err), nil
		}
		return loginChoice{Login: &l, Source: loginSrcEnv}, "", nil
	}
	return a.defaultLogin(cfg, p), "", nil
}

// defaultLogin covers steps 3–5: the profile's login (validated at load),
// then the provider default when the profile can take it, then home.
func (a *app) defaultLogin(cfg config.Config, p provider.Profile) loginChoice {
	switch p.Login {
	case login.Home:
		return loginChoice{Source: loginSrcProfile}
	case "":
	default:
		if l, ok := cfg.LoginByName(p.Login); ok {
			return loginChoice{Login: &l, Source: loginSrcProfile}
		}
	}
	if name, ok := cfg.ProviderLogins[p.Provider]; ok {
		if l, ok := cfg.LoginByName(name); ok && a.loginFits(p, l) == nil {
			return loginChoice{Login: &l, Source: loginSrcProvider}
		}
	}
	return loginChoice{Source: loginSrcHome}
}

// loginFits reports whether profile p can run on login l.
func (a *app) loginFits(p provider.Profile, l login.Login) error {
	if l.Provider != p.Provider {
		return fmt.Errorf("login %s is a %s login, not %s", l.Name, l.Provider, p.Provider)
	}
	ll, err := a.loginLauncher(p.Provider)
	if err != nil {
		return err
	}
	return login.Compatible(p, ll, l.Kind)
}

// loginLauncher returns provider id's login upgrade, or the reason it has
// none.
func (a *app) loginLauncher(id provider.ID) (provider.LoginLauncher, error) {
	prov, err := a.providerFor(id)
	if err != nil {
		return nil, err
	}
	ll, ok := prov.(provider.LoginLauncher)
	if !ok {
		return nil, errors.New(config.NoLoginsReason(id))
	}
	return ll, nil
}

// editor edits the loaded config file in place (comments kept). Callers
// set a.loaded = false after a successful edit so the next a.config()
// re-reads what was written.
func (a *app) editor(cfg config.Config) config.Editor {
	return config.Editor{Path: cfg.Path, UserHome: a.UserHome, Providers: a.Providers}
}

// tokenFile is tokens.yaml beside the config file.
func (a *app) tokenFile() (login.TokenFile, error) {
	path, _, err := a.ConfigPath()
	if err != nil {
		return login.TokenFile{}, fmt.Errorf("locate config: %w", err)
	}
	return login.TokenFile{Path: filepath.Join(filepath.Dir(path), login.TokenFileName)}, nil
}

// resolver resolves login values on this machine.
func (a *app) resolver() (login.Resolver, error) {
	tf, err := a.tokenFile()
	if err != nil {
		return login.Resolver{}, err
	}
	return login.Resolver{Tokens: tf, LookupEnv: a.LookupEnv, Dir: a.UserHome}, nil
}

// expiry returns when l stops working and whether that is known.
func (a *app) expiry(l login.Login) (time.Time, bool) {
	prov, err := a.providerFor(l.Provider)
	if err != nil {
		return time.Time{}, false
	}
	lt, ok := prov.(provider.LoginLifetimer)
	if !ok {
		return time.Time{}, false
	}
	return login.ExpiresAt(l, lt.LoginLifetime(l.Kind))
}

// resolveLoginValue resolves l's token. Past expiry a stored login is
// refused; a cmd/env login only warns, because agx knows when it was
// registered, not when the token behind it was minted — a token rotated in
// the user's store keeps working (logins spec §14 D13). Never falls back to
// the home login (logins spec §7, D9).
func (a *app) resolveLoginValue(ctx context.Context, l login.Login) (string, error) {
	if at, ok := a.expiry(l); ok && !a.Now().Before(at) {
		if login.Owned(l) {
			return "", fmt.Errorf("login %s expired on %s; renew: agx login add %s --setup --replace", l.Name, at.Format(time.DateOnly), l.Name)
		}
		a.warn(fmt.Sprintf("login %s is past its approximate expiry (%s, counted from when it was added); running anyway — if the token was not rotated, renew: agx login add %s --replace", l.Name, at.Format(time.DateOnly), l.Name))
	}
	r, err := a.resolver()
	if err != nil {
		return "", err
	}
	return r.Resolve(ctx, l)
}

// applyLogin resolves lc's token and applies its env to env.
func (a *app) applyLogin(ctx context.Context, env []string, lc loginChoice) ([]string, error) {
	ll, err := a.loginLauncher(lc.Login.Provider)
	if err != nil {
		return nil, err
	}
	val, err := a.resolveLoginValue(ctx, *lc.Login)
	if err != nil {
		return nil, err
	}
	set, unset := ll.LoginEnv(lc.Login.Kind, val)
	return provider.ApplyLoginEnv(env, set, unset), nil
}

// dryRunLoginEnv applies only the removals and returns NAME=<token:source>
// lines for the variables a real run would set; the token is not resolved.
func (a *app) dryRunLoginEnv(env []string, lc loginChoice) ([]string, []string) {
	ll, err := a.loginLauncher(lc.Login.Provider)
	if err != nil {
		return env, nil
	}
	set, unset := ll.LoginEnv(lc.Login.Kind, "")
	lines := make([]string, 0, len(set))
	for k := range set {
		lines = append(lines, k+"=<token:"+string(lc.Login.Source)+">")
	}
	return provider.ApplyLoginEnv(env, nil, unset), lines
}

func newLoginCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "login",
		Short: "Named logins: run any home on any account's quota",
		Long: "A login is a named credential (a long-lived `claude setup-token` token or an API key)\n" +
			"that decides whose quota a run spends, independent of the home that holds the history.\n" +
			"Raw tokens live in ~/.agx/tokens.yaml (0600), never in config.yaml.",
		Example: "  pbpaste | agx login add personal --stdin\n" +
			"  agx login add work --cmd \"gopass show -o personal/ai/claude-oauth-work\"\n" +
			"  agx login use personal -p work      # clw now spends personal quota\n" +
			"  agx run -p work --login home        # this run only: the home's own login",
		Args: usageArgs(cobra.NoArgs),
	}
	c.AddCommand(newLoginAddCmd(a), newLoginLsCmd(a), newLoginCheckCmd(a), newLoginRmCmd(a), newLoginUseCmd(a), newLoginExistsCmd(a))
	return c
}

// loginAddOpts configure `agx login add`.
type loginAddOpts struct {
	stdin, setup, replace bool
	cmd, env              string
	provider, kind        string
	account               string
}

func newLoginAddCmd(a *app) *cobra.Command {
	o := &loginAddOpts{}
	c := &cobra.Command{
		Use:   "add NAME",
		Short: "Add a login (default: paste the token at a hidden prompt)",
		Long: "Registers a login. The token source is one of: a hidden paste (default) or --stdin, both\n" +
			"stored in tokens.yaml; --cmd, a command printing the token at launch; --env, another env var.\n" +
			"There is deliberately no --token flag: argv is visible in ps and shell history.",
		Example: "  agx login add personal                       # hidden paste\n" +
			"  pbpaste | agx login add personal --stdin\n" +
			"  agx login add personal --setup               # run `claude setup-token` first\n" +
			"  agx login add work --cmd \"op read op://Private/claude-work/token\"\n" +
			"  agx login add ci --env CLAUDE_WORK_TOKEN\n" +
			"  agx login add personal --stdin --replace     # rotate",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return a.runLoginAdd(ctxOf(cmd), o, args[0]) },
	}
	f := c.Flags()
	f.BoolVar(&o.stdin, "stdin", false, "read the token from stdin")
	f.BoolVar(&o.setup, "setup", false, "run `claude setup-token` first, then ask for the paste")
	f.StringVar(&o.cmd, "cmd", "", "command that prints the token at launch (run with sh -c)")
	f.StringVar(&o.env, "env", "", "env var that holds the token at launch")
	f.StringVar(&o.provider, flagProvider, string(provider.Claude), "provider the login is for")
	f.StringVar(&o.kind, "kind", "", "credential kind: oauth-token (plan quota, default) or api-key")
	f.StringVar(&o.account, "account", "", "account label (e.g. the email), for your eyes")
	f.BoolVar(&o.replace, "replace", false, "replace an existing login of the same name (token rotation)")
	return c
}

func (a *app) runLoginAdd(ctx context.Context, o *loginAddOpts, name string) error {
	if err := config.ValidLoginName(name); err != nil {
		return usageErr(err)
	}
	sources := 0
	for _, on := range []bool{o.cmd != "", o.env != "", o.stdin || o.setup} {
		if on {
			sources++
		}
	}
	if sources > 1 {
		return usageErr(errors.New("give one token source: --cmd, --env, or a paste (--stdin / --setup / the prompt)"))
	}
	if o.stdin && o.setup {
		return usageErr(errors.New("--setup asks for the paste at a prompt; it cannot be combined with --stdin"))
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	id := provider.ID(o.provider)
	ll, err := a.loginLauncher(id)
	if err != nil {
		return usageErr(err)
	}
	old, exists := cfg.LoginByName(name)
	if exists && !o.replace {
		return usageErr(fmt.Errorf("login %q already exists (use --replace to rotate its token)", name))
	}
	// Rotation (--replace) changes the token, not what the login is: a kind
	// or account not given again is kept, so rotating an api-key login does
	// not silently turn it into an oauth-token one. `added` is reset, since
	// expiry counts from the new token.
	rawKind, account := o.kind, o.account
	if exists && old.Provider == id {
		if rawKind == "" {
			rawKind = string(old.Kind)
		}
		if account == "" {
			account = old.Account
		}
	}
	kind, err := config.KindFor(ll, rawKind)
	if err != nil {
		return usageErr(err)
	}
	spec := config.LoginSpec{Name: name, Provider: string(id), Account: account, Added: a.Now().Format(time.DateOnly)}
	if kind != ll.LoginKinds()[0] {
		spec.Kind = string(kind)
	}
	tf, err := a.tokenFile()
	if err != nil {
		return err
	}
	stored := false
	var prev previousToken
	switch {
	case o.cmd != "":
		spec.TokenCmd = o.cmd
	case o.env != "":
		spec.TokenEnv = o.env
	default:
		val, err := a.readToken(o, id)
		if err != nil {
			return err
		}
		// Keep what tokens.yaml held under this name, so a refused config
		// edit can put it back: losing a rotated-out token that config
		// still points at would break the login.
		if prev, err = loadPreviousToken(tf, name); err != nil {
			return err
		}
		if err := tf.Set(name, val); err != nil {
			return err
		}
		spec.Token, stored = login.StoredMarker, true
	}
	ed := a.editor(cfg)
	if err := ed.AddLogin(spec, o.replace); err != nil {
		if stored {
			// Roll back so tokens.yaml matches the unchanged config again.
			if rbErr := prev.restore(tf, name); rbErr != nil {
				a.warn("could not restore tokens.yaml: " + rbErr.Error())
			}
		}
		return err
	}
	a.loaded = false
	if exists && old.Source == login.SourceStored && !stored {
		// The login no longer reads tokens.yaml, so its old raw token must
		// not stay there with nothing using it (rotated away, still valid).
		if _, err := tf.Remove(name); err != nil {
			a.warn("could not remove the old stored token: " + err.Error())
		}
	}
	verb := "added"
	if exists {
		verb = "replaced"
	}
	source := login.SourceStored
	switch {
	case spec.TokenCmd != "":
		source = login.SourceCmd
	case spec.TokenEnv != "":
		source = login.SourceEnv
	}
	if _, err := fmt.Fprintf(a.Stdout, "%s login %s (%s, %s, %s)\n", verb, name, id, kind, source); err != nil {
		return err
	}
	if stored {
		a.warn(syncWarning + " (" + render.ShortenHome(tf.Path, a.UserHome) + ")")
	}
	if err := a.checkOne(ctx, name, false); err != nil {
		return err
	}
	a.note("see its usage with `agx login check " + name + "` (one probe: a few tokens of its quota)")
	return nil
}

// previousToken is what tokens.yaml held under a name before `login add`
// overwrote it; ok is false when nothing was stored.
type previousToken struct {
	value string
	ok    bool
}

// loadPreviousToken reads name's current entry from tf (none when the
// file does not exist yet).
func loadPreviousToken(tf login.TokenFile, name string) (previousToken, error) {
	all, err := tf.Load()
	if err != nil {
		return previousToken{}, err
	}
	v, ok := all[name]
	return previousToken{value: v, ok: ok}, nil
}

// restore puts the previous entry back, or removes name when there was none.
func (p previousToken) restore(tf login.TokenFile, name string) error {
	if p.ok {
		return tf.Set(name, p.value)
	}
	_, err := tf.Remove(name)
	return err
}

// readToken reads a raw token from stdin or a hidden prompt, after running
// the vendor's setup command when asked.
func (a *app) readToken(o *loginAddOpts, id provider.ID) (string, error) {
	if o.setup {
		if id != provider.Claude {
			return "", usageErr(fmt.Errorf("--setup is only known for %s", provider.Claude))
		}
		a.note("running `" + claude.Binary + " " + claude.SetupTokenSubcommand + "` — copy the token it prints, then paste it below")
		if err := a.RunInteractive(provider.Command{Path: claude.Binary, Args: []string{claude.SetupTokenSubcommand}, Env: a.Environ()}); err != nil {
			return "", fmt.Errorf("%s %s: %w", claude.Binary, claude.SetupTokenSubcommand, err)
		}
	}
	var raw string
	if o.stdin {
		b, err := io.ReadAll(io.LimitReader(a.Stdin, maxTokenInput))
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		raw = string(b)
	} else {
		v, err := a.PromptSecret("paste the token (input hidden): ")
		if err != nil {
			return "", err
		}
		raw = v
	}
	val, err := login.Clean(raw)
	if err != nil {
		return "", usageErr(err)
	}
	return val, nil
}

// loginRow is one entry of `agx login ls`; it never carries a value.
type loginRow struct {
	Name     string             `json:"name"`
	Provider provider.ID        `json:"provider"`
	Kind     provider.LoginKind `json:"kind"`
	Source   login.Source       `json:"source"`
	// State is "ok", "missing" (stored, no entry) or "unreadable".
	State   string     `json:"state"`
	Account string     `json:"account,omitempty"`
	Added   string     `json:"added,omitempty"`
	Expires *time.Time `json:"expires,omitempty"`
	// ExpiresApprox is true when agx does not own the value (cmd / env).
	ExpiresApprox bool     `json:"expires_approx,omitempty"`
	UsedBy        []string `json:"used_by,omitempty"`
}

// Login row states.
const (
	stateOK         = "ok"
	stateMissing    = "missing"
	stateUnreadable = "unreadable"
)

func newLoginLsCmd(a *app) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List logins (never their values)",
		Example: "  agx login ls\n  agx login ls --json | jq '.data[].name'",
		Args:    usageArgs(cobra.NoArgs),
		RunE:    func(cmd *cobra.Command, _ []string) error { return a.runLoginLs(asJSON) },
	}
	c.Flags().BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

func (a *app) loginRows(cfg config.Config) ([]loginRow, error) {
	tf, err := a.tokenFile()
	if err != nil {
		return nil, err
	}
	stored, loadErr := tf.Load()
	rows := make([]loginRow, 0, len(cfg.Logins))
	for _, l := range cfg.Logins {
		r := loginRow{Name: l.Name, Provider: l.Provider, Kind: l.Kind, Source: l.Source, State: stateOK, Account: l.Account, UsedBy: cfg.UsedBy(l.Name)}
		if !l.Added.IsZero() {
			r.Added = l.Added.Format(time.DateOnly)
		}
		if at, ok := a.expiry(l); ok {
			r.Expires, r.ExpiresApprox = &at, !login.Owned(l)
		}
		if l.Source == login.SourceStored {
			switch _, ok := stored[l.Name]; {
			case loadErr != nil:
				r.State = stateUnreadable
			case !ok:
				r.State = stateMissing
			}
		}
		rows = append(rows, r)
	}
	if loadErr != nil {
		a.warn(loadErr.Error())
	}
	return rows, nil
}

func (a *app) runLoginLs(asJSON bool) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	rows, err := a.loginRows(cfg)
	if err != nil {
		return err
	}
	if asJSON {
		return output.JSON(a.Stdout, kindLoginList, rows, len(rows))
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(a.Stdout, "no logins — add one with `agx login add NAME` (see `agx login add --help`)")
		return err
	}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		src := string(r.Source)
		switch r.State {
		case stateMissing:
			src += cellMissing
		case stateUnreadable:
			src += cellRefused
		}
		expires := cellNever
		if r.Expires != nil {
			expires = r.Expires.Format(time.DateOnly)
			if r.ExpiresApprox {
				expires = approxMark + expires
			}
		}
		table = append(table, []string{r.Name, string(r.Provider), string(r.Kind), src, orCell(r.Account), orCell(r.Added), expires, orCell(strings.Join(r.UsedBy, ", "))})
	}
	return render.Table(a.Stdout, []string{"NAME", "PROVIDER", "KIND", "SOURCE", "ACCOUNT", "ADDED", "EXPIRES", "USED-BY"}, table)
}

func newLoginCheckCmd(a *app) *cobra.Command {
	var noProbe bool
	c := &cobra.Command{
		Use:   "check [NAME]",
		Short: "Resolve a login's token and show its usage (all logins without NAME)",
		Long: "Resolves the token from its source, then probes its usage: one minimal request whose\n" +
			"response headers carry the account's 5-hour and weekly windows. A probe spends a few\n" +
			"tokens of the login's plan quota and counts toward rate limits; --no-probe only resolves.\n" +
			"The value is never printed.",
		Example: "  agx login check\n  agx login check personal\n  agx login check --no-probe",
		Args:    usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := ctxOf(cmd)
			if len(args) == 1 {
				return a.checkOne(ctx, args[0], !noProbe)
			}
			cfg, err := a.config()
			if err != nil {
				return err
			}
			failed := false
			for _, l := range cfg.Logins {
				if err := a.checkOne(ctx, l.Name, !noProbe); err != nil {
					failed = true
				}
			}
			if failed {
				return errPartial
			}
			return nil
		},
	}
	c.Flags().BoolVar(&noProbe, flagNoProbe, false, "only resolve the token; do not spend a probe on its usage")
	return c
}

// checkOne resolves login name and prints one outcome line, then (probe)
// its usage windows; a failure is already printed, so it is errPartial.
func (a *app) checkOne(ctx context.Context, name string, probe bool) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	l, ok := cfg.LoginByName(name)
	if !ok {
		return errUnknownLogin(name)
	}
	if _, err := a.resolveLoginValue(ctx, l); err != nil {
		if _, werr := fmt.Fprintf(a.Stdout, "login %s: failed: %v\n", name, err); werr != nil {
			return werr
		}
		return errPartial
	}
	if !probe {
		_, err = fmt.Fprintf(a.Stdout, "login %s: %s\n", name, checkNoProbe)
		return err
	}
	r := a.probeLogin(ctx, l, defaultUsageTimeout)
	if _, err := fmt.Fprintf(a.Stdout, "login %s: %s\n", name, checkResolved); err != nil {
		return err
	}
	opt := render.Options{Now: a.Now(), Home: a.UserHome}
	if err := render.Usage(a.Stdout, []render.UsageReport{r}, opt); err != nil {
		return err
	}
	if anyFailed([]render.UsageReport{r}) {
		return errPartial
	}
	return nil
}

func newLoginRmCmd(a *app) *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:     "rm NAME",
		Aliases: []string{"remove"},
		Short:   "Remove a login from config.yaml and tokens.yaml",
		Long: "Refuses while a profile or provider default uses the login, unless --force, which also\n" +
			"clears those references. Removing a login does not revoke the token.",
		Example: "  agx login rm personal\n  agx login rm personal --force",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE:    func(cmd *cobra.Command, args []string) error { return a.runLoginRm(args[0], force) },
	}
	c.Flags().BoolVar(&force, "force", false, "remove even when profiles use it, clearing their login:")
	return c
}

func (a *app) runLoginRm(name string, force bool) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	l, ok := cfg.LoginByName(name)
	if !ok {
		return errUnknownLogin(name)
	}
	if used := cfg.UsedBy(name); len(used) > 0 && !force {
		return fmt.Errorf("login %s is used by %s — point them elsewhere (`agx login use home -p NAME`) or pass --force", name, strings.Join(used, ", "))
	}
	ed := a.editor(cfg)
	cleared, err := ed.RemoveLogin(name)
	if err != nil {
		return err
	}
	a.loaded = false
	var b strings.Builder
	for _, c := range cleared {
		fmt.Fprintf(&b, "cleared %s\n", c)
	}
	if l.Source == login.SourceStored {
		tf, err := a.tokenFile()
		if err != nil {
			return err
		}
		if _, err := tf.Remove(name); err != nil {
			return fmt.Errorf("removed from config, but tokens.yaml: %w", err)
		}
	}
	fmt.Fprintf(&b, "removed login %s\n", name)
	if _, err := io.WriteString(a.Stdout, b.String()); err != nil {
		return err
	}
	a.warn(revokeHint)
	return nil
}

func newLoginUseCmd(a *app) *cobra.Command {
	var profile, prov string
	c := &cobra.Command{
		Use:   "use NAME|home (-p PROFILE | --provider ID)",
		Short: "Set the default login of a profile or a whole provider",
		Long: "Edits config.yaml in place (comments kept) and prints the changed key before and after.\n" +
			"For this terminal only, use the `aglogin NAME` shell function from `agx shell-init`.",
		Example: "  agx login use personal -p work          # clw spends personal quota\n" +
			"  agx login use home -p work              # back to the work home's own login\n" +
			"  agx login use personal --provider claude\n" +
			"  agx login use home --provider claude",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return a.runLoginUse(args[0], profile, prov) },
	}
	c.Flags().StringVarP(&profile, flagProfile, "p", "", "profile whose default login to set")
	c.Flags().StringVar(&prov, flagProvider, "", "provider whose default login to set (every profile of it)")
	return c
}

func (a *app) runLoginUse(name, profile, prov string) error {
	if (profile == "") == (prov == "") {
		return usageErr(errors.New("give exactly one of -p PROFILE or --provider ID"))
	}
	if name == login.Auto {
		return usageErr(config.ErrAutoLogin)
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	var l login.Login
	if name != login.Home {
		var ok bool
		if l, ok = cfg.LoginByName(name); !ok {
			return errUnknownLogin(name)
		}
	}
	ed := a.editor(cfg)
	var ch config.Change
	if profile != "" {
		p, ok := cfg.ByName(profile)
		if !ok {
			return errUnknownProfile(profile)
		}
		if name != login.Home {
			if err := a.loginFits(p, l); err != nil {
				return usageErr(err)
			}
		}
		ch, err = ed.SetProfileLogin(cfg, p, name)
	} else {
		id := provider.ID(prov)
		if _, err := a.loginLauncher(id); err != nil {
			return usageErr(err)
		}
		if name != login.Home && l.Provider != id {
			return usageErr(fmt.Errorf("login %s is a %s login, not %s", name, l.Provider, id))
		}
		ch, err = ed.SetProviderLogin(id, name)
	}
	if err != nil {
		return err
	}
	a.loaded = false
	var b strings.Builder
	for _, n := range ch.Notes {
		fmt.Fprintf(&b, "note: %s\n", n)
	}
	fmt.Fprintf(&b, "%s: %s\n- %s: %s\n+ %s: %s\n", render.ShortenHome(cfg.Path, a.UserHome), ch.Where, flagLogin, ch.Before, flagLogin, ch.After)
	_, err = io.WriteString(a.Stdout, b.String())
	return err
}

// newLoginExistsCmd backs the `aglogin` shell function: exit 0 when NAME
// can be exported as AGX_LOGIN, so a typo fails now, not on the next run.
func newLoginExistsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:    "exists NAME",
		Short:  "Exit 0 when NAME is a login or \"home\" (used by aglogin)",
		Hidden: true,
		Args:   usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case login.Home:
				return nil
			case login.Auto:
				return usageErr(config.ErrAutoLogin)
			}
			cfg, err := a.config()
			if err != nil {
				return err
			}
			if _, ok := cfg.LoginByName(args[0]); !ok {
				names := make([]string, 0, len(cfg.Logins))
				for _, l := range cfg.Logins {
					names = append(names, l.Name)
				}
				slices.Sort(names)
				return fmt.Errorf("unknown login %q (known: %s, home)", args[0], strings.Join(names, ", "))
			}
			return nil
		},
	}
}
