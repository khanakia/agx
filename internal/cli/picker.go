package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Picker chooses one item interactively. It returns the chosen index or
// ErrCancelled.
type Picker interface {
	Pick(prompt string, items []string) (int, error)
}

// MultiPicker is an optional upgrade: choose any number of items at once.
// Callers fall back to a single Pick when the picker lacks it.
type MultiPicker interface {
	PickMany(prompt string, items []string) ([]int, error)
}

// ErrCancelled means the user dismissed the picker.
var ErrCancelled = errors.New("cancelled")

// ErrNoTerminal means an interactive choice was needed but no terminal is
// attached; callers suggest --last / --list instead.
var ErrNoTerminal = errors.New("no terminal to pick from (use --last, or --list to see ids)")

// Picker implementation details.
const (
	fzfBin = "fzf"
	// fzfCancelled is fzf's exit status for Esc / Ctrl-C.
	fzfCancelled = 130
	// fzfNoMatch is fzf's exit status when the query matched nothing.
	fzfNoMatch = 1
	ttyPath    = "/dev/tty"
	// fzfHeight keeps the picker inline below the prompt instead of full-screen.
	fzfHeight = "40%"
	// itemSep separates the hidden index from the shown text in fzf input.
	itemSep = "\t"
)

// TerminalPicker uses fzf when installed (it draws on /dev/tty itself), and
// otherwise a numbered prompt read from /dev/tty. Reading the terminal
// directly — not stdin — keeps it working inside the shell layer's plan
// protocol and under redirection.
type TerminalPicker struct {
	// FzfBin overrides the fzf executable (tests point it at a fake); empty
	// means fzf on PATH.
	FzfBin string
	// NoFzf forces the numbered prompt even when fzf is installed.
	NoFzf bool
}

// Pick implements Picker.
func (t TerminalPicker) Pick(prompt string, items []string) (int, error) {
	if len(items) == 0 {
		return 0, ErrCancelled
	}
	bin := t.FzfBin
	if bin == "" {
		bin = fzfBin
	}
	if !t.NoFzf {
		if path, err := exec.LookPath(bin); err == nil {
			return pickFzf(path, prompt, items)
		}
	}
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return 0, ErrNoTerminal
	}
	// The tty was only prompted and read; a close error cannot lose anything.
	defer func() { _ = tty.Close() }()
	return pickNumbered(tty, prompt, items)
}

// PickMany implements MultiPicker: fzf --multi (Tab marks, Enter accepts;
// Enter with nothing marked takes the current row), else numbers typed as
// "1 3 4" or "1,3".
func (t TerminalPicker) PickMany(prompt string, items []string) ([]int, error) {
	if len(items) == 0 {
		return nil, ErrCancelled
	}
	bin := t.FzfBin
	if bin == "" {
		bin = fzfBin
	}
	if !t.NoFzf {
		if path, err := exec.LookPath(bin); err == nil {
			return runFzf(path, prompt, items, true)
		}
	}
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return nil, ErrNoTerminal
	}
	// The tty was only prompted and read; a close error cannot lose anything.
	defer func() { _ = tty.Close() }()
	return pickNumberedMany(tty, prompt, items)
}

// pickFzf feeds "index<TAB>text" lines to fzf, shows only the text, and
// parses the index back from the selection.
func pickFzf(bin, prompt string, items []string) (int, error) {
	idx, err := runFzf(bin, prompt, items, false)
	if err != nil {
		return 0, err
	}
	return idx[0], nil
}

// runFzf runs fzf over items and returns the selected indexes (one unless
// multi).
func runFzf(bin, prompt string, items []string, multi bool) ([]int, error) {
	var in bytes.Buffer
	for i, it := range items {
		fmt.Fprintf(&in, "%d%s%s\n", i, itemSep, strings.ReplaceAll(it, "\n", " "))
	}
	args := []string{"--with-nth=2..", "--delimiter=" + itemSep, "--prompt=" + prompt + "> ", "--height=" + fzfHeight, "--reverse", "--no-sort"}
	if multi {
		args = append(args, "--multi")
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && (exitErr.ExitCode() == fzfCancelled || exitErr.ExitCode() == fzfNoMatch) {
		return nil, ErrCancelled
	}
	if err != nil {
		return nil, fmt.Errorf("fzf: %w", err)
	}
	var picked []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		idx, _, _ := strings.Cut(line, itemSep)
		n, err := strconv.Atoi(idx)
		if err != nil || n < 0 || n >= len(items) {
			return nil, fmt.Errorf("fzf returned an unexpected selection %q", strings.TrimSpace(string(out)))
		}
		picked = append(picked, n)
	}
	return picked, nil
}

// pickNumbered prints a numbered list to tty and reads a number from it.
func pickNumbered(tty io.ReadWriter, prompt string, items []string) (int, error) {
	var b strings.Builder
	for i, it := range items {
		fmt.Fprintf(&b, "%3d) %s\n", i+1, it)
	}
	fmt.Fprintf(&b, "%s [1-%d, Enter to cancel]: ", prompt, len(items))
	if _, err := io.WriteString(tty, b.String()); err != nil {
		return 0, fmt.Errorf("write prompt: %w", err)
	}
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil {
		return 0, ErrCancelled
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, ErrCancelled
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(items) {
		return 0, fmt.Errorf("not a choice between 1 and %d: %q", len(items), line)
	}
	return n - 1, nil
}

// pickNumberedMany prints a numbered list to tty and reads one or more
// numbers ("1 3" or "1,3") from it.
func pickNumberedMany(tty io.ReadWriter, prompt string, items []string) ([]int, error) {
	var b strings.Builder
	for i, it := range items {
		fmt.Fprintf(&b, "%3d) %s\n", i+1, it)
	}
	fmt.Fprintf(&b, "%s [numbers 1-%d, e.g. 1 3; Enter to cancel]: ", prompt, len(items))
	if _, err := io.WriteString(tty, b.String()); err != nil {
		return nil, fmt.Errorf("write prompt: %w", err)
	}
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil {
		return nil, ErrCancelled
	}
	fields := strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	if len(fields) == 0 {
		return nil, ErrCancelled
	}
	var picked []int
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(items) {
			return nil, fmt.Errorf("not a choice between 1 and %d: %q", len(items), f)
		}
		picked = append(picked, n-1)
	}
	return picked, nil
}
