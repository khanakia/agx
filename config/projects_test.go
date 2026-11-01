package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/khanakia/agx/provider"
)

// TestMatchProject_Matrix walks the folder-matching rule across the shapes a
// real tree has: exact folder, subfolder, sibling sharing a name prefix,
// nested bindings, the filesystem root, unicode and spaces. Every expected
// value was worked out from the rule (longest path-segment prefix wins), not
// read off the code's output.
func TestMatchProject_Matrix(t *testing.T) {
	t.Parallel()
	bound := []Project{
		{Path: "/a/foo", Profile: "foo"},
		{Path: "/a/foo/sub", Login: "sub"},
		{Path: "/b", Profile: "b"},
		{Path: "/c/my project", Profile: "space"},
		{Path: "/c/проект", Profile: "unicode"},
	}
	withRoot := append(slices.Clone(bound), Project{Path: "/", Profile: "root"})
	for _, tc := range []struct {
		name     string
		projects []Project
		dir      string
		want     string // bound path, "" for no match
	}{
		{"exact folder", bound, "/a/foo", "/a/foo"},
		{"child", bound, "/a/foo/x", "/a/foo"},
		{"deep child", bound, "/a/foo/x/y/z", "/a/foo"},
		{"sibling sharing prefix", bound, "/a/foobar", ""},
		{"sibling child sharing prefix", bound, "/a/foobar/x", ""},
		{"parent of binding", bound, "/a", ""},
		{"subfolder beats parent", bound, "/a/foo/sub", "/a/foo/sub"},
		{"inside subfolder beats parent", bound, "/a/foo/sub/deep", "/a/foo/sub"},
		{"subfolder sibling falls to parent", bound, "/a/foo/subway", "/a/foo"},
		{"space in path", bound, "/c/my project/src", "/c/my project"},
		{"space prefix only", bound, "/c/my", ""},
		{"unicode path", bound, "/c/проект/x", "/c/проект"},
		{"unicode prefix only", bound, "/c/про", ""},
		{"unbound", bound, "/z", ""},
		{"root binding covers everything", withRoot, "/z/q", "/"},
		{"root binding itself", withRoot, "/", "/"},
		{"longer binding beats root", withRoot, "/b/x", "/b"},
		{"no bindings", nil, "/a/foo", ""},
		{"empty dir", bound, "", ""},
	} {
		got, ok := MatchProject(tc.projects, tc.dir)
		if ok != (tc.want != "") || got.Path != tc.want {
			t.Errorf("%s: MatchProject(%q) = %q, %v; want %q", tc.name, tc.dir, got.Path, ok, tc.want)
		}
	}
}

// TestMatchProject_OrderIndependent: the winner is the longest match whatever
// order the bindings appear in the file.
func TestMatchProject_OrderIndependent(t *testing.T) {
	t.Parallel()
	ps := []Project{{Path: "/a", Profile: "a"}, {Path: "/a/b", Profile: "b"}, {Path: "/a/b/c", Profile: "c"}}
	for i := range 3 {
		rot := append(slices.Clone(ps[i:]), ps[:i]...)
		if got, _ := MatchProject(rot, "/a/b/c/d"); got.Profile != "c" {
			t.Errorf("rotation %d: got %q, want c", i, got.Profile)
		}
		if got, _ := MatchProject(rot, "/a/bc"); got.Profile != "a" {
			t.Errorf("rotation %d: /a/bc got %q, want a", i, got.Profile)
		}
	}
}

