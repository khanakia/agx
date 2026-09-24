package account

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Discover lists the Claude Code config directories under home: ~/.claude
// first (when present), then every ~/.claude-* directory that contains a
// config marker, sorted by name.
//
// An empty result with a nil error means no account exists on this machine;
// callers should say so rather than print nothing.
func Discover(home string) ([]string, error) {
	var dirs []string
	def := filepath.Join(home, defaultDirName)
	if isConfigDir(def) {
		dirs = append(dirs, def)
	}

	extra, err := filepath.Glob(filepath.Join(home, extraDirGlob))
	if err != nil {
		return nil, fmt.Errorf("account: glob config dirs: %w", err)
	}
	sort.Strings(extra)
	for _, d := range extra {
		if isConfigDir(d) {
			dirs = append(dirs, d)
		}
	}
	return dirs, nil
}

// IsDefaultDir reports whether dir is the default config dir (~/.claude),
// which uses different file locations than CLAUDE_CONFIG_DIR dirs.
func IsDefaultDir(dir, home string) bool {
	return filepath.Clean(dir) == filepath.Join(home, defaultDirName)
}

// isConfigDir reports whether path is a directory holding any config marker.
func isConfigDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	for _, m := range configMarkers {
		if _, err := os.Stat(filepath.Join(path, m)); err == nil {
			return true
		}
	}
	return false
}
