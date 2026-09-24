//go:build unix

package claude

import (
	"errors"
	"syscall"
)

// processAlive reports whether pid is a running process. Signal 0 checks
// existence without delivering anything; EPERM means it exists but belongs
// to someone else, which still counts as running.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
