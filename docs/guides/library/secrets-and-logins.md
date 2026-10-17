# Secrets and logins

Two packages turn references into credential values at the moment of launch, so values never sit in `config.yaml`, in a plan file or in an error message. `secret` resolves `scheme:ref` references (`gopass:<path>`, `env:<NAME>`), the values a profile's `secrets:` map holds. `login` resolves named logins: a long-lived token kept in the private `tokens.yaml`, printed by a command, or read from another variable, which a provider then sets on the child process instead of the home's own login. You want this page when your tool launches a vendor CLI on a credential it fetches itself, or reuses agx's logins.

## Before you start

- Neither package prints, logs or wraps a value into an error. Keep it that way in your code: print lengths or `<set>`, as the examples do.
- Programs quoted here: [`examples/secret-refs`](../../../examples/secret-refs/) and [`examples/login-resolve`](../../../examples/login-resolve/). Both replace the real environment and command runner with fakes; no gopass, shell or real token is involved.

## Step 1 — Parse and resolve secret references

`secret.Parse` accepts only the known schemes. `secret.System` resolves `env:` from the environment (or `LookupEnv`) and `gopass:` by running `gopass show -o`. `secret.ResolveAll` resolves a whole `secrets` map, and takes any `Resolver`, so another backend is one method.

<!-- snip: examples/secret-refs/main.go type:vaultResolver -->

Source: [`examples/secret-refs/main.go`](../../../examples/secret-refs/main.go), `vaultResolver`, quoted verbatim.

```go
// vaultResolver shows that any backend fits: implement Resolve and pass it
// to ResolveAll. Here it answers gopass references from a map.
type vaultResolver struct{}
```

<!-- /snip -->

<!-- snip: examples/secret-refs/main.go func:Resolve -->

Source: [`examples/secret-refs/main.go`](../../../examples/secret-refs/main.go), `Resolve`, quoted verbatim.

```go
// Resolve implements secret.Resolver.
func (vaultResolver) Resolve(_ context.Context, ref secret.Ref) (string, error) {
	if ref.Scheme != secret.SchemeGopass {
		return "", fmt.Errorf("vault: cannot resolve %s references", ref.Scheme)
	}
	v, ok := vault[ref.Path]
	if !ok {
		return "", fmt.Errorf("vault: %s not found", ref.Path)
	}
	return v, nil
}
```

<!-- /snip -->

<!-- snip: examples/secret-refs/main.go func:run -->

Source: [`examples/secret-refs/main.go`](../../../examples/secret-refs/main.go), `run`, quoted verbatim.

```go
func run() error {
	for _, raw := range []string{"gopass:team/claude", "env:KIMI_TOKEN", "KIMI_TOKEN", "vault:x"} {
		ref, err := secret.Parse(raw)
		if err != nil {
			fmt.Printf("parse %-20q error: %v\n", raw, err)
			continue
		}
		fmt.Printf("parse %-20q scheme=%s path=%s\n", raw, ref.Scheme, ref.Path)
	}

	ctx := context.Background()
	sys := secret.System{LookupEnv: func(k string) (string, bool) { v, ok := fakeEnv[k]; return v, ok }}
	vals, err := secret.ResolveAll(ctx, sys, map[string]string{"ANTHROPIC_AUTH_TOKEN": "env:KIMI_TOKEN"})
	if err != nil {
		return err
	}
	printLengths("system", vals)

	vals, err = secret.ResolveAll(ctx, vaultResolver{}, map[string]string{"ANTHROPIC_API_KEY": "gopass:team/claude"})
	if err != nil {
		return err
	}
	printLengths("custom", vals)

	// Errors name the variable and the reference, never a value.
	_, err = secret.ResolveAll(ctx, sys, map[string]string{"ANTHROPIC_AUTH_TOKEN": "env:MISSING_TOKEN"})
	fmt.Printf("unset: %v\n", err)
	empty := secret.System{LookupEnv: func(string) (string, bool) { return "", true }}
	_, err = secret.ResolveAll(ctx, empty, map[string]string{"ANTHROPIC_AUTH_TOKEN": "env:KIMI_TOKEN"})
	fmt.Printf("empty: %v (empty=%v)\n", err, errors.Is(err, secret.ErrEmpty))
	return nil
}
```

<!-- /snip -->

## Step 2 — Store a token in the private token file

`login.TokenFile` is a name to token map in YAML, written atomically with mode 0600 and refused when group or others can read it. `config.yaml` only ever says `token: stored`; the value lives here.

<!-- snip: examples/login-resolve/main.go func:run -->

Source: [`examples/login-resolve/main.go`](../../../examples/login-resolve/main.go), `run`, quoted verbatim.

```go
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
```

<!-- /snip -->

## Step 3 — Resolve a login and apply it to a launch

`login.Resolver.Resolve` returns one login's value from its source, trimmed and checked to be a single line. A provider that implements `provider.LoginLauncher` names the variables to set and remove; `provider.ApplyLoginEnv` applies them to a launch command's environment. For Claude Code an `oauth-token` login sets `CLAUDE_CODE_OAUTH_TOKEN` and removes `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN`, so the child cannot silently bill a different way.

