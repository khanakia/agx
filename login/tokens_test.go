package login

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// TestTokenFile_CreateAndRewrite: a first Set creates a 0600 file in a 0700
// dir; later Sets and Removes rewrite it atomically (no temp files left,
// still 0600) and keep every other entry.
func TestTokenFile_CreateAndRewrite(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "agx")
	tf := TokenFile{Path: filepath.Join(dir, TokenFileName)}
	values := map[string]string{
		"plain":   fakeSecret,
		"quotes":  `a"b'c`,
		"colon":   "x: y",
		"hash":    "#not-a-comment",
		"unicode": "tök€n",
		"yamlish": "- [1, {a: b}]",
		"bool":    "true",
	}
	for n, v := range values {
		if err := tf.Set(n, v); err != nil {
			t.Fatal(err)
		}
	}
	if got := mode(t, dir); got != dirPerm {
		t.Errorf("dir mode %04o, want %04o", got, dirPerm)
	}
	if got := mode(t, tf.Path); got != filePerm {
		t.Errorf("file mode %04o, want %04o", got, filePerm)
	}
	all, err := tf.Load()
	if err != nil {
		t.Fatal(err)
	}
	for n, v := range values {
		if all[n] != v {
			t.Errorf("%s round-tripped as %q, want %q", n, all[n], v)
		}
	}
	// Rotate one, remove one, remove a missing one.
	if err := tf.Set("plain", "rotated"); err != nil {
		t.Fatal(err)
	}
	if removed, err := tf.Remove("hash"); err != nil || !removed {
		t.Errorf("Remove(hash) = %v, %v", removed, err)
	}
	if removed, err := tf.Remove("nosuch"); err != nil || removed {
		t.Errorf("Remove(nosuch) = %v, %v", removed, err)
	}
	all, _ = tf.Load()
	if all["plain"] != "rotated" || all["quotes"] != values["quotes"] || len(all) != len(values)-1 {
		t.Errorf("after rewrite: %d entries, plain=%q", len(all), all["plain"])
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
	if got := mode(t, tf.Path); got != filePerm {
		t.Errorf("rewritten file mode %04o", got)
	}
	raw, _ := os.ReadFile(tf.Path)
	if !strings.HasPrefix(string(raw), "# agx raw login tokens") {
		t.Errorf("header missing:\n%s", raw)
	}
	// Removing the last entries leaves a valid, empty file.
	for n := range all {
		if _, err := tf.Remove(n); err != nil {
			t.Fatal(err)
		}
	}
	if all, err := tf.Load(); err != nil || len(all) != 0 {
		t.Errorf("emptied file: %v %v", all, err)
	}
}

// TestTokenFile_RefusesSharedModes: any group/other bit makes every
// operation refuse with the exact chmod fix, before the file is read.
func TestTokenFile_RefusesSharedModes(t *testing.T) {
	t.Parallel()
	for _, m := range []os.FileMode{0o640, 0o644, 0o604, 0o660, 0o606, 0o620, 0o602, 0o610, 0o601, 0o666, 0o777} {
		tf := writeTokens(t, "p: \""+fakeSecret+"\"\n")
		if err := os.Chmod(tf.Path, m); err != nil {
			t.Fatal(err)
		}
		ops := map[string]func() error{
			"CheckPerm": tf.CheckPerm,
			"Load":      func() error { _, err := tf.Load(); return err },
			"Set":       func() error { return tf.Set("q", "v") },
			"Remove":    func() error { _, err := tf.Remove("p"); return err },
		}
		for name, op := range ops {
			err := op()
			if !errors.Is(err, ErrInsecure) {
				t.Errorf("%04o %s: err = %v, want ErrInsecure", m, name, err)
				continue
			}
			if !strings.Contains(err.Error(), "chmod 600 "+tf.Path) || strings.Contains(err.Error(), fakeSecret) {
				t.Errorf("%04o %s: %v", m, name, err)
			}
		}
		if got := mode(t, tf.Path); got != m {
			t.Errorf("%04o: a refused Set changed the mode to %04o", m, got)
		}
	}
	// Owner-only modes are private, whatever the owner bits.
	for _, m := range []os.FileMode{0o600, 0o400, 0o700} {
		tf := writeTokens(t, "p: x\n")
		if err := os.Chmod(tf.Path, m); err != nil {
			t.Fatal(err)
		}
		if err := tf.CheckPerm(); err != nil {
			t.Errorf("%04o refused: %v", m, err)
		}
	}
}

// TestTokenFile_DecodeErrorNeverQuotesContent: yaml errors quote the bad
// line, which here would be a token.
func TestTokenFile_DecodeErrorNeverQuotesContent(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		fakeSecret + ": [unclosed\n",
		"p: " + fakeSecret + "\n  bad indent: " + fakeSecret + "\n",
		"- " + fakeSecret + "\n",
		"p: {a: " + fakeSecret + "}\n",
	} {
		tf := writeTokens(t, body)
		_, err := tf.Load()
		if err == nil {
			t.Errorf("no error for %q", body)
			continue
		}
		if strings.Contains(err.Error(), fakeSecret) || !strings.Contains(err.Error(), tf.Path) {
			t.Errorf("decode error: %v", err)
		}
	}
}

func TestTokenFile_EmptyAndUnset(t *testing.T) {
	t.Parallel()
	if all, err := (TokenFile{}).Load(); err != nil || len(all) != 0 {
		t.Errorf("zero TokenFile: %v %v", all, err)
	}
	if err := (TokenFile{}).Set("a", "b"); err == nil {
		t.Error("Set without a path succeeded")
	}
	for _, body := range []string{"", "  \n\n", "# only a comment\n"} {
		if all, err := writeTokens(t, body).Load(); err != nil || len(all) != 0 {
			t.Errorf("%q: %v %v", body, all, err)
		}
	}
	missing := TokenFile{Path: filepath.Join(t.TempDir(), "nope", TokenFileName)}
	if err := missing.CheckPerm(); err != nil {
		t.Errorf("CheckPerm on a missing file: %v", err)
	}
	if all, err := missing.Load(); err != nil || len(all) != 0 {
		t.Errorf("missing file: %v %v", all, err)
	}
}
