package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/khanakia/agx/login"
)

// Project binds a folder (and everything under it) to a profile, a login or
// both. Bindings live in config.yaml keyed by path, never in a file inside the
// repo: which account a folder runs on is the machine owner's choice, not
// something a clone should carry.
//
// Profile and Login are names, deliberately NOT validated at load: a binding
// left dangling by a removed profile must still load, so `agx project rm` and
// `agx doctor` can reach it. The launch that would use it refuses instead.
type Project struct {
	// Path is absolute with symlinks resolved when agx wrote it. A
	// hand-written "~/x" is expanded at load.
	Path string `yaml:"path" json:"path"`
	// Profile is a profile name; "" keeps the usual profile choice.
	Profile string `yaml:"profile" json:"profile,omitempty"`
	// Login is a login name or "home"; "" keeps the usual login choice.
	Login string `yaml:"login" json:"login,omitempty"`
}

// resolveProjects validates the shape of every binding and expands its path.
func resolveProjects(specs []Project, userHome string) ([]Project, error) {
	var out []Project
	seen := map[string]bool{}
	for i, s := range specs {
		where := fmt.Sprintf("projects[%d]", i)
		if s.Path != "" {
			where += " (" + s.Path + ")"
		}
		if s.Path != "~" && !strings.HasPrefix(s.Path, "~/") && !filepath.IsAbs(s.Path) {
			return nil, fmt.Errorf("%w: %s: path must be absolute", ErrInvalid, where)
		}
		if s.Profile == "" && s.Login == "" {
			return nil, fmt.Errorf("%w: %s: set profile, login or both", ErrInvalid, where)
		}
		if s.Login == login.Auto {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, where, ErrAutoLogin)
		}
		s.Path = expand(s.Path, userHome)
		if seen[s.Path] {
			return nil, fmt.Errorf("%w: %s: duplicate path", ErrInvalid, where)
		}
		seen[s.Path] = true
		out = append(out, s)
	}
	return out, nil
}

// MatchProject returns the binding covering dir: the longest bound path that
// is dir or an ancestor of it, so a subfolder binding beats its parent. The
// comparison is on path segments, so /a/foo does not cover /a/foobar. dir must
// already be canonical (CanonicalPath); this function never touches the disk.
func MatchProject(projects []Project, dir string) (Project, bool) {
	var best Project
	found := false
	for _, p := range projects {
		if covers(p.Path, dir) && (!found || len(p.Path) > len(best.Path)) {
			best, found = p, true
		}
	}
	return best, found
}

// covers reports whether dir is root or inside it. The separator is trimmed
// first because the filesystem root is the one clean path that ends in it.
func covers(root, dir string) bool {
	if root == dir {
		return true
	}
	return strings.HasPrefix(dir, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}

// ProjectFor returns the binding covering dir.
func (c Config) ProjectFor(dir string) (Project, bool) { return MatchProject(c.Projects, dir) }

// CanonicalPath is the form bindings are stored and matched in: absolute with
// symlinks resolved, because the shell's $PWD can spell one folder several
// ways (macOS /tmp is /private/tmp). A path that cannot be resolved, such as
// a deleted folder, is returned clean so its binding can still be removed.
func CanonicalPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}

// ExpandPath resolves a leading "~" and makes p absolute and clean. Empty
// stays empty.
func ExpandPath(p, userHome string) string { return expand(p, userHome) }

// ValidProfileName checks a new profile's name: the pattern every configured
// name follows, and not the word reserved for auto-pick.
func ValidProfileName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("profile name must match %s", namePattern)
	}
	if name == AutoProfile {
		return errors.New(`profile name "` + AutoProfile + `" is reserved for auto-pick`)
	}
	return nil
}
