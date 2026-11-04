package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// CheckPicker is an optional upgrade over MultiPicker: a checklist where
// every row shows whether it is marked, so a user can review a selection
// before confirming it. `agx usage --pick` prefers it and falls back to
// MultiPicker.
type CheckPicker interface {
	PickChecked(prompt string, items []string) ([]int, error)
}

// checkKey is one decoded keypress of the checklist.
type checkKey int

const (
	keyNone checkKey = iota
	keyUp
	keyDown
	keyToggle
	keyToggleAll
	keyConfirm
	keyCancel
)

// Raw bytes the checklist reads from the terminal in raw mode.
const (
	byteEsc      = 0x1b
	byteCtrlC    = 0x03
	byteCR       = '\r'
	byteLF       = '\n'
	byteSpace    = ' '
	byteAll      = 'a'
	byteQuit     = 'q'
	byteVimUp    = 'k'
	byteVimDown  = 'j'
	seqArrowUp   = "\x1b[A"
	seqArrowDown = "\x1b[B"
	// seqArrowUpSS3 / seqArrowDownSS3 are the arrow keys in a terminal's
	// application-cursor mode (tmux and some emulators send these).
	seqArrowUpSS3   = "\x1bOA"
	seqArrowDownSS3 = "\x1bOB"
)

// checklistHint is the key help printed after the prompt.
const checklistHint = "space: mark · a: all/none · enter: show · esc/q: cancel"

// ANSI cursor control for redrawing the checklist in place.
const (
	ansiClearDown = "\x1b[J"
	ansiUpFmt     = "\x1b[%dA"
)

// decodeKey maps one terminal read to a key. A lone ESC is cancel, while
// ESC followed by an arrow sequence is cursor movement; anything else is
// ignored rather than guessed at.
func decodeKey(b []byte) checkKey {
	s := string(b)
	switch s {
	case seqArrowUp, seqArrowUpSS3:
		return keyUp
	case seqArrowDown, seqArrowDownSS3:
		return keyDown
	}
	if len(b) != 1 {
		return keyNone
	}
	switch b[0] {
	case byteVimUp:
		return keyUp
	case byteVimDown:
		return keyDown
	case byteSpace:
		return keyToggle
	case byteAll:
		return keyToggleAll
	case byteCR, byteLF:
		return keyConfirm
	case byteEsc, byteQuit, byteCtrlC:
		return keyCancel
	}
	return keyNone
}

// checklist is the picker's state; apply is pure so the key handling is
// tested without a terminal.
type checklist struct {
	cursor  int
	checked []bool
}

func newChecklist(n int) *checklist { return &checklist{checked: make([]bool, n)} }

// apply handles one key. done is true once the user confirmed or
// cancelled; on confirm, picked holds the marked rows in order, or the
// cursor row when nothing is marked (the same rule as fzf --multi).
func (c *checklist) apply(k checkKey) (picked []int, done bool, err error) {
	n := len(c.checked)
	switch k {
	case keyUp:
		c.cursor = (c.cursor - 1 + n) % n
	case keyDown:
		c.cursor = (c.cursor + 1) % n
	case keyToggle:
		c.checked[c.cursor] = !c.checked[c.cursor]
	case keyToggleAll:
		// "a" selects all unless everything already is, then clears all.
		all := true
		for _, v := range c.checked {
			all = all && v
		}
		for i := range c.checked {
			c.checked[i] = !all
		}
	case keyConfirm:
		for i, v := range c.checked {
			if v {
				picked = append(picked, i)
			}
		}
		if len(picked) == 0 {
			picked = []int{c.cursor}
		}
		return picked, true, nil
	case keyCancel:
		return nil, true, ErrCancelled
	}
	return nil, false, nil
}

// view renders the checklist lines (no trailing newline handling: the
// caller joins them with CRLF for raw mode).
func (c *checklist) view(prompt string, items []string) []string {
	lines := []string{fmt.Sprintf("%s  (%s)", prompt, checklistHint)}
	for i, it := range items {
		cur, mark := " ", " "
		if i == c.cursor {
			cur = ">"
		}
		if c.checked[i] {
			mark = "x"
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %s", cur, mark, it))
	}
	return lines
}

// readBufSize holds one keypress, including a multi-byte escape sequence.
const readBufSize = 16

// PickChecked implements CheckPicker on /dev/tty in raw mode: arrows or
// j/k move, space marks, a marks all or none, Enter confirms, Esc / q /
// Ctrl-C cancel. The list is erased afterwards so the caller's output
// starts on a clean line.
func (t TerminalPicker) PickChecked(prompt string, items []string) ([]int, error) {
	if len(items) == 0 {
		return nil, ErrCancelled
	}
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return nil, ErrNoTerminal
	}
	// The tty was only drawn on and read; a close error cannot lose anything.
	defer func() { _ = tty.Close() }()
	fd := int(tty.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return nil, ErrNoTerminal
	}
	defer func() { _ = term.Restore(fd, old) }()
	return runChecklist(tty, prompt, items)
}

// runChecklist draws, reads keys and redraws until done. It is separate
// from PickChecked so a test can drive it over a pipe.
func runChecklist(tty io.ReadWriter, prompt string, items []string) ([]int, error) {
	c := newChecklist(len(items))
	drawn := 0
	draw := func(lines []string) error {
		var b strings.Builder
		if drawn > 0 {
			fmt.Fprintf(&b, "\r"+ansiUpFmt, drawn)
		}
		b.WriteString("\r" + ansiClearDown)
		for _, l := range lines {
			b.WriteString(l + "\r\n")
		}
		drawn = len(lines)
		_, err := io.WriteString(tty, b.String())
		return err
	}
	buf := make([]byte, readBufSize)
	for {
		if err := draw(c.view(prompt, items)); err != nil {
			return nil, err
		}
		n, err := tty.Read(buf)
		if err != nil {
			return nil, ErrCancelled
		}
		picked, done, perr := c.apply(decodeKey(buf[:n]))
		if done {
			// Erase the list: the table that follows replaces it.
			_ = draw(nil)
			return picked, perr
		}
	}
}
