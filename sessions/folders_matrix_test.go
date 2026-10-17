package sessions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// at is a fixed creation time for folder names.
var at = time.Date(2026, 9, 24, 10, 15, 0, 0, time.Local)

// TestSlug_Matrix: only [a-z0-9] survive; every other run (including
// non-ASCII letters) is one separator, and separators never lead or trail.
func TestSlug_Matrix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		words []string
		want  string
	}{
		{nil, ""},
		{[]string{""}, ""},
		{[]string{"   "}, ""},
		{[]string{"a", "b"}, "a_b"},
		{[]string{"A", "", "B"}, "a_b"},
		{[]string{"café ünïcode"}, "caf_n_code"},
		{[]string{"日本"}, ""},
		{[]string{"x/../y"}, "x_y"},
		{[]string{"a;rm -rf ~"}, "a_rm_rf"},
		{[]string{"__init__"}, "init"},
		{[]string{"v1.2.3"}, "v1_2_3"},
		{[]string{"tab\there\nnl"}, "tab_here_nl"},
	} {
		if got := Slug(c.words...); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.words, got, c.want)
		}
	}
}

func TestCreate_Matrix(t *testing.T) {
	t.Parallel()
	// An empty slug is just the stamp.
	root := t.TempDir()
	p, err := Create(root, "", at)
	if err != nil || filepath.Base(p) != "20260924_101500" {
		t.Errorf("empty slug: %s, %v", p, err)
	}
	// A non-ASCII root is fine (the slug is the caller's job).
	uni := filepath.Join(t.TempDir(), "wörk 日本", "sessions")
	if p, err := Create(uni, "x", at); err != nil || filepath.Dir(p) != uni {
		t.Errorf("unicode root: %s, %v", p, err)
	}
	// Root is a regular file.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(file, "x", at); err == nil || !strings.Contains(err.Error(), "create root") {
		t.Errorf("root is a file: %v", err)
	}
	// Read-only root: Mkdir fails with something other than ErrExist and
	// the error is returned at once, not retried as a collision.
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	if _, err := Create(ro, "x", at); err == nil || strings.Contains(err.Error(), "already exist") {
		t.Errorf("read-only root: %v", err)
	}
}

// TestCreate_CollisionLimit: past maxCollisions folders of one name, Create
// refuses instead of looping, and never reuses an existing folder.
func TestCreate_CollisionLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	seen := map[string]bool{}
	for i := 0; i < maxCollisions; i++ {
		p, err := Create(root, "x", at)
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if seen[p] {
			t.Fatalf("reused %s", p)
		}
		seen[p] = true
	}
	if _, err := Create(root, "x", at); err == nil || !strings.Contains(err.Error(), "already exist") {
		t.Errorf("past the limit: %v", err)
	}
}

// TestList_InspectMatrix: creation stamps (with and without a collision
// suffix, invalid dates, hand-made names), file counting and ordering.
func TestList_InspectMatrix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	dirs := map[string]bool{
		"a_20260101_000000":    true,
		"b_20260102_000000_2":  true,
		"20260103_000000":      true,
		"bad_20261399_999999":  false, // invalid date: no Created
		"handmade":             false,
		"wörk_20260104_000000": true,
	}
	for d := range dirs {
		mkdir(t, filepath.Join(root, d))
	}
	write(t, filepath.Join(root, "a_20260101_000000", "deep", "x.go"))
	write(t, filepath.Join(root, "a_20260101_000000", ".DS_Store"))
	write(t, filepath.Join(root, "handmade", ".DS_Store"))
	write(t, filepath.Join(root, "not-a-dir")) // files under root are ignored
	for _, d := range []string{"handmade", "bad_20261399_999999"} {
		if err := os.Chtimes(filepath.Join(root, d), old, old); err != nil {
			t.Fatal(err)
		}
	}
	hand := filepath.Join(root, "handmade", ".DS_Store")
	if err := os.Chtimes(hand, old, old); err != nil {
		t.Fatal(err)
	}

	folders, err := List(root, func(dir string) bool { return strings.HasPrefix(filepath.Base(dir), "b_") })
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != len(dirs) {
		t.Fatalf("%d folders, want %d", len(folders), len(dirs))
	}
	var order []string
	for _, f := range folders {
		order = append(order, f.Name)
		if dirs[f.Name] == f.Created.IsZero() {
			t.Errorf("%s: Created = %v", f.Name, f.Created)
		}
		if f.HistoryUnknown || f.HasHistory != strings.HasPrefix(f.Name, "b_") {
			t.Errorf("%s: history %v/%v", f.Name, f.HasHistory, f.HistoryUnknown)
		}
		wantFiles := 0
		if f.Name == "a_20260101_000000" {
			wantFiles = 1 // .DS_Store is not content
		}
		if f.Files != wantFiles {
			t.Errorf("%s: %d files, want %d", f.Name, f.Files, wantFiles)
		}
	}
	// Stamped folders newest first; the two old mtime-only folders last.
	want := "wörk_20260104_000000,20260103_000000,b_20260102_000000_2,a_20260101_000000"
	if got := strings.Join(order[:4], ","); got != want {
		t.Errorf("order %s, want %s", got, want)
	}
	if got := order[4:]; len(got) != 2 || (got[0] != "handmade" && got[0] != "bad_20261399_999999") {
		t.Errorf("tail %v", got)
	}

	// A root that is a file is an error, not an empty list.
	if _, err := List(filepath.Join(root, "not-a-dir"), nil); err == nil {
		t.Error("List of a file: no error")
	}
	if fs, err := List(filepath.Join(root, "missing"), nil); err != nil || fs != nil {
		t.Errorf("missing root: %v %v", fs, err)
	}
	if _, err := inspect(filepath.Join(root, "missing")); err == nil {
		t.Error("inspect of a missing folder: no error")
	}
}

