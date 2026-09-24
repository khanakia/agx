package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/khanakia/agx/provider"
)

// Per-conversation data Claude Code keeps outside the project folder, keyed
// by conversation id (verified against v2.1.281).
const (
	// fileHistoryDir holds file checkpoints used by rewind: file-history/<id>/.
	fileHistoryDir = "file-history"
	// sessionEnvDir holds a small environment snapshot: session-env/<id>/.
	sessionEnvDir = "session-env"
	// liveSessionsDir is Claude Code's registry of running processes:
	// sessions/<pid>.json with {pid, sessionId, cwd, …}.
	liveSessionsDir = "sessions"
	// memoryDir is the project's auto-memory folder inside projects/<key>/.
	memoryDir = "memory"
	// registryExt is the extension of a live-session registry entry.
	registryExt = ".json"
)

var (
	_ provider.ConversationMover = (*Provider)(nil)
)

// liveSession is the subset of a sessions/<pid>.json entry agx reads.
// Claude Code writes pid as a number; it is decoded with json.Number so a
// future string form does not break the check.
type liveSession struct {
	PID       json.Number `json:"pid"`
	SessionID string      `json:"sessionId"`
	Cwd       string      `json:"cwd"`
}

// PlanMove implements provider.ConversationMover for Claude Code: every
// conversation recorded for req.Dir in req.FromHome, plus its tool results,
// file checkpoints and environment snapshot, and the folder's project
// memory files. It changes nothing.
//
// Blockers: a Claude process currently running in the folder (in either
// home), or any target path that already exists. Unknown entries in the
// source project folder are listed in Left, never silently dropped.
func (p *Provider) PlanMove(_ context.Context, req provider.MoveRequest) (provider.MovePlan, error) {
	plan := provider.MovePlan{Provider: provider.Claude, FromHome: req.FromHome, ToHome: req.ToHome, Dir: filepath.Clean(req.Dir)}
	src := HistoryDir(req.FromHome, plan.Dir)
	dst := HistoryDir(req.ToHome, plan.Dir)

	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return plan, nil // no history for this folder in this home
	}
	if err != nil {
		return plan, fmt.Errorf("claude: read %s: %w", src, err)
	}

	ids := conversationIDs(entries)
	if len(req.OnlyIDs) > 0 {
		var picked []string
		for _, want := range req.OnlyIDs {
			id, ok := matchID(ids, want)
			if !ok {
				return plan, fmt.Errorf("claude: no conversation %q for %s in %s", want, plan.Dir, req.FromHome)
			}
			picked = append(picked, id)
		}
		ids = picked
	}
	plan.Conversations = ids

	moving := map[string]bool{}
	add := func(from, to string) {
		if _, err := os.Lstat(from); err != nil {
			return // optional per-conversation data that does not exist
		}
		if _, err := os.Lstat(to); err == nil {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("%s already exists in the target account", to))
		}
		plan.Items = append(plan.Items, provider.MoveItem{From: from, To: to})
		moving[from] = true
	}
	for _, id := range ids {
		add(filepath.Join(src, id+transcriptExt), filepath.Join(dst, id+transcriptExt))
		add(filepath.Join(src, id), filepath.Join(dst, id))
		add(filepath.Join(req.FromHome, fileHistoryDir, id), filepath.Join(req.ToHome, fileHistoryDir, id))
		add(filepath.Join(req.FromHome, sessionEnvDir, id), filepath.Join(req.ToHome, sessionEnvDir, id))
	}

	// Project memory belongs to the folder, so it follows a full move. A
	// partial (--only) move leaves it, since other conversations stay behind.
	if len(req.OnlyIDs) == 0 {
		if mem, err := os.ReadDir(filepath.Join(src, memoryDir)); err == nil {
			for _, m := range mem {
				add(filepath.Join(src, memoryDir, m.Name()), filepath.Join(dst, memoryDir, m.Name()))
			}
		}
	}

	for _, e := range entries {
		path := filepath.Join(src, e.Name())
		if !moving[path] && e.Name() != memoryDir {
			plan.Left = append(plan.Left, path)
		}
	}
	// A full move empties the folder's history; drop the empty shells so the
	// source account no longer reports history for it (innermost first).
	if len(req.OnlyIDs) == 0 {
		plan.Cleanup = []string{filepath.Join(src, memoryDir), src}
	}

	for _, home := range []string{req.FromHome, req.ToHome} {
		for _, s := range liveSessions(home) {
			if filepath.Clean(s.Cwd) == plan.Dir || slices.Contains(ids, s.SessionID) {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("Claude is running in this folder (pid %s) — exit it first", s.PID))
			}
		}
	}
	return plan, nil
}

// conversationIDs returns the ids of transcripts in a project folder,
// sorted for deterministic plans.
func conversationIDs(entries []os.DirEntry) []string {
	var ids []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), transcriptExt) {
			ids = append(ids, strings.TrimSuffix(e.Name(), transcriptExt))
		}
	}
	slices.Sort(ids)
	return ids
}

// matchID accepts a full id or a unique prefix (ids are long UUIDs).
func matchID(ids []string, want string) (string, bool) {
	var found string
	for _, id := range ids {
		if id == want {
			return id, true
		}
		if strings.HasPrefix(id, want) {
			if found != "" {
				return "", false // ambiguous prefix
			}
			found = id
		}
	}
	return found, found != ""
}

// liveSessions returns registry entries whose process is still alive.
// Unreadable or stale entries are ignored: the registry is advisory, and a
// dead pid must not block a move forever.
func liveSessions(home string) []liveSession {
	entries, err := os.ReadDir(filepath.Join(home, liveSessionsDir))
	if err != nil {
		return nil
	}
	var out []liveSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), registryExt) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(home, liveSessionsDir, e.Name()))
		if err != nil {
			continue
		}
		var s liveSession
		if json.Unmarshal(raw, &s) != nil || s.PID == "" {
			continue
		}
		pid, err := strconv.Atoi(s.PID.String())
		if err != nil || !processAlive(pid) {
			continue
		}
		out = append(out, s)
	}
	return out
}
