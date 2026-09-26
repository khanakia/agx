package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/khanakia/voltkit/output"
	"github.com/spf13/cobra"

	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/internal/shellinit"
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
		add("config", levelFail, err.Error())
		return out
	}
	cfg, err := a.config()
	if err != nil {
		add("config", levelFail, err.Error())
		return out
	}
	if cfg.Found {
		add("config", levelOK, fmt.Sprintf("%s (decided by %s)", render.ShortenHome(path, a.UserHome), rung))
	} else {
		add("config", levelInfo, fmt.Sprintf("no file at %s — using discovered profiles and defaults", render.ShortenHome(path, a.UserHome)))
	}

	switch info, err := os.Stat(cfg.SessionsRoot); {
	case err != nil:
		add("sessions.root", levelInfo, render.ShortenHome(cfg.SessionsRoot, a.UserHome)+" does not exist yet (created by `agx new`)")
	case !info.IsDir():
		add("sessions.root", levelFail, cfg.SessionsRoot+" is not a directory")
	default:
		if repo := enclosingRepo(cfg.SessionsRoot); repo != "" {
			add("sessions.root", levelWarn, "inside git repo "+repo+" — Claude Code would pool every session's memory into it")
		} else {
			add("sessions.root", levelOK, render.ShortenHome(cfg.SessionsRoot, a.UserHome))
		}
	}

	now := a.Now()
	for _, p := range cfg.Profiles {
		area := "profile " + p.Name
		// Config-only checks first (secrets): they do not depend on the
		// binary or home, and must be reported even when those are broken —
		// the later checks `continue` on the first failure.
		for name, raw := range p.Secrets {
			ref, err := secret.Parse(raw)
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

	if v, ok := a.LookupEnv(appmeta.EnvShellInit); ok && v != "" {
		add("shell", levelOK, "shell layer active ("+v+")")
	} else {
		add("shell", levelInfo, fmt.Sprintf("shell layer not loaded — add `eval \"$(%s shell-init %s)\"` to your shell rc so new/resume can cd", appmeta.Name, shellinit.Zsh))
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
