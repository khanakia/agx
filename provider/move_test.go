package provider

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestApplyMovePlan_MovesAndBacksUp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	writeTestFile(t, filepath.Join(from, "projects", "k", "a.jsonl"), "A")
	writeTestFile(t, filepath.Join(from, "projects", "k", "a", "tool-results", "r.txt"), "R")
	writeTestFile(t, filepath.Join(to, "projects", "k", "existing.jsonl"), "E") // target folder already exists: merge
	plan := MovePlan{FromHome: from, ToHome: to, Items: []MoveItem{
		{filepath.Join(from, "projects", "k", "a.jsonl"), filepath.Join(to, "projects", "k", "a.jsonl")},
		{filepath.Join(from, "projects", "k", "a"), filepath.Join(to, "projects", "k", "a")},
	}}
	backup := filepath.Join(from, "backups", "b1")
	if err := ApplyMovePlan(plan, backup); err != nil {
		t.Fatal(err)
	}
	if readTestFile(t, filepath.Join(to, "projects", "k", "a.jsonl")) != "A" || readTestFile(t, filepath.Join(to, "projects", "k", "a", "tool-results", "r.txt")) != "R" {
		t.Error("items not moved")
	}
	if readTestFile(t, filepath.Join(to, "projects", "k", "existing.jsonl")) != "E" {
		t.Error("existing target content disturbed")
	}
	if _, err := os.Stat(filepath.Join(from, "projects", "k", "a.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Error("source still present")
	}
	// The backup keeps paths relative to the source home.
	if readTestFile(t, filepath.Join(backup, "projects", "k", "a.jsonl")) != "A" || readTestFile(t, filepath.Join(backup, "projects", "k", "a", "tool-results", "r.txt")) != "R" {
		t.Error("backup incomplete")
	}
}

// Cleanup removes emptied source dirs but never a non-empty one.
func TestApplyMovePlan_Cleanup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	emptied, kept := filepath.Join(root, "from", "emptied"), filepath.Join(root, "from", "kept")
	writeTestFile(t, filepath.Join(emptied, "a.jsonl"), "A")
	writeTestFile(t, filepath.Join(kept, "b.jsonl"), "B")
	writeTestFile(t, filepath.Join(kept, "left.bin"), "stays")
	plan := MovePlan{
		Items: []MoveItem{
			{filepath.Join(emptied, "a.jsonl"), filepath.Join(root, "to", "a.jsonl")},
			{filepath.Join(kept, "b.jsonl"), filepath.Join(root, "to", "b.jsonl")},
		},
		Cleanup: []string{emptied, kept, filepath.Join(root, "never-existed")},
	}
	if err := ApplyMovePlan(plan, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(emptied); !errors.Is(err, os.ErrNotExist) {
		t.Error("emptied source dir not removed")
	}
	if readTestFile(t, filepath.Join(kept, "left.bin")) != "stays" {
		t.Error("cleanup touched a non-empty dir")
	}
}

func TestApplyMovePlan_RefusesBlockedPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(root, "x")
	writeTestFile(t, src, "x")
	plan := MovePlan{Items: []MoveItem{{src, filepath.Join(root, "y")}}, Blockers: []string{"running"}}
	if err := ApplyMovePlan(plan, ""); !errors.Is(err, ErrMoveBlocked) {
		t.Fatalf("err = %v", err)
	}
	if readTestFile(t, src) != "x" {
		t.Error("blocked plan moved something")
	}
}

// A failure half-way must put back what already moved.
func TestApplyMovePlan_RollsBackOnFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first, second := filepath.Join(root, "from", "1"), filepath.Join(root, "from", "2")
	writeTestFile(t, first, "one")
	writeTestFile(t, second, "two")
	blocker := filepath.Join(root, "to", "notadir")
	writeTestFile(t, blocker, "file where a directory is needed")
	plan := MovePlan{Items: []MoveItem{
		{first, filepath.Join(root, "to", "1")},
		{second, filepath.Join(blocker, "2")}, // parent is a file → MkdirAll fails
	}}
	err := ApplyMovePlan(plan, "")
	if err == nil {
		t.Fatal("want failure")
	}
	if readTestFile(t, first) != "one" || readTestFile(t, second) != "two" {
		t.Errorf("rollback incomplete (err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(root, "to", "1")); !errors.Is(err, os.ErrNotExist) {
		t.Error("moved item left in the target after rollback")
	}
}

func TestCopyTree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	writeTestFile(t, filepath.Join(src, "a", "b.txt"), "B")
	if err := os.Symlink("a/b.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "dst")
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if readTestFile(t, filepath.Join(dst, "a", "b.txt")) != "B" {
		t.Error("file not copied")
	}
	if l, err := os.Readlink(filepath.Join(dst, "link")); err != nil || l != "a/b.txt" {
		t.Errorf("symlink = %q, %v (must be copied as a link)", l, err)
	}
	if err := CopyTree(src, dst); err == nil {
		t.Error("copying onto an existing destination must fail")
	}
	single := filepath.Join(root, "one.txt")
	writeTestFile(t, single, "1")
	if err := CopyTree(single, filepath.Join(root, "copy.txt")); err != nil || readTestFile(t, filepath.Join(root, "copy.txt")) != "1" {
		t.Errorf("single file copy: %v", err)
	}
}
