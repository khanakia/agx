//go:build !unix

package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/khanakia/agx/provider"
)

// execReplace runs cmd as a child with the terminal attached, on platforms
// without exec(2) (Windows). agx then exits with the child's status.
func execReplace(cmd provider.Command) error {
	path, err := exec.LookPath(cmd.Path)
	if err != nil {
		return fmt.Errorf("%s not found on PATH: %w", cmd.Path, err)
	}
	c := exec.Command(path, cmd.Args...)
	c.Dir, c.Env = cmd.Dir, cmd.Env
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{Code: exitErr.ExitCode(), Err: err}
	}
	return err
}