<!-- snip: provider/claude/login.go func:LoginEnv -->

Source: [`provider/claude/login.go`](../../../provider/claude/login.go), `LoginEnv`, quoted verbatim.

```go
// LoginEnv implements provider.LoginLauncher. An unknown kind sets nothing,
// which callers have already refused at config validation.
func (p *Provider) LoginEnv(kind provider.LoginKind, value string) (map[string]string, []string) {
	switch kind {
	case provider.LoginOAuthToken:
		return map[string]string{OAuthTokenEnv: value}, []string{APIKeyEnv, AuthTokenEnv}
	case provider.LoginAPIKey:
		return map[string]string{APIKeyEnv: value}, []string{OAuthTokenEnv, AuthTokenEnv}
	}
	return nil, nil
}
```

<!-- /snip -->

`login.Compatible` refuses a profile that already sets one of those variables in its own `env` or `secrets`, or that is billed per token. `login.ExpiresAt` combines a login's `added` date with the provider's `LoginLifetimer` (365 days for a Claude setup-token, unverified upstream).

## Try it

```sh
go run ./examples/secret-refs
```

<!-- out: secret-refs -->

```text
parse "gopass:team/claude" scheme=gopass path=team/claude
parse "env:KIMI_TOKEN"     scheme=env path=KIMI_TOKEN
parse "KIMI_TOKEN"         error: secret: reference must be scheme:ref: "KIMI_TOKEN"
parse "vault:x"            error: secret: unknown scheme "vault" (want one of [gopass env])
system: ANTHROPIC_AUTH_TOKEN resolved (15 chars)
custom: ANTHROPIC_API_KEY resolved (16 chars)
unset: secret for ANTHROPIC_AUTH_TOKEN: secret: env MISSING_TOKEN is not set
empty: secret for ANTHROPIC_AUTH_TOKEN: secret: resolved to an empty value: env:KIMI_TOKEN (empty=true)
```

<!-- /out -->

```sh
go run ./examples/login-resolve
```

<!-- out: login-resolve -->

```text
token file written with mode 0600
personal source=stored resolved 22 chars, owned by agx=true
work     source=env    resolved 19 chars, owned by agx=false
team     source=cmd    resolved 19 chars, owned by agx=false
personal expires 2027-10-01
work has no added date: expiry unknown
missing stored: true
multi-line cmd: login file (cmd): login: token has more than one line
compatible: login: profile already has its own credential: profile kimi sets ANTHROPIC_API_KEY in env
world-readable token file: insecure=true
```

<!-- /out -->

## Pitfalls

- `secret.System` runs gopass with the terminal's stdin and stderr attached so a passphrase prompt works; in a non-interactive caller a prompt waits until `Timeout` (60 seconds by default) (`secret/secret.go`, `gopass`, `TestSystemResolveGopass`).
- An empty resolved value is an error (`secret.ErrEmpty`), because an empty token would otherwise surface much later as a confusing auth failure in the launched tool (`secret/secret.go`, `TestSystemResolveEnv`).
- `secret.ResolveAll` stops at the first failure. It ranges over a map, so with several broken references which one is reported can change between runs (`secret/secret.go`, `ResolveAll`).
- A token command runs through `sh -c` with no stdin, a 10-second timeout and the resolver's `Dir`; a command that prints more than one non-empty line is `login.ErrMultiline`, never a token (`login/login.go`, `TestResolve_CmdTimeout`, `TestClean`).
- When a failed token command echoes its own output to stderr (for example under `set -x`), lines of 8 characters or more are replaced with `<redacted>` before stderr reaches the error (`login/login.go`, `redact`, `TestRedact`).
- `TokenFile.Load` refuses a file with any group or other permission bit before opening it, and a decode error never quotes the file, because the offending line would be a token (`login/tokens.go`, `TestTokenFile_RefusesSharedModes`, `TestTokenFile_DecodeErrorNeverQuotesContent`).
- The zero `TokenFile` reads as empty, so a `Resolver` without `Tokens` reports every stored login as `login.ErrMissing` rather than failing loudly at construction (`login/tokens.go`, `TestTokenFile_EmptyAndUnset`).
- A raw token in `config.yaml`'s `token:` field is rejected at load; only `stored` is accepted there, and the error never echoes the value (`config/logins.go`, `resolveLogin`, `TestResolve_Logins_Invalid`).
- `auto` is accepted only as the `--login` flag of a launch, never as a configured default, because it probes every login and spends a few tokens of each account's quota (`config/logins.go`, `ErrAutoLogin`).
- `ApplyLoginEnv` sets variables in sorted name order, so the resulting env slice is the same on every run (`provider/login.go`, `ApplyLoginEnv`).
- Codex has no token logins: it does not implement `provider.LoginLauncher`, and `config.NoLoginsReason(provider.Codex)` explains why (`config/logins.go`).

## Related

- [Library overview](README.md)
- [Claude Code and Codex](claude-and-codex.md): `LoginUsage` reads a token login's usage
- [Config, pick and sessions](config-pick-sessions.md)
- [examples/secret-refs](../../../examples/secret-refs/), [examples/login-resolve](../../../examples/login-resolve/)
