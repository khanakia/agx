package secret

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unicodeSecret is a fake non-ASCII secret value.
const unicodeSecret = "tök€n-FAKE-0001-日本"

// TestSystem_Defaults: a zero System reads the real environment and runs
// gopass from PATH. Not parallel: it sets process variables.
func TestSystem_Defaults(t *testing.T) {
	const envName = "AGX_SECRET_TEST_FAKE"
	t.Setenv(envName, unicodeSecret)
	ctx := context.Background()
	if v, err := (System{}).Resolve(ctx, Ref{SchemeEnv, envName}); err != nil || v != unicodeSecret {
		t.Errorf("env default = %q, %v", v, err)
	}
	if _, err := (System{}).Resolve(ctx, Ref{SchemeEnv, envName + "_UNSET"}); err == nil || !strings.Contains(err.Error(), "is not set") {
		t.Errorf("unset = %v", err)
	}

	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'fake-token-0001\\r\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, GopassBin), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if v, err := (System{}).Resolve(ctx, Ref{SchemeGopass, "x"}); err != nil || v != "fake-token-0001" {
		t.Errorf("gopass from PATH = %q, %v", v, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (System{}).Resolve(ctx, Ref{SchemeGopass, "x"}); err == nil || !strings.Contains(err.Error(), "is not installed") {
		t.Errorf("gopass not on PATH = %v", err)
	}
}

// TestSystem_GopassOutputShapes: trailing CR/LF is trimmed, inner bytes kept,
// and a newline-only answer is empty.
func TestSystem_GopassOutputShapes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, c := range []struct {
		name, body, want string
		empty            bool
	}{
		{"unicode", "printf '%s\\n' '" + unicodeSecret + "'", unicodeSecret, false},
		{"crlf", "printf 'abc\\r\\n'", "abc", false},
		{"inner spaces kept", "printf ' a b \\n'", " a b ", false},
		{"newlines only", "printf '\\n\\r\\n'", "", true},
	} {
		s := System{GopassBin: fakeGopass(t, c.body)}
		v, err := s.Resolve(ctx, Ref{SchemeGopass, "p"})
		if c.empty {
			if !errors.Is(err, ErrEmpty) {
				t.Errorf("%s: err = %v", c.name, err)
			}
			continue
		}
		if err != nil || v != c.want {
			t.Errorf("%s: %q, %v", c.name, v, err)
		}
	}
}

func TestResolveAll_EmptyAndUnknownScheme(t *testing.T) {
	t.Parallel()
	got, err := ResolveAll(context.Background(), mapResolver{}, nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("nil refs = %v, %v", got, err)
	}
	if _, err := ResolveAll(context.Background(), mapResolver{}, map[string]string{"X": "vault:a"}); !errors.Is(err, ErrUnknownScheme) || !strings.Contains(err.Error(), "secret for X") {
		t.Errorf("unknown scheme = %v", err)
	}
}
