package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/khanakia/voltkit/output"
	"github.com/spf13/cobra"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/sessions"
)

// defaultGCMinAge protects folders created recently (an agent may be about
// to write into them).
const defaultGCMinAge = 24 * time.Hour

// gitBin is run by `promote --git-init`.
const gitBin = "git"

// Yes / no cells.
const (
	cellYes = "yes"
	cellNo  = "—"
)

func newSessionsCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "sessions",
		Short: "Session folders: list, clean up, promote to a project, move to another account",
		Args:  usageArgs(cobra.NoArgs),
	}
	c.AddCommand(newSessionsLsCmd(a), newSessionsGCCmd(a), newSessionsPromoteCmd(a), newSessionsMoveCmd(a))
	return c
}

func newSessionsLsCmd(a *app) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List session folders with files and conversation history",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.config()
			if err != nil {
				return err
			}
			folders, err := sessions.List(cfg.SessionsRoot, a.historyFunc(cfg))
			if err != nil {
				return err
			}
			if asJSON {
				return output.JSON(a.Stdout, kindSessionList, folders, len(folders))
			}
			now := a.Now()
			rows := make([][]string, 0, len(folders))
			for _, f := range folders {
				rows = append(rows, []string{f.Name, render.FormatAgo(now, f.Modified), strconv.Itoa(f.Files), yesNo(f.HasHistory)})
			}
			a.note(fmt.Sprintf("%s (%d folders)", render.ShortenHome(cfg.SessionsRoot, a.UserHome), len(folders)))
			return render.Table(a.Stdout, []string{"FOLDER", "LAST ACTIVITY", "FILES", "HISTORY"}, rows)
		},
	}
	c.Flags().BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

func newSessionsGCCmd(a *app) *cobra.Command {
	var (
		yes    bool
		asJSON bool
		minAge time.Duration
	)
	c := &cobra.Command{
		Use:   "gc",
		Short: "Remove empty session folders that have no conversation history",
		Long: "A folder is removed only when it has no files (.DS_Store ignored), no provider holds a\n" +
			"conversation started in it, and it is older than --min-age. Without --yes nothing is\n" +
			"deleted. Deletion can only remove empty directories, so real work is never at risk.",
		Example: "  agx sessions gc            # show the plan\n  agx sessions gc --yes",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.config()
			if err != nil {
				return err
			}
			folders, err := sessions.List(cfg.SessionsRoot, a.historyFunc(cfg))
			if err != nil {
				return err
			}
			plan := sessions.PlanGC(folders, a.Now(), minAge)
			var failed int
			if yes {
				for _, it := range plan {
					if !it.Remove {
						continue
					}
					if err := sessions.RemoveEmpty(it.Folder.Path); err != nil {
						a.warn(err.Error())
						failed++
					}
				}
			}
			if asJSON {
				if err := output.JSON(a.Stdout, kindSessionGC, plan, len(plan)); err != nil {
					return err
				}
			} else if err := a.printGC(plan, yes); err != nil {
				return err
			}
			if failed > 0 {
				return errPartial
			}
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&yes, "yes", false, "actually remove the folders")
	f.BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	f.DurationVar(&minAge, "min-age", defaultGCMinAge, "keep folders younger than this")
	return c
}

// printGC lists removable folders on stdout (data) and a summary on stderr.
func (a *app) printGC(plan []sessions.GCItem, applied bool) error {
	var b strings.Builder
	var n int
	for _, it := range plan {
		if it.Remove {
			n++
			b.WriteString(it.Folder.Name + "\n")
		}
	}
	if _, err := io.WriteString(a.Stdout, b.String()); err != nil {
		return err
	}
	switch {
	case n == 0:
		a.note("nothing to remove")
	case applied:
		a.note(fmt.Sprintf("removed %d empty folder(s)", n))
	default:
		a.note(fmt.Sprintf("%d empty folder(s) would be removed — re-run with --yes", n))
	}
	return nil
}

func newSessionsPromoteCmd(a *app) *cobra.Command {
	var (
		to      string
		dryRun  bool
		gitInit bool
		asJSON  bool
	)
	c := &cobra.Command{
		Use:   "promote <folder> <name>",
		Short: "Move a session folder into a project, taking its Claude history along",
		Long: "Moves <folder> (a name under sessions.root, or a path) to <promote_root>/<name>.\n" +
			"Conversation history keyed by the folder's path (Claude Code) is moved too, so\n" +
			"`agx resume` keeps working from the new location. Never overwrites.",
		Example: "  agx sessions promote pglite_golang_20260831_123205 pglite-go\n" +
			"  agx sessions promote . my-tool --git-init",
		Args: usageArgs(cobra.ExactArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runPromote(ctxOf(cmd), args[0], args[1], to, dryRun, gitInit, asJSON)
		},
	}
	f := c.Flags()
	f.StringVar(&to, "to", "", "destination parent (default: sessions.promote_root)")
	f.BoolVar(&dryRun, flagDryRun, false, "print the plan and change nothing")
	f.BoolVar(&gitInit, "git-init", false, "run `git init` in the new location")
	f.BoolVar(&asJSON, flagJSON, false, "print the JSON envelope")
	return c
}

