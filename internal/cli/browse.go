package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/output"

	"github.com/khanakia/agx/config"
	"github.com/khanakia/agx/internal/render"
)

// Workspace row layout. The folder is the column the user navigates by, so
// it gets the width the resume rows spend on title and id.
const (
	countWidth    = 9 // "123 chats"
	profilesWidth = 20
	wsDirWidth    = 56
)

// workspace is one folder that has conversations, across every profile and
// provider: what `resume --browse` lists before the user picks a chat.
type workspace struct {
	Dir     string    `json:"dir"`
	Updated time.Time `json:"updated"`
	Count   int       `json:"count"`
	// Profiles are the profiles the folder's conversations resume on, in
	// order of their newest conversation.
	Profiles []string `json:"profiles"`
	// convs are the folder's conversations, newest first (not in JSON: the
	// conversation.list kind carries them).
	convs []conversationView
}

// newWsCmd is `agx ws`, a top-level name for `resume --browse`: the user
// asked to navigate chats without remembering which folder holds them, and a
// short command is what gets typed. It shares resumeOpts, so ws and
// resume -b cannot drift apart.
func newWsCmd(a *app) *cobra.Command {
	o := &resumeOpts{browse: true}
	c := &cobra.Command{
		Use:   "ws [query...] [-- agent-args...]",
		Short: "Pick a workspace, then a chat in it, and resume it (resume --browse)",
		Long: "Lists every folder that has conversations (newest activity first, across all profiles\n" +
			"and providers); pick one, then pick a chat in it, and it resumes on the profile whose\n" +
			"home stores it. Esc on the chat list goes back to the workspaces. Words after `--` are\n" +
			"passed to the agent.",
		Example: "  agx ws                # browse workspaces → chats\n" +
			"  agx ws invoice        # only workspaces with a chat matching \"invoice\"\n" +
			"  agx ws --list         # print the workspaces",
		Args: cobra.ArbitraryArgs,
		RunE: a.runResumeArgs(o),
	}
	f := c.Flags()
	f.BoolVar(&o.list, "list", false, "print the workspaces instead of picking")
	f.BoolVar(&o.json, flagJSON, false, "with --list: print the JSON envelope")
	f.BoolVar(&o.dryRun, flagDryRun, false, "print what would run and exit")
	f.StringVarP(&o.profile, flagProfile, "p", "", "resume on this profile instead of the home's own (advanced)")
	bindLoginFlag(f, &o.login)
	return c
}

// groupWorkspaces groups conversations (newest first) by folder, keeping
// that order: a workspace's activity is its newest conversation.
func groupWorkspaces(vs []conversationView) []workspace {
	var out []workspace
	at := map[string]int{}
	for _, v := range vs {
		i, ok := at[v.Dir]
		if !ok {
			i = len(out)
			at[v.Dir] = i
			out = append(out, workspace{Dir: v.Dir, Updated: v.Updated})
		}
		w := &out[i]
		w.Count++
		w.convs = append(w.convs, v)
		if !slices.Contains(w.Profiles, v.Profile) {
			w.Profiles = append(w.Profiles, v.Profile)
		}
	}
	return out
}

// tailDir fits a folder into n runes keeping its END, the part that names
// the project: `~`-relative first, then cut from the left.
func tailDir(dir, userHome string, n int) string {
	r := []rune(render.ShortenHome(dir, userHome))
	if len(r) <= n {
		return string(r)
	}
	return ellipsis + string(r[len(r)-(n-1):])
}

// workspaceLine formats one level-1 row on a single line.
func (a *app) workspaceLine(w workspace) string {
	chats := fmt.Sprintf("%d chats", w.Count)
	if w.Count == 1 {
		chats = "1 chat"
	}
	return fmt.Sprintf("%-*s %-*s %-*s %s",
		agoWidth, render.FormatAgo(a.Now(), w.Updated),
		countWidth, chats,
		profilesWidth, truncate(strings.Join(w.Profiles, ","), profilesWidth),
		tailDir(w.Dir, a.UserHome, wsDirWidth))
}

// runBrowse is `resume --browse`: workspaces first, then that folder's
// chats; Esc on the chats returns to the workspaces, Esc on the workspaces
// exits. Every folder is searched — the point is not having to cd first.
func (a *app) runBrowse(ctx context.Context, cfg config.Config, o *resumeOpts, q string) error {
	ws, err := a.loadWorkspaces(ctx, cfg, q)
	if err != nil {
		return err
	}
	if o.list {
		if o.json {
			return output.JSON(a.Stdout, kindWorkspaceList, ws, len(ws))
		}
		var b strings.Builder
		for _, w := range ws {
			b.WriteString(a.workspaceLine(w))
			b.WriteByte('\n')
		}
		_, err := io.WriteString(a.Stdout, b.String())
		return err
	}
	for {
		wi, err := a.pickWorkspace(ws)
		if err != nil {
			return err
		}
		chats := ws[wi].convs
		// Esc here means "back to the workspaces", so not pickOne.
		ci, err := a.Picker.Pick("chat (esc: back)", a.conversationItems(chats))
		if errors.Is(err, ErrCancelled) {
			continue
		}
		if err != nil {
			return err
		}
		return a.resumeConversation(ctx, cfg, o, chats[ci].Conversation)
	}
}

// loadWorkspaces lists every folder with conversations matching q, across
// every profile and provider, newest activity first. Every folder is
// searched: the point is not having to cd (or remember a name) first.
func (a *app) loadWorkspaces(ctx context.Context, cfg config.Config, q string) ([]workspace, error) {
	cwd, err := a.Getwd()
	if err != nil {
		return nil, err
	}
	convs, err := a.findConversations(ctx, cfg, cwd, true, 0)
	if err != nil {
		return nil, err
	}
	all := groupWorkspaces(conversationViews(cfg, filterConversations(convs, q)))
	// A deleted or unmounted folder can be neither resumed in nor moved, so
	// it is hidden rather than offered and then refused.
	ws := all[:0]
	for _, w := range all {
		if info, err := os.Stat(w.Dir); err == nil && info.IsDir() {
			ws = append(ws, w)
		}
	}
	return ws, nil
}

// pickWorkspace is the level-1 picker shared by `resume --browse` and
// `sessions move --pick`. Esc exits non-zero (ExitError), so callers only
// handle the chosen index.
func (a *app) pickWorkspace(ws []workspace) (int, error) {
	if len(ws) == 0 {
		return 0, errors.New("no matching conversations")
	}
	items := make([]string, len(ws))
	for i, w := range ws {
		items[i] = a.workspaceLine(w)
	}
	return a.pickOne("workspace", items)
}
