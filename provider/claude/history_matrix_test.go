package claude

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestEncodePath_Matrix pins the history key Claude Code uses: every byte
// that is not an ASCII letter or digit becomes '-', one per byte.
func TestEncodePath_Matrix(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":                   "",
		"/":                  "-",
		"/a/b":               "-a-b",
		"/a/b/":              "-a-b-",
		"/A-Z_09.x~y":        "-A-Z-09-x-y",
		"/tab\there":         "-tab-here",
		"/日本":                "-------", // 2 runes × 3 bytes + leading slash
		"/emoji😀":            "-emoji----",
		"C:\\Users\\me":      "C--Users-me",
		"/a b/c:d@e+f=g,h;i": "-a-b-c-d-e-f-g-h-i",
	} {
		if got := EncodePath(in); got != want {
			t.Errorf("EncodePath(%q) = %q, want %q", in, got, want)
		}
	}
	// Distinct dirs may share a key (Claude Code's own scheme): agx must look
	// up by this key, so the collision is pinned rather than "fixed".
	if EncodePath("/a_b") != EncodePath("/a.b") {
		t.Error("expected /a_b and /a.b to share a history key")
	}
}

func TestMoveHistory_Matrix(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	home := t.TempDir()

	// No history: nothing moves, no error.
	if moved, err := p.MoveHistory(home, "/no/such", "/x"); moved || err != nil {
		t.Errorf("no history = %v, %v", moved, err)
	}

	// Destination exists: refused, source untouched.
	writeFile(t, filepath.Join(HistoryDir(home, "/src"), "a.jsonl"), "{}")
	writeFile(t, filepath.Join(HistoryDir(home, "/dst"), "b.jsonl"), "{}")
	if moved, err := p.MoveHistory(home, "/src", "/dst"); moved || !errors.Is(err, ErrHistoryExists) {
		t.Errorf("existing dst = %v, %v", moved, err)
	}
	if !p.HasHistory(home, "/src") {
		t.Error("refused move changed the source")
	}

	// Unicode and spaced folders move and are found by the new key.
	writeFile(t, filepath.Join(HistoryDir(home, "/projets/été 2026"), "c.jsonl"), "{}")
	if moved, err := p.MoveHistory(home, "/projets/été 2026", "/archive/été"); !moved || err != nil {
		t.Fatalf("unicode move = %v, %v", moved, err)
	}
	if p.HasHistory(home, "/projets/été 2026") || !p.HasHistory(home, "/archive/été") {
		t.Error("history not re-keyed")
	}

	// A file (not a dir) at the history key is not history.
	writeFile(t, HistoryDir(home, "/file-not-dir"), "x")
	if p.HasHistory(home, "/file-not-dir") {
		t.Error("a plain file counted as history")
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	// The destination cannot be checked (unreadable projects dir): a real
	// error, not a silent "moved".
	locked := t.TempDir()
	writeFile(t, filepath.Join(HistoryDir(locked, "/s"), "a.jsonl"), "{}")
	projects := filepath.Join(locked, projectsDir)
	if err := os.Chmod(projects, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(projects, 0o700) })
	if moved, err := p.MoveHistory(locked, "/s", "/d"); moved || err == nil {
		t.Errorf("unreadable projects dir = %v, %v", moved, err)
	}
}
