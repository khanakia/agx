package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/sessions"
)

// Backups of moved conversations go under <source home>/backups/ — the
// folder Claude Code itself uses for backups — named after the folder and
// the time of the move, so repeated moves never collide.
const (
	backupsDir         = "backups"
	moveBackupPrefix   = "agx-move-"
	moveBackupStampFmt = "20060102-150405"
	kindSessionMove    = "session.move"
)

// Errors surfaced as usage errors (exit 2): the request itself is wrong.
var (
	errCrossProvider = errors.New("providers store conversations in different formats")
	errSameHome      = errors.New("source and target use the same home, so there is nothing to move")
)

// errNoMover is the fallback reason for a provider that neither moves
// conversations nor explains why.
var errNoMover = errors.New("this provider cannot move conversations")

// errMoveBlocked is returned (exit 1) after printing a plan with blockers.
var errMoveBlocked = errors.New("move blocked — see the reasons above; nothing was changed")

type moveOpts struct {
	to       string
	from     string
	only     []string
	dryRun   bool
	noBackup bool
	json     bool
	pick     bool
	all      bool
}

func newSessionsMoveCmd(a *app) *cobra.Command {
	o := &moveOpts{}
	c := &cobra.Command{
		Use:   "move [folder] --to <profile>",
		Short: "Move a folder's conversations to another account of the same provider",
		Long: "Moves every conversation started in [folder] (default: here) from the other accounts of\n" +
			"the target's provider into the --to account, so `claude -c`, `agx resume` and friends\n" +
			"continue them there. The target profile decides the provider; moving between providers\n" +
			"is refused. Aborts before changing anything if the agent is running in the folder or a\n" +
			"conversation already exists in the target. Sources are backed up first (--no-backup to skip).",
		Example: "  agx sessions move --to work --dry-run          # this folder, show the plan\n" +
			"  agx sessions move --to work                    # this folder, personal → work\n" +
			"  agx sessions move docker_setup_mac --to work   # a folder by name or path\n" +
			"  agx sessions move . --to personal --from work  # back again\n" +
			"  agx sessions move . --to work --only aeba9843  # one conversation (id or unique prefix)\n" +
			"  agx sessions move -i                           # pick this folder's chats, then the target\n" +
			"  agx sessions move -i --all --to work --dry-run # pick any folder, then its chats",

		Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			folder := "."
			if len(args) == 1 {
				folder = args[0]
			}
			if o.pick {
				if len(args) == 1 || len(o.only) > 0 {
					return usageErr(errors.New("--pick chooses the folder and the chats; drop the folder argument and --only"))
				}
				dir, err := a.pickMove(ctxOf(cmd), o)
				if err != nil {
					return err
				}
				folder = dir
			}
			return a.runMove(ctxOf(cmd), folder, o)
		},
	}
	f := c.Flags()
	f.StringVar(&o.to, "to", "", "target profile (required) — decides the provider")
	f.StringVar(&o.from, "from", "", "only move from this profile (default: every other account of the provider)")
	f.StringSliceVar(&o.only, "only", nil, "only these conversation ids (full id or unique prefix; comma-separated)")
	f.BoolVar(&o.dryRun, flagDryRun, false, "print the plan and change nothing")
	f.BoolVar(&o.noBackup, "no-backup", false, "skip copying the sources to <home>/backups first")
	f.BoolVar(&o.json, flagJSON, false, "print the JSON envelope")
	f.BoolVarP(&o.pick, "pick", "i", false, "pick this folder's chats to move (and --to, when not given); --all to pick the folder first")
	f.BoolVar(&o.all, "all", false, "with --pick: choose the folder from every workspace first")
	return c
}

// pickMove is `sessions move --pick`. It is scoped to the folder you are
// in and lists its chats; only --all opens the workspace picker first (no
// automatic global fallback when this folder has none). The chosen chats
// become --only, and --to is picked when not given. Returns the folder.
func (a *app) pickMove(ctx context.Context, o *moveOpts) (string, error) {
	cfg, err := a.config()
	if err != nil {
		return "", err
	}
	cwd, err := a.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	var chats []conversationView
	if !o.all {
		convs, scoped, err := a.findConversations(ctx, cfg, cwd, false, 0)
		if err != nil {
			return "", err
		}
		if scoped && len(convs) > 0 {
			chats = conversationViews(cfg, convs)
		}
	}
	if !o.all && chats == nil {
		// Scoped on purpose: the user asked for "this folder" by default and
		// a global list only by name (--all), never as a silent fallback.
		return "", fmt.Errorf("no chats in %s; use --all to pick from every folder", render.ShortenHome(cwd, a.UserHome))
	}
	if o.all {
		ws, err := a.loadWorkspaces(ctx, cfg, "")
		if err != nil {
			return "", err
		}
		wi, err := a.pickWorkspace(ws)
		if err != nil {
			return "", err
		}
		dir, chats = ws[wi].Dir, ws[wi].convs
	}
	picked, err := a.pickChats(chats)
	if err != nil {
		return "", err
	}
	o.only = nil
	for _, v := range picked {
		o.only = append(o.only, v.ID)
	}
	if o.to == "" {
		to, err := a.pickMoveTarget(cfg, picked)
		if err != nil {
			return "", err
		}
		o.to = to
	}
	return dir, nil
}

