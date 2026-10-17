package provider

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Modes the CopyTree matrix checks are carried over.
const (
	modeExec    os.FileMode = 0o750
	modePrivate os.FileMode = 0o600
	modeShared  os.FileMode = 0o644
)

func TestApplyMovePlan_BlockersTouchNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	from := filepath.Join(root, "from", "a.jsonl")
	writeTestFile(t, from, "A")
	backup := filepath.Join(root, "backup")
	plan := MovePlan{FromHome: filepath.Join(root, "from"), Blockers: []string{"agent running", "id exists"},
		Items: []MoveItem{{From: from, To: filepath.Join(root, "to", "a.jsonl")}}}
	err := ApplyMovePlan(plan, backup)
	if !errors.Is(err, ErrMoveBlocked) || !strings.Contains(err.Error(), "agent running") || !strings.Contains(err.Error(), "id exists") {
		t.Fatalf("err = %v, want ErrMoveBlocked naming every blocker", err)
	}
	if readTestFile(t, from) != "A" {
		t.Error("source changed")
	}
	if _, err := os.Stat(backup); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a blocked move must not even write the backup")
	}
}

// TestApplyMovePlan_RollbackOnLaterFailure: the second item fails (its source
// is gone), so the first, already renamed, must be renamed back.
func TestApplyMovePlan_RollbackOnLaterFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	first := filepath.Join(from, "p", "a.jsonl")
	writeTestFile(t, first, "A")
	plan := MovePlan{FromHome: from, ToHome: to, Items: []MoveItem{
		{From: first, To: filepath.Join(to, "p", "a.jsonl")},
		{From: filepath.Join(from, "p", "missing.jsonl"), To: filepath.Join(to, "p", "missing.jsonl")},
	}, Cleanup: []string{filepath.Join(from, "p")}}
	err := ApplyMovePlan(plan, "")
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("err = %v, want a rolled-back failure", err)
	}
	if readTestFile(t, first) != "A" {
		t.Error("first item not rolled back")
	}
	if _, err := os.Stat(filepath.Join(to, "p", "a.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("first item still in the target after rollback")
	}
	if _, err := os.Stat(filepath.Join(from, "p")); err != nil {
		t.Error("cleanup ran after a failed move")
	}
}

// TestRollback_ReportsEveryFailure: rollback renames newest first and joins
// every failure, so ApplyMovePlan can tell the user to restore from backup.
// A read-only target dir makes renaming out of it fail (inducing that inside
// ApplyMovePlan on one filesystem would need a race).
func TestRollback_ReportsEveryFailure(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("relies on POSIX dir permissions as a non-root user")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "to")
	var done []MoveItem
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		to := filepath.Join(locked, name)
		writeTestFile(t, to, name)
		done = append(done, MoveItem{From: filepath.Join(root, "from", name), To: to})
	}
	if err := os.MkdirAll(filepath.Join(root, "from"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	err := rollback(done)
	if err == nil || strings.Count(err.Error(), "\n")+1 != len(done) {
		t.Fatalf("rollback err = %v, want one joined error per item", err)
	}
	if err := os.Chmod(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := rollback(done); err != nil {
		t.Errorf("rollback once writable = %v", err)
	}
	for _, it := range done {
		if readTestFile(t, it.From) != filepath.Base(it.From) {
			t.Errorf("%s not restored", it.From)
		}
	}
	if err := rollback(nil); err != nil {
		t.Errorf("rollback(nil) = %v", err)
	}
}

func TestApplyMovePlan_BackupErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	from := filepath.Join(root, "from")
	item := filepath.Join(from, "p", "a.jsonl")
	writeTestFile(t, item, "A")
	backup := filepath.Join(root, "bk")
	// Backup target already exists: CopyTree refuses, nothing is moved.
	writeTestFile(t, filepath.Join(backup, "p", "a.jsonl"), "OLD")
	plan := MovePlan{FromHome: from, Items: []MoveItem{{From: item, To: filepath.Join(root, "to", "p", "a.jsonl")}}}
	if err := ApplyMovePlan(plan, backup); err == nil || !strings.Contains(err.Error(), "backup") {
		t.Fatalf("err = %v, want a backup error", err)
	}
	if readTestFile(t, item) != "A" || readTestFile(t, filepath.Join(backup, "p", "a.jsonl")) != "OLD" {
		t.Error("a failed backup changed files")
	}
	// Item source missing: backup fails before any rename.
	plan.Items[0].From = filepath.Join(from, "p", "gone.jsonl")
	if err := ApplyMovePlan(plan, filepath.Join(root, "bk2")); err == nil || !strings.Contains(err.Error(), "backup") {
		t.Errorf("missing source err = %v", err)
	}
}

func TestApplyMovePlan_CleanupOnlyEmptyDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	empty, keep := filepath.Join(from, "empty"), filepath.Join(from, "keep")
	writeTestFile(t, filepath.Join(empty, "a.jsonl"), "A")
	writeTestFile(t, filepath.Join(keep, "left.txt"), "L")
	writeTestFile(t, filepath.Join(keep, "b.jsonl"), "B")
	plan := MovePlan{FromHome: from, ToHome: to, Items: []MoveItem{
		{From: filepath.Join(empty, "a.jsonl"), To: filepath.Join(to, "empty", "a.jsonl")},
		{From: filepath.Join(keep, "b.jsonl"), To: filepath.Join(to, "keep", "b.jsonl")},
	}, Cleanup: []string{empty, keep, filepath.Join(from, "never-existed")}}
	if err := ApplyMovePlan(plan, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); !errors.Is(err, fs.ErrNotExist) {
		t.Error("empty source dir not cleaned up")
	}
	if readTestFile(t, filepath.Join(keep, "left.txt")) != "L" {
		t.Error("non-empty dir (left-in-place entries) was removed")
	}
}