// TestResolveProjects_Matrix: the load-time checks and path forms. A binding
// with a dangling name still loads (rm and doctor must reach it).
func TestResolveProjects_Matrix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		yaml     string
		wantPath string // first binding's path after load
		wantErr  string
	}{
		{"absolute", "projects:\n  - {path: /w/app, profile: work}\n", "/w/app", ""},
		{"tilde subfolder", "projects:\n  - {path: ~/w/app, login: x}\n", "/home/me/w/app", ""},
		{"bare tilde", "projects:\n  - {path: '~', profile: work}\n", "/home/me", ""},
		{"trailing slash", "projects:\n  - {path: /w/app/, profile: work}\n", "/w/app", ""},
		{"dot segments", "projects:\n  - {path: /w/x/../app, profile: work}\n", "/w/app", ""},
		{"root", "projects:\n  - {path: /, profile: work}\n", "/", ""},
		{"space and unicode", "projects:\n  - {path: '/w/my проект', profile: work}\n", "/w/my проект", ""},
		{"dangling profile loads", "projects:\n  - {path: /w, profile: gone}\n", "/w", ""},
		{"dangling login loads", "projects:\n  - {path: /w, login: gone}\n", "/w", ""},
		{"home login", "projects:\n  - {path: /w, login: home}\n", "/w", ""},
		{"relative", "projects:\n  - {path: w/app, profile: work}\n", "", "path must be absolute"},
		{"empty path", "projects:\n  - {profile: work}\n", "", "path must be absolute"},
		{"tilde user", "projects:\n  - {path: ~bob/x, profile: work}\n", "", "path must be absolute"},
		{"nothing bound", "projects:\n  - {path: /w}\n", "", "set profile, login or both"},
		{"auto login", "projects:\n  - {path: /w, login: auto}\n", "", `"auto" is accepted only as --login`},
		{"duplicate", "projects:\n  - {path: /w, profile: work}\n  - {path: /w/, login: x}\n", "", "duplicate path"},
		{"duplicate via tilde", "projects:\n  - {path: /home/me/w, profile: work}\n  - {path: ~/w, login: x}\n", "", "duplicate path"},
	} {
		cfg, err := resolve(t, tc.yaml)
		if tc.wantErr != "" {
			if err == nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: err = %v, want ErrInvalid containing %q", tc.name, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(cfg.Projects) == 0 || cfg.Projects[0].Path != tc.wantPath {
			t.Errorf("%s: projects = %+v, want path %q", tc.name, cfg.Projects, tc.wantPath)
		}
	}
	cfg, err := resolve(t, "")
	if err != nil || len(cfg.Projects) != 0 {
		t.Errorf("no projects key: %+v, %v", cfg.Projects, err)
	}
	if _, ok := cfg.ProjectFor("/anything"); ok {
		t.Error("ProjectFor matched with no bindings")
	}
}