// pickChats multi-selects conversations in the resume row format, falling
// back to a single choice when the picker cannot select several.
func (a *app) pickChats(chats []conversationView) ([]conversationView, error) {
	items := make([]string, len(chats))
	for i, v := range chats {
		items[i] = a.conversationLine(v)
	}
	var idx []int
	var err error
	if mp, ok := a.Picker.(MultiPicker); ok {
		idx, err = mp.PickMany("move (tab: mark)", items)
	} else {
		var i int
		i, err = a.Picker.Pick("move", items)
		idx = []int{i}
	}
	if errors.Is(err, ErrCancelled) {
		return nil, &ExitError{Code: ExitFailure, Err: err}
	}
	if err != nil {
		return nil, err
	}
	out := make([]conversationView, len(idx))
	for i, n := range idx {
		out[i] = chats[n]
	}
	return out, nil
}

// pickMoveTarget lists the profiles the chosen chats could move to (same
// provider as one of them), each marked with how many of those chats its
// home holds now, and returns the chosen profile's name.
func (a *app) pickMoveTarget(cfg config.Config, chats []conversationView) (string, error) {
	here := map[provider.ID]bool{}
	held := map[string]int{} // provider + clean home → chats in this folder
	key := func(id provider.ID, home string) string { return string(id) + "\x00" + filepath.Clean(home) }
	for _, v := range chats {
		here[v.Provider] = true
		held[key(v.Provider, v.Home)]++
	}
	var names, items []string
	for _, p := range cfg.Profiles {
		if !here[p.Provider] {
			continue
		}
		mark := ""
		if n := held[key(p.Provider, p.Home)]; n > 0 {
			mark = fmt.Sprintf("← %d chat(s) here now", n)
		}
		names = append(names, p.Name)
		items = append(items, fmt.Sprintf("%-*s %-*s %-*s %s", profileWidth, p.Name, providerWidth, p.Provider,
			dirWidth, render.ShortenHome(p.Home, a.UserHome), mark))
	}
	if len(names) == 0 {
		return "", errors.New("no profile shares a provider with this folder's conversations")
	}
	i, err := a.Picker.Pick("move to", items)
	if errors.Is(err, ErrCancelled) {
		return "", &ExitError{Code: ExitFailure, Err: err}
	}
	if err != nil {
		return "", err
	}
	return names[i], nil
}

// moveResult is one source home's entry in the session.move payload.
type moveResult struct {
	provider.MovePlan
	Titles  map[string]string `json:"titles,omitempty"`
	Backup  string            `json:"backup,omitempty"`
	DryRun  bool              `json:"dry_run"`
	Applied bool              `json:"applied"`
}

