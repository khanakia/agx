package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFzf writes a stand-in for fzf that runs body (stdin = the item list).
func fakeFzf(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fzf")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPickFzf(t *testing.T) {
	t.Parallel()
	items := []string{"first", "second\nwith newline", "third"}
	// fzf echoes the chosen "index<TAB>text" line; here: the second line.
	if i, err := pickFzf(fakeFzf(t, "sed -n 2p"), "resume", items); err != nil || i != 1 {
		t.Errorf("select = %d, %v", i, err)
	}
	// Newlines inside an item must not split it into two picker lines.
	if i, err := pickFzf(fakeFzf(t, "sed -n 3p"), "resume", items); err != nil || i != 2 {
		t.Errorf("newline item broke indexing: %d, %v", i, err)
	}
	for name, body := range map[string]string{"esc": "exit 130", "no match": "exit 1"} {
		if _, err := pickFzf(fakeFzf(t, body), "resume", items); !errors.Is(err, ErrCancelled) {
			t.Errorf("%s = %v, want ErrCancelled", name, err)
		}
	}
	if _, err := pickFzf(fakeFzf(t, "echo garbage"), "resume", items); err == nil || errors.Is(err, ErrCancelled) {
		t.Errorf("garbage selection = %v", err)
	}
	if _, err := pickFzf(fakeFzf(t, "exit 2"), "resume", items); err == nil || errors.Is(err, ErrCancelled) {
		t.Errorf("fzf error = %v", err)
	}
}

// ttyStub is an in-memory terminal: reads from in, records what was shown.
type ttyStub struct {
	in  io.Reader
	out bytes.Buffer
}

func (s *ttyStub) Read(p []byte) (int, error)  { return s.in.Read(p) }
func (s *ttyStub) Write(p []byte) (int, error) { return s.out.Write(p) }

func TestPickNumbered(t *testing.T) {
	t.Parallel()
	items := []string{"a", "b", "c"}
	for _, tc := range []struct {
		input   string
		want    int
		wantErr error
		anyErr  bool
	}{
		{"2\n", 1, nil, false},
		{" 3 \n", 2, nil, false},
		{"\n", 0, ErrCancelled, false},
		{"", 0, ErrCancelled, false}, // EOF
		{"9\n", 0, nil, true},
		{"x\n", 0, nil, true},
	} {
		tty := &ttyStub{in: strings.NewReader(tc.input)}
		got, err := pickNumbered(tty, "resume", items)
		switch {
		case tc.wantErr != nil && !errors.Is(err, tc.wantErr):
			t.Errorf("%q: err = %v, want %v", tc.input, err, tc.wantErr)
		case tc.anyErr && err == nil:
			t.Errorf("%q: want an error", tc.input)
		case tc.wantErr == nil && !tc.anyErr && (err != nil || got != tc.want):
			t.Errorf("%q: = %d, %v; want %d", tc.input, got, err, tc.want)
		}
		if !strings.Contains(tty.out.String(), "  2) b") {
			t.Errorf("%q: list not shown: %q", tc.input, tty.out.String())
		}
	}
}

func TestTerminalPicker(t *testing.T) {
	t.Parallel()
	if _, err := (TerminalPicker{}).Pick("x", nil); !errors.Is(err, ErrCancelled) {
		t.Errorf("empty list = %v", err)
	}
	p := TerminalPicker{FzfBin: fakeFzf(t, "sed -n 1p")}
	if i, err := p.Pick("x", []string{"only", "two"}); err != nil || i != 0 {
		t.Errorf("fzf path = %d, %v", i, err)
	}
}