// promoteResult is the session.promote payload.
type promoteResult struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	DryRun  bool     `json:"dry_run"`
	History []string `json:"history_moved"`
	// Left lists homes whose history for the folder could not follow it
	// (providers that key history by id, e.g. Codex).
	Left    []string `json:"history_left,omitempty"`
	GitInit bool     `json:"git_init"`
}

// validFolderName requires a single path element, so a promote target can
// never escape the destination parent (no "..", no separators).
func validFolderName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return usageErr(fmt.Errorf("name must be a single folder name, got %q", name))
	}
	return nil
}

func (a *app) runPromote(ctx context.Context, folder, name, to string, dryRun, gitInit, asJSON bool) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	if err := validFolderName(name); err != nil {
		return err
	}
	src, err := a.resolveFolder(cfg, folder)
	if err != nil {
		return err
	}
	parent := cfg.PromoteRoot
	if to != "" {
		parent = to
	}
	dst := filepath.Join(parent, name)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%w: %s", sessions.ErrExists, dst)
	}

	res := promoteResult{From: src, To: dst, DryRun: dryRun, GitInit: gitInit}
	type move struct {
		mover provider.HistoryMover
		home  string
	}
	var moves []move
	for _, p := range cfg.Homes() {
		prov, err := a.providerFor(p.Provider)
		if err != nil {
			return err
		}
		checker, hasChecker := prov.(provider.HistoryChecker)
		if !hasChecker || !checker.HasHistory(p.Home, src) {
			continue
		}
		if mover, ok := prov.(provider.HistoryMover); ok {
			moves = append(moves, move{mover, p.Home})
			res.History = append(res.History, p.Home)
		} else {
			res.Left = append(res.Left, p.Home)
		}
	}

	if !dryRun {
		if err := sessions.Promote(src, dst); err != nil {
			return err
		}
		for _, m := range moves {
			if _, err := m.mover.MoveHistory(m.home, src, dst); err != nil {
				return fmt.Errorf("folder moved to %s, but history in %s did not move: %w", dst, m.home, err)
			}
		}
		if gitInit {
			c := exec.CommandContext(ctx, gitBin, "init", "-q", dst)
			c.Stdout, c.Stderr = a.Stderr, a.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("git init: %w", err)
			}
		}
	}

	if asJSON {
		return output.JSON(a.Stdout, kindSessionPromote, res, 0)
	}
	verb := "moved"
	if dryRun {
		verb = "would move"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s → %s\n", verb, render.ShortenHome(src, a.UserHome), render.ShortenHome(dst, a.UserHome))
	for _, h := range res.History {
		fmt.Fprintf(&b, "  history in %s %s with it\n", render.ShortenHome(h, a.UserHome), verb)
	}
	if _, err := io.WriteString(a.Stdout, b.String()); err != nil {
		return err
	}
	for _, h := range res.Left {
		a.warn(fmt.Sprintf("history in %s is keyed by id and stays; resume still finds it but opens the old path", render.ShortenHome(h, a.UserHome)))
	}
	return nil
}

// resolveFolder turns a folder argument into an existing directory: "." is
// the cwd; a bare name is looked up under sessions.root, then in the cwd,
// then matched against the cwd's own name; anything else is a path.
func (a *app) resolveFolder(cfg config.Config, arg string) (string, error) {
	var path string
	switch {
	case arg == ".":
		wd, err := a.Getwd()
		if err != nil {
			return "", err
		}
		path = wd
	case filepath.Base(arg) == arg:
		// A bare name is, in order: a session folder, a folder here, or the
		// folder you are in (`agx sessions move docker_setup_mac` typed from
		// inside docker_setup_mac must not look for docker_setup_mac/docker_setup_mac).
		path = filepath.Join(cfg.SessionsRoot, arg)
		if _, err := os.Stat(path); err != nil {
			wd, werr := a.Getwd()
			if werr != nil {
				return "", werr
			}
			path = filepath.Join(wd, arg)
			if _, err := os.Stat(path); err != nil && filepath.Base(wd) == arg {
				path = wd
			}
		}
	default:
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", err
		}
		path = abs
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", usageErr(fmt.Errorf("no such folder: %s", path))
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", usageErr(fmt.Errorf("not a folder: %s", path))
	}
	return filepath.Clean(path), nil
}

// historyFunc answers "does any provider hold a conversation for dir?"
// across every distinct home.
func (a *app) historyFunc(cfg config.Config) sessions.HistoryFunc {
	type check struct {
		checker provider.HistoryChecker
		home    string
	}
	var checks []check
	for _, p := range cfg.Homes() {
		prov, err := a.providerFor(p.Provider)
		if err != nil {
			continue
		}
		if c, ok := prov.(provider.HistoryChecker); ok {
			checks = append(checks, check{c, p.Home})
		}
	}
	return func(dir string) bool {
		for _, c := range checks {
			if c.checker.HasHistory(c.home, dir) {
				return true
			}
		}
		return false
	}
}

func yesNo(b bool) string {
	if b {
		return cellYes
	}
	return cellNo
}