// TestPlanGC_Matrix: files beat history beat unknown beat age, and the
// age boundary is inclusive (exactly minAge old is collectable).
func TestPlanGC_Matrix(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	const minAge = time.Hour
	for _, c := range []struct {
		name   string
		f      Folder
		remove bool
		reason GCReason
	}{
		{"files+unknown", Folder{Files: 1, HistoryUnknown: true}, false, GCHasFiles},
		{"files+history", Folder{Files: 2, HasHistory: true}, false, GCHasFiles},
		{"unknown+history", Folder{HistoryUnknown: true, HasHistory: true}, false, GCHistoryUnknown},
		{"history old", Folder{HasHistory: true, Created: now.Add(-48 * time.Hour)}, false, GCHasHistory},
		{"too new by created", Folder{Created: now.Add(-minAge + time.Second)}, false, GCTooNew},
		{"exactly min age", Folder{Created: now.Add(-minAge)}, true, GCCollect},
		{"future stamp", Folder{Created: now.Add(time.Hour)}, false, GCTooNew},
		{"created wins over mtime", Folder{Created: now.Add(-2 * minAge), Modified: now}, true, GCCollect},
		{"mtime when no stamp: new", Folder{Modified: now}, false, GCTooNew},
		{"mtime when no stamp: old", Folder{Modified: now.Add(-2 * minAge)}, true, GCCollect},
	} {
		got := PlanGC([]Folder{c.f}, now, minAge)
		if len(got) != 1 || got[0].Remove != c.remove || got[0].Reason != c.reason {
			t.Errorf("%s: %+v, want remove=%v reason=%q", c.name, got, c.remove, c.reason)
		}
	}
	if got := PlanGC(nil, now, minAge); got == nil || len(got) != 0 {
		t.Errorf("nil folders: %v", got)
	}
}

func TestRemoveEmpty_Matrix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Nested empty dirs with only ignored files go entirely.
	empty := filepath.Join(root, "empty")
	write(t, filepath.Join(empty, "a", "b", ".DS_Store"))
	mkdir(t, filepath.Join(empty, "c"))
	if err := RemoveEmpty(empty); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); !errors.Is(err, os.ErrNotExist) {
		t.Error("empty tree left behind")
	}
	// A real file anywhere refuses and the file survives.
	full := filepath.Join(root, "full")
	work := filepath.Join(full, "deep", "work.txt")
	write(t, work)
	if err := RemoveEmpty(full); err == nil || !strings.Contains(err.Error(), "is not empty") {
		t.Errorf("non-empty: %v", err)
	}
	if _, err := os.Stat(work); err != nil {
		t.Errorf("work deleted: %v", err)
	}
	if err := RemoveEmpty(filepath.Join(root, "missing")); err == nil {
		t.Error("missing path: no error")
	}
}

func TestPromote_Matrix(t *testing.T) {
	t.Parallel()
	t.Run("unicode destination", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		src := filepath.Join(root, "s", "x_20260101_000000")
		write(t, filepath.Join(src, "f"))
		dst := filepath.Join(root, "projects", "wörk 日本")
		if err := Promote(src, dst); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dst, "f")); err != nil {
			t.Error(err)
		}
	})
	t.Run("onto itself", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "s")
		write(t, filepath.Join(src, "f"))
		if err := Promote(src, src); !errors.Is(err, ErrExists) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("into its own child", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "s")
		write(t, filepath.Join(src, "f"))
		if err := Promote(src, filepath.Join(src, "new", "dst")); err == nil {
			t.Fatal("promote into own child succeeded")
		}
		if _, err := os.Stat(filepath.Join(src, "new")); !errors.Is(err, os.ErrNotExist) {
			t.Error("parent created for a failed promote was left")
		}
		if _, err := os.Stat(filepath.Join(src, "f")); err != nil {
			t.Error("source content lost")
		}
	})
	t.Run("destination under a file", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		src := filepath.Join(root, "s")
		mkdir(t, src)
		file := filepath.Join(root, "file")
		write(t, file)
		if err := Promote(src, filepath.Join(file, "dst")); err == nil || !strings.Contains(err.Error(), "sessions: stat") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("read-only parent", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		src := filepath.Join(root, "s")
		mkdir(t, src)
		ro := filepath.Join(root, "ro")
		mkdir(t, ro)
		if err := os.Chmod(ro, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
		err := Promote(src, filepath.Join(ro, "a", "b", "dst"))
		if err == nil || !strings.Contains(err.Error(), "create parent") {
			t.Errorf("err = %v", err)
		}
		if _, err := os.Stat(src); err != nil {
			t.Error("source moved by a failed promote")
		}
	})
}

// TestMkdirParents reports exactly the directories it made, innermost first.
func TestMkdirParents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "a"))
	got, err := mkdirParents(filepath.Join(root, "a", "b", "c") + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "a", "b", "c"), filepath.Join(root, "a", "b")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("created %v, want %v", got, want)
	}
	if got, err := mkdirParents(root); err != nil || len(got) != 0 {
		t.Errorf("existing dir: %v %v", got, err)
	}
}
