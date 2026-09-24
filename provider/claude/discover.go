package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discovered-profile naming.
const (
	// defaultProfileName names the profile for ~/.claude. "personal" matches
	// how a second, work account is usually added beside the default one.
	defaultProfileName = "personal"
)

// DiscoveredHome is a config dir found on disk and the profile name it
// implies.
type DiscoveredHome struct {
	Name string
	Home string
}

// DiscoverHomes lists Claude Code config dirs under userHome: ~/.claude first
// (as "personal", when present), then every ~/.claude-<x> directory holding a
// config marker (as "<x>"), sorted by name.
//
// An empty result with a nil error means no account exists on this machine.
func DiscoverHomes(userHome string) ([]DiscoveredHome, error) {
	var out []DiscoveredHome
	def := filepath.Join(userHome, defaultDirName)
	if isConfigDir(def) {
		out = append(out, DiscoveredHome{Name: defaultProfileName, Home: def})
	}

	extra, err := filepath.Glob(filepath.Join(userHome, extraDirPrefix+"*"))
	if err != nil {
		return nil, fmt.Errorf("claude: glob config dirs: %w", err)
	}
	sort.Strings(extra)
	for _, d := range extra {
		if !isConfigDir(d) {
			continue
		}
		name := strings.TrimPrefix(filepath.Base(d), extraDirPrefix)
		out = append(out, DiscoveredHome{Name: name, Home: d})
	}
	return out, nil
}

// IsDefaultHome reports whether home is the default config dir (~/.claude),
// which differs in keychain naming, profile-file location and launch env.
func IsDefaultHome(home, userHome string) bool {
	return filepath.Clean(home) == filepath.Join(userHome, defaultDirName)
}

// DefaultHomeFor returns ~/.claude for userHome.
func DefaultHomeFor(userHome string) string {
	return filepath.Join(userHome, defaultDirName)
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
