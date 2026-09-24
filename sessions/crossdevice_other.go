//go:build !unix

package sessions

import (
	"errors"
	"syscall"
)

// errNotSameDevice is Windows' ERROR_NOT_SAME_DEVICE, returned by a rename
// across volumes.
const errNotSameDevice syscall.Errno = 17

// isCrossDevice reports a rename that failed because source and destination
// are on different volumes.
func isCrossDevice(err error) bool { return errors.Is(err, errNotSameDevice) }
