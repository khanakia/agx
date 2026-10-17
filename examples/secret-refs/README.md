# Secret references

Parses `scheme:ref` references with `secret.Parse` and resolves a profile-style `secrets` map with `secret.ResolveAll`, once with `secret.System` (env scheme, fake lookup) and once with a custom `Resolver` that answers `gopass:` references from a map.

## Run

From the repository root:

```sh
go run ./examples/secret-refs
```

## Output

Captured from a real run:

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

## What to look at

- Only `gopass` and `env` are valid schemes; a bare name is not a reference.
- Any type with a `Resolve` method can back `ResolveAll`.
- Errors name the variable and the reference, never a value.
- An empty value is an error (`secret.ErrEmpty`), not an empty token.

## Related

- Guide: [secrets-and-logins.md](../../docs/guides/library/secrets-and-logins.md)
- Source: [main.go](main.go)
- Test: [examples_test.go](../examples_test.go) builds and runs this program and checks the lines above.
