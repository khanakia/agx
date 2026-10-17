// Command login-resolve resolves named logins (package login) from each of
// the three sources — the private token file, another env var, a token
// command — and shows the refusals that keep a token from leaking or being
// misused: a token file others can read, a command that prints more than
// one line, a profile that already carries its own credential.
//
// Token values are fake and never printed; only their length is.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/khanakia/agx/login"
	"github.com/khanakia/agx/provider"
	"github.com/khanakia/agx/provider/claude"
)

// Fake token values, one per source.
const (
	storedValue = "fake-stored-token-0001"
	envValue    = "fake-env-token-0002"
	cmdValue    = "fake-cmd-token-0003"
	// envName is the variable an env login reads.
	envName = "WORK_CLAUDE_TOKEN"
)

// worldReadable is the mode that makes the token file insecure.
const worldReadable os.FileMode = 0o644

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	agxDir, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(agxDir) }()

	tokens := login.TokenFile{Path: filepath.Join(agxDir, login.TokenFileName)}
	if err := tokens.Set("personal", storedValue); err != nil {
		return err
	}
	info, err := os.Stat(tokens.Path)
	if err != nil {
		return err
	}
	fmt.Printf("token file written with mode %04o\n", info.Mode().Perm())

	// LookupEnv and RunCmd replace the real environment and `sh -c`, so the
	// example runs no shell; leave them nil to use the real ones.
	r := login.Resolver{
		Tokens:    tokens,
		LookupEnv: func(k string) (string, bool) { return envValue, k == envName },
		RunCmd: func(_ context.Context, command string) (string, error) {
			if command == "print-two-lines" {
				return "line one\nline two\n", nil
			}
			return "  " + cmdValue + "\n", nil // surrounding whitespace is trimmed
		},
	}
	added := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	logins := []login.Login{
		{Name: "personal", Provider: provider.Claude, Kind: provider.LoginOAuthToken, Source: login.SourceStored, Added: added},
		{Name: "work", Provider: provider.Claude, Kind: provider.LoginOAuthToken, Source: login.SourceEnv, Env: envName},
		{Name: "team", Provider: provider.Claude, Kind: provider.LoginOAuthToken, Source: login.SourceCmd, Cmd: "gopass show -o team/claude"},
	}
	ctx := context.Background()
	for _, l := range logins {
		v, err := r.Resolve(ctx, l)
		if err != nil {
			return err
		}
		fmt.Printf("%-8s source=%-6s resolved %d chars, owned by agx=%v\n", l.Name, l.Source, len(v), login.Owned(l))
	}

	cl := claude.New("") // only the login upgrades are used: no keychain read
	if exp, ok := login.ExpiresAt(logins[0], cl.LoginLifetime(provider.LoginOAuthToken)); ok {
		fmt.Printf("personal expires %s\n", exp.Format(time.DateOnly))
	}
	if _, ok := login.ExpiresAt(logins[1], cl.LoginLifetime(provider.LoginOAuthToken)); !ok {
		fmt.Println("work has no added date: expiry unknown")
	}

	_, err = r.Resolve(ctx, login.Login{Name: "spare", Source: login.SourceStored})
	fmt.Printf("missing stored: %v\n", errors.Is(err, login.ErrMissing))
	_, err = r.Resolve(ctx, login.Login{Name: "file", Source: login.SourceCmd, Cmd: "print-two-lines"})
	fmt.Printf("multi-line cmd: %v\n", err)

	// A profile that already sets the variable a login would remove.
	kimi := provider.Profile{Name: "kimi", Provider: provider.Claude, Env: map[string]string{claude.APIKeyEnv: "x"}}
	fmt.Printf("compatible: %v\n", login.Compatible(kimi, cl, provider.LoginOAuthToken))

	// The token file is refused, before it is read, once others can read it.
	if err := os.Chmod(tokens.Path, worldReadable); err != nil {
		return err
	}
	_, err = r.Resolve(ctx, logins[0])
	fmt.Printf("world-readable token file: insecure=%v\n", errors.Is(err, login.ErrInsecure))
	return nil
}
