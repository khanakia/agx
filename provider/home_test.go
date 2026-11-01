package provider

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPlanHomeLinks_Matrix: source present/absent × target absent / file /
// folder / live symlink / broken symlink. A link is planned only for
// source-present + target-absent; everything else is skipped with its reason
// and nothing on disk changes during planning.
func TestPlanHomeLinks_Matrix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	def := filepath.Join(root, "default")
	home := filepath.Join(root, "new")
	for _, d := range []string{def, home, filepath.Join(def, "skills")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) {
		t.Helper()
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"free", "file", "dir", "live", "broken"} {
		write(filepath.Join(def, n))
	}
	write(filepath.Join(home, "file"))
	if err := os.Mkdir(filepath.Join(home, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(def, "free"), filepath.Join(home, "live")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(home, "broken")); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"free":    "",
		"skills":  "", // a folder source links like a file
		"file":    LinkSkipExists,
		"dir":     LinkSkipExists,
		"live":    LinkSkipExists,
		"broken":  LinkSkipExists, // somebody's dangling link is still theirs
		"missing": LinkSkipNoSource,
	}
	names := []string{"free", "skills", "file", "dir", "live", "broken", "missing"}
	links := PlanHomeLinks(def, home, names)
	if len(links) != len(names) {
		t.Fatalf("planned %d links for %d names", len(links), len(names))
	}
	for i, l := range links {
		if l.Name != names[i] || l.Source != filepath.Join(def, names[i]) || l.Target != filepath.Join(home, names[i]) {
			t.Errorf("link %d = %+v", i, l)
		}
		if l.Skip != want[l.Name] {
			t.Errorf("%s: skip %q, want %q", l.Name, l.Skip, want[l.Name])
		}
	}
	if _, err := os.Lstat(filepath.Join(home, "free")); err == nil {
		t.Error("planning created a link")
	}
	if err := ApplyHomeLinks(links); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"free", "skills"} {
		dest, err := os.Readlink(filepath.Join(home, n))
		if err != nil || dest != filepath.Join(def, n) {
			t.Errorf("%s: link -> %q, %v", n, dest, err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(home, "file")); err != nil || string(b) != "x" {
		t.Errorf("existing file changed: %q %v", b, err)
	}
	if dest, _ := os.Readlink(filepath.Join(home, "broken")); dest != filepath.Join(root, "nowhere") {
		t.Errorf("broken link replaced: %q", dest)
	}
	if PlanHomeLinks(def, home, nil) == nil {
		t.Error("nil names should plan an empty, non-nil list")
	}
}

// TestApplyHomeLinks_NeverReplaces: a file that appears between planning and
// applying is kept, and the error names the link without a value in it.
func TestApplyHomeLinks_NeverReplaces(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(src, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "home", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	links := PlanHomeLinks(root, filepath.Dir(target), []string{"CLAUDE.md"})
	if links[0].Skip != "" {
		t.Fatalf("planned skip %q", links[0].Skip)
	}
	if err := os.WriteFile(target, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := ApplyHomeLinks(links)
	if err == nil {
		t.Fatal("ApplyHomeLinks replaced a file that appeared after planning")
	}
	if b, _ := os.ReadFile(target); string(b) != "mine" {
		t.Errorf("target content = %q", b)
	}
}
