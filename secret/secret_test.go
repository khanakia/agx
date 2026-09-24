package secret

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]Ref{
		"gopass:personal/ai/key": {SchemeGopass, "personal/ai/key"},
		"env:TOKEN":              {SchemeEnv, "TOKEN"},
		"env:A:B":                {SchemeEnv, "A:B"}, // only the first ':' splits
	} {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v", in, got, err)
		}
	}
	for in, want := range map[string]error{
		"":          ErrBadRef,
		"gopass":    ErrBadRef,
		"gopass:":   ErrBadRef,
		":x":        ErrBadRef,
		"vault:x/y": ErrUnknownScheme,
	} {
		if _, err := Parse(in); !errors.Is(err, want) {
			t.Errorf("Parse(%q) err = %v, want %v", in, err, want)
		}
	}
}

func TestSystemResolveEnv(t *testing.T) {
	t.Parallel()
	s := System{LookupEnv: func(k string) (string, bool) {
		switch k {
		case "SET":
			return "v", true
		case "EMPTY":
			return "", true
		}
		return "", false
	}}
	ctx := context.Background()
	if v, err := s.Resolve(ctx, Ref{SchemeEnv, "SET"}); err != nil || v != "v" {
		t.Errorf("SET = %q, %v", v, err)
	}
	if _, err := s.Resolve(ctx, Ref{SchemeEnv, "MISSING"}); err == nil {
		t.Error("missing env should fail")
	}
	if _, err := s.Resolve(ctx, Ref{SchemeEnv, "EMPTY"}); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty env err = %v", err)
	}
	if _, err := s.Resolve(ctx, Ref{Scheme("nope"), "x"}); !errors.Is(err, ErrUnknownScheme) {
		t.Errorf("unknown scheme err = %v", err)
	}
}

// mapResolver resolves from a map, failing on unknown refs.
type mapResolver map[string]string

func (m mapResolver) Resolve(_ context.Context, ref Ref) (string, error) {
	v, ok := m[string(ref.Scheme)+":"+ref.Path]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func TestResolveAll(t *testing.T) {
	t.Parallel()
	r := mapResolver{"env:A": "1", "gopass:x": "2"}
	got, err := ResolveAll(context.Background(), r, map[string]string{"TOK": "env:A", "KEY": "gopass:x"})
	if err != nil || got["TOK"] != "1" || got["KEY"] != "2" {
		t.Errorf("ResolveAll = %v, %v", got, err)
	}
	_, err = ResolveAll(context.Background(), r, map[string]string{"BAD": "gopass:missing"})
	if err == nil || !strings.Contains(err.Error(), "BAD") {
		t.Errorf("error should name the variable: %v", err)
	}
	if _, err := ResolveAll(context.Background(), r, map[string]string{"X": "nocolon"}); !errors.Is(err, ErrBadRef) {
		t.Errorf("bad ref err = %v", err)
	}
}

// fakeGopass writes a stand-in for the gopass CLI that runs body.
func fakeGopass(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gopass")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSystemResolveGopass(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ref := Ref{SchemeGopass, "ai/kimi"}

	// `gopass show -o <path>` prints the secret; the trailing newline is trimmed.
	s := System{GopassBin: fakeGopass(t, `[ "$1" = show ] && [ "$2" = -o ] && [ "$3" = ai/kimi ] && printf 'sk-123\n'`)}
	if v, err := s.Resolve(ctx, ref); err != nil || v != "sk-123" {
		t.Errorf("resolve = %q, %v", v, err)
	}
	// An empty secret is an error, not an empty token.
	s = System{GopassBin: fakeGopass(t, "printf ''")}
	if _, err := s.Resolve(ctx, ref); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty = %v", err)
	}
	// A failing gopass never leaks what it printed into the error.
	s = System{GopassBin: fakeGopass(t, "printf 'partial-secret'; exit 3")}
	if _, err := s.Resolve(ctx, ref); err == nil || strings.Contains(err.Error(), "partial-secret") {
		t.Errorf("failure = %v", err)
	}
	// Not installed.
	s = System{GopassBin: filepath.Join(t.TempDir(), "missing")}
	if _, err := s.Resolve(ctx, ref); err == nil {
		t.Error("missing gopass should fail")
	}
	// A pinentry prompt nobody answers times out.
	s = System{GopassBin: fakeGopass(t, "sleep 5"), Timeout: 100 * time.Millisecond}
	if _, err := s.Resolve(ctx, ref); err == nil {
		t.Error("hung gopass should time out")
	}
}