func TestApplyMovePlan_EmptyPlan(t *testing.T) {
	t.Parallel()
	if err := ApplyMovePlan(MovePlan{}, ""); err != nil {
		t.Errorf("empty plan = %v", err)
	}
	if err := ApplyMovePlan(MovePlan{}, filepath.Join(t.TempDir(), "bk")); err != nil {
		t.Errorf("empty plan with backup = %v", err)
	}
}

func TestCopyTree_Matrix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	files := map[string]os.FileMode{
		"run.sh":               modeExec,
		"secret.json":          modePrivate,
		"nested/deep/ünï cødé": modeShared,
		"empty":                modePrivate,
	}
	for rel, mode := range files {
		p := filepath.Join(src, rel)
		writeTestFile(t, p, "content of "+rel)
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "empty"), nil, modePrivate); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "emptydir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("run.sh", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(src, "dangling")); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(root, "dst")
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for rel, mode := range files {
		p := filepath.Join(dst, rel)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("%s not copied: %v", rel, err)
			continue
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s mode = %v, want %v", rel, info.Mode().Perm(), mode)
		}
	}
	if readTestFile(t, filepath.Join(dst, "nested/deep/ünï cødé")) != "content of nested/deep/ünï cødé" {
		t.Error("content differs")
	}
	if readTestFile(t, filepath.Join(dst, "empty")) != "" {
		t.Error("empty file not empty")
	}
	if info, err := os.Stat(filepath.Join(dst, "emptydir")); err != nil || !info.IsDir() {
		t.Error("empty dir not copied")
	}
	// Symlinks are copied as links (not followed), dangling ones included.
	for name, want := range map[string]string{"link": "run.sh", "dangling": filepath.Join(root, "nowhere")} {
		got, err := os.Readlink(filepath.Join(dst, name))
		if err != nil || got != want {
			t.Errorf("%s readlink = %q, %v; want %q", name, got, err, want)
		}
	}

	// A single file is a valid tree too.
	one := filepath.Join(root, "one-copy")
	if err := CopyTree(filepath.Join(src, "secret.json"), one); err != nil || readTestFile(t, one) != "content of secret.json" {
		t.Errorf("single file copy = %v", err)
	}
	// dst must not exist — file, dir, or dangling symlink.
	for _, existing := range []string{dst, one, filepath.Join(dst, "dangling")} {
		if err := CopyTree(src, existing); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("CopyTree onto existing %s = %v", existing, err)
		}
	}
	// A missing source is an error, and leaves no dst behind.
	missingDst := filepath.Join(root, "from-missing")
	if err := CopyTree(filepath.Join(root, "no-such-src"), missingDst); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing src = %v", err)
	}
	if _, err := os.Lstat(missingDst); !errors.Is(err, fs.ErrNotExist) {
		t.Error("missing src created dst")
	}
}

func TestCopyFile_RefusesExistingAndUnreadable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src, dst := filepath.Join(root, "a"), filepath.Join(root, "b")
	writeTestFile(t, src, "A")
	writeTestFile(t, dst, "B")
	if err := copyFile(src, dst, modePrivate); !errors.Is(err, fs.ErrExist) {
		t.Errorf("existing dst = %v, want ErrExist (O_EXCL)", err)
	}
	if readTestFile(t, dst) != "B" {
		t.Error("existing dst overwritten")
	}
	if err := copyFile(filepath.Join(root, "missing"), filepath.Join(root, "c"), modePrivate); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing src = %v", err)
	}
	// Source is a directory: open works, reading fails mid-copy.
	if err := copyFile(root, filepath.Join(root, "d"), modePrivate); err == nil {
		t.Error("copying a directory as a file should fail")
	}
}
