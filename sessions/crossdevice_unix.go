//go:build unix

package sessions

import (
	"errors"
	"syscall"
)

// isCrossDevice reports a rename that failed because source and destination
// are on different filesystems (EXDEV).
func isCrossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }
