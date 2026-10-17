package login

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unicodeSecret is a token-shaped value outside ASCII; it must survive every
// source unchanged.
const unicodeSecret = "tök€n-FAKE-0001-日本"

// TestResolve_UnicodeAndCRLF: a non-ASCII token round-trips through each
// source, and a Windows line ending is whitespace, not a second line.
func TestResolve_UnicodeAndCRLF(t *testing.T) {
	t.Parallel()
	tf := TokenFile{Path: filepath.Join(t.TempDir(), TokenFileName)}
	if err := tf.Set("u", unicodeSecret+"\r\n"); err != nil {
		t.Fatal(err)
	}
	r := Resolver{
		Tokens:    tf,
		Dir:       t.TempDir(),
		LookupEnv: func(string) (string, bool) { return unicodeSecret + "\r\n", true },
		RunCmd:    func(context.Context, string) (string, error) { return "\r\n" + unicodeSecret + "\r\n", nil },
	}
	for _, l := range []Login{
		{Name: "u", Source: SourceStored},
		{Name: "u", Source: SourceEnv, Env: "TOK"},
		{Name: "u", Source: SourceCmd, Cmd: "x"},
	} {
		got, err := r.Resolve(context.Background(), l)
		if err != nil || got != unicodeSecret {
			t.Errorf("%s: got %q, %v", l.Source, got, err)
		}
	}
}

// TestResolve_DefaultLookupEnv: with no LookupEnv override the real
// environment is read. Not parallel: it sets a process variable.
func TestResolve_DefaultLookupEnv(t *testing.T) {
	const envName = "AGX_LOGIN_TEST_FAKE_TOKEN"
	t.Setenv(envName, " fake-token-0001 ")
	got, err := (Resolver{}).Resolve(context.Background(), Login{Name: "e", Source: SourceEnv, Env: envName})
	if err != nil || got != "fake-token-0001" {
		t.Errorf("got %q, %v", got, err)
	}
	_, err = (Resolver{}).Resolve(context.Background(), Login{Name: "e", Source: SourceEnv, Env: envName + "_UNSET"})
	if err == nil || !strings.Contains(err.Error(), "is not set") {
		t.Errorf("unset: %v", err)
	}
}

// TestTokenFile_IOErrors: a stat or read failure is an error (never "empty"
// and never ErrInsecure), and a failed save leaves the old file and its mode.
func TestTokenFile_IOErrors(t *testing.T) {
	t.Parallel()
	// Path under a regular file: stat fails with ENOTDIR.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, filePerm); err != nil {
		t.Fatal(err)
	}
	under := TokenFile{Path: filepath.Join(file, TokenFileName)}
	if err := under.CheckPerm(); err == nil || errors.Is(err, ErrInsecure) {
		t.Errorf("CheckPerm under a file: %v", err)
	}
	if _, err := under.Load(); err == nil {
		t.Error("Load under a file: no error")
	}
	if err := under.Set("a", "b"); err == nil {
		t.Error("Set under a file: no error")
	}

	// A private directory where the file should be: read fails.
	dir := filepath.Join(t.TempDir(), TokenFileName)
	if err := os.Mkdir(dir, dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, filePerm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, dirPerm) })
	if _, err := (TokenFile{Path: dir}).Load(); err == nil {
		t.Error("Load of a directory: no error")
	}

	// Read-only parent: the temp file cannot be created; nothing changes.
	ro := t.TempDir()
	tf := TokenFile{Path: filepath.Join(ro, TokenFileName)}
	if err := tf.Set("keep", "fake-token-0001"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(tf.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, dirPerm) })
	if err := tf.Set("new", "fake-token-0002"); err == nil {
		t.Error("Set in a read-only dir: no error")
	}
	if _, err := tf.Remove("keep"); err == nil {
		t.Error("Remove in a read-only dir: no error")
	}
	after, err := os.ReadFile(tf.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || mode(t, tf.Path) != filePerm {
		t.Errorf("failed save changed the file: %q", after)
	}
}

// TestTokenFile_OddNames: names are YAML keys; ones that look like YAML
// syntax must round-trip as plain strings. "null" is a valid login name
// (it matches the name pattern), and a plain `null:` key decodes as a null
// key, so its token was written but never readable.
func TestTokenFile_OddNames(t *testing.T) {
	t.Parallel()
	tf := TokenFile{Path: filepath.Join(t.TempDir(), TokenFileName)}
	names := []string{"true", "null", "~", "1", "- x", "a: b", "#c", "wörk", "日本", "  pad  "}
	for i, n := range names {
		if err := tf.Set(n, "fake-token-000"+string(rune('a'+i))); err != nil {
			t.Fatalf("%q: %v", n, err)
		}
	}
	all, err := tf.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range names {
		if all[n] != "fake-token-000"+string(rune('a'+i)) {
			t.Errorf("%q: got %q", n, all[n])
		}
	}
	if len(all) != len(names) {
		t.Errorf("%d entries, want %d: %v", len(all), len(names), all)
	}
	// Removing the last entry leaves a valid, empty, private file.
	for _, n := range names {
		if _, err := tf.Remove(n); err != nil {
			t.Fatal(err)
		}
	}
	if all, err := tf.Load(); err != nil || len(all) != 0 || mode(t, tf.Path) != filePerm {
		t.Errorf("after removing all: %v %v", all, err)
	}
}
