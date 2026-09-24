// Package sessions manages agx's session folders: the timestamped working
// directories `agx new` creates under a root, listing them, planning safe
// garbage collection, and promoting one into a real project.
//
// It touches only the filesystem. Whether a folder has conversation history
// is asked through a callback, so this package never imports a provider.
package sessions

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Naming (spec §9): [slug_]YYYYMMDD_HHMMSS in local time — identical to the
// zsh `_clnew` it replaces, so old and new folders sort together.
const (
	stampLayout = "20060102_150405"
	slugSep     = "_"
	// collisionSep joins a numeric suffix when two folders are created in the
	// same second.
	collisionSep = "_"
	// maxCollisions bounds the suffix search; hitting it means something is
	// creating folders in a loop.
	maxCollisions = 100
	// dirPerm matches what the shell `mkdir -p` produced.
	dirPerm os.FileMode = 0o755
)

// ignoredFiles do not count as content: gc treats a folder holding only
// these as empty (and removes them first).
var ignoredFiles = map[string]bool{".DS_Store": true}

// slugInvalid matches runs of characters not allowed in a slug.
var slugInvalid = regexp.MustCompile(`[^a-z0-9]+`)

// stampPattern recognises the timestamp tail of a session folder name.
var stampPattern = regexp.MustCompile(`(\d{8}_\d{6})(?:_\d+)?$`)

// Slug lowercases words and collapses every run of non-[a-z0-9] into "_",
// trimming separators from both ends. Empty input yields "".
func Slug(words ...string) string {
	s := strings.ToLower(strings.Join(words, " "))
	s = slugInvalid.ReplaceAllString(s, slugSep)
	return strings.Trim(s, slugSep)
}

// Name builds a folder name for slug at t.
func Name(slug string, t time.Time) string {
	stamp := t.Format(stampLayout)
	if slug == "" {
		return stamp
	}
	return slug + slugSep + stamp
}

// Create makes a new session folder under root and returns its path. When
// the name already exists (two creates in one second) a numeric suffix is
// added instead of reusing someone else's folder.
func Create(root, slug string, now time.Time) (string, error) {
	if err := os.MkdirAll(root, dirPerm); err != nil {
		return "", fmt.Errorf("sessions: create root: %w", err)
	}
	base := filepath.Join(root, Name(slug, now))
	for i := 1; i <= maxCollisions; i++ {
		path := base
		if i > 1 {
			path = base + collisionSep + strconv.Itoa(i)
		}
		err := os.Mkdir(path, dirPerm)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("sessions: create %s: %w", path, err)
		}
	}
	return "", fmt.Errorf("sessions: %d folders named %s already exist", maxCollisions, filepath.Base(base))
}

// Folder describes one session folder.
type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Created is parsed from the name's timestamp; zero when the name does
	// not carry one (a folder made by hand).
	Created time.Time `json:"created"`
	// Modified is the newest mtime among the folder and its files.
	Modified time.Time `json:"modified"`
	// Files counts regular files, recursively, excluding ignoredFiles.
	Files int `json:"files"`
	// HasHistory reports whether any provider holds a conversation started
	// in this folder.
	HasHistory bool `json:"has_history"`
}

// HistoryFunc reports whether any provider has conversation history for dir.
type HistoryFunc func(dir string) bool

// List returns the folders directly under root, newest first. A missing root
// is an empty list, not an error.
func List(root string, hasHistory HistoryFunc) ([]Folder, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sessions: read %s: %w", root, err)
	}
	var out []Folder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		f, err := inspect(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		if hasHistory != nil {
			f.HasHistory = hasHistory(f.Path)
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return sortKey(out[i]).After(sortKey(out[j])) })
	return out, nil
}

// sortKey orders by creation stamp, falling back to mtime for hand-made
// folders.
func sortKey(f Folder) time.Time {
	if !f.Created.IsZero() {
		return f.Created
	}
	return f.Modified
}

