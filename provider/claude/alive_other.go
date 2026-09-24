//go:build !unix

package claude

import "os"

// processAlive reports whether pid is a running process. On Windows
// os.FindProcess opens the process and fails when it does not exist.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release() // only the existence check mattered; nothing to report
	return true
}
