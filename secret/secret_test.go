package secret

import (
	"context"
	"errors"
	"strings"
	"testing"
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
