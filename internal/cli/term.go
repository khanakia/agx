package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"golang.org/x/term"

	"github.com/khanakia/agx/provider"
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

// runInteractive runs cmd with the terminal attached and waits for it.
func runInteractive(c provider.Command) error {
	cmd := exec.Command(c.Path, c.Args...)
	cmd.Env, cmd.Dir = c.Env, c.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// errNoTerminal means a hidden prompt was needed but stdin is not a tty.
var errNoTerminal = errors.New("stdin is not a terminal: pipe the token with --stdin")

// promptSecret prints prompt to stderr and reads a line from the terminal
// without echoing it, so a pasted token never shows on screen.
func promptSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errNoTerminal
	}
	if _, err := io.WriteString(os.Stderr, prompt); err != nil {
		return "", err
	}
	b, err := term.ReadPassword(fd)
	// The newline the user typed was not echoed either.
	_, _ = io.WriteString(os.Stderr, "\n")
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	return string(b), nil
}
