package login

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanakia/agx/provider"
)

// fakeSecret is planted in every input; no error may ever contain it.
const fakeSecret = "sk-ant-oat01-FAKE-login-test-0000"

// writeTokens writes a private tokens.yaml with body and returns it.
func writeTokens(t *testing.T, body string) TokenFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), TokenFileName)
	if err := os.WriteFile(path, []byte(body), filePerm); err != nil {
		t.Fatal(err)
	}
	return TokenFile{Path: path}
}

// TestResolve_Matrix crosses every source with every outcome a user can
// produce: a clean token, surrounding whitespace, empty, whitespace-only,
// multi-line, a missing entry, a failing command and an unset variable.
// Every error is checked for the planted secret.
func TestResolve_Matrix(t *testing.T) {
	t.Parallel()
	type tc struct {
		name    string
		login   Login
		tokens  string // tokens.yaml body; "" = no file
		env     map[string]string
		want    string
		wantErr error  // errors.Is target, when there is one
		errHas  string // substring of the error, when there is one
	}
	stored := Login{Name: "p", Source: SourceStored}
	cmd := func(c string) Login { return Login{Name: "c", Source: SourceCmd, Cmd: c} }
	env := Login{Name: "e", Source: SourceEnv, Env: "TOK"}
	cases := []tc{
		{name: "stored/ok", login: stored, tokens: "p: \"" + fakeSecret + "\"\n", want: fakeSecret},
		{name: "stored/padded", login: stored, tokens: "p: \"  " + fakeSecret + " \\n\"\n", want: fakeSecret},
		{name: "stored/empty", login: stored, tokens: "p: \"\"\n", wantErr: ErrEmpty},
		{name: "stored/whitespace", login: stored, tokens: "p: \"   \\t \"\n", wantErr: ErrEmpty},
		{name: "stored/multiline", login: stored, tokens: "p: \"" + fakeSecret + "\\n" + fakeSecret + "\"\n", wantErr: ErrMultiline},
		{name: "stored/missing-entry", login: stored, tokens: "other: \"" + fakeSecret + "\"\n", wantErr: ErrMissing, errHas: "--replace"},
		{name: "stored/no-file", login: stored, wantErr: ErrMissing},
		{name: "cmd/ok", login: cmd("printf '%s\\n' " + fakeSecret), want: fakeSecret},
		{name: "cmd/pipe", login: cmd("echo " + fakeSecret + " | tr -d x"), want: fakeSecret},
		{name: "cmd/empty", login: cmd("true"), wantErr: ErrEmpty},
		{name: "cmd/whitespace", login: cmd("printf '  \\n\\n'"), wantErr: ErrEmpty},
		{name: "cmd/multiline", login: cmd("printf '%s\\n%s\\n' " + fakeSecret + " " + fakeSecret), wantErr: ErrMultiline},
		{name: "cmd/exit-with-stderr", login: cmd("echo store locked >&2; exit 3"), errHas: "store locked"},
		{name: "cmd/exit-no-stderr", login: cmd("exit 4"), errHas: "exit status 4"},
		// A command that prints the token and then fails: stdout must not
		// reach the error.
		{name: "cmd/token-then-fail", login: cmd("echo " + fakeSecret + "; exit 1"), errHas: "exit status 1"},
		// A traced or echoing command repeats its stdout on stderr: stderr
		// is shown, with the stdout lines redacted.
		{name: "cmd/token-on-stderr-too", login: cmd("echo " + fakeSecret + "; echo store said " + fakeSecret + " >&2; exit 1"), errHas: "store said <redacted>"},
		{name: "cmd/no-stdin", login: cmd("cat"), wantErr: ErrEmpty},
		{name: "env/ok", login: env, env: map[string]string{"TOK": fakeSecret}, want: fakeSecret},
		{name: "env/padded", login: env, env: map[string]string{"TOK": "\n" + fakeSecret + "  "}, want: fakeSecret},
		{name: "env/empty", login: env, env: map[string]string{"TOK": ""}, wantErr: ErrEmpty},
		{name: "env/whitespace", login: env, env: map[string]string{"TOK": " \t"}, wantErr: ErrEmpty},
		{name: "env/multiline", login: env, env: map[string]string{"TOK": fakeSecret + "\n" + fakeSecret}, wantErr: ErrMultiline},
		{name: "env/unset", login: env, env: map[string]string{"OTHER": fakeSecret}, errHas: "TOK is not set"},
		{name: "unknown-source", login: Login{Name: "x", Source: "file"}, errHas: "unknown source"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := Resolver{Dir: t.TempDir(), LookupEnv: func(k string) (string, bool) {
				v, ok := c.env[k]
				return v, ok
			}}
			if c.tokens != "" {
				r.Tokens = writeTokens(t, c.tokens)
			} else {
				r.Tokens = TokenFile{Path: filepath.Join(t.TempDir(), TokenFileName)}
			}
			got, err := r.Resolve(context.Background(), c.login)
			if c.wantErr == nil && c.errHas == "" {
				if err != nil || got != c.want {
					t.Fatalf("got %q, %v; want %q", got, err, c.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("want an error, got value of length %d", len(got))
			}
			if got != "" {
				t.Errorf("value returned with an error")
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Errorf("err = %v, want %v", err, c.wantErr)
			}
			if c.errHas != "" && !strings.Contains(err.Error(), c.errHas) {
				t.Errorf("err = %v, want it to contain %q", err, c.errHas)
			}
			if strings.Contains(err.Error(), fakeSecret) {
				t.Errorf("error leaks the token: %v", err)
			}
			if !strings.Contains(err.Error(), c.login.Name) && !errors.Is(err, ErrMissing) {
				t.Errorf("error does not name login %s: %v", c.login.Name, err)
			}
		})
	}
}

// TestResolve_CmdTimeout pins that a token command that never returns stops
// the launch, even when it left a grandchild holding stdout open (sh forks
// sleep here because another command follows it).
func TestResolve_CmdTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Resolver{Dir: t.TempDir()}.Resolve(ctx, Login{Name: "slow", Source: SourceCmd, Cmd: "sleep 30; echo " + fakeSecret})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("timeout took %s: the command's children kept it alive", took)
	}
}