// TestCanonicalPath: a symlinked folder and its target match the same
// binding; a relative path is made absolute; a missing folder comes back
// clean so its binding can still be removed.
func TestCanonicalPath(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "real project")
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		real:                       real,
		real + "/":                 real,
		link:                       real,
		filepath.Join(link, "sub"): filepath.Join(real, "sub"),
		filepath.Join(root, "gone", "..", "gone2/"): filepath.Join(root, "gone2"),
		filepath.Join(root, "missing", "x"):         filepath.Join(root, "missing", "x"),
	} {
		if got := CanonicalPath(in); got != want {
			t.Errorf("CanonicalPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := CanonicalPath("rel"); !filepath.IsAbs(got) {
		t.Errorf("CanonicalPath(rel) = %q, not absolute", got)
	}
	bound := []Project{{Path: real, Profile: "p"}}
	for _, dir := range []string{link, filepath.Join(link, "sub")} {
		if _, ok := MatchProject(bound, CanonicalPath(dir)); !ok {
			t.Errorf("symlinked %q does not match the binding of its target", dir)
		}
	}
}

func TestExpandPath(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":         "",
		"~":        "/home/me",
		"~/x/":     "/home/me/x",
		"/a/../b/": "/b",
	} {
		if got := ExpandPath(in, userHome); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidProfileName(t *testing.T) {
	t.Parallel()
	for name, ok := range map[string]bool{
		"client": true, "client-2": true, "a_b": false, "Client": false,
		"auto": false, "": false, "has space": false, "a/b": false, "-x": false,
	} {
		if err := ValidProfileName(name); (err == nil) != ok {
			t.Errorf("ValidProfileName(%q) = %v, want ok=%v", name, err, ok)
		}
	}
}

// TestUsedBy_Projects: a binding counts as a user of its login, after the
// profiles and provider defaults.
func TestUsedBy_Projects(t *testing.T) {
	t.Parallel()
	cfg, err := resolveLoginsYAML("logins:\n  - {name: x, token_env: X}\nproviders:\n  claude: {login: x}\nprofiles:\n  - {name: work, provider: claude, home: ~/.claude-work, login: x}\nprojects:\n  - {path: /w, login: x}\n  - {path: /v, profile: work}\n  - {path: /u, profile: work, login: x}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"work", "providers.claude", "project /w", "project /u"}
	if got := cfg.UsedBy("x"); !slices.Equal(got, want) {
		t.Errorf("UsedBy = %v, want %v", got, want)
	}
	if got := cfg.UsedBy("nosuch"); len(got) != 0 {
		t.Errorf("UsedBy(nosuch) = %v", got)
	}
}

// commentedProjects has comments on and inside the projects block that every
// project edit must keep.
const commentedProjects = commented + `logins:
  - {name: x, token_env: X}
  - {name: y, token_env: Y}
projects: # folder bindings
  # the client repo
  - path: /w/client # bound by hand
    profile: work
    login: x
  - {path: /w/other, login: y}
`

var projectComments = []string{"# folder bindings", "# the client repo", "# bound by hand"}

// TestEditor_Projects walks set (new, replace, login-only → profile+login,
// clear a key), remove (one, missing, last) and RemoveLogin's binding
// cleanup, checking after every step that the file loads and no comment was
// lost.
func TestEditor_Projects(t *testing.T) {
	t.Parallel()
	e := newEditor(t, commentedProjects, 0o600)
	check := func(step string, want []Project) {
		t.Helper()
		cfg := e.load(t)
		if !slices.Equal(cfg.Projects, want) {
			t.Errorf("%s: projects = %+v, want %+v", step, cfg.Projects, want)
		}
		body := read(t, e.Path)
		for _, c := range append(slices.Clone(commentLines), projectComments...) {
			if !strings.Contains(body, c) {
				t.Errorf("%s: lost comment %q\n%s", step, c, body)
			}
		}
		if mode(t, e.Path) != 0o600 {
			t.Errorf("%s: mode changed", step)
		}
	}
	client := Project{Path: "/w/client", Profile: "work", Login: "x"}
	other := Project{Path: "/w/other", Login: "y"}
	check("start", []Project{client, other})

	if err := e.SetProject(Project{Path: "/w/new", Profile: "personal"}); err != nil {
		t.Fatal(err)
	}
	newP := Project{Path: "/w/new", Profile: "personal"}
	check("add new", []Project{client, other, newP})

	client.Login = "y"
	if err := e.SetProject(client); err != nil {
		t.Fatal(err)
	}
	check("replace login", []Project{client, other, newP})

	client.Login = ""
	if err := e.SetProject(client); err != nil {
		t.Fatal(err)
	}
	check("clear login", []Project{client, other, newP})

	other.Profile = "work"
	if err := e.SetProject(other); err != nil {
		t.Fatal(err)
	}
	check("login-only gains profile", []Project{client, other, newP})
	if body := read(t, e.Path); !strings.Contains(body, "{path: /w/other, profile: work, login: y}") {
		t.Errorf("profile not written before login:\n%s", body)
	}

	if err := e.RemoveProject("/w/missing"); err == nil || !strings.Contains(err.Error(), "no project binding for /w/missing") {
		t.Errorf("remove missing: %v", err)
	}
	check("remove missing changes nothing", []Project{client, other, newP})

	// RemoveLogin clears y from the binding that also has a profile.
	if _, err := e.RemoveLogin("y"); err != nil {
		t.Fatal(err)
	}
	other.Login = ""
	check("rm login y", []Project{client, other, newP})

	if err := e.RemoveProject("/w/new"); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveProject("/w/other"); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveProject("/w/client"); err != nil {
		t.Fatal(err)
	}
	if body := read(t, e.Path); strings.Contains(body, "projects:") {
		t.Errorf("empty projects key left behind:\n%s", body)
	}
}

// TestEditor_RemoveLoginDropsLoginOnlyBinding: a binding whose only setting
// was the removed login has nothing left to bind and goes; one that also
// names a profile keeps the profile. The cleared list names each.
func TestEditor_RemoveLoginDropsLoginOnlyBinding(t *testing.T) {
	t.Parallel()
	e := newEditor(t, commented+"logins:\n  - {name: x, token_env: X}\nprojects:\n  - {path: /a, login: x}\n  - {path: /b, profile: work, login: x}\n  - {path: /c, login: home}\n", 0o600)
	cleared, err := e.RemoveLogin("x")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"projects[/a].login", "projects[/b].login"}; !slices.Equal(cleared, want) {
		t.Errorf("cleared = %v, want %v", cleared, want)
	}
	want := []Project{{Path: "/b", Profile: "work"}, {Path: "/c", Login: "home"}}
	if got := e.load(t).Projects; !slices.Equal(got, want) {
		t.Errorf("projects = %+v, want %+v", got, want)
	}
	e2 := newEditor(t, commented+"logins:\n  - {name: x, token_env: X}\nprojects:\n  - {path: /a, login: x}\n", 0o600)
	if _, err := e2.RemoveLogin("x"); err != nil {
		t.Fatal(err)
	}
	if body := read(t, e2.Path); strings.Contains(body, "projects") {
		t.Errorf("last binding removed but key kept:\n%s", body)
	}
}

// TestEditor_SetProjectTildeEntry: a hand-written ~/ path is the same
// binding as its expansion, so `project use` edits it instead of adding a
// duplicate that would then fail to load.
func TestEditor_SetProjectTildeEntry(t *testing.T) {
	t.Parallel()
	e := newEditor(t, commented+"projects:\n  - {path: ~/w, profile: work}\n", 0o600)
	if err := e.SetProject(Project{Path: "/home/me/w", Profile: "personal"}); err != nil {
		t.Fatal(err)
	}
	if got := e.load(t).Projects; len(got) != 1 || got[0].Profile != "personal" {
		t.Errorf("projects = %+v", got)
	}
	if err := e.RemoveProject("/home/me/w"); err != nil {
		t.Errorf("remove by expansion: %v", err)
	}
}

// TestEditor_AddProfile: a new profile is appended with its home in ~ form;
// naming a discovered profile adopts it (written once, with the login);
// discovered siblings are written alongside so they keep existing.
func TestEditor_AddProfile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		body      string
		spec      ProfileSpec
		wantNotes int
		wantLogin string
		wantHome  string
		wantCount int // profiles named spec.Name in the file
	}{
		{"new into file with profiles", commented + "logins:\n  - {name: x, token_env: X}\n", ProfileSpec{Name: "client", Provider: "claude", Home: "/home/me/.claude-client", Login: "x"}, 0, "x", "~/.claude-client", 1},
		{"new without login", commented, ProfileSpec{Name: "client", Provider: "claude", Home: "/elsewhere/c"}, 0, "", "/elsewhere/c", 1},
		{"empty file writes discovered", "", ProfileSpec{Name: "client", Provider: "claude", Home: "/home/me/.claude-client"}, 2, "", "~/.claude-client", 1},
		{"adopt discovered", "", ProfileSpec{Name: "work", Provider: "claude", Home: "/home/me/.claude-work", Login: "home"}, 2, "home", "~/.claude-work", 1},
		{"codex new", "", ProfileSpec{Name: "cx2", Provider: "codex", Home: "/home/me/.codex-cx2"}, 1, "", "~/.codex-cx2", 1},
	} {
		e := newEditor(t, tc.body, 0o600)
		notes, err := e.AddProfile(e.load(t), tc.spec)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(notes) != tc.wantNotes {
			t.Errorf("%s: notes = %v, want %d", tc.name, notes, tc.wantNotes)
		}
		body := read(t, e.Path)
		if n := strings.Count(body, "name: "+tc.spec.Name+"\n"); n != tc.wantCount {
			t.Errorf("%s: %d entries named %s\n%s", tc.name, n, tc.spec.Name, body)
		}
		if !strings.Contains(body, "home: "+tc.wantHome) {
			t.Errorf("%s: home %q not written\n%s", tc.name, tc.wantHome, body)
		}
		p, ok := e.load(t).ByName(tc.spec.Name)
		if !ok || p.Login != tc.wantLogin || p.Provider != provider.ID(tc.spec.Provider) {
			t.Errorf("%s: profile = %+v", tc.name, p)
		}
		if tc.body != "" {
			for _, c := range commentLines {
				if strings.Contains(tc.body, c) && !strings.Contains(body, c) {
					t.Errorf("%s: lost comment %q", tc.name, c)
				}
			}
		}
	}
}