// inspect gathers one folder's stats.
func inspect(path string) (Folder, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Folder{}, fmt.Errorf("sessions: stat %s: %w", path, err)
	}
	f := Folder{Name: filepath.Base(path), Path: path, Modified: info.ModTime()}
	if m := stampPattern.FindStringSubmatch(f.Name); m != nil {
		if t, err := time.ParseInLocation(stampLayout, m[1], time.Local); err == nil {
			f.Created = t
		}
	}
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry: count what we can
		}
		if d.IsDir() || ignoredFiles[d.Name()] {
			return nil
		}
		f.Files++
		if fi, err := d.Info(); err == nil && fi.ModTime().After(f.Modified) {
			f.Modified = fi.ModTime()
		}
		return nil
	})
	if err != nil {
		return Folder{}, fmt.Errorf("sessions: walk %s: %w", path, err)
	}
	return f, nil
}

// GCReason explains why a folder is or is not collectable.
type GCReason string

const (
	GCCollect    GCReason = "empty, no history"
	GCHasFiles   GCReason = "has files"
	GCHasHistory GCReason = "has conversation history"
	GCTooNew     GCReason = "newer than min age"
)

// GCItem is one folder's gc decision.
type GCItem struct {
	Folder Folder   `json:"folder"`
	Remove bool     `json:"remove"`
	Reason GCReason `json:"reason"`
}

// PlanGC decides, for each folder, whether gc may remove it (spec §9): no
// files, no history in any provider, and older than minAge (by creation
// stamp, else mtime). It changes nothing.
func PlanGC(folders []Folder, now time.Time, minAge time.Duration) []GCItem {
	out := make([]GCItem, 0, len(folders))
	for _, f := range folders {
		item := GCItem{Folder: f}
		switch {
		case f.Files > 0:
			item.Reason = GCHasFiles
		case f.HasHistory:
			item.Reason = GCHasHistory
		case now.Sub(sortKey(f)) < minAge:
			item.Reason = GCTooNew
		default:
			item.Remove, item.Reason = true, GCCollect
		}
		out = append(out, item)
	}
	return out
}

// RemoveEmpty deletes an empty folder tree: ignored files are removed, then
// directories bottom-up with os.Remove, which refuses to delete a directory
// that still has content. It therefore cannot delete real work even if the
// plan was stale — a file created after planning makes it fail instead.
func RemoveEmpty(path string) error {
	var dirs []string
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, p)
			return nil
		}
		if !ignoredFiles[d.Name()] {
			return fmt.Errorf("sessions: %s is not empty (%s)", path, p)
		}
		return os.Remove(p)
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Remove(dirs[i]); err != nil {
			return fmt.Errorf("sessions: remove %s: %w", dirs[i], err)
		}
	}
	return nil
}

// ErrExists means a promote destination already exists.
var ErrExists = errors.New("sessions: destination already exists")

// ErrCrossDevice means the destination is on a different disk. Promote uses
// a single rename — atomic, and never leaves a half-copied project — which
// the OS cannot do across filesystems, so it refuses instead of copying.
var ErrCrossDevice = errors.New("sessions: destination is on a different disk; promote only moves within one disk (move it yourself, e.g. with mv)")

// Promote moves src to dst with os.Rename (same volume), refusing to
// overwrite. Parent directories of dst are created.
func Promote(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, dst)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("sessions: stat %s: %w", dst, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return fmt.Errorf("sessions: create parent of %s: %w", dst, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return renameError(src, dst, err)
	}
	return nil
}

// renameError classifies a failed rename, mapping a cross-device move to
// ErrCrossDevice so callers and users get an actionable message.
func renameError(src, dst string, err error) error {
	if isCrossDevice(err) {
		return fmt.Errorf("%w: %s → %s", ErrCrossDevice, src, dst)
	}
	return fmt.Errorf("sessions: move %s → %s: %w", src, dst, err)
}
