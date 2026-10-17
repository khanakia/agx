package sessions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSlugAndName(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":                   "",
		"PGlite Golang":      "pglite_golang",
		"  a--b__c  ":        "a_b_c",
		"Estonia visa! (v2)": "estonia_visa_v2",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	at := time.Date(2026, 9, 24, 10, 15, 0, 0, time.Local)
	if Name("", at) != "20260924_101500" || Name("x", at) != "x_20260924_101500" {
		t.Errorf("Name = %q / %q", Name("", at), Name("x", at))
	}
}

func TestCreate_Collision(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "sessions") // created on demand
	at := time.Date(2026, 9, 24, 10, 15, 0, 0, time.Local)
	a, err := Create(root, "x", at)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Create(root, "x", at)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(a) != "x_20260924_101500" || filepath.Base(b) != "x_20260924_101500_2" {
		t.Errorf("created %s, %s", a, b)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListAndPlanGC(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "20260101_000000"))                        // empty, old → remove
	write(t, filepath.Join(root, "20260101_000001", ".DS_Store"))           // only ignored file → remove
	write(t, filepath.Join(root, "work_20260101_000002", "sub", "main.go")) // has files
	mkdir(t, filepath.Join(root, "chat_20260101_000003"))                   // empty but has history
	mkdir(t, filepath.Join(root, "20260924_100000"))                        // empty, too new
	write(t, filepath.Join(root, "not-a-folder.txt"))

	withHistory := filepath.Join(root, "chat_20260101_000003")
	folders, err := List(root, func(dir string) bool { return dir == withHistory })
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 5 {
		t.Fatalf("listed %d folders: %+v", len(folders), folders)
	}
	if folders[0].Name != "20260924_100000" {
		t.Errorf("not newest first: %s", folders[0].Name)
	}

	now := time.Date(2026, 9, 24, 10, 30, 0, 0, time.Local)
	plan := PlanGC(folders, now, 24*time.Hour)
	reasons := map[string]GCReason{}
	for _, it := range plan {
		reasons[it.Folder.Name] = it.Reason
		if it.Remove != (it.Reason == GCCollect) {
			t.Errorf("%s: Remove=%v but reason %q", it.Folder.Name, it.Remove, it.Reason)
		}
	}
	want := map[string]GCReason{
		"20260101_000000":      GCCollect,
		"20260101_000001":      GCCollect,
		"work_20260101_000002": GCHasFiles,
		"chat_20260101_000003": GCHasHistory,
		"20260924_100000":      GCTooNew,
	}
	for k, v := range want {
		if reasons[k] != v {
			t.Errorf("%s: %q, want %q", k, reasons[k], v)
		}
	}
	if missing, err := List(filepath.Join(root, "nope"), nil); err != nil || missing != nil {
		t.Errorf("missing root = %v, %v", missing, err)
	}
}

func TestRemoveEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	write(t, filepath.Join(empty, "nested", ".DS_Store"))
	if err := RemoveEmpty(empty); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); !errors.Is(err, os.ErrNotExist) {
		t.Error("empty tree not removed")
	}

	// A real file anywhere inside must make it refuse and keep everything.
	full := filepath.Join(root, "full")
	write(t, filepath.Join(full, "deep", "work.txt"))
	if err := RemoveEmpty(full); err == nil {
		t.Fatal("RemoveEmpty deleted a folder with content")
	}
	if _, err := os.Stat(filepath.Join(full, "deep", "work.txt")); err != nil {
		t.Error("content lost")
	}
}

func TestPromote(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(root, "sessions", "x_20260101_000000")
	write(t, filepath.Join(src, "main.go"))
	dst := filepath.Join(root, "projects", "newname")
	if err := Promote(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "main.go")); err != nil {
		t.Error("content not moved")
	}
	other := filepath.Join(root, "sessions", "y")
	mkdir(t, other)
	if err := Promote(other, dst); !errors.Is(err, ErrExists) {
		t.Errorf("overwrite err = %v", err)
	}
}

// A promote that fails at the rename (a cross-disk destination, a vanished
// source) must leave the disk exactly as it was: no parent folders created
// for a destination that never arrived, and no pre-existing folder removed.
func TestPromote_FailureLeavesNoParents(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		existing string // relative dir that exists before the promote
		dst      string // relative destination
		gone     []string
		kept     []string
	}{
		{name: "all-parents-new", dst: "a/b/c/dst", gone: []string{"a"}},
		{name: "some-parents-exist", existing: "a", dst: "a/b/c/dst", gone: []string{"a/b"}, kept: []string{"a"}},
		{name: "parent-exists", existing: "a/b", dst: "a/b/dst", kept: []string{"a/b"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if c.existing != "" {
				mkdir(t, filepath.Join(root, c.existing))
			}
			// A missing source makes the rename fail the same way EXDEV does:
			// after the parents were made.
			err := Promote(filepath.Join(root, "missing"), filepath.Join(root, c.dst))
			if err == nil {
				t.Fatal("promote of a missing source succeeded")
			}
			for _, g := range c.gone {
				if _, err := os.Stat(filepath.Join(root, g)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s left behind after a failed promote", g)
				}
			}
			for _, k := range c.kept {
				if _, err := os.Stat(filepath.Join(root, k)); err != nil {
					t.Errorf("pre-existing %s removed: %v", k, err)
				}
			}
		})
	}
}

// With no HistoryFunc, history is unknown, and gc must keep the folder: an
// empty folder can still have conversations recorded for it.
func TestPlanGC_UnknownHistoryKept(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "20260101_000000"))
	folders, err := List(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || !folders[0].HistoryUnknown || folders[0].HasHistory {
		t.Fatalf("folders = %+v", folders)
	}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	plan := PlanGC(folders, now, time.Hour)
	if plan[0].Remove || plan[0].Reason != GCHistoryUnknown {
		t.Errorf("unknown history: %+v", plan[0])
	}

	known, err := List(root, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if known[0].HistoryUnknown {
		t.Error("a HistoryFunc was given, history is known")
	}
	if p := PlanGC(known, now, time.Hour); !p[0].Remove {
		t.Errorf("known-empty folder kept: %+v", p[0])
	}
}
