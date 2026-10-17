# Login resolution

Resolves three named logins with `login.Resolver`, one per source (the private token file, an env var, a token command), then shows expiry and the refusals: a missing stored token, a command printing two lines, a profile that already sets the credential variable, and a token file others can read. The env lookup and the command runner are replaced with fakes, and only token lengths are printed.

## Run

From the repository root:

```sh
go run ./examples/login-resolve
```

## Output

Captured from a real run:

<!-- out: login-resolve -->

```text
token file written with mode 0600
personal source=stored resolved 22 chars, owned by agx=true
work     source=env    resolved 19 chars, owned by agx=false
team     source=cmd    resolved 19 chars, owned by agx=false
personal expires 2027-10-01
work has no added date: expiry unknown
missing stored: true
multi-line cmd: login file (cmd): login: token command printed more than one line
compatible: login: profile already has its own credential: profile kimi sets ANTHROPIC_API_KEY in env
world-readable token file: insecure=true
```

<!-- /out -->

## What to look at

- `TokenFile.Set` writes the file with mode 0600, atomically.
- Only stored logins are "owned" by agx; for env and cmd logins the `added` date is just when the login was registered.
- `login.ExpiresAt` needs both a lifetime (from `LoginLifetimer`) and an added date.
- A token file with group or other bits set is refused before it is read (`login.ErrInsecure`).

## Related

- Guide: [secrets-and-logins.md](../../docs/guides/library/secrets-and-logins.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
