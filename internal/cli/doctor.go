package cli

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/internal/shellinit"
	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
	"github.com/khanakia/agx/secret"
)

// Check levels.
type level string

const (
	levelOK   level = "ok"
	levelInfo level = "info"
	levelWarn level = "warn"
	levelFail level = "fail"
)

// levelMarks are the text-mode prefixes.
var levelMarks = map[level]string{levelOK: "✓", levelInfo: "·", levelWarn: "!", levelFail: "✗"}

// loggedInNoEmail describes a login whose email is only known server-side.
const loggedInNoEmail = "logged in"

// areaWidth aligns the check-name column in text output.
const areaWidth = 18

// gitDirName marks a git work tree when walking up from sessions.root.
const gitDirName = ".git"

// Doctor areas: the first column of the text report and `area` in JSON.
// Scripts filter on them, so they are fixed names; per-item areas are the
// prefix plus the profile or login name.
const (
	areaConfig       = "config"
	areaSessionsRoot = "sessions.root"
	areaShell        = "shell"
	areaProfilePfx   = "profile "
	areaLoginPfx     = "login "
)

// finding is one doctor check result.
type finding struct {
	Area   string `json:"area"`
	Level  level  `json:"level"`
	Detail string `json:"detail"`
}

func newDoctorCmd(a *app) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check config, profiles, logins, binaries, secrets and shell integration",
		Long: "Runs local checks only (no network, never resolves secret values) and explains where\n" +
			"each setting came from. Exits 1 when any check fails.",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			fs := a.doctor()
			if asJSON {
				if err := output.JSON(a.Stdout, kindDoctorReport, fs, len(fs)); err != nil {
					return err
				}
			} else {
				var b strings.Builder
				for _, f := range fs {
					fmt.Fprintf(&b, "%s %-*s %s\n", levelMarks[f.Level], areaWidth, f.Area, f.Detail)
				}
				if _, err := io.WriteString(a.Stdout, b.String()); err != nil {
					return err
				}
			}
			for _, f := range fs {
				if f.Level == levelFail {
					return errPartial
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

func (a *app) doctor() []finding {
	var out []finding
	add := func(area string, l level, detail string) { out = append(out, finding{area, l, detail}) }

	path, rung, err := a.ConfigPath()
	if err != nil {
		add(areaConfig, levelFail, err.Error())
		return out
	}
	cfg, err := a.config()
	if err != nil {
		add(areaConfig, levelFail, err.Error())
		return out
	}
	if cfg.Found {
		add(areaConfig, levelOK, fmt.Sprintf("%s (decided by %s)", render.ShortenHome(path, a.UserHome), rung))
	} else {
		add(areaConfig, levelInfo, fmt.Sprintf("no file at %s — using discovered profiles and defaults", render.ShortenHome(path, a.UserHome)))
	}

	switch info, err := os.Stat(cfg.SessionsRoot); {
	case err != nil:
		add(areaSessionsRoot, levelInfo, render.ShortenHome(cfg.SessionsRoot, a.UserHome)+" does not exist yet (created by `agx new`)")
	case !info.IsDir():
		add(areaSessionsRoot, levelFail, cfg.SessionsRoot+" is not a directory")
	default:
		if repo := enclosingRepo(cfg.SessionsRoot); repo != "" {
			add(areaSessionsRoot, levelWarn, "inside git repo "+repo+" — Claude Code would pool every session's memory into it")
		} else {
			add(areaSessionsRoot, levelOK, render.ShortenHome(cfg.SessionsRoot, a.UserHome))
		}
	}

	now := a.Now()
	for _, p := range cfg.Profiles {
		area := areaProfilePfx + p.Name
		// Config-only checks first (secrets): they do not depend on the
		// binary or home, and must be reported even when those are broken —
		// the later checks `continue` on the first failure.
		// Sorted: map order would shuffle the report (and its JSON) run to run.
		for _, name := range slices.Sorted(maps.Keys(p.Secrets)) {
			ref, err := secret.Parse(p.Secrets[name])
			if err != nil {
				add(area, levelFail, name+": "+err.Error())
				continue
			}
			if ref.Scheme == secret.SchemeGopass {
				if _, err := a.LookPath(secret.GopassBin); err != nil {
					add(area, levelFail, name+" needs gopass, which is not on PATH")
				}
			}
		}
		for _, w := range a.apiBillingEnvWarnings(cfg, p) {
			add(area, levelWarn, w)
		}
		prov, err := a.providerFor(p.Provider)
		if err != nil {
			add(area, levelFail, err.Error())
			continue
		}
		if _, err := a.LookPath(prov.Binary()); err != nil {
			add(area, levelFail, prov.Binary()+" is not on PATH")
			continue
		}
		if _, err := os.Stat(p.Home); err != nil {
			add(area, levelWarn, "home "+p.Home+" does not exist")
			continue
		}
		d := a.describeProfile(p, now)
		switch d.Login {
		case loginOK:
			who := d.Email
			if who == "" {
				who = loggedInNoEmail // e.g. Codex keeps no email locally
			}
			add(area, levelOK, strings.TrimSpace(who+" "+d.Plan))
		case loginAPI:
			add(area, levelOK, "api-billed (secrets: "+fmt.Sprint(d.SecretVars)+")")
		case loginExpired:
			add(area, levelWarn, "login expired — run `agx run -p "+p.Name+"` once to refresh")
		case loginNone:
			add(area, levelWarn, "not logged in — run `agx run -p "+p.Name+"` and log in")
		default:
			add(area, levelFail, d.Error)
		}
	}

	out = append(out, a.doctorLogins(cfg)...)

	if v, ok := a.LookupEnv(appmeta.EnvShellInit); ok && v != "" {
		add(areaShell, levelOK, "shell layer active ("+v+")")
	} else {
		add(areaShell, levelInfo, fmt.Sprintf("shell layer not loaded — add `eval \"$(%s shell-init %s)\"` to your shell rc so new/resume can cd", appmeta.Name, shellinit.Zsh))
	}
	return out
}

// enclosingRepo returns the git work tree containing dir, or "".
func enclosingRepo(dir string) string {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, gitDirName)); err == nil {
			return d
		}
		if parent := filepath.Dir(d); parent == d {
			return ""
		}
	}
}

