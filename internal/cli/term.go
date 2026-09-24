package cli

import (
	"io"
	"os"
	"os/exec"
)

// isTerminal reports whether w is a character device (an interactive
// terminal), which decides auto colour and whether a picker can prompt.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// lookPath is exec.LookPath, named for injection.
func lookPath(name string) (string, error) { return exec.LookPath(name) }
