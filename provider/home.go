package provider

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// HomeSetup is the optional upgrade for providers that can prepare a new home
// (`agx profile add`). A provider without it can still have profiles; it just
// cannot have one set up for it.
type HomeSetup interface {
	// SharedHomeFiles names entries of the default home a new home should
	// reach through a symlink, so instructions and settings stay in one
	// place. nil means the vendor has nothing that is safe to share.
	SharedHomeFiles() []string
	// LoginCommand builds the vendor's interactive login for p's home. env
	// is the base environment.
	LoginCommand(p Profile, env []string) Command
}

// Reasons a HomeLink is not made.
const (
	LinkSkipNoSource = "not in the default home"
	LinkSkipExists   = "already present"
)

// HomeLink is one planned symlink Target -> Source. Skip, when set, says why
// it will not be made.
type HomeLink struct {
	Name   string
	Source string
	Target string
	Skip   string
}

// PlanHomeLinks decides which of names to link from the default home into
// home, without changing anything. A link needs its source to exist and its
// target to be absent; Lstat is used on the target because a broken symlink
// there is still somebody's file.
func PlanHomeLinks(defaultHome, home string, names []string) []HomeLink {
	out := make([]HomeLink, 0, len(names))
	for _, n := range names {
		l := HomeLink{Name: n, Source: filepath.Join(defaultHome, n), Target: filepath.Join(home, n)}
		if _, err := os.Stat(l.Source); err != nil {
			l.Skip = LinkSkipNoSource
		} else if _, err := os.Lstat(l.Target); !errors.Is(err, fs.ErrNotExist) {
			l.Skip = LinkSkipExists
		}
		out = append(out, l)
	}
	return out
}

// ApplyHomeLinks makes every link that is not skipped. os.Symlink refuses an
// existing target, so a file that appeared after planning is never replaced.
func ApplyHomeLinks(links []HomeLink) error {
	for _, l := range links {
		if l.Skip != "" {
			continue
		}
		if err := os.Symlink(l.Source, l.Target); err != nil {
			return fmt.Errorf("provider: link %s: %w", l.Name, err)
		}
	}
	return nil
}
