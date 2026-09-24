//go:build unix

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/khanakia/agx/provider"
)

// execReplace replaces the agx process with cmd, so the vendor CLI owns the
// terminal directly (signals, job control, exit status) and agx leaves no
// parent process behind.
func execReplace(cmd provider.Command) error {
	path, err := exec.LookPath(cmd.Path)
	if err != nil {
		return fmt.Errorf("%s not found on PATH: %w", cmd.Path, err)
	}
	if cmd.Dir != "" {
		if err := os.Chdir(cmd.Dir); err != nil {
			return fmt.Errorf("enter %s: %w", cmd.Dir, err)
		}
	}
	argv := append([]string{cmd.Path}, cmd.Args...)
	if err := syscall.Exec(path, argv, cmd.Env); err != nil {
		return fmt.Errorf("exec %s: %w", path, err)
	}
	return nil // unreachable: Exec only returns on failure
}