// apiBillingEnv are the variables that make Claude Code bill the API instead
// of the account's plan when they are present in its environment.
var apiBillingEnv = []string{claude.APIKeyEnv, claude.AuthTokenEnv}

// apiBillingEnvWarnings warns when the shell exports an API credential that
// a plan-billed Claude profile on its home login would inherit: Claude Code
// then bills the API, not the plan. agx removes these variables only for
// token logins (they set their own); a launch on the home login passes the
// environment through on purpose, so this is a warning, not a fix. A
// variable the profile sets itself (env / secrets) is the user's choice.
func (a *app) apiBillingEnvWarnings(cfg config.Config, p provider.Profile) []string {
	if p.Provider != provider.Claude || p.Billing != provider.BillingPlan {
		return nil
	}
	if lc, _, err := a.pickLogin(cfg, p, ""); err != nil || lc.Login != nil {
		return nil
	}
	var out []string
	for _, name := range apiBillingEnv {
		_, ownEnv := p.Env[name]
		_, ownSecret := p.Secrets[name]
		if v, ok := a.LookupEnv(name); ok && v != "" && !ownEnv && !ownSecret {
			out = append(out, name+" is set in this shell, so Claude Code bills the API instead of this plan — unset it, or give the profile a login (`agx login use`)")
		}
	}
	return out
}

// doctorLogins checks every login without resolving any value: token file
// permissions and location, stored entries present, env sources set, and
// expiry (warn within expiryWarnWindow, fail after).
func (a *app) doctorLogins(cfg config.Config) []finding {
	var out []finding
	add := func(area string, l level, detail string) { out = append(out, finding{area, l, detail}) }
	tf, err := a.tokenFile()
	if err != nil {
		add(areaTokens, levelFail, err.Error())
		return out
	}
	shown := render.ShortenHome(tf.Path, a.UserHome)
	var stored map[string]string
	refused := false
	if _, statErr := os.Stat(tf.Path); statErr == nil {
		if err := tf.CheckPerm(); err != nil {
			add(areaTokens, levelFail, err.Error())
			refused = true
		} else {
			add(areaTokens, levelOK, shown+" (0600)")
		}
		if info, err := os.Stat(filepath.Dir(tf.Path)); err == nil && info.Mode().Perm()&tokenDirOtherBits != 0 {
			add(areaTokens, levelWarn, fmt.Sprintf("%s is mode %04o; keep it private: chmod 700 %s", filepath.Dir(tf.Path), info.Mode().Perm(), filepath.Dir(tf.Path)))
		}
		if repo := enclosingRepo(filepath.Dir(tf.Path)); repo != "" {
			add(areaTokens, levelWarn, "inside git work tree "+repo+" — raw tokens must never be committed")
		}
		if !refused {
			if stored, err = tf.Load(); err != nil {
				add(areaTokens, levelFail, err.Error())
				refused = true
			}
		}
	}
	now := a.Now()
	for _, l := range cfg.Logins {
		area := areaLoginPfx + l.Name
		switch l.Source {
		case login.SourceStored:
			if refused {
				add(area, levelFail, "token unreadable: "+shown+" was refused (see above)")
				continue
			}
			if _, ok := stored[l.Name]; !ok {
				add(area, levelFail, "no token in "+shown+" — run `agx login add "+l.Name+" --stdin --replace`")
				continue
			}
		case login.SourceEnv:
			if _, ok := a.LookupEnv(l.Env); !ok {
				// One row per login: an unset variable is the finding; an
				// expiry row after it would contradict it.
				add(area, levelWarn, l.Env+" is not set in this shell")
				continue
			}
		case login.SourceCmd:
		}
		at, ok := a.expiry(l)
		approx := ""
		if !login.Owned(l) {
			approx = "approximately "
		}
		switch {
		case !ok:
			add(area, levelOK, string(l.Kind)+" ("+string(l.Source)+"), no known expiry")
		case !now.Before(at) && login.Owned(l):
			add(area, levelFail, fmt.Sprintf("expired %s; renew: agx login add %s --setup --replace", at.Format(time.DateOnly), l.Name))
		case !now.Before(at):
			// D13: agx does not know when a cmd/env token was minted, so
			// runs go ahead; this is a reminder, not a failure.
			add(area, levelWarn, fmt.Sprintf("past its approximate expiry %s; runs go ahead — if the token was not rotated, renew: agx login add %s --replace", at.Format(time.DateOnly), l.Name))
		case at.Sub(now) < expiryWarnWindow:
			add(area, levelWarn, fmt.Sprintf("expires %s%s (in %s); renew: agx login add %s --setup --replace", approx, at.Format(time.DateOnly), render.FormatUntil(at.Sub(now)), l.Name))
		default:
			add(area, levelOK, fmt.Sprintf("%s (%s), expires %s%s", l.Kind, l.Source, approx, at.Format(time.DateOnly)))
		}
	}
	return out
}

// areaTokens is the doctor area for tokens.yaml (login.TokenFileName).
const areaTokens = login.TokenFileName

// tokenDirOtherBits are the group/other bits that should be clear on the
// dir holding tokens.yaml (agx's config dir, 0700 by the logins spec §4.1).
const tokenDirOtherBits os.FileMode = 0o077
