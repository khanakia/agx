package claude

import (
	"errors"
	"fmt"
	"os"
)

// ErrHistoryExists means the destination already has a history folder;
// MoveHistory refuses rather than merging two histories.
var ErrHistoryExists = errors.New("claude: destination already has history")

// MoveHistory re-keys the history Claude Code recorded for fromDir so it is
// found from toDir, by renaming <home>/projects/<encode(from)> to
// <home>/projects/<encode(to)>.
//
// Why a rename is enough: Claude Code looks conversations up by the encoded
// working directory; the cwd recorded inside old transcript lines is not
// used for lookup. moved is false (nil error) when fromDir had no history.
// Never overwrites: an existing destination is ErrHistoryExists.
func (p *Provider) MoveHistory(home, fromDir, toDir string) (bool, error) {
	src := HistoryDir(home, fromDir)
	dst := HistoryDir(home, toDir)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("claude: stat history %s: %w", src, err)
	}
	if _, err := os.Stat(dst); err == nil {
		return false, fmt.Errorf("%w: %s", ErrHistoryExists, dst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("claude: stat history %s: %w", dst, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return false, fmt.Errorf("claude: move history: %w", err)
	}
	return true, nil
}

// HasHistory implements provider.HistoryChecker with a single stat: Claude
// Code keys history by EncodePath(dir), so no transcript is read.
func (p *Provider) HasHistory(home, dir string) bool {
	info, err := os.Stat(HistoryDir(home, dir))
	return err == nil && info.IsDir()
}
