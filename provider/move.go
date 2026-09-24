package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// MoveRequest asks a provider to plan moving one folder's conversations from
// one home (account) to another home of the same provider.
type MoveRequest struct {
	FromHome string
	ToHome   string
	// Dir is the working folder whose conversations move (absolute, clean).
	Dir string
	// OnlyIDs limits the move to these conversation ids; empty means all.
	OnlyIDs []string
}

// MoveItem is one filesystem entry (file or directory) to rename.
type MoveItem struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MovePlan is everything a move will do, computed without changing anything,
// so it can be shown (--dry-run), checked (Blockers) and then applied.
type MovePlan struct {
	Provider ID     `json:"provider"`
	FromHome string `json:"from_home"`
	ToHome   string `json:"to_home"`
	Dir      string `json:"dir"`
	// Conversations are the ids that will move, newest first.
	Conversations []string   `json:"conversations"`
	Items         []MoveItem `json:"items"`
	// Blockers are reasons the move must not run (an agent running in the
	// folder, an id already in the target). Any blocker aborts the move
	// before a single file is touched.
	Blockers []string `json:"blockers,omitempty"`
	// Left lists entries found for the folder that the provider does not
	// move, so nothing is silently dropped.
	Left []string `json:"left,omitempty"`
	// Cleanup lists source directories to remove after a successful move if
	// (and only if) they ended up empty, innermost first — so a fully moved
	// folder leaves no empty history behind that would still look like
	// "has history" to listing and promote.
	Cleanup []string `json:"cleanup,omitempty"`
}

// ConversationMover is an optional upgrade for providers whose conversations
// can be moved between homes by moving files. The plan is pure inspection;
// ApplyMovePlan does the moving for every provider the same way.
type ConversationMover interface {
	PlanMove(ctx context.Context, req MoveRequest) (MovePlan, error)
}

// MoveUnsupported is an optional upgrade for providers that cannot move
// conversations, explaining why (shown to the user instead of a generic
// "not supported").
type MoveUnsupported interface {
	MoveUnsupportedReason() string
}

// ErrMoveBlocked means ApplyMovePlan was given a plan that has blockers.
var ErrMoveBlocked = errors.New("move blocked")

// dirPerm is used for parent directories created in the target home;
// provider homes are private to the user.
const dirPerm os.FileMode = 0o700

// ApplyMovePlan executes a plan: optionally back up every source into
// backupDir (keeping paths relative to the source home), then rename each
// item into place. If any rename fails, the items already moved are renamed
// back, so a failed move leaves both homes as they were.
//
// Renames need source and target on the same filesystem; provider homes
// normally sit side by side in the user's home dir. A cross-device target
// fails the rename and triggers the rollback.
func ApplyMovePlan(plan MovePlan, backupDir string) error {
	if len(plan.Blockers) > 0 {
		return fmt.Errorf("%w: %v", ErrMoveBlocked, plan.Blockers)
	}
	if backupDir != "" {
		for _, it := range plan.Items {
			rel, err := filepath.Rel(plan.FromHome, it.From)
			if err != nil {
				return fmt.Errorf("backup path for %s: %w", it.From, err)
			}
			if err := CopyTree(it.From, filepath.Join(backupDir, rel)); err != nil {
				return fmt.Errorf("backup %s: %w", it.From, err)
			}
		}
	}
	var done []MoveItem
	for _, it := range plan.Items {
		err := os.MkdirAll(filepath.Dir(it.To), dirPerm)
		if err == nil {
			err = os.Rename(it.From, it.To)
		}
		if err != nil {
			rollbackErr := rollback(done)
			if rollbackErr != nil {
				return fmt.Errorf("move %s: %w (rollback also failed: %v — restore from %s)", it.From, err, rollbackErr, backupDir)
			}
			return fmt.Errorf("move %s: %w (nothing was changed)", it.From, err)
		}
		done = append(done, it)
	}
	for _, dir := range plan.Cleanup {
		// os.Remove only deletes an empty directory; a non-empty one (e.g.
		// the "left in place" entries) or an already missing one is exactly
		// the case to leave alone, so the error is intentionally ignored.
		_ = os.Remove(dir)
	}
	return nil
}

// rollback renames moved items back, newest first.
func rollback(done []MoveItem) error {
	var errs []error
	for i := len(done) - 1; i >= 0; i-- {
		if err := os.Rename(done[i].To, done[i].From); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CopyTree copies a file or directory tree from src to dst, preserving file
// modes. dst must not exist. Symlinks are copied as links, not followed.
func CopyTree(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("copy: %s already exists", dst)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|dirPerm)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
				return err
			}
			return copyFile(path, target, info.Mode().Perm())
		}
	})
}

// copyFile copies one regular file.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	// Read-only handle: a close error cannot lose data.
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // the copy error is the one worth reporting
		return err
	}
	return out.Close() // a failed close can mean a failed write: report it
}