func (a *app) runMove(ctx context.Context, folder string, o *moveOpts) error {
	if o.to == "" {
		return usageErr(errors.New("--to <profile> is required (see `agx profiles`)"))
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	to, ok := cfg.ByName(o.to)
	if !ok {
		return usageErr(fmt.Errorf("unknown profile %q (see `agx profiles`)", o.to))
	}
	// Order of checks, most useful message first: a malformed request
	// (cross-provider, same home) is wrong whatever the provider can do; then
	// whether the provider can move at all; then which accounts exist.
	from, hasFrom, err := a.checkMovePair(cfg, to, o.from)
	if err != nil {
		return err
	}
	prov, err := a.providerFor(to.Provider)
	if err != nil {
		return err
	}
	mover, ok := prov.(provider.ConversationMover)
	if !ok {
		reason := errNoMover.Error()
		if u, ok := prov.(provider.MoveUnsupported); ok {
			reason = u.MoveUnsupportedReason()
		}
		return fmt.Errorf("moving %s conversations is not supported: %s", to.Provider, reason)
	}
	sources, err := a.moveSources(cfg, to, from, hasFrom)
	if err != nil {
		return err
	}
	dir, err := a.resolveFolder(cfg, folder)
	if err != nil {
		return err
	}

	var results []moveResult
	blocked := false
	for _, from := range sources {
		plan, err := mover.PlanMove(ctx, provider.MoveRequest{FromHome: from.Home, ToHome: to.Home, Dir: dir, OnlyIDs: o.only})
		if err != nil {
			return err
		}
		if len(plan.Conversations) == 0 {
			continue
		}
		r := moveResult{MovePlan: plan, DryRun: o.dryRun, Titles: a.conversationTitles(ctx, prov, from.Home, dir)}
		blocked = blocked || len(plan.Blockers) > 0
		results = append(results, r)
	}
	if len(results) == 0 {
		a.note(fmt.Sprintf("no %s conversations for %s outside %s — nothing to move", to.Provider, render.ShortenHome(dir, a.UserHome), to.Name))
		if o.json {
			return output.JSON(a.Stdout, kindSessionMove, results, 0)
		}
		return nil
	}

	if !blocked && !o.dryRun {
		for i := range results {
			if !o.noBackup {
				results[i].Backup = filepath.Join(results[i].FromHome, backupsDir,
					moveBackupPrefix+sessions.Slug(filepath.Base(dir))+"-"+a.Now().Format(moveBackupStampFmt))
			}
			if err := provider.ApplyMovePlan(results[i].MovePlan, results[i].Backup); err != nil {
				return err
			}
			results[i].Applied = true
		}
	}

	if o.json {
		if err := output.JSON(a.Stdout, kindSessionMove, results, len(results)); err != nil {
			return err
		}
	} else if err := a.printMove(results, to); err != nil {
		return err
	}
	if blocked {
		return &ExitError{Code: ExitFailure, Err: errMoveBlocked}
	}
	return nil
}

// checkMovePair resolves and validates an explicit --from against --to:
// same provider, different homes. hasFrom is false when --from was not given
// (sources are then derived by moveSources).
func (a *app) checkMovePair(cfg config.Config, to provider.Profile, fromName string) (from provider.Profile, hasFrom bool, err error) {
	if fromName == "" {
		return provider.Profile{}, false, nil
	}
	from, ok := cfg.ByName(fromName)
	if !ok {
		return provider.Profile{}, false, usageErr(fmt.Errorf("unknown profile %q (see `agx profiles`)", fromName))
	}
	if from.Provider != to.Provider {
		return provider.Profile{}, false, usageErr(fmt.Errorf("can't move conversations from %s (%s) to %s (%s): %w — choose a --to profile of the same provider",
			from.Name, from.Provider, to.Name, to.Provider, errCrossProvider))
	}
	if filepath.Clean(from.Home) == filepath.Clean(to.Home) {
		return provider.Profile{}, false, usageErr(fmt.Errorf("%s and %s both use %s: %w", from.Name, to.Name, render.ShortenHome(to.Home, a.UserHome), errSameHome))
	}
	return from, true, nil
}

// moveSources returns the homes to move from: the validated --from, or
// every other home of the target's provider.
func (a *app) moveSources(cfg config.Config, to, from provider.Profile, hasFrom bool) ([]provider.Profile, error) {
	if hasFrom {
		return []provider.Profile{from}, nil
	}
	var out []provider.Profile
	for _, p := range cfg.Homes() {
		if p.Provider == to.Provider && filepath.Clean(p.Home) != filepath.Clean(to.Home) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, usageErr(fmt.Errorf("%s is the only %s account: %w", to.Name, to.Provider, errSameHome))
	}
	return out, nil
}

// conversationTitles maps id → title for display; best effort.
func (a *app) conversationTitles(ctx context.Context, prov provider.Provider, home, dir string) map[string]string {
	lister, ok := prov.(provider.ConversationLister)
	if !ok {
		return nil
	}
	convs, err := lister.Conversations(ctx, home, provider.ConversationQuery{Dir: dir})
	if err != nil {
		return nil
	}
	titles := map[string]string{}
	for _, c := range convs {
		if c.Title != "" {
			titles[c.ID] = c.Title
		}
	}
	return titles
}

// printMove renders the plan / result as text.
func (a *app) printMove(results []moveResult, to provider.Profile) error {
	var b strings.Builder
	for _, r := range results {
		verb := "moved"
		switch {
		case len(r.Blockers) > 0:
			verb = "can't move"
		case r.DryRun:
			verb = "would move"
		}
		fmt.Fprintf(&b, "%s %d %s conversation(s) for %s\n  from %s → %s (%s)\n", verb, len(r.Conversations), r.Provider,
			render.ShortenHome(r.Dir, a.UserHome), render.ShortenHome(r.FromHome, a.UserHome), render.ShortenHome(r.ToHome, a.UserHome), to.Name)
		for _, id := range r.Conversations {
			title := r.Titles[id]
			if title == "" {
				title = untitled
			}
			fmt.Fprintf(&b, "    %s  %s\n", id, title)
		}
		fmt.Fprintf(&b, "  %d file(s)/folder(s) involved\n", len(r.Items))
		for _, bl := range r.Blockers {
			fmt.Fprintf(&b, "  ✗ %s\n", bl)
		}
		for _, l := range r.Left {
			fmt.Fprintf(&b, "  · left in place: %s\n", render.ShortenHome(l, a.UserHome))
		}
		if r.Backup != "" && r.Applied {
			fmt.Fprintf(&b, "  backup: %s\n", render.ShortenHome(r.Backup, a.UserHome))
		}
	}
	if len(results) > 0 && results[0].Applied {
		fmt.Fprintf(&b, "done — continue with: cd %s && agx resume  (or claude -c on %s)\n", render.ShortenHome(results[0].Dir, a.UserHome), to.Name)
	}
	_, err := io.WriteString(a.Stdout, b.String())
	return err
}