// TestResolve_CmdRunsInDir pins that token commands run in Resolver.Dir
// (the user's home), not wherever agx was started.
func TestResolve_CmdRunsInDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := Resolver{Dir: dir}.Resolve(context.Background(), Login{Name: "d", Source: SourceCmd, Cmd: "pwd"})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if gotReal, _ := filepath.EvalSymlinks(got); gotReal != want {
		t.Errorf("ran in %q, want %q", got, dir)
	}
}

// TestResolve_RunCmdOverride pins the injection seam tests and embedders use.
func TestResolve_RunCmdOverride(t *testing.T) {
	t.Parallel()
	r := Resolver{RunCmd: func(_ context.Context, c string) (string, error) {
		if c != "my-store get x" {
			return "", errors.New("unexpected command")
		}
		return fakeSecret + "\n", nil
	}}
	if got, err := r.Resolve(context.Background(), Login{Name: "o", Source: SourceCmd, Cmd: "my-store get x"}); err != nil || got != fakeSecret {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ stderr, stdout, want string }{
		{"boom", "", "boom"},
		{"bad " + fakeSecret, fakeSecret + "\n", "bad <redacted>"},
		{"+ echo " + fakeSecret + "\n+ exit 1", "  " + fakeSecret + "  \n", "+ echo <redacted>\n+ exit 1"},
		{"a b c", "a\n", "a b c"}, // too short to be a token
		{"x " + fakeSecret + " y " + fakeSecret, "other-line-123\n" + fakeSecret, "x <redacted> y <redacted>"},
	} {
		if got := redact(c.stderr, c.stdout); got != c.want {
			t.Errorf("redact(%q, %q) = %q, want %q", c.stderr, c.stdout, got, c.want)
		}
	}
}

