// Command conversations lists, re-keys and moves Claude Code conversations
// in a synthetic home: listing reads each transcript's head (cwd) and tail
// (title) only, a folder lookup goes straight to the EncodePath key, a moved
// project takes its history with MoveHistory, and a move between two homes
// is planned first and then applied with a backup.
//
// Transcripts are written by the program; no real home is read.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// Working folders the fake conversations were "started" in. They do not
// need to exist: Claude Code keys history by the path string.
const (
	apiDir   = "/srv/demo/api"
	webDir   = "/srv/demo/web"
	movedDir = "/srv/demo/api-v2"
)

// File modes for the fake home.
const (
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// transcript is one fake conversation: the folder it ran in, its records
// (JSON lines, compact as Claude Code writes them) and how long ago it was
// last written, which decides the newest-first order.
type transcript struct {
	dir   string
	id    string
	lines []string
	age   time.Duration
}

var transcripts = []transcript{
	{apiDir, "a1", []string{
		`{"type":"user","cwd":"/srv/demo/api","sessionId":"a1"}`,
		`{"type":"ai-title","aiTitle":"Fix login bug"}`,
		// A title the user set with /rename wins over the generated one.
		`{"type":"custom-title","customTitle":"Login fix (final)"}`,
	}, time.Hour},
	{apiDir, "a2", []string{`{"type":"user","cwd":"/srv/demo/api","sessionId":"a2"}`}, 2 * time.Hour},
	{webDir, "w1", []string{
		`{"type":"user","cwd":"/srv/demo/web","sessionId":"w1"}`,
		`{"type":"ai-title","aiTitle":"Add dark mode"}`,
	}, 30 * time.Minute},
	// No record carries a cwd: it cannot be resumed anywhere, so it is skipped.
	{webDir, "w0", []string{`{"type":"summary"}`}, 10 * time.Minute},
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	userHome, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	tilde := func(p string) string { return strings.Replace(p, userHome, "~", 1) }

	home := claude.DefaultHomeFor(userHome)
	now := time.Now()
	for _, t := range transcripts {
		if err := writeTranscript(home, t, now); err != nil {
			return err
		}
	}
	fmt.Printf("history key for %s: %s\n", apiDir, claude.EncodePath(apiDir))

	p := &claude.Provider{UserHome: userHome} // Store nil: no keychain
	ctx := context.Background()

	all, err := p.Conversations(ctx, home, provider.ConversationQuery{})
	if err != nil {
		return err
	}
	fmt.Println("all conversations, newest first:")
	printConversations(all)

	one, err := p.Conversations(ctx, home, provider.ConversationQuery{Dir: apiDir, Limit: 1})
	if err != nil {
		return err
	}
	fmt.Println("newest in", apiDir+":")
	printConversations(one)

	fmt.Printf("has history: %s=%v /srv/demo/none=%v\n", apiDir, p.HasHistory(home, apiDir), p.HasHistory(home, "/srv/demo/none"))

	// The project folder moved: re-key its history so resume finds it.
	moved, err := p.MoveHistory(home, apiDir, movedDir)
	if err != nil {
		return err
	}
	after, err := p.Conversations(ctx, home, provider.ConversationQuery{Dir: movedDir})
	if err != nil {
		return err
	}
	fmt.Printf("moved=%v; now listed under %s:\n", moved, movedDir)
	printConversations(after)

	// Move web's conversations to a second account's home: plan, check, apply.
	other := filepath.Join(userHome, ".claude-work")
	plan, err := p.PlanMove(ctx, provider.MoveRequest{FromHome: home, ToHome: other, Dir: webDir})
	if err != nil {
		return err
	}
	fmt.Printf("plan: %d conversation(s), %d item(s), %d blocker(s)\n", len(plan.Conversations), len(plan.Items), len(plan.Blockers))
	for _, it := range plan.Items {
		fmt.Printf("  %s\n    → %s\n", tilde(it.From), tilde(it.To))
	}
	backup := filepath.Join(userHome, "move-backup")
	if err := provider.ApplyMovePlan(plan, backup); err != nil {
		return err
	}
	movedConvs, err := p.Conversations(ctx, other, provider.ConversationQuery{Dir: webDir})
	if err != nil {
		return err
	}
	fmt.Printf("after apply, %s holds %d conversation(s) for %s\n", tilde(other), len(movedConvs), webDir)
	return nil
}

// writeTranscript writes t under home/projects/<EncodePath(dir)>/<id>.jsonl
// and backdates it by t.age.
func writeTranscript(home string, t transcript, now time.Time) error {
	dir := claude.HistoryDir(home, t.dir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	path := filepath.Join(dir, t.id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(t.lines, "\n")+"\n"), filePerm); err != nil {
		return err
	}
	mtime := now.Add(-t.age)
	return os.Chtimes(path, mtime, mtime)
}

// printConversations prints id, folder and title, one per line.
func printConversations(cs []provider.Conversation) {
	for _, c := range cs {
		fmt.Printf("  %-3s %-17s %q\n", c.ID, c.Dir, c.Title)
	}
}