func TestClean(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]error{
		"tok": nil, " tok ": nil, "tok\n": nil, "\r\ntok\r\n": nil, "\n\ntok\n\n": nil,
		"": ErrEmpty, " ": ErrEmpty, "\n\n": ErrEmpty, "\t\r\n": ErrEmpty,
		"a\nb": ErrMultiline, "a\n\nb\n": ErrMultiline, "a\r\nb": ErrMultiline,
	} {
		got, err := Clean(in)
		if !errors.Is(err, want) {
			t.Errorf("Clean(%q) err = %v, want %v", in, err, want)
		}
		if want == nil && got != "tok" {
			t.Errorf("Clean(%q) = %q", in, got)
		}
	}
}

func TestExpiresAtAndOwned(t *testing.T) {
	t.Parallel()
	added := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	year := 365 * 24 * time.Hour
	for _, c := range []struct {
		name     string
		added    time.Time
		lifetime time.Duration
		want     time.Time
		ok       bool
	}{
		{"known", added, year, added.Add(year), true},
		{"no-lifetime", added, 0, time.Time{}, false},
		{"negative-lifetime", added, -time.Hour, time.Time{}, false},
		{"no-added", time.Time{}, year, time.Time{}, false},
	} {
		got, ok := ExpiresAt(Login{Added: c.added}, c.lifetime)
		if ok != c.ok || !got.Equal(c.want) {
			t.Errorf("%s: got %v %v, want %v %v", c.name, got, ok, c.want, c.ok)
		}
	}
	for src, want := range map[Source]bool{SourceStored: true, SourceCmd: false, SourceEnv: false} {
		if Owned(Login{Source: src}) != want {
			t.Errorf("Owned(%s) != %v", src, want)
		}
	}
}

// fakeLauncher mirrors Claude's shape: each kind sets one variable and
// removes the other two.
type fakeLauncher struct{}

func (fakeLauncher) LoginKinds() []provider.LoginKind {
	return []provider.LoginKind{provider.LoginOAuthToken, provider.LoginAPIKey}
}

func (fakeLauncher) LoginEnv(kind provider.LoginKind, v string) (map[string]string, []string) {
	if kind == provider.LoginAPIKey {
		return map[string]string{"API_KEY": v}, []string{"OAUTH", "AUTH_TOKEN"}
	}
	return map[string]string{"OAUTH": v}, []string{"API_KEY", "AUTH_TOKEN"}
}

// TestCompatible_Matrix: billing × where the profile sets a variable (env,
// secrets) × which variable (set by the login, removed by it, unrelated) ×
// kind.
func TestCompatible_Matrix(t *testing.T) {
	t.Parallel()
	for _, kind := range provider.LoginKindValues {
		for _, billing := range []provider.Billing{"", provider.BillingPlan, provider.BillingAPI} {
			for _, where := range []string{"none", "env", "secrets"} {
				for _, v := range []string{"OAUTH", "API_KEY", "AUTH_TOKEN", "UNRELATED"} {
					p := provider.Profile{Name: "x", Billing: billing}
					switch where {
					case "env":
						p.Env = map[string]string{v: "1"}
					case "secrets":
						p.Secrets = map[string]string{v: "gopass:x"}
					}
					err := Compatible(p, fakeLauncher{}, kind)
					wantErr := billing == provider.BillingAPI || (where != "none" && v != "UNRELATED")
					if (err != nil) != wantErr {
						t.Errorf("%s/%s/%s/%s: err = %v, want error %v", kind, billing, where, v, err, wantErr)
					}
					if err != nil && !errors.Is(err, ErrIncompatible) {
						t.Errorf("%s/%s/%s/%s: err %v is not ErrIncompatible", kind, billing, where, v, err)
					}
				}
			}
		}
	}
}
